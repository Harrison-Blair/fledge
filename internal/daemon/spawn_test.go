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
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/filebridge"
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
		if method == "pane.current" {
			reply = `{"id":"1","result":{"type":"pane_current","pane":{"pane_id":"w1:p1","focused":true}}}`
		} else if method == "agent.get" {
			reply = `{"id":"1","result":{"type":"agent","agent":{"pane_id":"w1:p2","agent_status":"idle"}}}`
		} else {
			reply = `{"id":"1","result":{}}`
		}
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

func (f *fakeHerdr) methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.got))
	for _, env := range f.got {
		out = append(out, strings.Trim(string(env["method"]), `"`))
	}
	return out
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

// writeCatalog installs generated catalog entries in the workspace.
func writeCatalog(t *testing.T, root string, configs map[string]agentcfg.Config) {
	t.Helper()
	data, err := json.Marshal(configs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, scaffold.DirName, agentcfg.CatalogName), data, 0o644); err != nil {
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

const dedicatedWorkspaceReply = `{"id":"1","result":{"workspace":{"workspace_id":"w9"},"tab":{"tab_id":"w9:t1"},"root_pane":{"pane_id":"w9:p1"}}}`

func writeDedicatedDefinition(t *testing.T, root, model string) {
	t.Helper()
	name := filepath.Join(root, scaffold.DirName, agentcfg.AgentsDir, agentcfg.UserDir, "context-planner", "context-planner.agent.md")
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
name: context-planner
description: Plan repository context.
model: ` + model + `
fledge:
  profile: context-profile
  workspace:
    label: fledge-context
    tab: context
---
Plan context.
`
	if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSpawnDefinitionInDedicatedWorkspace(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"workspace.create":  dedicatedWorkspaceReply,
		"agent.start":       `{"id":"1","result":{"agent":{"pane_id":"w9:p2","terminal_id":"term_x"}}}`,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":4242}}}`,
	})
	d := boundDaemon(t, f)
	writeDedicatedDefinition(t, d.root, "claude-opus-4")

	resp, err := d.spawn(&protocol.Request{Agent: "context-planner"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if resp.Name != "context-planner-emperor" || resp.PaneID != "w9:p2" {
		t.Fatalf("spawn = %+v", resp)
	}
	create := f.params("workspace.create")
	if create["label"] != "fledge-context" || create["focus"] != false || create["cwd"] != d.root {
		t.Fatalf("workspace.create = %+v", create)
	}
	if rename := f.params("tab.rename"); rename["tab_id"] != "w9:t1" || rename["label"] != "context" {
		t.Fatalf("tab.rename = %+v", rename)
	}
	start := f.params("agent.start")
	if start["workspace_id"] != "w9" || start["tab_id"] != "w9:t1" || start["focus"] != false {
		t.Fatalf("agent.start placement = %+v", start)
	}
	if close := f.params("pane.close"); close["pane_id"] != "w9:p1" {
		t.Fatalf("initial shell close = %+v", close)
	}
	if focus := f.params("pane.focus"); focus["pane_id"] != "w1:p1" {
		t.Fatalf("restored focus = %+v", focus)
	}

	agent := d.agents[resp.Name]
	if agent.WorkspaceID != "w9" || agent.WorkspaceLabel != "fledge-context" {
		t.Fatalf("roster workspace = %+v", agent)
	}
	spawned := findEvent(t, d, evSpawned, resp.Name)
	if spawned.WorkspaceID != "w9" || spawned.WorkspaceLabel != "fledge-context" {
		t.Fatalf("journal workspace = %+v", spawned)
	}
	replayed, err := replay(journalPath(d.root, d.flockName))
	if err != nil {
		t.Fatal(err)
	}
	if got := replayed.agents[resp.Name]; got.WorkspaceID != "w9" || got.WorkspaceLabel != "fledge-context" {
		t.Fatalf("replayed workspace = %+v", got)
	}

	if _, err := d.stop(&protocol.Request{Name: resp.Name}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if close := f.params("workspace.close"); close["workspace_id"] != "w9" {
		t.Fatalf("workspace close = %+v", close)
	}
	if got := f.count("pane.close"); got != 1 {
		t.Fatalf("pane.close count = %d, want only initial shell cleanup", got)
	}
}

func TestDedicatedWorkspaceRejectsPiProfile(t *testing.T) {
	d := boundDaemon(t, nil)
	writeDedicatedDefinition(t, d.root, "gpt-x")
	_, err := d.spawn(&protocol.Request{Agent: "context-planner"})
	if err == nil || !strings.Contains(err.Error(), "do not support Herdr workspace placement") || !strings.Contains(err.Error(), "claude or codex") {
		t.Fatalf("error = %v", err)
	}
	if len(d.agents) != 0 {
		t.Fatalf("placement failure reserved an agent: %+v", d.agents)
	}
}

func TestDedicatedWorkspaceLaunchFailuresRollBack(t *testing.T) {
	failure := `{"id":"1","error":{"code":"failed","message":"nope"}}`
	for _, tt := range []struct {
		name    string
		replies map[string]string
		closed  bool
	}{
		{"create", map[string]string{"workspace.create": failure}, false},
		{"incomplete ids", map[string]string{"workspace.create": `{"id":"1","result":{"workspace":{"workspace_id":"w9"}}}`}, true},
		{"tab rename", map[string]string{"workspace.create": dedicatedWorkspaceReply, "tab.rename": failure}, true},
		{"agent start", map[string]string{"workspace.create": dedicatedWorkspaceReply, "agent.start": failure}, true},
		{"shell cleanup", map[string]string{"workspace.create": dedicatedWorkspaceReply, "agent.start": paneStartedReply, "pane.close": failure}, true},
		{"focus restore", map[string]string{"workspace.create": dedicatedWorkspaceReply, "agent.start": paneStartedReply, "pane.focus": failure}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := serveHerdr(t, tt.replies)
			d := boundDaemon(t, f)
			writeDedicatedDefinition(t, d.root, "claude-opus-4")
			if _, err := d.spawn(&protocol.Request{Agent: "context-planner"}); err == nil {
				t.Fatal("spawn succeeded")
			}
			if got := f.count("workspace.close") > 0; got != tt.closed {
				t.Fatalf("workspace close = %v, want %v; methods %v", got, tt.closed, f.methods())
			}
			if len(d.agents) != 0 {
				t.Fatalf("failed launch left roster residue: %+v", d.agents)
			}
		})
	}
}

func TestUnjournaledDedicatedLaunchClosesWorkspace(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"workspace.create": dedicatedWorkspaceReply,
		"agent.start":      `{"id":"1","result":{"agent":{"pane_id":"w9:p2","terminal_id":"term_x"}}}`,
	})
	d := boundDaemon(t, f)
	writeDedicatedDefinition(t, d.root, "claude-opus-4")
	d.journal.Close()

	if _, err := d.spawn(&protocol.Request{Agent: "context-planner"}); err == nil {
		t.Fatal("spawn succeeded with a closed journal")
	}
	if close := f.params("workspace.close"); close["workspace_id"] != "w9" {
		t.Fatalf("workspace rollback = %+v", close)
	}
	if got := f.count("pane.close"); got != 1 {
		t.Fatalf("pane.close count = %d, want only initial shell cleanup", got)
	}
	if len(d.agents) != 0 {
		t.Fatalf("failed journal left roster residue: %+v", d.agents)
	}
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

