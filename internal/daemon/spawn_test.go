package daemon

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

// fakeHerdr is a Herdr session socket: one request per connection, a canned
// reply looked up by method, every request recorded.
type fakeHerdr struct {
	t      *testing.T
	socket string

	mu  sync.Mutex
	got []map[string]json.RawMessage
}

const paneStartedReply = `{"id":"1","result":{"agent":{"pane_id":"w1:p2","terminal_id":"term_x"}}}`

func serveHerdr(t *testing.T, replies map[string]string) *fakeHerdr {
	t.Helper()
	f := &fakeHerdr{t: t, socket: filepath.Join(t.TempDir(), "h.sock")}
	ln, err := net.Listen("unix", f.socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	done := make(chan struct{})
	t.Cleanup(func() {
		ln.Close()
		<-done
	})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			f.handle(conn, replies)
		}
	}()
	return f
}

func (f *fakeHerdr) handle(conn net.Conn, replies map[string]string) {
	defer conn.Close()
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		return
	}

	f.mu.Lock()
	f.got = append(f.got, env)
	f.mu.Unlock()

	method := strings.Trim(string(env["method"]), `"`)
	reply, ok := replies[method]
	if !ok {
		reply = `{"id":"1","result":{}}`
	}
	conn.Write([]byte(reply + "\n"))
}

// call returns the params of the first request for method, and whether the
// server saw one at all.
func (f *fakeHerdr) call(method string) (map[string]any, bool) {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, env := range f.got {
		if strings.Trim(string(env["method"]), `"`) != method {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(env["params"], &p); err != nil {
			f.t.Fatalf("decode %s params: %v", method, err)
		}
		return p, true
	}
	return nil, false
}

// count returns how many requests for method the server has seen.
func (f *fakeHerdr) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	n := 0
	for _, env := range f.got {
		if strings.Trim(string(env["method"]), `"`) == method {
			n++
		}
	}
	return n
}

// params is call for a method the test requires to have been called.
func (f *fakeHerdr) params(method string) map[string]any {
	f.t.Helper()
	p, ok := f.call(method)
	if !ok {
		f.t.Fatalf("herdr never received %s", method)
	}
	return p
}

// Stub agent bodies. piEcho answers every prompt; piCrash does too but dies
// abnormally when stdin closes, which is what makes Runner.Stop report an
// error even though the stop itself worked; piSelfExit never reads stdin at
// all, so it is gone before anyone asks it to stop.
const (
	piEcho = `printf '{"type":"agent_start"}\n'
while IFS= read -r line; do
	case "$line" in
	*'"type":"prompt"'*)
		id=$(printf '%s' "$line" | sed 's/.*"id":"\([^"]*\)".*/\1/')
		printf '{"id":"%s","type":"agent_settled"}\n' "$id"
		;;
	esac
done
`
	piCrash    = piEcho + "exit 2\n"
	piSelfExit = "printf '{\"type\":\"agent_start\"}\\n'\nexit 0\n"
	// piPidFile drops its pid in the launch cwd so a test can check the reap.
	piPidFile = "printf '%s' \"$$\" > pi.pid\n" + piEcho
)

// installPi puts a fake `pi` with the given body first on PATH.
func installPi(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pi"), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// piStub installs the cooperative stub: it announces itself, then answers every
// prompt frame with a settled frame carrying the same id.
func piStub(t *testing.T) {
	t.Helper()
	installPi(t, piEcho)
}

// writeAgents installs an agents.json in the workspace.
func writeAgents(t *testing.T, root string, configs map[string]agentcfg.Config) {
	t.Helper()
	data, err := json.Marshal(configs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, scaffold.DirName, agentcfg.FileName), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// boundDaemon is newTestDaemon with the daemon bound to a fake Herdr session.
func boundDaemon(t *testing.T, f *fakeHerdr) *Daemon {
	t.Helper()
	d := newTestDaemon(t)
	if f != nil {
		d.session = herdr.Session{Name: "sess", SocketPath: f.socket}
	}
	return d
}

// events reads back every journal line the daemon has written.
func events(t *testing.T, d *Daemon) []event {
	t.Helper()
	data, err := os.ReadFile(journalPath(d.root, d.flockName))
	if err != nil {
		t.Fatal(err)
	}

	var out []event
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("journal line %q: %v", line, err)
		}
		out = append(out, e)
	}
	return out
}

// findEvent returns the first journaled event of this name for this agent.
func findEvent(t *testing.T, d *Daemon, name, agent string) event {
	t.Helper()
	for _, e := range events(t, d) {
		if e.Event == name && e.Name == agent {
			return e
		}
	}
	t.Fatalf("no %s event for %s in journal", name, agent)
	return event{}
}

// countEvents reports how many journaled events of this name name this agent.
func countEvents(t *testing.T, d *Daemon, name, agent string) int {
	t.Helper()
	n := 0
	for _, e := range events(t, d) {
		if e.Event == name && e.Name == agent {
			n++
		}
	}
	return n
}

func hasEvent(t *testing.T, d *Daemon, name, agent string) bool {
	t.Helper()
	for _, e := range events(t, d) {
		if e.Event == name && e.Name == agent {
			return true
		}
	}
	return false
}

func agentState(d *Daemon, name string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.agents[name].State
}

// awaitState polls until the agent reaches want, which is how a test observes
// state that only moves when a frame arrives from the agent's own goroutine.
func awaitState(t *testing.T, d *Daemon, name, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := agentState(d, name); got == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s state = %q, want %q", name, agentState(d, name), want)
}

func strs(t *testing.T, v any) []string {
	t.Helper()
	raw, ok := v.([]any)
	if !ok {
		t.Fatalf("value %#v is not a list", v)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, item.(string))
	}
	return out
}

