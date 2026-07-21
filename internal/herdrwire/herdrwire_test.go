package herdrwire

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeHerdr serves one request per connection like a real session server:
// read a line, write the canned answer, close. Every received request line is
// recorded in order.
type fakeHerdr struct {
	t      *testing.T
	socket string

	mu  sync.Mutex
	got []map[string]json.RawMessage
}

// serve starts a fake server whose reply is looked up per request by method.
func serve(t *testing.T, replies map[string]string) *fakeHerdr {
	t.Helper()
	f := &fakeHerdr{t: t, socket: filepath.Join(t.TempDir(), "h.sock")}
	ln, err := net.Listen("unix", f.socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	done := make(chan struct{})
	// Closing the listener is what ends the accept loop, so both must happen
	// in one cleanup — registering them separately deadlocks on LIFO order.
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

// request returns the n-th received envelope.
func (f *fakeHerdr) request(n int) map[string]json.RawMessage {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if n >= len(f.got) {
		f.t.Fatalf("only %d requests received, want index %d", len(f.got), n)
	}
	return f.got[n]
}

// count returns how many requests the server has seen.
func (f *fakeHerdr) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.got)
}

// params returns the params object of the n-th received request, decoded.
func (f *fakeHerdr) params(n int) map[string]any {
	f.t.Helper()
	var p map[string]any
	if err := json.Unmarshal(f.request(n)["params"], &p); err != nil {
		f.t.Fatalf("decode params: %v", err)
	}
	return p
}

func TestCallEnvelope(t *testing.T) {
	f := serve(t, nil)

	if err := Call(f.socket, "some.method", nil, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}

	env := f.request(0)
	if id := strings.Trim(string(env["id"]), `"`); id == "" {
		t.Errorf("id = %q, want non-empty", env["id"])
	}
	if m := string(env["method"]); m != `"some.method"` {
		t.Errorf("method = %s, want %q", m, "some.method")
	}
	// params is mandatory: nil must go out as {}, not null and not absent.
	raw, ok := env["params"]
	if !ok {
		t.Fatalf("params absent from request: %s", env)
	}
	if string(raw) != "{}" {
		t.Errorf("params = %s, want {}", raw)
	}
}

func TestCallIsOneShotPerConnection(t *testing.T) {
	f := serve(t, nil)

	for i := range 2 {
		if err := Call(f.socket, "some.method", nil, nil); err != nil {
			t.Fatalf("Call %d: %v", i, err)
		}
	}
	if n := f.count(); n != 2 {
		t.Fatalf("server saw %d requests, want 2", n)
	}
	if a, b := string(f.request(0)["id"]), string(f.request(1)["id"]); a == b {
		t.Errorf("ids not unique: both %s", a)
	}
}

const agentStartedReply = `{"id":"1","result":{"type":"agent_started","agent":{"terminal_id":"term_x","name":"n","pane_id":"w1:p2","agent_status":"unknown","workspace_id":"w1","tab_id":"w1:t1","focused":false,"revision":0},"argv":["claude"]}}`

func TestAgentStartUnwrapsResult(t *testing.T) {
	f := serve(t, map[string]string{"agent.start": agentStartedReply})

	got, err := AgentStart(f.socket, "n", "", []string{"claude"}, nil, "")
	if err != nil {
		t.Fatalf("AgentStart: %v", err)
	}
	if got.PaneID != "w1:p2" {
		t.Errorf("PaneID = %q, want w1:p2", got.PaneID)
	}
	if got.TerminalID != "term_x" {
		t.Errorf("TerminalID = %q, want term_x", got.TerminalID)
	}
}

func TestAgentStartParams(t *testing.T) {
	f := serve(t, map[string]string{"agent.start": agentStartedReply})

	if _, err := AgentStart(f.socket, "worker", "/tmp/wd", []string{"claude", "-v"}, map[string]string{"K": "V"}, ""); err != nil {
		t.Fatalf("AgentStart: %v", err)
	}
	p := f.params(0)
	if p["name"] != "worker" {
		t.Errorf("name = %v, want worker", p["name"])
	}
	if p["cwd"] != "/tmp/wd" {
		t.Errorf("cwd = %v, want /tmp/wd", p["cwd"])
	}
	argv, _ := json.Marshal(p["argv"])
	if string(argv) != `["claude","-v"]` {
		t.Errorf("argv = %s, want [\"claude\",\"-v\"]", argv)
	}
	env, _ := json.Marshal(p["env"])
	if string(env) != `{"K":"V"}` {
		t.Errorf("env = %s, want {\"K\":\"V\"}", env)
	}

	// Empty cwd and nil env are omitted rather than sent empty.
	if _, err := AgentStart(f.socket, "worker", "", []string{"claude"}, nil, ""); err != nil {
		t.Fatalf("AgentStart: %v", err)
	}
	p = f.params(1)
	if _, ok := p["cwd"]; ok {
		t.Errorf("cwd present with empty value: %v", p["cwd"])
	}
	if _, ok := p["env"]; ok {
		t.Errorf("env present with nil value: %v", p["env"])
	}
}

