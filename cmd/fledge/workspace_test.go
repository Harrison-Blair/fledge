package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/client"
	"github.com/Harrison-Blair/fledge/internal/daemon"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

// TestMain doubles as the daemon entrypoint: spawnDaemon re-execs the current
// binary as `daemon run`, and under test the current binary is the test
// binary, so that argv must run the real command instead of the suite.
func TestMain(m *testing.M) {
	if len(os.Args) >= 3 && os.Args[1] == "daemon" && os.Args[2] == "run" {
		if err := run(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// scaffoldedWorkspace creates a temp workspace with a .fledge tree and a
// nested subdirectory, returning both.
func scaffoldedWorkspace(t *testing.T) (root, sub string) {
	t.Helper()
	root = t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	writeAgentCatalog(t, root, map[string]agentcfg.Config{
		"haikucl": {Integration: "claude", Model: "haiku"},
	})
	sub = filepath.Join(root, "sub", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, sub
}

// startDaemon runs an in-process daemon for root and registers cleanup.
func startDaemon(t *testing.T, root, flockName string) {
	t.Helper()
	d, err := daemon.New(root, flockName)
	if err != nil {
		t.Fatalf("daemon.New: %v", err)
	}
	done := make(chan struct{})
	go func() {
		d.Serve()
		close(done)
	}()
	t.Cleanup(func() {
		d.Close()
		<-done
	})
}

// fakeHerdr installs a stub herdr CLI on PATH that records the cwd and session
// name of every `herdr --session <name> server` launch under recDir, and
// reports that session as running on sock once one has been launched (or
// recDir/session has been pre-seeded).
func fakeHerdr(t *testing.T, recDir, sock string) {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
REC="` + recDir + `"
SOCK="` + sock + `"
case "$1" in
--session)
	printf '%s' "$PWD" > "$REC/pwd"
	printf '%s' "$2" > "$REC/session"
	exit 0
	;;
session)
	case "$2" in
	list)
		if [ -f "$REC/session" ]; then
			printf '{"sessions":[{"name":"%s","running":true,"default":false,"socket_path":"%s"}]}' "$(cat "$REC/session")" "$SOCK"
		else
			printf '{"sessions":[]}'
		fi
		;;
	stop)
		rm -f "$SOCK"
		;;
	delete)
		printf '%s' "$3" > "$REC/deleted"
		;;
	esac
	exit 0
	;;
*)
	exit 0
	;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// wireRecorder collects every protocol request sent to a fake session socket.
type wireRecorder struct {
	mu  sync.Mutex
	got []map[string]json.RawMessage
}

// methodParams returns the params of the first recorded request for method,
// or false if none arrived.
func (r *wireRecorder) methodParams(method string) (map[string]any, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, env := range r.got {
		if string(env["method"]) != `"`+method+`"` {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(env["params"], &p); err != nil {
			return nil, false
		}
		return p, true
	}
	return nil, false
}

func (r *wireRecorder) methods() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.got))
	for _, env := range r.got {
		out = append(out, strings.Trim(string(env["method"]), `"`))
	}
	return out
}

// liveSocket listens on sock so herdr.Up sees the fake session as alive,
// answering one wire request per connection with an empty result and
// recording it. Returns the recorder and an idempotent closer.
func liveSocket(t *testing.T, sock string) (rec *wireRecorder, closeFn func()) {
	t.Helper()
	return liveSocketReplies(t, sock, nil)
}

// liveSocketReplies is liveSocket with canned per-method answers, for the
// calls whose result the caller actually reads back.
func liveSocketReplies(t *testing.T, sock string, replies map[string]string) (rec *wireRecorder, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	rec = &wireRecorder{}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				line, err := bufio.NewReader(c).ReadString('\n')
				if err != nil {
					return
				}
				reply := `{"id":"1","result":{}}`
				var env map[string]json.RawMessage
				if json.Unmarshal([]byte(line), &env) == nil {
					rec.mu.Lock()
					rec.got = append(rec.got, env)
					rec.mu.Unlock()
					if r, ok := replies[strings.Trim(string(env["method"]), `"`)]; ok {
						reply = r
					}
				}
				c.Write([]byte(reply + "\n"))
			}(c)
		}
	}()
	closed := false
	return rec, func() {
		if closed {
			return
		}
		closed = true
		ln.Close()
	}
}