func TestSpawnClaudeByConfig(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start":       paneStartedReply,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":4242}}}`,
	})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"worker": {
			Integration:    "claude",
			Model:          "claude-opus-4",
			PermissionMode: "acceptEdits",
			Env:            map[string]string{"EXTRA": "1"},
		},
	})

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if resp.Name != "worker-emperor" {
		t.Fatalf("name = %q, want worker-emperor", resp.Name)
	}
	if resp.PaneID != "w1:p2" {
		t.Fatalf("pane = %q, want w1:p2", resp.PaneID)
	}

	start := f.params("agent.start")
	if start["name"] != "worker-emperor" {
		t.Fatalf("pane name = %v", start["name"])
	}
	argv := strs(t, start["argv"])
	for _, want := range []string{"--session-id", "--permission-mode", "acceptEdits", "--model", "claude-opus-4"} {
		if !slicesContains(argv, want) {
			t.Fatalf("argv %v is missing %q", argv, want)
		}
	}
	env := start["env"].(map[string]any)
	if env[flock.Env] != d.flockName {
		t.Fatalf("pane env %s = %v, want %q", flock.Env, env[flock.Env], d.flockName)
	}
	if env["EXTRA"] != "1" {
		t.Fatalf("config env did not reach the pane: %v", env)
	}
	if _, ok := f.call("pane.report_metadata"); !ok {
		t.Fatal("no pane.report_metadata; the pane was never titled")
	}

	// The pane's shell pid is what the roster reports liveness on.
	if got := d.agents["worker-emperor"].PID; got != 4242 {
		t.Fatalf("pid = %d, want 4242 from process_info", got)
	}

	if !hasEvent(t, d, evRegistered, "worker-emperor") {
		t.Fatal("no agent.registered line; an old replay would not see this agent")
	}
	spawned := findEvent(t, d, evSpawned, "worker-emperor")
	if spawned.Integration != "claude" || spawned.Config != "worker" || spawned.PaneID != "w1:p2" {
		t.Fatalf("agent.spawned = %+v", spawned)
	}
	if spawned.SessionID == "" || spawned.Cwd == "" {
		t.Fatalf("agent.spawned dropped session id or cwd: %+v", spawned)
	}
}

func slicesContains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestSpawnCodexByConfig(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start":       paneStartedReply,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":4242}}}`,
	})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"worker": {
			Integration: "codex",
			Model:       "gpt-5.6-sol",
			Sandbox:     "workspace-write",
		},
	})

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if resp.Name != "worker-emperor" || resp.PaneID != "w1:p2" {
		t.Fatalf("spawn = %+v, want worker-emperor in w1:p2", resp)
	}

	argv := strs(t, f.params("agent.start")["argv"])
	if argv[0] != "codex" {
		t.Fatalf("argv %v does not launch codex", argv)
	}
	for _, want := range []string{"--sandbox", "workspace-write", "--model", "gpt-5.6-sol"} {
		if !slicesContains(argv, want) {
			t.Fatalf("argv %v is missing %q", argv, want)
		}
	}
	// codex has no --session-id equivalent; a claude flag leaking in would be
	// passed to a binary that does not know it.
	if slicesContains(argv, "--session-id") {
		t.Fatalf("argv %v carries claude's --session-id", argv)
	}
	if _, ok := f.call("pane.report_metadata"); !ok {
		t.Fatal("no pane.report_metadata; the pane was never titled")
	}

	spawned := findEvent(t, d, evSpawned, "worker-emperor")
	if spawned.Integration != "codex" || spawned.PaneID != "w1:p2" {
		t.Fatalf("agent.spawned = %+v", spawned)
	}
	if spawned.SessionID != "" {
		t.Fatalf("codex journal line carries session id %q; codex owns its sessions", spawned.SessionID)
	}
}

func TestSpawnPiByModel(t *testing.T) {
	piStub(t)
	d := boundDaemon(t, nil)

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if resp.Name != "pi-emperor" {
		t.Fatalf("name = %q, want pi-emperor", resp.Name)
	}
	if resp.PaneID != "" {
		t.Fatalf("pi agent reported pane %q; it has no pane", resp.PaneID)
	}

	agent := d.agents["pi-emperor"]
	if agent.Integration != "pi" || agent.Model != "gpt-x" {
		t.Fatalf("roster entry = %+v", agent)
	}
	if agent.PID <= 0 {
		t.Fatalf("pid = %d, want the runner's", agent.PID)
	}

	// The stub announces itself on startup, which is the agent's first frame.
	awaitState(t, d, "pi-emperor", stateBusy)

	frames, err := os.ReadFile(filepath.Join(flock.Dir(d.root, d.flockName), "pi-pi-emperor.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(frames), `"agent_start"`) {
		t.Fatalf("frame log = %q, want the raw agent_start frame", frames)
	}
}

// Launching drops d.mu for as long as a process start takes, so concurrent
// spawns must not both walk away with the same species.
func TestConcurrentSpawnsGetDistinctNames(t *testing.T) {
	piStub(t)
	d := boundDaemon(t, nil)

	const n = 4
	names := make(chan string, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"})
			if err != nil {
				errs <- err
				return
			}
			names <- resp.Name
		}()
	}
	wg.Wait()
	close(names)
	close(errs)

	for err := range errs {
		t.Fatalf("spawn: %v", err)
	}
	seen := map[string]bool{}
	for name := range names {
		if seen[name] {
			t.Fatalf("%q was handed out twice; a launch in flight did not hold its name", name)
		}
		seen[name] = true
	}
	if len(seen) != n {
		t.Fatalf("%d distinct names from %d spawns", len(seen), n)
	}
}

// A launch that fails must hand its name back rather than parking a dead
// reservation on the slug forever.
func TestFailedLaunchReleasesItsName(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start": `{"id":"1","error":{"code":"no_workspace","message":"cannot start"}}`,
	})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "claude"}})

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"}); err == nil {
		t.Fatal("spawn reported success though agent.start failed")
	}

	d.mu.Lock()
	_, inRoster := d.agents["worker-emperor"]
	order := append([]string(nil), d.order...)
	d.mu.Unlock()

	if inRoster {
		t.Fatal("a failed launch left its reservation on the roster")
	}
	if slicesContains(order, "worker-emperor") {
		t.Fatalf("order = %v, still lists the released name", order)
	}
}