func TestProcessInfo(t *testing.T) {
	f := serve(t, map[string]string{
		"pane.process_info": `{"id":"1","result":{"type":"pane_process_info","process_info":{"shell_pid":4242,"foreground_pid":4243}}}`,
	})

	pid, err := ProcessInfo(f.socket, "w1:p2")
	if err != nil {
		t.Fatalf("ProcessInfo: %v", err)
	}
	if pid != 4242 {
		t.Errorf("shellPID = %d, want 4242", pid)
	}
	if p := f.params(0); p["pane_id"] != "w1:p2" {
		t.Errorf("pane_id = %v, want w1:p2", p["pane_id"])
	}
}

func TestProcessInfoNullShellPID(t *testing.T) {
	f := serve(t, map[string]string{
		"pane.process_info": `{"id":"1","result":{"type":"pane_process_info","process_info":{"shell_pid":null}}}`,
	})

	pid, err := ProcessInfo(f.socket, "w1:p2")
	if err != nil {
		t.Fatalf("ProcessInfo: %v", err)
	}
	if pid != 0 {
		t.Errorf("shellPID = %d, want 0 for null", pid)
	}
}

func TestSendInputKeys(t *testing.T) {
	f := serve(t, nil)

	if err := SendInput(f.socket, "w1:p2", "hello", true); err != nil {
		t.Fatalf("SendInput: %v", err)
	}
	p := f.params(0)
	if p["text"] != "hello" {
		t.Errorf("text = %v, want hello", p["text"])
	}
	keys, _ := json.Marshal(p["keys"])
	if string(keys) != `["enter"]` {
		t.Errorf("keys = %s, want [\"enter\"]", keys)
	}

	if err := SendInput(f.socket, "w1:p2", "hello", false); err != nil {
		t.Fatalf("SendInput: %v", err)
	}
	p = f.params(1)
	if p["text"] != "hello" {
		t.Errorf("text = %v, want hello", p["text"])
	}
	if _, ok := p["keys"]; ok {
		t.Errorf("keys present without pressEnter: %v", p["keys"])
	}
}

const unknownPaneReply = `{"id":"1","error":{"code":"unknown_pane","message":"no such pane"}}`