func TestSpawnDiscoveredClaudeFamilyUsesNativeLauncher(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start":       paneStartedReply,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":4242}}}`,
	})
	d := boundDaemon(t, f)
	writeCatalog(t, d.root, map[string]agentcfg.Config{
		"opus": {Integration: "claude", Model: "opus"},
	})

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "opus"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if resp.Name != "opus-emperor" {
		t.Fatalf("name = %q, want opus-emperor", resp.Name)
	}

	argv := strs(t, f.params("agent.start")["argv"])
	if len(argv) != 5 || argv[0] != "claude" || argv[1] != "--session-id" || argv[2] == "" ||
		argv[3] != "--model" || argv[4] != "opus" {
		t.Fatalf("argv = %v, want claude --session-id <uuid> --model opus", argv)
	}
	if slicesContains(argv, "--provider") {
		t.Fatalf("argv %v carries an API-provider flag", argv)
	}
}

func TestSpawnDiscoveredClaudeDefaultLeavesModelUnspecified(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start":       paneStartedReply,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":4242}}}`,
	})
	d := boundDaemon(t, f)
	writeCatalog(t, d.root, map[string]agentcfg.Config{
		"default": {Integration: "claude"},
	})

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "default"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if resp.Name != "default-emperor" {
		t.Fatalf("name = %q, want default-emperor", resp.Name)
	}

	argv := strs(t, f.params("agent.start")["argv"])
	if len(argv) != 3 || argv[0] != "claude" || argv[1] != "--session-id" || argv[2] == "" {
		t.Fatalf("argv = %v, want claude --session-id <uuid>", argv)
	}
	for _, unwanted := range []string{"--model", "--provider"} {
		if slicesContains(argv, unwanted) {
			t.Fatalf("argv %v carries %s for the default launcher", argv, unwanted)
		}
	}
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
	f := serveHerdr(t, map[string]string{
		"agent.start":       paneStartedReply,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":4242}}}`,
	})
	d := boundDaemon(t, f)

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if resp.Name != "pi-emperor" || resp.PaneID != "w1:p2" {
		t.Fatalf("spawn = %+v, want pi-emperor in w1:p2", resp)
	}

	agent := d.agents["pi-emperor"]
	if agent.Integration != "pi" || agent.Model != "gpt-x" || agent.PaneID != "w1:p2" {
		t.Fatalf("roster entry = %+v", agent)
	}

	argv := strs(t, f.params("agent.start")["argv"])
	want := []string{"pi", "--provider", "openai-codex", "--model", "gpt-x"}
	if len(argv) != len(want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	for i, w := range want {
		if argv[i] != w {
			t.Fatalf("argv = %v, want %v", argv, want)
		}
	}

	spawned := findEvent(t, d, evSpawned, "pi-emperor")
	if spawned.Integration != "pi" || spawned.PaneID != "w1:p2" {
		t.Fatalf("agent.spawned = %+v", spawned)
	}
	if spawned.SessionID != "" {
		t.Fatalf("pi journal line carries session id %q; pi owns its sessions", spawned.SessionID)
	}

	// No RPC subprocess means no frame log.
	if _, err := os.Stat(filepath.Join(flock.Dir(d.root, d.flockName), "pi-pi-emperor.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("frame log exists for a pane-hosted pi: %v", err)
	}
}

func TestFileBridgeDispatchesSpawn(t *testing.T) {
	d := boundDaemon(t, serveHerdr(t, map[string]string{"agent.start": paneStartedReply}))
	go d.serveFileRequests()

	id, err := filebridge.Submit(d.root, d.flockName, protocol.Request{
		Op: protocol.OpSpawn, Model: "gpt-x",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := filebridge.Await(d.root, d.flockName, id, time.Second, time.Second)
	if err != nil {
		t.Fatalf("file bridge spawn: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("file bridge spawn response: %s", resp.Error)
	}
	if resp.Name != "pi-emperor" {
		t.Fatalf("spawned name = %q", resp.Name)
	}
	d.mu.Lock()
	spawned := d.agents[resp.Name]
	d.mu.Unlock()
	if spawned.Integration != "pi" || spawned.State == stateStopped {
		t.Fatalf("spawned agent = %+v", spawned)
	}
}

func TestFileBridgeDispatchesMessageWait(t *testing.T) {
	d := newTestDaemon(t)
	if _, err := d.register(&protocol.Request{Type: "sender", PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.register(&protocol.Request{Type: "receiver", PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	go d.serveFileRequests()

	waitID, err := filebridge.Submit(d.root, d.flockName, protocol.Request{
		Op: protocol.OpWait, As: "receiver-emperor", TimeoutMS: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	waited := make(chan protocol.Response, 1)
	waitErr := make(chan error, 1)
	go func() {
		resp, err := filebridge.Await(d.root, d.flockName, waitID, time.Second, 2*time.Second)
		waited <- resp
		waitErr <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		d.mu.Lock()
		parked := len(d.waiters)
		d.mu.Unlock()
		if parked == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("file bridge wait never parked")
		}
		time.Sleep(5 * time.Millisecond)
	}

	sendID, err := filebridge.Submit(d.root, d.flockName, protocol.Request{
		Op: protocol.OpSend, From: "sender-emperor", To: "receiver-emperor", Body: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp, err := filebridge.Await(d.root, d.flockName, sendID, time.Second, time.Second); err != nil || resp.Error != "" {
		t.Fatalf("file bridge send = %+v, %v", resp, err)
	}
	resp := <-waited
	if err := <-waitErr; err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" || resp.Message == nil || resp.Message.Body != "hello" {
		t.Fatalf("file bridge wait = %+v", resp)
	}
}

// Launching drops d.mu for as long as a process start takes, so concurrent
// spawns must not both walk away with the same species.
func TestConcurrentSpawnsGetDistinctNames(t *testing.T) {
	d := boundDaemon(t, serveHerdr(t, map[string]string{"agent.start": paneStartedReply}))

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
	d := boundDaemon(t, serveHerdr(t, map[string]string{"agent.start": paneStartedReply}))

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
	d := boundDaemon(t, serveHerdr(t, map[string]string{"agent.start": paneStartedReply}))

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

func TestSpawnPiNeedsBoundSession(t *testing.T) {
	d := boundDaemon(t, nil)

	_, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"})
	if err == nil {
		t.Fatal("pi spawned on an unbound daemon; it has no session to open a pane in")
	}
	if !strings.Contains(err.Error(), "not bound") || !strings.Contains(err.Error(), "pi") {
		t.Fatalf("error = %v, want the unbound-session refusal naming pi", err)
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
	if !strings.Contains(err.Error(), "exactly one agent, profile, config, or model") {
		t.Fatalf("error = %v, want the missing-selector refusal", err)
	}
}

// A config and a model each fully determine what to launch, so a request
// carrying both is refused rather than silently resolved by one of them.
func TestSpawnRejectsConfigAndModelTogether(t *testing.T) {
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

	sent, err := d.send(&protocol.Request{Op: protocol.OpSend, From: sender.Name, To: "worker-emperor", Body: "do the thing"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	input := f.params("pane.send_input")
	want := directMessagePrompt(protocol.Message{ID: sent.ID, From: sender.Name, To: "worker-emperor", Body: "do the thing"})
	if input["text"] != want {
		t.Fatalf("pane text = %v, want %q", input["text"], want)
	}
	if keys := strs(t, input["keys"]); len(keys) != 1 || keys[0] != "enter" {
		t.Fatalf("pane keys = %v, want [enter]", keys)
	}
}

func TestDirectMessagePromptExposesCorrelationMetadata(t *testing.T) {
	msg := protocol.Message{ID: "task-123", From: "lead-emperor", To: "worker-emperor", Body: `{"work":"now"}`, ReplyTo: "earlier-9"}
	want := "Fledge direct message\nid: task-123\nfrom: lead-emperor\nreply_to: earlier-9\n\n{\"work\":\"now\"}"
	if got := directMessagePrompt(msg); got != want {
		t.Fatalf("direct prompt = %q, want %q", got, want)
	}
}

func TestSendToOrchestratorWaitsInMailbox(t *testing.T) {
	f := claudeHerdr(t)
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{
		agentcfg.ReservedOrchestrator: {Integration: "claude", Model: "claude-opus-4"},
	})
	if _, err := d.spawn(&protocol.Request{Config: agentcfg.ReservedOrchestrator}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	sender, err := d.register(&protocol.Request{Type: "lead", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.send(&protocol.Request{From: sender.Name, To: agentcfg.ReservedOrchestrator, Body: "hello operator"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := f.count("pane.send_input"); got != 0 {
		t.Fatalf("orchestrator pane inputs = %d, want 0", got)
	}
	d.mu.Lock()
	queued := len(d.pending)
	d.mu.Unlock()
	if queued != 1 {
		t.Fatalf("pending messages = %d, want 1", queued)
	}
	resp, err := d.wait(&protocol.Request{As: agentcfg.ReservedOrchestrator}, make(chan struct{}))
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if resp.Message == nil || resp.Message.Body != "hello operator" {
		t.Fatalf("wait response = %+v", resp.Message)
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
	want := directMessagePrompt(protocol.Message{ID: sent.ID, From: sender.Name, To: "worker-emperor", Body: "do the thing"})
	if input["text"] != want {
		t.Fatalf("pane text = %v, want %q", input["text"], want)
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

func TestSendToPiAgentGoesToItsPane(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	sender, err := d.register(&protocol.Request{Op: protocol.OpRegister, Type: "lead", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}

	sent, err := d.send(&protocol.Request{Op: protocol.OpSend, From: sender.Name, To: "pi-emperor", Body: "ship it"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	input := f.params("pane.send_input")
	want := directMessagePrompt(protocol.Message{ID: sent.ID, From: sender.Name, To: "pi-emperor", Body: "ship it"})
	if input["text"] != want {
		t.Fatalf("pane text = %v, want %q", input["text"], want)
	}
	if keys := strs(t, input["keys"]); len(keys) != 1 || keys[0] != "enter" {
		t.Fatalf("pane keys = %v, want [enter]", keys)
	}
}

func TestStopPiClosesItsPaneAndFreesItsSpecies(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)

	if _, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if _, err := d.stop(&protocol.Request{Op: protocol.OpStop, Name: "pi-emperor"}); err != nil {
		t.Fatalf("stop: %v", err)
	}

	if closed := f.params("pane.close"); closed["pane_id"] != "w1:p2" {
		t.Fatalf("pane.close pane_id = %v, want w1:p2", closed["pane_id"])
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

	// A pi agent lives in a pane exactly like claude, so its state stands too.
	pi := restarted.agents["pi-emperor"]
	if pi.State != stateRunning || pi.PaneID != "w1:p2" {
		t.Fatalf("replayed pi = %+v, want running in its pane", pi)
	}
	if got := restarted.agents["pi-king"].State; got != stateStopped {
		t.Fatalf("stopped pi replayed as %q, want stopped", got)
	}
}

// A journal written before pi was pane-hosted records spawned pi agents with no
// pane_id, and may carry agent.settled lines from the removed RPC shape. Such a
// journal must still replay cleanly: the pane-less agent is unreachable (its
// pipes died with the daemon that owned them), so it comes back orphaned.
func TestReplayOrphansLegacyPanelessAgents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	lines := []string{
		`{"event":"agent.registered","name":"pi-emperor","type":"pi","species":"emperor","pid":1234}`,
		`{"event":"agent.spawned","name":"pi-emperor","type":"pi","species":"emperor","pid":1234,"integration":"pi","model":"gpt-x"}`,
		`{"event":"agent.settled","name":"pi-emperor","msg_id":"abc123"}`,
		`{"event":"agent.registered","name":"pi-king","type":"pi","species":"king","pid":1235}`,
		`{"event":"agent.spawned","name":"pi-king","type":"pi","species":"king","pid":1235,"integration":"pi","model":"gpt-x","pane_id":"w1:p9"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := replay(path)
	if err != nil {
		t.Fatalf("replay of a legacy journal failed: %v", err)
	}
	if got := s.agents["pi-emperor"].State; got != stateOrphaned {
		t.Fatalf("pane-less legacy agent replayed as %q, want orphaned", got)
	}
	if got := s.agents["pi-king"].State; got != stateRunning {
		t.Fatalf("pane-hosted pi replayed as %q, want running", got)
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
	if got := f.count("pane.swap"); got != 0 {
		t.Fatalf("ordinary spawn made %d pane.swap calls, want none", got)
	}
}

func TestSpawnEarlyPlacementFailureClosesPaneAndReleasesName(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		replies   map[string]string
	}{
		{
			name:      "swap",
			operation: "swap",
			replies: map[string]string{
				"agent.start": paneStartedReply,
				"pane.swap":   `{"id":"1","error":{"code":"swap_failed","message":"cannot swap"}}`,
			},
		},
		{
			name:      "focus",
			operation: "focus",
			replies: map[string]string{
				"agent.start": paneStartedReply,
				"pane.focus":  `{"id":"1","error":{"code":"focus_failed","message":"cannot focus"}}`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := serveHerdr(t, tt.replies)
			d := boundDaemon(t, f)
			writeAgents(t, d.root, map[string]agentcfg.Config{
				agentcfg.ReservedOrchestrator: {Integration: "claude", Model: "claude-opus-4"},
			})

			_, err := d.spawn(&protocol.Request{
				Op: protocol.OpSpawn, Config: agentcfg.ReservedOrchestrator,
				Split: "right", AnchorPane: "w1:p1",
			})
			if err == nil || !strings.Contains(err.Error(), tt.operation) {
				t.Fatalf("spawn error = %v, want %s placement failure", err, tt.operation)
			}
			if got := f.count("pane.close"); got != 1 {
				t.Fatalf("pane.close calls = %d, want 1", got)
			}
			if got := f.params("pane.close")["pane_id"]; got != "w1:p2" {
				t.Fatalf("closed pane = %v, want w1:p2", got)
			}
			if hasEvent(t, d, evRegistered, agentcfg.ReservedOrchestrator) ||
				hasEvent(t, d, evSpawned, agentcfg.ReservedOrchestrator) {
				t.Fatal("placement failure journaled the orchestrator")
			}
			d.mu.Lock()
			_, held := d.agents[agentcfg.ReservedOrchestrator]
			d.mu.Unlock()
			if held {
				t.Fatal("placement failure left the orchestrator name reserved")
			}

			resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: agentcfg.ReservedOrchestrator})
			if err != nil {
				t.Fatalf("reuse name: %v", err)
			}
			if resp.Name != agentcfg.ReservedOrchestrator {
				t.Fatalf("reused name = %q", resp.Name)
			}
		})
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

func TestSpawnAcceptsKebabCaseConfigs(t *testing.T) {
	d := boundDaemon(t, claudeHerdr(t))
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"some-agent": {Integration: "claude", Model: "claude-opus-4"},
	})

	resp, err := d.spawn(&protocol.Request{Op: protocol.OpSpawn, Config: "some-agent"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Name != "some-agent-emperor" {
		t.Errorf("name = %q", resp.Name)
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
		Op: protocol.OpRegister, Type: agentcfg.ReservedOrchestrator, Agent: agentcfg.ReservedOrchestrator,
		Source: "fledge/fledge-orchestrator/fledge-orchestrator.agent.md", PID: os.Getpid(),
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
		Op: protocol.OpRegister, Type: agentcfg.ReservedOrchestrator, Agent: agentcfg.ReservedOrchestrator, PID: os.Getpid(),
	}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, err := d.register(&protocol.Request{
		Op: protocol.OpRegister, Type: agentcfg.ReservedOrchestrator, Agent: agentcfg.ReservedOrchestrator, PID: os.Getpid(),
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
		Op: protocol.OpRegister, Type: agentcfg.ReservedOrchestrator, Agent: agentcfg.ReservedOrchestrator,
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