// Reusing a stopped agent's slug must not erase that agent when the new launch
// fails: the journal still records it, so the live roster would disagree with a
// replay of its own journal.
func TestFailedLaunchRestoresTheStoppedAgentItReused(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "claude"}})

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if _, err := d.stop(&protocol.Request{Op: protocol.OpStop, Name: "worker-emperor"}); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// The slug is free again, so this reserves the stopped agent's own name and
	// then fails to launch.
	failing := serveHerdr(t, map[string]string{
		"agent.start": `{"id":"1","error":{"code":"no_workspace","message":"cannot start"}}`,
	})
	d.session = herdr.Session{Name: "sess", SocketPath: failing.socket}
	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"}); err == nil {
		t.Fatal("spawn reported success though agent.start failed")
	}

	d.mu.Lock()
	got, ok := d.agents["worker-emperor"]
	d.mu.Unlock()
	if !ok {
		t.Fatal("the failed launch erased the stopped agent whose slug it reused")
	}
	if got.State != stateStopped || got.PaneID != "w1:p2" {
		t.Fatalf("restored entry = %+v, want the stopped agent unchanged", got)
	}

	// The live roster must match what this journal replays to.
	d.Close()
	restarted, err := New(d.root, d.flockName)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer restarted.Close()
	if replayed := restarted.agents["worker-emperor"]; replayed.State != got.State || replayed.PaneID != got.PaneID {
		t.Fatalf("replay = %+v, live roster = %+v; they disagree", replayed, got)
	}
}