func TestCallWireError(t *testing.T) {
	f := serve(t, map[string]string{"pane.close": unknownPaneReply})

	err := PaneClose(f.socket, "w1:p2")
	if err == nil {
		t.Fatal("PaneClose: want error, got nil")
	}
	for _, want := range []string{"pane.close", "no such pane", "unknown_pane"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestAgentAlive(t *testing.T) {
	live := serve(t, map[string]string{"agent.get": `{"id":"1","result":{"type":"agent","agent":{"pane_id":"w1:p2"}}}`})
	alive, err := AgentAlive(live.socket, "w1:p2")
	if err != nil {
		t.Fatalf("AgentAlive: %v", err)
	}
	if !alive {
		t.Error("alive = false, want true")
	}
	if p := live.params(0); p["target"] != "w1:p2" {
		t.Errorf("target = %v, want w1:p2", p["target"])
	}

	gone := serve(t, map[string]string{"agent.get": unknownPaneReply})
	alive, err = AgentAlive(gone.socket, "w1:p2")
	if err != nil {
		t.Fatalf("AgentAlive on wire error: want nil error, got %v", err)
	}
	if alive {
		t.Error("alive = true, want false")
	}
}

func TestAgentAliveTransportError(t *testing.T) {
	// No server: dialing a nonexistent socket must propagate, since a dial
	// failure says nothing about the pane.
	if _, err := AgentAlive(filepath.Join(t.TempDir(), "absent.sock"), "w1:p2"); err == nil {
		t.Fatal("want transport error, got nil")
	}
}

func TestCallTimesOutOnSilentServer(t *testing.T) {
	// A server that accepts and then says nothing models a wedged Herdr
	// socket: the dial succeeds, so only the read deadline can end the call.
	socket := filepath.Join(t.TempDir(), "h.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	done := make(chan struct{})
	t.Cleanup(func() {
		ln.Close()
		<-done
	})
	var held []net.Conn
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				for _, c := range held {
					c.Close()
				}
				return
			}
			held = append(held, conn)
		}
	}()

	prev := callTimeout
	callTimeout = 100 * time.Millisecond
	t.Cleanup(func() { callTimeout = prev })

	start := time.Now()
	err = Call(socket, "some.method", nil, nil)
	if err == nil {
		t.Fatal("want timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "some.method") {
		t.Errorf("error %q missing method name", err)
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Errorf("error %q is not a timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Call blocked %v, deadline not applied", elapsed)
	}
}

func TestReleaseAgentParams(t *testing.T) {
	f := serve(t, nil)

	if err := ReleaseAgent(f.socket, "w1:p2", "custom:fledge", "worker-1"); err != nil {
		t.Fatalf("ReleaseAgent: %v", err)
	}
	p := f.params(0)
	for k, want := range map[string]string{"pane_id": "w1:p2", "source": "custom:fledge", "agent": "worker-1"} {
		if p[k] != want {
			t.Errorf("%s = %v, want %v", k, p[k], want)
		}
	}
	if m := string(f.request(0)["method"]); m != `"pane.release_agent"` {
		t.Errorf("method = %s", m)
	}
}

func TestWindowTitleSetParams(t *testing.T) {
	f := serve(t, map[string]string{
		"client.window_title.set": `{"id":"1","result":{"type":"client_window_title","changed":true,"reason":"set"}}`,
	})

	changed, err := WindowTitleSet(f.socket, "fledge · flock1")
	if err != nil {
		t.Fatalf("WindowTitleSet: %v", err)
	}
	if !changed {
		t.Error("changed = false, want true when herdr reports the title set")
	}
	if p := f.params(0); p["title"] != "fledge · flock1" {
		t.Errorf("title = %v, want %q", p["title"], "fledge · flock1")
	}
	if m := string(f.request(0)["method"]); m != `"client.window_title.set"` {
		t.Errorf("method = %s", m)
	}
}

// A session nobody has attached to yet answers no_foreground_client and
// changes nothing. That is not an error, but it must not be read as success:
// the caller retries on it.
func TestWindowTitleSetNoForegroundClient(t *testing.T) {
	f := serve(t, map[string]string{
		"client.window_title.set": `{"id":"1","result":{"type":"client_window_title","changed":false,"reason":"no_foreground_client"}}`,
	})

	changed, err := WindowTitleSet(f.socket, "fledge · flock1")
	if err != nil {
		t.Fatalf("WindowTitleSet: %v", err)
	}
	if changed {
		t.Error("changed = true, want false when no client is attached")
	}
}

const workspaceCreatedReply = `{"id":"1","result":{"type":"workspace_created","workspace":{"workspace_id":"w1","label":"lbl","active_tab_id":"w1:t1"},"tab":{"tab_id":"w1:t1","label":"1"},"root_pane":{"pane_id":"w1:p1"}}}`

func TestWorkspaceCreateParams(t *testing.T) {
	f := serve(t, map[string]string{"workspace.create": workspaceCreatedReply})

	if _, err := WorkspaceCreate(f.socket, "/tmp/ws", "lbl"); err != nil {
		t.Fatalf("WorkspaceCreate: %v", err)
	}
	p := f.params(0)
	if p["cwd"] != "/tmp/ws" {
		t.Errorf("cwd = %v, want /tmp/ws", p["cwd"])
	}
	if p["focus"] != true {
		t.Errorf("focus = %v, want true", p["focus"])
	}
	if m := string(f.request(0)["method"]); m != `"workspace.create"` {
		t.Errorf("method = %s", m)
	}
}

func TestReportMetadataParams(t *testing.T) {
	f := serve(t, nil)

	if err := ReportMetadata(f.socket, "w1:p2", "custom:fledge", "worker-1: building"); err != nil {
		t.Fatalf("ReportMetadata: %v", err)
	}
	p := f.params(0)
	for k, want := range map[string]string{"pane_id": "w1:p2", "source": "custom:fledge", "title": "worker-1: building"} {
		if p[k] != want {
			t.Errorf("%s = %v, want %v", k, p[k], want)
		}
	}
	if m := string(f.request(0)["method"]); m != `"pane.report_metadata"` {
		t.Errorf("method = %s", m)
	}
}

// A split direction rides on agent.start as the "split" param; empty means
// "let herdr place it" and must not be sent at all.
func TestAgentStartSplit(t *testing.T) {
	f := serve(t, map[string]string{"agent.start": agentStartedReply})

	if _, err := AgentStart(f.socket, "worker", "", []string{"claude"}, nil, "right"); err != nil {
		t.Fatalf("AgentStart: %v", err)
	}
	if got := f.params(0)["split"]; got != "right" {
		t.Errorf("split = %v, want right", got)
	}

	if _, err := AgentStart(f.socket, "worker", "", []string{"claude"}, nil, ""); err != nil {
		t.Fatalf("AgentStart: %v", err)
	}
	if _, ok := f.params(1)["split"]; ok {
		t.Error("split sent with an empty value")
	}
}

func TestPaneCurrent(t *testing.T) {
	f := serve(t, map[string]string{
		"pane.current": `{"id":"1","result":{"type":"pane_current","pane":{"pane_id":"w1:p1","focused":true}}}`,
	})

	got, err := PaneCurrent(f.socket)
	if err != nil {
		t.Fatalf("PaneCurrent: %v", err)
	}
	if got != "w1:p1" {
		t.Errorf("PaneCurrent = %q, want w1:p1", got)
	}
}

func TestPaneSwapParams(t *testing.T) {
	f := serve(t, nil)

	if err := PaneSwap(f.socket, "w1:p1", "w1:p2"); err != nil {
		t.Fatalf("PaneSwap: %v", err)
	}
	p := f.params(0)
	if p["source_pane_id"] != "w1:p1" {
		t.Errorf("source_pane_id = %v, want w1:p1", p["source_pane_id"])
	}
	if p["target_pane_id"] != "w1:p2" {
		t.Errorf("target_pane_id = %v, want w1:p2", p["target_pane_id"])
	}
}

func TestPaneFocusParams(t *testing.T) {
	f := serve(t, nil)

	if err := PaneFocus(f.socket, "w1:p2"); err != nil {
		t.Fatalf("PaneFocus: %v", err)
	}
	if got := f.params(0)["pane_id"]; got != "w1:p2" {
		t.Errorf("pane_id = %v, want w1:p2", got)
	}
	if got := string(f.request(0)["method"]); got != `"pane.focus"` {
		t.Errorf("method = %s, want pane.focus", got)
	}
}

// The workspace label rides on creation, so no rename call is needed; an empty
// label is omitted rather than sent as "".
func TestWorkspaceCreateLabel(t *testing.T) {
	f := serve(t, map[string]string{"workspace.create": workspaceCreatedReply})

	if _, err := WorkspaceCreate(f.socket, "/tmp/ws", "fledge-orchestrator"); err != nil {
		t.Fatalf("WorkspaceCreate: %v", err)
	}
	if got := f.params(0)["label"]; got != "fledge-orchestrator" {
		t.Errorf("label = %v, want fledge-orchestrator", got)
	}

	if _, err := WorkspaceCreate(f.socket, "/tmp/ws", ""); err != nil {
		t.Fatalf("WorkspaceCreate: %v", err)
	}
	if _, ok := f.params(1)["label"]; ok {
		t.Error("label sent with an empty value")
	}
}

// The tab herdr opens with the workspace comes back in the same reply, which is
// what lets start label it without a tab.list lookup.
func TestWorkspaceCreateReturnsInitialTabID(t *testing.T) {
	f := serve(t, map[string]string{"workspace.create": workspaceCreatedReply})

	tabID, err := WorkspaceCreate(f.socket, "/tmp/ws", "lbl")
	if err != nil {
		t.Fatalf("WorkspaceCreate: %v", err)
	}
	if tabID != "w1:t1" {
		t.Errorf("tabID = %q, want w1:t1", tabID)
	}
}

func TestTabRenameParams(t *testing.T) {
	f := serve(t, nil)

	if err := TabRename(f.socket, "w1:t1", "orchestrator"); err != nil {
		t.Fatalf("TabRename: %v", err)
	}
	p := f.params(0)
	if p["tab_id"] != "w1:t1" {
		t.Errorf("tab_id = %v, want w1:t1", p["tab_id"])
	}
	if p["label"] != "orchestrator" {
		t.Errorf("label = %v, want orchestrator", p["label"])
	}
	if m := string(f.request(0)["method"]); m != `"tab.rename"` {
		t.Errorf("method = %s, want tab.rename", m)
	}
}