// waitDaemonUp polls until the flock's daemon answers, failing the test if it
// never does.
func waitDaemonUp(t *testing.T, root, flockName string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if client.Running(root, flockName) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("daemon for %s at %s never came up", flockName, root)
}

// waitDaemonDown waits for spawned daemons to follow their (now closed) fake
// session down, so tests do not leak detached processes. Bound daemons poll
// every daemon.WatchInterval, so this allows several ticks.
func waitDaemonDown(t *testing.T, flockName string, roots ...string) {
	t.Helper()
	deadline := time.Now().Add(4 * daemon.WatchInterval)
	for time.Now().Before(deadline) {
		up := false
		for _, root := range roots {
			if client.Running(root, flockName) {
				up = true
			}
		}
		if !up {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("daemon for %s still up after session closed; it dies with the test process", flockName)
}

func canonical(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestFlockStatusResolvesWorkspaceFromSubdirectory(t *testing.T) {
	root, sub := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	startDaemon(t, root, "flock1")
	t.Chdir(sub)

	out, err := captureRun(t, "flock", "status", "flock1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "daemon: up") {
		t.Errorf("status from subdirectory does not see the daemon:\n%s", out)
	}
}

func TestAgentListResolvesWorkspaceFromSubdirectory(t *testing.T) {
	root, sub := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	startDaemon(t, root, "flock1")

	resp, err := client.Do(root, "flock1", protocol.Request{
		Op: protocol.OpRegister, Type: "tester", PID: os.Getpid(),
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv(flock.Env, "flock1")
	t.Chdir(sub)

	out, err := captureRun(t, "agent", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, resp.Name) {
		t.Errorf("agent list from subdirectory missing %s:\n%s", resp.Name, out)
	}
}

func TestStartReusesRunningFlock(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	recDir := t.TempDir()
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	rec, closeSession := liveSocket(t, sock)
	// Pre-seed the record so the fake reports the session the daemon binds to.
	if err := os.WriteFile(filepath.Join(recDir, "session"), []byte("boundsession"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeHerdr(t, recDir, sock)

	done := make(chan struct{})
	go func() {
		daemon.RunBound(root, "flock1", "boundsession")
		close(done)
	}()
	waitDaemonUp(t, root, "flock1")
	t.Cleanup(func() {
		closeSession()
		select {
		case <-done:
		case <-time.After(4 * daemon.WatchInterval):
			t.Log("bound daemon still up; it dies with the test process")
		}
	})

	t.Chdir(root)
	out, err := captureRun(t, "start", "--flock", "flock1")
	if err != nil {
		t.Fatalf("start while already running: %v", err)
	}
	if !strings.Contains(out, "daemon: up") {
		t.Errorf("reused start missing daemon summary:\n%s", out)
	}
	if _, ok := rec.methodParams("workspace.create"); ok {
		t.Error("reattaching to a running flock created another workspace")
	}
}

// A managed Herdr session can outlive a crashed or otherwise missing daemon.
// Its old orchestrator identity is not part of the new daemon's authoritative
// journal, so start must replace that session instead of colliding with the
// stale fledge-orchestrator pane.
func TestStartRecreatesStaleManagedSession(t *testing.T) {
	root, sub := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	record := t.TempDir()
	staleSocket := filepath.Join(t.TempDir(), "stale.sock")
	freshSocket := filepath.Join(t.TempDir(), "fresh.sock")
	staleRec, closeStale := liveSocket(t, staleSocket)
	freshRec, closeFresh := liveSocketReplies(t, freshSocket, map[string]string{
		"workspace.create": createdWorkspaceReply,
	})

	bin := t.TempDir()
	script := `#!/bin/sh
REC="` + record + `"
STALE="` + staleSocket + `"
FRESH="` + freshSocket + `"
case "$1" in
--session)
	touch "$REC/started"
	printf 'start %s\n' "$2" >> "$REC/actions"
	exit 0
	;;
session)
	case "$2" in
	list)
		if [ -f "$REC/started" ]; then
			printf '{"sessions":[{"name":"%s","running":true,"default":false,"socket_path":"%s"}]}' "$(cat "$REC/name")" "$FRESH"
		elif [ -f "$REC/stopped" ]; then
			printf '{"sessions":[{"name":"%s","running":false,"default":false,"socket_path":"%s"}]}' "$(cat "$REC/name")" "$REC/stopped.sock"
		else
			printf '{"sessions":[{"name":"%s","running":true,"default":false,"socket_path":"%s"}]}' "$(cat "$REC/name")" "$STALE"
		fi
		;;
	stop)
		printf 'stop %s\n' "$3" >> "$REC/actions"
		touch "$REC/stopped"
		;;
	delete)
		printf 'delete %s\n' "$3" >> "$REC/actions"
		;;
	esac
	exit 0
	;;
esac
exit 8
`
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	managed := flock.SessionName(canonical(t, root), "flock1")
	if err := os.WriteFile(filepath.Join(record, "name"), []byte(managed), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeFresh()
		closeStale()
		waitDaemonDown(t, "flock1", root, sub)
	})

	t.Chdir(root)
	out, err := captureRun(t, "start", "--flock", "flock1")
	if err != nil {
		t.Fatalf("start with stale managed session: %v\n%s", err, out)
	}
	actions, err := os.ReadFile(filepath.Join(record, "actions"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"stop " + managed, "delete " + managed, "start " + managed} {
		if !strings.Contains(string(actions), want) {
			t.Errorf("session actions %q missing %q", actions, want)
		}
	}
	if _, ok := freshRec.methodParams("workspace.create"); !ok {
		t.Errorf("fresh managed session did not receive its primary workspace; actions=%q output=%q stale methods=%v fresh methods=%v", actions, out, staleRec.methods(), freshRec.methods())
	}
}

func TestStartRunsHerdrAndDaemonFromWorkspaceRoot(t *testing.T) {
	root, sub := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	recDir := t.TempDir()
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, closeSession := liveSocket(t, sock)
	fakeHerdr(t, recDir, sock)
	t.Cleanup(func() {
		closeSession()
		waitDaemonDown(t, "flock1", root, sub)
	})

	t.Chdir(sub)
	if _, err := captureRun(t, "start", "--flock", "flock1"); err != nil {
		t.Fatal(err)
	}

	pwd, err := os.ReadFile(filepath.Join(recDir, "pwd"))
	if err != nil {
		t.Fatalf("fake herdr never launched a session server: %v", err)
	}
	if got, want := string(pwd), canonical(t, root); got != want {
		t.Errorf("herdr session server started in %q, want workspace root %q", got, want)
	}

	session, err := os.ReadFile(filepath.Join(recDir, "session"))
	if err != nil {
		t.Fatal(err)
	}
	name := string(session)
	if name == "fledge-flock1" {
		t.Errorf("session name %q is not workspace-scoped", name)
	}
	if !strings.HasPrefix(name, "fledge-") || !strings.HasSuffix(name, "-flock1") {
		t.Errorf("session name %q missing fledge-/-flock1 branding", name)
	}

	if !client.Running(root, "flock1") {
		t.Error("daemon is not bound to the workspace root")
	}
}

// stopFlockBoundTo runs a daemon bound to session via a fake herdr, stops the
// flock through the CLI, and returns the record directory for assertions.
func stopFlockBoundTo(t *testing.T, session string) (recDir string) {
	t.Helper()
	root, _ := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	recDir = t.TempDir()
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, closeSession := liveSocket(t, sock)
	t.Cleanup(closeSession)
	if err := os.WriteFile(filepath.Join(recDir, "session"), []byte(session), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeHerdr(t, recDir, sock)

	done := make(chan struct{})
	go func() {
		daemon.RunBound(root, "flock1", session)
		close(done)
	}()
	waitDaemonUp(t, root, "flock1")

	t.Chdir(root)
	out, err := captureRun(t, "flock", "stop", "flock1")
	if err != nil {
		t.Fatalf("flock stop: %v", err)
	}
	if !strings.Contains(out, "stopped") {
		t.Errorf("flock stop output missing confirmation:\n%s", out)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("daemon still up after flock stop returned")
	}
	return recDir
}

// A managed session's record is useless once its flock is gone; stop must
// delete it or herdr's session list collects a corpse per stopped flock.
func TestFlockStopDeletesManagedSession(t *testing.T) {
	recDir := stopFlockBoundTo(t, "fledge-testws-abc123-flock1")
	deleted, err := os.ReadFile(filepath.Join(recDir, "deleted"))
	if err != nil {
		t.Fatalf("flock stop never deleted the session record: %v", err)
	}
	if got := string(deleted); got != "fledge-testws-abc123-flock1" {
		t.Errorf("deleted session %q, want fledge-testws-abc123-flock1", got)
	}
}

// A session the operator named with --session is theirs: stop ends it but
// must leave its record and logs in herdr's session list.
func TestFlockStopKeepsOperatorNamedSession(t *testing.T) {
	recDir := stopFlockBoundTo(t, "mysession")
	if _, err := os.Stat(filepath.Join(recDir, "deleted")); err == nil {
		t.Error("flock stop deleted an operator-named session's record")
	}
}

// Herdr manufactures a fresh session's first workspace at the attaching
// client's connect time with no reliable cwd (observed: $HOME on 0.7.4), so
// start must create the workspace itself, explicitly rooted at the workspace
// root, before anyone attaches.
func TestStartCreatesWorkspaceAtRoot(t *testing.T) {
	root, sub := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	recDir := t.TempDir()
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	rec, closeSession := liveSocket(t, sock)
	fakeHerdr(t, recDir, sock)
	t.Cleanup(func() {
		closeSession()
		waitDaemonDown(t, "flock1", root, sub)
	})

	t.Chdir(sub)
	if _, err := captureRun(t, "start", "--flock", "flock1"); err != nil {
		t.Fatal(err)
	}

	p, ok := rec.methodParams("workspace.create")
	if !ok {
		t.Fatal("start never issued workspace.create on the session socket")
	}
	if got, want := p["cwd"], canonical(t, root); got != want {
		t.Errorf("workspace.create cwd = %v, want workspace root %q", got, want)
	}
}

func TestInitWarnsWhenNestedInsideWorkspace(t *testing.T) {
	stubDiscovery(t)
	parent := t.TempDir()
	if _, err := scaffold.Ensure(parent); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "init", filepath.Join(parent, "child"))
	if err != nil {
		t.Fatal(err)
	}
	want := "nested inside workspace at " + canonical(t, parent)
	if !strings.Contains(out, want) {
		t.Errorf("init output missing %q:\n%s", want, out)
	}
}

// stubStdoutTerminal pins the interactive path: captureRun swaps stdout for a
// pipe, which is never a char device, so start would otherwise always take the
// scripted branch.
func stubStdoutTerminal(t *testing.T) {
	t.Helper()
	original := stdoutIsTerminal
	stdoutIsTerminal = func() bool { return true }
	t.Cleanup(func() { stdoutIsTerminal = original })
}

// stubAttach replaces the exec-into-herdr call, which would otherwise replace
// the test process. The returned pointer reports whether start reached it.
func stubAttach(t *testing.T) *bool {
	t.Helper()
	called := false
	original := attachHerdr
	originalStart := startAfterAttach
	startAfterAttach = func(start func()) {
		if !called {
			t.Error("orchestrator launch began before the Herdr UI attached")
		}
		start()
	}
	attachHerdr = func(session, root string, attached func()) error {
		called = true
		if attached != nil {
			attached()
		}
		return nil
	}
	t.Cleanup(func() {
		attachHerdr = original
		startAfterAttach = originalStart
	})
	return &called
}

// writeAgentCatalog sets the workspace's generated catalog profiles.
func writeAgentCatalog(t *testing.T, root string, configs map[string]agentcfg.Config) {
	t.Helper()
	data, err := json.Marshal(agentcfg.Index{Version: agentcfg.IndexVersion, Agents: map[string]agentcfg.AgentRecord{}, Profiles: configs})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, scaffold.DirName, agentcfg.CatalogName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

const startedPaneReply = `{"id":"1","result":{"type":"agent_started","agent":{"terminal_id":"term_x","name":"n","pane_id":"w1:p2","workspace_id":"w1","tab_id":"w1:t1"}}}`

const currentPaneReply = `{"id":"1","result":{"type":"pane_current","pane":{"pane_id":"w1:p1","focused":true}}}`

const idleAgentReply = `{"id":"1","result":{"type":"agent","agent":{"pane_id":"w1:p2","agent_status":"idle"}}}`

// interactiveStart wires a fake session whose catalog is configs, runs an
// interactive `start`, and hands back everything the assertions need.
func interactiveStart(t *testing.T, configs map[string]agentcfg.Config, stdin string) (rec *wireRecorder, root string, attached *bool, out string, err error) {
	t.Helper()
	return interactiveStartReplies(t, configs, stdin, nil)
}

func interactiveStartReplies(t *testing.T, configs map[string]agentcfg.Config, stdin string, extraReplies map[string]string) (rec *wireRecorder, root string, attached *bool, out string, err error) {
	t.Helper()
	root, sub := scaffoldedWorkspace(t)
	runtimeDir, err := os.MkdirTemp(os.TempDir(), "fledge-runtime-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(runtimeDir) })
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	writeAgentCatalog(t, root, configs)

	recDir := t.TempDir()
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	replies := map[string]string{
		"agent.start":      startedPaneReply,
		"agent.get":        idleAgentReply,
		"pane.current":     currentPaneReply,
		"workspace.create": createdWorkspaceReply,
	}
	for method, reply := range extraReplies {
		replies[method] = reply
	}
	rec, closeSession := liveSocketReplies(t, sock, replies)
	fakeHerdr(t, recDir, sock)
	t.Cleanup(func() {
		closeSession()
		waitDaemonDown(t, "flock1", root, sub)
	})

	stubStdoutTerminal(t)
	stubStdinTerminal(t)
	withStdin(t, stdin)
	attached = stubAttach(t)

	// The fake pane has no LLM to execute the initial readiness prompt, so
	// emulate that action as soon as agent.start exposes the injected token.
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			start, started := rec.methodParams("agent.start")
			if started {
				env, _ := start["env"].(map[string]any)
				name, _ := start["name"].(string)
				token, _ := env[protocol.ReadyTokenEnv].(string)
				if name != "" && token != "" {
					if _, err := client.Do(root, "flock1", protocol.Request{Op: protocol.OpReady, Name: name, Token: token}); err == nil {
						return
					}
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()

	t.Chdir(root)
	out, err = captureRun(t, "start", "--flock", "flock1")
	return rec, root, attached, out, err
}

// Interactive start attaches the UI before it asks the daemon to bring the
// orchestrator up, so the operator can watch registration and readiness.
func TestStartInteractiveSpawnsOrchestrator(t *testing.T) {
	rec, _, attached, out, err := interactiveStart(t, map[string]agentcfg.Config{
		"orchestrator-profile": {Integration: "claude", Model: "claude-opus-4"},
	}, "1\n")
	if err != nil {
		t.Fatalf("interactive start: %v\n%s", err, out)
	}

	start, ok := rec.methodParams("agent.start")
	if !ok {
		t.Fatal("start never spawned an orchestrator")
	}
	if got := start["split"]; got != "right" {
		t.Errorf("agent.start split = %v, want right", got)
	}
	if !*attached {
		t.Error("start did not attach before spawning the orchestrator")
	}
	if !strings.Contains(out, "Run orchestrator with which profile?") {
		t.Errorf("profile picker missing:\n%s", out)
	}
}

// agent.start split:"right" puts the new pane on the RIGHT of the pane it
// split (verified live on herdr 0.7.4/protocol 16). The daemon swaps and
// focuses it immediately after agent.start resolves.
func TestStartInteractivePlacesOrchestratorLeftAndFocused(t *testing.T) {
	rec, _, _, out, err := interactiveStart(t, map[string]agentcfg.Config{
		"orchestrator-profile": {Integration: "claude", Model: "claude-opus-4"},
	}, "1\n")
	if err != nil {
		t.Fatalf("interactive start: %v\n%s", err, out)
	}

	swap, ok := rec.methodParams("pane.swap")
	if !ok {
		t.Fatal("start never swapped the orchestrator pane left of the shell")
	}
	if swap["source_pane_id"] != "w1:p1" || swap["target_pane_id"] != "w1:p2" {
		t.Errorf("pane.swap = %v, want the shell pane swapped with the agent pane", swap)
	}
	focus, ok := rec.methodParams("pane.focus")
	if !ok {
		t.Fatal("start never focused the orchestrator pane after the swap")
	}
	if focus["pane_id"] != "w1:p2" {
		t.Errorf("pane.focus pane_id = %v, want the orchestrator pane w1:p2", focus["pane_id"])
	}

	want := []string{"agent.start", "pane.swap", "pane.focus"}
	var relevant []string
	for _, method := range rec.methods() {
		switch method {
		case "agent.start", "pane.swap", "pane.focus":
			relevant = append(relevant, method)
		}
		if len(relevant) == len(want) {
			break
		}
	}
	if !slices.Equal(relevant, want) {
		t.Errorf("startup call order = %v, want %v", relevant, want)
	}
	methods := rec.methods()
	if slices.Contains(methods, "agent.get") {
		t.Errorf("startup unexpectedly polled agent status: %v", methods)
	}
	if got := strings.Count(strings.Join(methods, ","), "workspace.create"); got != 1 {
		t.Errorf("startup created %d workspaces, want only the orchestrator workspace: %v", got, methods)
	}
}

func TestStartInteractivePlacementFailureRollsBack(t *testing.T) {
	_, root, attached, out, err := interactiveStartReplies(t, map[string]agentcfg.Config{
		"orchestrator-profile": {Integration: "claude", Model: "claude-opus-4"},
	}, "1\n", map[string]string{
		"pane.swap": `{"id":"1","error":{"code":"swap_failed","message":"cannot swap"}}`,
	})
	if err == nil {
		t.Fatalf("interactive start succeeded after placement failure:\n%s", out)
	}
	if !strings.Contains(err.Error(), "swap") || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("placement error = %v", err)
	}
	if !*attached {
		t.Fatal("placement failed before the UI-first attach")
	}
	if client.Running(root, "flock1") {
		t.Fatal("placement failure left the interactive flock running")
	}
}

// A missing orchestrator profile falls back to the picker built for `agent
// spawn`, and the pick becomes the orchestrator — under the reserved name, not
// under a <config>-<species> one: the picker at start is choosing which model
// runs as the orchestrator, so the operator still gets the name they know.
func TestStartInteractiveMissingConfigOffersPicker(t *testing.T) {
	rec, _, attached, out, err := interactiveStart(t, map[string]agentcfg.Config{
		"opus48": {Integration: "claude", Model: "claude-opus-4-8"},
	}, "1\n")
	if err != nil {
		t.Fatalf("interactive start: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Run orchestrator with which profile?") {
		t.Errorf("picker not shown for a missing orchestrator config:\n%s", out)
	}
	start, ok := rec.methodParams("agent.start")
	if !ok {
		t.Fatal("the picked config was never spawned")
	}
	if got := start["name"]; got != agentcfg.ReservedOrchestrator {
		t.Errorf("picked agent name = %v, want the reserved %q", got, agentcfg.ReservedOrchestrator)
	}
	if !*attached {
		t.Error("start did not attach before launching the picked orchestrator")
	}
}

// The invariant: no orchestrator means no attach and no flock left running.
func TestStartInteractiveEmptyCatalogRollsBack(t *testing.T) {
	_, root, attached, out, err := interactiveStart(t, map[string]agentcfg.Config{}, "")
	if err == nil {
		t.Fatalf("start succeeded with nothing to run as an orchestrator:\n%s", out)
	}
	if !strings.Contains(err.Error(), "no profiles available") {
		t.Errorf("error missing the profile hint: %v", err)
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Errorf("error does not say the start was rolled back: %v", err)
	}
	if *attached {
		t.Error("start attached without an orchestrator")
	}
	if client.Running(root, "flock1") {
		t.Error("the flock is still up after a start that could not finish")
	}
}

// Cancelling the picker is the same dead end as having nothing to pick.
func TestStartInteractiveCancelledPickRollsBack(t *testing.T) {
	_, root, attached, out, err := interactiveStart(t, map[string]agentcfg.Config{
		"opus48": {Integration: "claude", Model: "claude-opus-4-8"},
	}, "\n")
	if err == nil {
		t.Fatalf("start succeeded though the orchestrator pick was cancelled:\n%s", out)
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("unexpected error: %v", err)
	}
	if *attached {
		t.Error("start attached after a cancelled orchestrator pick")
	}
	if client.Running(root, "flock1") {
		t.Error("the flock is still up after a cancelled orchestrator pick")
	}
}

// A scripted start is server-only: it must not spawn an orchestrator, must not
// print a menu, and must still succeed.
func TestStartNonInteractiveSkipsOrchestrator(t *testing.T) {
	root, sub := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	writeAgentCatalog(t, root, map[string]agentcfg.Config{
		"orchestrator-profile": {Integration: "claude", Model: "claude-opus-4"},
	})

	recDir := t.TempDir()
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	rec, closeSession := liveSocketReplies(t, sock, map[string]string{
		"agent.start":  startedPaneReply,
		"pane.current": currentPaneReply,
	})
	fakeHerdr(t, recDir, sock)
	t.Cleanup(func() {
		closeSession()
		waitDaemonDown(t, "flock1", root, sub)
	})
	attached := stubAttach(t)

	t.Chdir(root)
	out, err := captureRun(t, "start", "--flock", "flock1")
	if err != nil {
		t.Fatalf("scripted start: %v\n%s", err, out)
	}
	if _, ok := rec.methodParams("agent.start"); ok {
		t.Error("a scripted start spawned an orchestrator")
	}
	created := 0
	for _, method := range rec.methods() {
		if method == "workspace.create" {
			created++
		}
	}
	if created != 1 {
		t.Errorf("scripted start created %d workspaces, want only the primary workspace", created)
	}
	if strings.Contains(out, "Run orchestrator with which profile?") {
		t.Errorf("a scripted start printed the picker:\n%s", out)
	}
	if *attached {
		t.Error("a scripted start attached")
	}
	if !client.Running(root, "flock1") {
		t.Error("a scripted start did not leave the daemon up")
	}
}

const createdWorkspaceReply = `{"id":"1","result":{"type":"workspace_created","workspace":{"workspace_id":"w1","active_tab_id":"w1:t1"},"tab":{"tab_id":"w1:t1"},"root_pane":{"pane_id":"w1:p1"}}}`

// The workspace and tab labels are session metadata, not part of the
// interactive orchestrator flow, so a scripted start must apply them too.
func TestStartLabelsWorkspaceAndTab(t *testing.T) {
	root, sub := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	recDir := t.TempDir()
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	rec, closeSession := liveSocketReplies(t, sock, map[string]string{
		"workspace.create": createdWorkspaceReply,
	})
	fakeHerdr(t, recDir, sock)
	t.Cleanup(func() {
		closeSession()
		waitDaemonDown(t, "flock1", root, sub)
	})

	t.Chdir(root)
	if _, err := captureRun(t, "start", "--flock", "flock1"); err != nil {
		t.Fatal(err)
	}

	create, ok := rec.methodParams("workspace.create")
	if !ok {
		t.Fatal("start never created a workspace")
	}
	if got := create["label"]; got != "fledge-orchestrator" {
		t.Errorf("workspace label = %v, want fledge-orchestrator", got)
	}

	rename, ok := rec.methodParams("tab.rename")
	if !ok {
		t.Fatal("start never labelled the tab")
	}
	if got := rename["tab_id"]; got != "w1:t1" {
		t.Errorf("tab.rename tab_id = %v, want the id workspace.create returned", got)
	}
	if got := rename["label"]; got != "orchestrator" {
		t.Errorf("tab label = %v, want orchestrator", got)
	}
}