// Herdr reports a null shell_pid for a pane whose process has not appeared
// yet, which is normal right after agent.start and stores PID 0. Judging a
// spawned agent by its pid would then read every fresh pane as dead and hand
// its name to the next spawn, overwriting a live agent's roster entry.
func TestSpawnWithUnknownShellPIDKeepsItsName(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start":       paneStartedReply,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":null}}}`,
	})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "claude"}})

	first, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"})
	if err != nil {
		t.Fatalf("first spawn: %v", err)
	}
	if got := d.agents[first.Name].PID; got != 0 {
		t.Fatalf("test setup: pid = %d, want the unknown-pid case", got)
	}

	second, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"})
	if err != nil {
		t.Fatalf("second spawn: %v", err)
	}
	if second.Name == first.Name {
		t.Fatalf("both spawns took %q; the live agent's entry was overwritten", first.Name)
	}

	d.mu.Lock()
	roster := len(d.agents)
	d.mu.Unlock()
	if roster != 2 {
		t.Fatalf("roster holds %d agents, want both spawns", roster)
	}

	// Neither pane may be stranded: both must still be stoppable by name.
	for _, name := range []string{first.Name, second.Name} {
		if _, err := d.stop(&protocol.Request{Op: protocol.OpStop, Name: name}); err != nil {
			t.Fatalf("stop %s: %v", name, err)
		}
	}
}

// countingJournal wraps the real journal to count Write calls.
type countingJournal struct {
	io.WriteCloser
	writes int
}

func (c *countingJournal) Write(p []byte) (int, error) {
	c.writes++
	return c.WriteCloser.Write(p)
}

// agent.registered and agent.spawned only describe an agent together: a
// registered line alone replays as a live agent with no integration. Writing
// them in one call is what makes a failure between them impossible.
func TestSpawnJournalsItsPairInOneWrite(t *testing.T) {
	piStub(t)
	d := boundDaemon(t, nil)

	counter := &countingJournal{WriteCloser: d.journal}
	d.journal = counter

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if counter.writes != 1 {
		t.Fatalf("spawn made %d journal writes, want 1: the pair can tear between them", counter.writes)
	}

	// Both events are nonetheless present and in replay order.
	seen := []string{}
	for _, e := range events(t, d) {
		if e.Name == "pi-emperor" {
			seen = append(seen, e.Event)
		}
	}
	if len(seen) != 2 || seen[0] != evRegistered || seen[1] != evSpawned {
		t.Fatalf("journaled %v, want [%s %s]", seen, evRegistered, evSpawned)
	}
}

// A launch the journal could not record must leave nothing behind: no roster
// entry, no burned slug, no child process, and nothing for replay to find.
func TestUnjournaledLaunchIsUnwound(t *testing.T) {
	installPi(t, piPidFile)
	d := boundDaemon(t, nil)

	// Closing the journal makes every append fail, which is the failure this
	// unwind exists for.
	d.journal.Close()

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"}); err == nil {
		t.Fatal("spawn reported success though its journal write failed")
	}

	d.mu.Lock()
	_, inRoster := d.agents["pi-emperor"]
	residue := len(d.agents)
	runners := len(d.runners)
	d.mu.Unlock()
	if inRoster || residue != 0 {
		t.Fatalf("roster = %d entries, want none: the unwind left residue", residue)
	}
	if runners != 0 {
		t.Fatalf("%d runners left registered after an unwound launch", runners)
	}

	// The child must be reaped by the time spawn returns, not merely orphaned.
	data, err := os.ReadFile(filepath.Join(d.root, "pi.pid"))
	if err != nil {
		t.Fatalf("stub never recorded its pid: %v", err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if alive(pid) {
		t.Fatalf("pi child %d survived the unwind", pid)
	}

	// With a working journal the slug is free again, and a replay of what was
	// written sees no trace of the failed launch.
	journal, err := os.OpenFile(journalPath(d.root, d.flockName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	d.journal = journal

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"})
	if err != nil {
		t.Fatalf("respawn: %v", err)
	}
	if resp.Name != "pi-emperor" {
		t.Fatalf("respawn took %q; the failed launch burned its slug", resp.Name)
	}
}

// An agent can die before spawn has finished registering it. The watcher reads
// a runner missing from d.runners as one the daemon stopped on purpose, so if
// it reaches the lock first the death goes unrecorded: the roster keeps calling
// a dead agent running and never gives its species back. The seam forces that
// ordering, which is otherwise microseconds wide and passes a stress run even
// with the fix reverted.
func TestAgentDyingDuringLaunchIsStillRecorded(t *testing.T) {
	installPi(t, piSelfExit)
	d := boundDaemon(t, nil)

	spawnLaunchDelay = func() { time.Sleep(150 * time.Millisecond) }
	t.Cleanup(func() { spawnLaunchDelay = nil })

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}

	// The agent was already gone before it was registered; its death must still
	// be recorded exactly once.
	awaitState(t, d, "pi-emperor", stateStopped)
	if stopped := findEvent(t, d, evStopped, "pi-emperor"); stopped.Reason != "exited" {
		t.Fatalf("reason = %q, want exited", stopped.Reason)
	}
	if n := countEvents(t, d, evStopped, "pi-emperor"); n != 1 {
		t.Fatalf("%d agent.stopped lines, want exactly 1", n)
	}

	// A recorded death hands the slug back.
	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"})
	if err != nil {
		t.Fatalf("respawn: %v", err)
	}
	if resp.Name != "pi-emperor" {
		t.Fatalf("respawn took %q; the lost death leaked the slug", resp.Name)
	}
}

// A claude launch the journal could not record must close its pane. Nothing
// else will: the pane is not on the roster, so no stop can ever reach it.
func TestUnjournaledClaudeLaunchClosesItsPane(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "claude"}})

	d.journal.Close()

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"}); err == nil {
		t.Fatal("spawn reported success though its journal write failed")
	}

	closed, ok := f.call("pane.close")
	if !ok {
		t.Fatal("pane.close never sent; the unjournaled pane is leaked forever")
	}
	if closed["pane_id"] != "w1:p2" {
		t.Fatalf("pane.close pane_id = %v, want w1:p2", closed["pane_id"])
	}

	d.mu.Lock()
	residue := len(d.agents)
	d.mu.Unlock()
	if residue != 0 {
		t.Fatalf("roster = %d entries, want none after the unwind", residue)
	}

	// The slug is reclaimable once the journal works again.
	journal, err := os.OpenFile(journalPath(d.root, d.flockName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	d.journal = journal

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"})
	if err != nil {
		t.Fatalf("respawn: %v", err)
	}
	if resp.Name != "worker-emperor" {
		t.Fatalf("respawn took %q; the failed launch burned its slug", resp.Name)
	}
}

// A bridge failure must reach a waiting agent the same way an unbridged send
// would: a parked wait takes the message rather than watching it queue.
func TestFailedBridgeWakesAParkedWaiter(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "claude"}})

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	sender, err := d.register(&protocol.Request{Op: protocol.OpRegister, Type: "lead", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		resp protocol.Response
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := d.wait(&protocol.Request{Op: protocol.OpWait, As: "worker-emperor", TimeoutMS: 5000}, nil)
		done <- result{resp, err}
	}()

	// Park the wait before sending, or it would just take a pending message.
	deadline := time.Now().Add(2 * time.Second)
	for {
		d.mu.Lock()
		parked := len(d.waiters)
		d.mu.Unlock()
		if parked == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("wait never parked")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Break the pane so the bridge cannot deliver.
	d.session = herdr.Session{Name: "sess", SocketPath: filepath.Join(t.TempDir(), "gone.sock")}
	if _, err := d.send(&protocol.Request{Op: protocol.OpSend, From: sender.Name, To: "worker-emperor", Body: "fallback"}); err == nil {
		t.Fatal("send reported success though the pane write failed")
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("parked wait: %v", r.err)
		}
		if r.resp.Message == nil || r.resp.Message.Body != "fallback" {
			t.Fatalf("waiter got %+v, want the undelivered message", r.resp.Message)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the parked waiter was never woken; the message only queued")
	}

	d.mu.Lock()
	pending := len(d.pending)
	d.mu.Unlock()
	if pending != 0 {
		t.Fatalf("%d pending after the waiter took it, want 0", pending)
	}
}

// An orphaned agent's pid belonged to a daemon that is gone, so its slug is
// free even if some unrelated process now happens to own that pid.
func TestOrphanedAgentReleasesItsSpecies(t *testing.T) {
	piStub(t)
	d := boundDaemon(t, nil)

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// Replay is what produces an orphan; forge that state directly, with a pid
	// that is unambiguously alive so only the state rule can free the slug.
	d.mu.Lock()
	a := d.agents["pi-emperor"]
	a.State = stateOrphaned
	a.PID = os.Getpid()
	d.agents["pi-emperor"] = a
	delete(d.runners, "pi-emperor")
	d.mu.Unlock()

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"})
	if err != nil {
		t.Fatalf("respawn: %v", err)
	}
	if resp.Name != "pi-emperor" {
		t.Fatalf("respawn took %q; an orphaned agent must release its slug", resp.Name)
	}
}

// A hand-off that fails leaves the message on the live queue, so the running
// daemon agrees with what its own journal replays to.
func TestFailedBridgeRequeuesTheMessage(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "claude"}})

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	sender, err := d.register(&protocol.Request{Op: protocol.OpRegister, Type: "lead", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}

	// Point the daemon at a dead socket so the pane write cannot land.
	d.session = herdr.Session{Name: "sess", SocketPath: filepath.Join(t.TempDir(), "gone.sock")}
	if _, err := d.send(&protocol.Request{Op: protocol.OpSend, From: sender.Name, To: "worker-emperor", Body: "undeliverable"}); err == nil {
		t.Fatal("send reported success though the pane write failed")
	}

	d.mu.Lock()
	pending := append([]protocol.Message(nil), d.pending...)
	d.mu.Unlock()
	if len(pending) != 1 || pending[0].Body != "undeliverable" {
		t.Fatalf("pending = %+v, want the undelivered message queued", pending)
	}

	// It is queued, not lost: a wait takes it, and takes it exactly once.
	got, err := d.wait(&protocol.Request{Op: protocol.OpWait, As: "worker-emperor", TimeoutMS: 2000}, nil)
	if err != nil {
		t.Fatalf("wait for the re-queued message: %v", err)
	}
	if got.Message == nil || got.Message.Body != "undeliverable" {
		t.Fatalf("wait delivered %+v", got.Message)
	}
	if _, err := d.wait(&protocol.Request{Op: protocol.OpWait, As: "worker-emperor", TimeoutMS: 100}, nil); err == nil {
		t.Fatal("the re-queued message was delivered twice")
	}

	// Replaying the journal must agree: delivered once, nothing left pending.
	d.Close()
	restarted, err := New(d.root, d.flockName)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer restarted.Close()
	if len(restarted.pending) != 0 {
		t.Fatalf("replay has %d pending after delivery, want none", len(restarted.pending))
	}
}

func TestSpawnClaudeNeedsBoundSession(t *testing.T) {
	d := boundDaemon(t, nil)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "claude"}})

	_, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"})
	if err == nil {
		t.Fatal("claude spawned on an unbound daemon; it has no session to open a pane in")
	}
	// The refusal must be the binding check, not an incidental dial failure
	// against the empty socket path.
	if !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("error = %v, want the unbound-session refusal", err)
	}
	if hasEvent(t, d, evRegistered, "worker-emperor") {
		t.Fatal("a failed spawn left its name reserved in the journal")
	}
}

func TestSpawnCodexNeedsBoundSession(t *testing.T) {
	d := boundDaemon(t, nil)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "codex"}})

	_, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"})
	if err == nil {
		t.Fatal("codex spawned on an unbound daemon; it has no session to open a pane in")
	}
	// The refusal must name codex: it is the operator's clue about which
	// config needs the pane.
	if !strings.Contains(err.Error(), "not bound") || !strings.Contains(err.Error(), "codex") {
		t.Fatalf("error = %v, want the unbound-session refusal naming codex", err)
	}
}

func TestResolveSpawnIntegrationOverride(t *testing.T) {
	d := boundDaemon(t, nil)
	tests := []struct {
		desc        string
		req         protocol.Request
		integration string
		provider    string
		wantErr     bool
	}{
		{
			desc:        "codex override drops the routed pi provider",
			req:         protocol.Request{Model: "gpt-5.6-sol", Integration: "codex"},
			integration: "codex",
		},
		{
			desc:        "pi override matching the route keeps its provider",
			req:         protocol.Request{Model: "gpt-5.5", Integration: "pi"},
			integration: "pi",
			provider:    "openai-codex",
		},
		{desc: "override with a config", req: protocol.Request{Config: "worker", Integration: "codex"}, wantErr: true},
		{desc: "provider with a codex override", req: protocol.Request{Model: "gpt-5.5", Integration: "codex", Provider: "openai"}, wantErr: true},
		{desc: "unknown override", req: protocol.Request{Model: "gpt-5.5", Integration: "goose"}, wantErr: true},
	}

	for _, tt := range tests {
		tt.req.Op = protocol.OpSpawn
		cfg, agentType, err := d.resolveSpawn(&tt.req)
		if tt.wantErr {
			if err == nil {
				t.Errorf("%s: resolved to %+v, want error", tt.desc, cfg)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tt.desc, err)
			continue
		}
		if cfg.Integration != tt.integration || cfg.Provider != tt.provider {
			t.Errorf("%s: cfg = %+v, want integration %q provider %q", tt.desc, cfg, tt.integration, tt.provider)
		}
		if agentType != tt.integration {
			t.Errorf("%s: agent type = %q, want the overriding integration %q", tt.desc, agentType, tt.integration)
		}
	}
}

func TestSpawnCodexByModelOverride(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-5.6-sol", Integration: "codex"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if resp.Name != "codex-emperor" {
		t.Fatalf("name = %q, want codex-emperor: the override names the pool", resp.Name)
	}
	argv := strs(t, f.params("agent.start")["argv"])
	if argv[0] != "codex" || !slicesContains(argv, "gpt-5.6-sol") {
		t.Fatalf("argv = %v, want a codex launch of gpt-5.6-sol", argv)
	}
}

func TestSpawnRejectsUnknownConfigAndModel(t *testing.T) {
	d := boundDaemon(t, nil)

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "absent"}); err == nil {
		t.Fatal("spawn accepted a config name that is not in agents.json")
	}
	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "llama-3"}); err == nil {
		t.Fatal("spawn accepted an unroutable model")
	}
	_, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn})
	if err == nil {
		t.Fatal("spawn accepted a request naming neither a config nor a model")
	}
	// Not merely an unroutable empty model: the request itself is malformed.
	if !strings.Contains(err.Error(), "config name or a model") {
		t.Fatalf("error = %v, want the missing-selector refusal", err)
	}
}

// A config and a model each fully determine what to launch, so a request
// carrying both is refused rather than silently resolved by one of them.
func TestSpawnRejectsConfigAndModelTogether(t *testing.T) {
	piStub(t)
	d := boundDaemon(t, nil)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "pi", Provider: "openai"}})

	// Both selectors are individually valid, so only the guard can refuse this.
	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker", Model: "gpt-x"}); err == nil {
		t.Fatal("spawn accepted both a config and a model")
	}
}

func TestSendToCodexAgentGoesToItsPane(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "codex"}})

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	sender, err := d.register(&protocol.Request{Op: protocol.OpRegister, Type: "lead", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := d.send(&protocol.Request{Op: protocol.OpSend, From: sender.Name, To: "worker-emperor", Body: "do the thing"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	input := f.params("pane.send_input")
	if input["text"] != "do the thing" {
		t.Fatalf("pane text = %v", input["text"])
	}
	if keys := strs(t, input["keys"]); len(keys) != 1 || keys[0] != "enter" {
		t.Fatalf("pane keys = %v, want [enter]", keys)
	}
}

func TestStopCodexClosesItsPane(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "codex"}})

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if _, err := d.stop(&protocol.Request{Op: protocol.OpStop, Name: "worker-emperor"}); err != nil {
		t.Fatalf("stop: %v", err)
	}

	if closed := f.params("pane.close"); closed["pane_id"] != "w1:p2" {
		t.Fatalf("pane.close pane_id = %v, want w1:p2", closed["pane_id"])
	}
	if got := agentState(d, "worker-emperor"); got != stateStopped {
		t.Fatalf("state = %q, want stopped", got)
	}
}

func TestSendToClaudeAgentGoesToItsPane(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "claude"}})

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	sender, err := d.register(&protocol.Request{Op: protocol.OpRegister, Type: "lead", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}

	sent, err := d.send(&protocol.Request{Op: protocol.OpSend, From: sender.Name, To: "worker-emperor", Body: "do the thing"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	input := f.params("pane.send_input")
	if input["text"] != "do the thing" {
		t.Fatalf("pane text = %v", input["text"])
	}
	// Only keys:["enter"] actually submits in a TUI (EXP2).
	if keys := strs(t, input["keys"]); len(keys) != 1 || keys[0] != "enter" {
		t.Fatalf("keys = %v, want [enter]", keys)
	}

	var delivered bool
	for _, e := range events(t, d) {
		if e.Event == evDelivered && e.ID == sent.ID {
			delivered = true
		}
	}
	if !delivered {
		t.Fatal("bridged send was not journaled as delivered")
	}

	d.mu.Lock()
	pending := len(d.pending)
	d.mu.Unlock()
	if pending != 0 {
		t.Fatalf("%d messages queued; a bridged send must not sit in the pending queue", pending)
	}
}

func TestSendToPiAgentPromptsIt(t *testing.T) {
	piStub(t)
	d := boundDaemon(t, nil)

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	sender, err := d.register(&protocol.Request{Op: protocol.OpRegister, Type: "lead", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	awaitState(t, d, "pi-emperor", stateBusy)

	sent, err := d.send(&protocol.Request{Op: protocol.OpSend, From: sender.Name, To: "pi-emperor", Body: "ship it"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	// The stub settles the prompt it was given, which is the only thing that
	// can move the agent out of busy.
	awaitState(t, d, "pi-emperor", stateSettled)

	settled := findEvent(t, d, evSettled, "pi-emperor")
	if settled.MsgID != sent.ID {
		t.Fatalf("agent.settled msg_id = %q, want the prompt's id %q", settled.MsgID, sent.ID)
	}
}

func TestStopPiReapsItAndFreesItsSpecies(t *testing.T) {
	piStub(t)
	d := boundDaemon(t, nil)

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	pid := d.agents["pi-emperor"].PID

	if _, err := d.stop(&protocol.Request{Op: protocol.OpStop, Name: "pi-emperor"}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if alive(pid) {
		t.Fatalf("pid %d still alive after stop", pid)
	}
	if got := agentState(d, "pi-emperor"); got != stateStopped {
		t.Fatalf("state = %q, want stopped", got)
	}
	if stopped := findEvent(t, d, evStopped, "pi-emperor"); stopped.Reason != "requested" {
		t.Fatalf("agent.stopped reason = %q, want requested", stopped.Reason)
	}

	// A stopped agent hands its species back.
	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"})
	if err != nil {
		t.Fatalf("respawn: %v", err)
	}
	if resp.Name != "pi-emperor" {
		t.Fatalf("respawn took %q; a stopped agent's slug must be reusable", resp.Name)
	}
}

// An agent that dies badly is still an agent that stopped. Runner.Stop reports
// the abnormal exit, but the process is reaped either way, so the operator's
// stop must succeed rather than fail on the corpse's exit status.
func TestStopSucceedsWhenAgentExitsAbnormally(t *testing.T) {
	installPi(t, piCrash)
	d := boundDaemon(t, nil)

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	pid := d.agents["pi-emperor"].PID

	if _, err := d.stop(&protocol.Request{Op: protocol.OpStop, Name: "pi-emperor"}); err != nil {
		t.Fatalf("stop reported failure though the agent is down: %v", err)
	}
	if alive(pid) {
		t.Fatalf("pid %d still alive after stop", pid)
	}
	if got := agentState(d, "pi-emperor"); got != stateStopped {
		t.Fatalf("state = %q, want stopped", got)
	}
	if n := countEvents(t, d, evStopped, "pi-emperor"); n != 1 {
		t.Fatalf("%d agent.stopped lines, want exactly 1", n)
	}

	// A crashed agent releases its slug like any other stopped one.
	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"})
	if err != nil {
		t.Fatalf("respawn: %v", err)
	}
	if resp.Name != "pi-emperor" {
		t.Fatalf("respawn took %q, want the crashed agent's freed slug", resp.Name)
	}
}

// The watcher and an operator stop can both reach the lock for the same death.
// Whichever arrives first records it; the journal must not gain a second line.
func TestStopAfterSelfExitJournalsOnce(t *testing.T) {
	installPi(t, piSelfExit)
	d := boundDaemon(t, nil)

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// The agent exits on its own, so the watcher records the stop first.
	awaitState(t, d, "pi-emperor", stateStopped)
	if stopped := findEvent(t, d, evStopped, "pi-emperor"); stopped.Reason != "exited" {
		t.Fatalf("reason = %q, want exited", stopped.Reason)
	}

	if _, err := d.stop(&protocol.Request{Op: protocol.OpStop, Name: "pi-emperor"}); err != nil {
		t.Fatalf("stop on an already-dead agent: %v", err)
	}
	if n := countEvents(t, d, evStopped, "pi-emperor"); n != 1 {
		t.Fatalf("%d agent.stopped lines, want exactly 1: the stop double-journaled a death the watcher already recorded", n)
	}

	// A journal with one stop line per death replays to exactly that state.
	d.Close()
	restarted, err := New(d.root, d.flockName)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer restarted.Close()
	if got := restarted.agents["pi-emperor"].State; got != stateStopped {
		t.Fatalf("replayed state = %q, want stopped", got)
	}
}

func TestStopClaudeClosesItsPane(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "claude"}})

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if _, err := d.stop(&protocol.Request{Op: protocol.OpStop, Name: "worker-emperor"}); err != nil {
		t.Fatalf("stop: %v", err)
	}

	closed := f.params("pane.close")
	if closed["pane_id"] != "w1:p2" {
		t.Fatalf("pane.close pane_id = %v, want w1:p2", closed["pane_id"])
	}
	if got := agentState(d, "worker-emperor"); got != stateStopped {
		t.Fatalf("state = %q, want stopped", got)
	}
	if !hasEvent(t, d, evStopped, "worker-emperor") {
		t.Fatal("stopping a claude agent was not journaled")
	}
}

// A stopped agent's slug is free even while its process is still up: a closed
// pane's shell can outlive the close, and state, not pid, is what says a
// spawned agent is done.
func TestStopFreesSpeciesOfStillLiveProcess(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start": paneStartedReply,
		// The test process itself, so alive(pid) is unambiguously true.
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":` + strconv.Itoa(os.Getpid()) + `}}}`,
	})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "claude"}})

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if !alive(d.agents["worker-emperor"].PID) {
		t.Fatal("test setup: the spawned agent's pid must be live for this to prove anything")
	}
	if _, err := d.stop(&protocol.Request{Op: protocol.OpStop, Name: "worker-emperor"}); err != nil {
		t.Fatalf("stop: %v", err)
	}

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"})
	if err != nil {
		t.Fatalf("respawn: %v", err)
	}
	if resp.Name != "worker-emperor" {
		t.Fatalf("respawn took %q; a stopped agent's slug must be free despite its live pid", resp.Name)
	}
}

func TestStopRejectsSelfRegisteredAgent(t *testing.T) {
	d := boundDaemon(t, nil)
	resp, err := d.register(&protocol.Request{Op: protocol.OpRegister, Type: "lead", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := d.stop(&protocol.Request{Op: protocol.OpStop, Name: resp.Name}); err == nil {
		t.Fatal("stop reaped an agent fledge never spawned")
	}
}

func TestReplayRestoresSpawnedAgents(t *testing.T) {
	piStub(t)
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{"worker": {Integration: "claude", Model: "claude-opus-4"}})

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"}); err != nil {
		t.Fatalf("spawn claude: %v", err)
	}
	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"}); err != nil {
		t.Fatalf("spawn pi: %v", err)
	}
	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x", Species: "king"}); err != nil {
		t.Fatalf("spawn second pi: %v", err)
	}
	if _, err := d.stop(&protocol.Request{Op: protocol.OpStop, Name: "pi-king"}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	d.Close()

	restarted, err := New(d.root, d.flockName)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer restarted.Close()

	claude := restarted.agents["worker-emperor"]
	if claude.Integration != "claude" || claude.Config != "worker" || claude.PaneID != "w1:p2" || claude.Model != "claude-opus-4" {
		t.Fatalf("claude agent lost its metadata across replay: %+v", claude)
	}
	// The pane may well have outlived the daemon, so its state stands.
	if claude.State != stateRunning {
		t.Fatalf("claude state = %q, want running", claude.State)
	}

	// A pi agent's pipes died with the daemon that owned them.
	if got := restarted.agents["pi-emperor"].State; got != stateOrphaned {
		t.Fatalf("replayed pi state = %q, want orphaned", got)
	}
	if got := restarted.agents["pi-king"].State; got != stateStopped {
		t.Fatalf("stopped pi replayed as %q, want stopped", got)
	}
}

func TestCloseReapsRunners(t *testing.T) {
	piStub(t)
	d := boundDaemon(t, nil)

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	pid := d.agents["pi-emperor"].PID

	d.Close()

	if syscall.Kill(pid, 0) == nil {
		t.Fatalf("pi agent %d outlived the daemon that owned its pipes", pid)
	}
}

// The request's split reaches agent.start, so a caller can place the pane it
// asked for. A request that names no split must not send one.
func TestSpawnClaudePassesSplitThrough(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start":       paneStartedReply,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":4242}}}`,
	})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"worker": {Integration: "claude", Model: "claude-opus-4"},
	})

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker", Split: "right"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if got := f.params("agent.start")["split"]; got != "right" {
		t.Fatalf("agent.start split = %v, want right", got)
	}
}

func TestSpawnClaudeWithoutSplitSendsNone(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start":       paneStartedReply,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":4242}}}`,
	})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"worker": {Integration: "claude", Model: "claude-opus-4"},
	})

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "worker"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if _, ok := f.params("agent.start")["split"]; ok {
		t.Fatal("agent.start carried a split for a request that named none")
	}
}

// claudeHerdr is a fake session that answers everything a claude launch asks.
func claudeHerdr(t *testing.T) *fakeHerdr {
	t.Helper()
	return serveHerdr(t, map[string]string{
		"agent.start":       paneStartedReply,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":4242}}}`,
	})
}

// The orchestrator is addressed by a name the operator already knows, so it
// runs under its bare config name with no species suffix.
func TestSpawnOrchestratorRunsUnderItsBareName(t *testing.T) {
	f := claudeHerdr(t)
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{
		agentcfg.ReservedOrchestrator: {Integration: "claude", Model: "claude-opus-4"},
	})

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: agentcfg.ReservedOrchestrator})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if resp.Name != agentcfg.ReservedOrchestrator {
		t.Fatalf("name = %q, want the bare %q", resp.Name, agentcfg.ReservedOrchestrator)
	}
	if got := d.agents[agentcfg.ReservedOrchestrator].Species; got != "" {
		t.Errorf("orchestrator carries species %q, want none", got)
	}
	// The pane is titled with the same fixed name, so the roster and herdr agree.
	if got := f.params("agent.start")["name"]; got != agentcfg.ReservedOrchestrator {
		t.Errorf("pane name = %v, want %q", got, agentcfg.ReservedOrchestrator)
	}
	if !hasEvent(t, d, evRegistered, agentcfg.ReservedOrchestrator) {
		t.Error("the bare orchestrator name never reached the journal")
	}
}

// The carve-out is a name, not a licence: a hyphenated config that is not the
// reserved one is still refused at spawn, not just at config validation.
func TestSpawnStillRejectsOtherHyphenatedConfigs(t *testing.T) {
	d := boundDaemon(t, claudeHerdr(t))
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"some-agent": {Integration: "claude", Model: "claude-opus-4"},
	})

	_, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "some-agent"})
	if err == nil {
		t.Fatal("spawn accepted a hyphenated non-reserved config")
	}
	if !strings.Contains(err.Error(), "lowercase letters and digits only") {
		t.Errorf("unexpected error: %v", err)
	}
}

// The orchestrator's name pool is that one name, so a second spawn collides
// with the live first exactly as an exhausted species pool does.
func TestSpawnSecondOrchestratorCollides(t *testing.T) {
	d := boundDaemon(t, claudeHerdr(t))
	writeAgents(t, d.root, map[string]agentcfg.Config{
		agentcfg.ReservedOrchestrator: {Integration: "claude", Model: "claude-opus-4"},
	})

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: agentcfg.ReservedOrchestrator}); err != nil {
		t.Fatalf("first spawn: %v", err)
	}
	_, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: agentcfg.ReservedOrchestrator})
	if err == nil {
		t.Fatal("a second orchestrator spawned alongside the live one")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("unexpected error: %v", err)
	}
}

// A species is meaningless for a fixed-name agent, so asking for one is an
// error rather than a silently ignored flag.
func TestSpawnOrchestratorRejectsRequestedSpecies(t *testing.T) {
	d := boundDaemon(t, claudeHerdr(t))
	writeAgents(t, d.root, map[string]agentcfg.Config{
		agentcfg.ReservedOrchestrator: {Integration: "claude", Model: "claude-opus-4"},
	})

	_, err := d.spawn(&protocol.Request{
		Op: protocol.OpSpawn, Config: agentcfg.ReservedOrchestrator, Species: "emperor",
	})
	if err == nil {
		t.Fatal("spawn accepted a species for the orchestrator")
	}
	if !strings.Contains(err.Error(), "takes no species") {
		t.Errorf("unexpected error: %v", err)
	}
}

// The picker at `fledge start` chooses which model runs as the orchestrator, so
// a marked spawn keeps the picked config but takes the reserved bare name.
func TestSpawnMarkedConfigRunsUnderTheReservedName(t *testing.T) {
	f := claudeHerdr(t)
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"opus48": {Integration: "claude", Model: "claude-opus-4-8"},
	})

	resp, err := d.spawn(&protocol.Request{
		Op: protocol.OpSpawn, Config: "opus48", Orchestrator: true,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if resp.Name != agentcfg.ReservedOrchestrator {
		t.Fatalf("name = %q, want the bare %q", resp.Name, agentcfg.ReservedOrchestrator)
	}

	// The roster stays truthful about which config is running as the
	// orchestrator, so `agent list` still names the model behind the name.
	a := d.agents[agentcfg.ReservedOrchestrator]
	if a.Config != "opus48" {
		t.Errorf("roster config = %q, want the picked opus48", a.Config)
	}
	if a.Model != "claude-opus-4-8" {
		t.Errorf("roster model = %q, want the picked config's model", a.Model)
	}
	if a.Species != "" {
		t.Errorf("orchestrator carries species %q, want none", a.Species)
	}
	if got := f.params("agent.start")["name"]; got != agentcfg.ReservedOrchestrator {
		t.Errorf("pane name = %v, want %q", got, agentcfg.ReservedOrchestrator)
	}
}

// Containment: the marker is what moves a config onto the reserved name. The
// same config spawned without it keeps ordinary <config>-<species> naming, so
// `fledge agent spawn` is untouched by the feature.
func TestSpawnWithoutMarkerKeepsSpeciesNaming(t *testing.T) {
	d := boundDaemon(t, claudeHerdr(t))
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"opus48": {Integration: "claude", Model: "claude-opus-4-8"},
	})

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "opus48"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if !strings.HasPrefix(resp.Name, "opus48-") || resp.Name == "opus48-" {
		t.Fatalf("name = %q, want an opus48-<species> name", resp.Name)
	}
	if _, ok := d.agents[agentcfg.ReservedOrchestrator]; ok {
		t.Error("an unmarked spawn claimed the reserved orchestrator name")
	}
}

// A species is as meaningless on a marked orchestrator as on the reserved
// config itself, so asking for one is an error rather than a silent override.
func TestSpawnMarkedConfigRejectsRequestedSpecies(t *testing.T) {
	d := boundDaemon(t, claudeHerdr(t))
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"opus48": {Integration: "claude", Model: "claude-opus-4-8"},
	})

	_, err := d.spawn(&protocol.Request{
		Op: protocol.OpSpawn, Config: "opus48", Orchestrator: true, Species: "emperor",
	})
	if err == nil {
		t.Fatal("spawn accepted a species for a marked orchestrator")
	}
	if !strings.Contains(err.Error(), "takes no species") {
		t.Errorf("unexpected error: %v", err)
	}
}

// The reserved name is one name however the agent on it got there, so a marked
// spawn collides with a live orchestrator exactly as a second reserved one does.
func TestSpawnMarkedConfigCollidesWithLiveOrchestrator(t *testing.T) {
	d := boundDaemon(t, claudeHerdr(t))
	writeAgents(t, d.root, map[string]agentcfg.Config{
		agentcfg.ReservedOrchestrator: {Integration: "claude", Model: "claude-opus-4"},
		"opus48":                      {Integration: "claude", Model: "claude-opus-4-8"},
	})

	if _, err := d.spawn(&protocol.Request{
		Op: protocol.OpSpawn, Config: agentcfg.ReservedOrchestrator,
	}); err != nil {
		t.Fatalf("first spawn: %v", err)
	}
	_, err := d.spawn(&protocol.Request{
		Op: protocol.OpSpawn, Config: "opus48", Orchestrator: true,
	})
	if err == nil {
		t.Fatal("a marked orchestrator spawned alongside the live one")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("unexpected error: %v", err)
	}
}

// A self-registered orchestrator takes the same bare reserved name a spawned
// one does, so an orchestrator is the same agent whichever way it joined.
func TestRegisterReservedTypeTakesTheBareName(t *testing.T) {
	d := newTestDaemon(t)

	resp, err := d.register(&protocol.Request{
		Op: protocol.OpRegister, Type: agentcfg.ReservedOrchestrator, PID: os.Getpid(),
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if resp.Name != agentcfg.ReservedOrchestrator {
		t.Fatalf("name = %q, want the bare %q", resp.Name, agentcfg.ReservedOrchestrator)
	}
	if got := d.agents[agentcfg.ReservedOrchestrator].Species; got != "" {
		t.Errorf("registered orchestrator carries species %q, want none", got)
	}
}

// The reserved pool is one name, so a second live registration collides just
// as an exhausted species pool does.
func TestRegisterSecondReservedCollides(t *testing.T) {
	d := newTestDaemon(t)

	if _, err := d.register(&protocol.Request{
		Op: protocol.OpRegister, Type: agentcfg.ReservedOrchestrator, PID: os.Getpid(),
	}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, err := d.register(&protocol.Request{
		Op: protocol.OpRegister, Type: agentcfg.ReservedOrchestrator, PID: os.Getpid(),
	})
	if err == nil {
		t.Fatal("a second orchestrator registered alongside the live one")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("unexpected error: %v", err)
	}
}

// A species is as meaningless on a registered orchestrator as on a spawned
// one, so asking for one is an error rather than a silently ignored flag.
func TestRegisterReservedTypeRejectsRequestedSpecies(t *testing.T) {
	d := newTestDaemon(t)

	_, err := d.register(&protocol.Request{
		Op: protocol.OpRegister, Type: agentcfg.ReservedOrchestrator,
		Species: "emperor", PID: os.Getpid(),
	})
	if err == nil {
		t.Fatal("register accepted a species for the orchestrator")
	}
	if !strings.Contains(err.Error(), "takes no species") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Ordinary types are untouched by the reserved carve-out.
func TestRegisterOrdinaryTypeKeepsItsSpecies(t *testing.T) {
	d := newTestDaemon(t)

	resp, err := d.register(&protocol.Request{
		Op: protocol.OpRegister, Type: "reviewer", PID: os.Getpid(),
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if resp.Name != "reviewer-emperor" {
		t.Errorf("name = %q, want reviewer-emperor", resp.Name)
	}
}
