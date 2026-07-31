package fledge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/herdrtest"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

// okResult is what Herdr answers a method that reports nothing of its own.
var okResult = map[string]any{"type": "ok"}

type fakeLifecycle struct {
	mu               sync.Mutex
	snapshot         herdr.Snapshot
	startCalls       int
	workspaceCreates int
	workspaceCWD     string
	tabCreates       int
	tabRenames       int
	paneRenames      int
	paneSplits       int
	splitParams      map[string]any
	focusedPaneIDs   []string
	failMethod       string
	dropMethod       string
	startArgs        []string
	promptWait       map[string]any
	promptTarget     string
	waitTarget       string
	readTarget       string
	sendKeysTarget   string
	sendKeys         []string
	sendKeysTargets  []string
	methodCalls      []string
	sendInputCalls   int
	sendInputPaneID  string
	sendInputText    string
	sendInputKeys    []string
	serverStopped    bool
	serverStopError  string
	serverStopHook   func()
	runningMarker    string
	pongProtocol     int
	ignoreExit       bool
}

func newFakeLifecycle(t *testing.T) (*Service, *fakeLifecycle) {
	t.Helper()
	fake := &fakeLifecycle{snapshot: herdrtest.EmptySnapshot(), pongProtocol: herdrtest.Protocol}
	socket := herdrtest.Server{
		Snapshot: &fake.snapshot,
		Mutex:    &fake.mu,
		IDs: herdrtest.IDs{
			Workspace: "w1", WorkspaceTab: "t1", WorkspacePane: "p1",
			Tab: "t-new", TabPane: "p-new", SplitPane: "p-right",
		},
		Observe: fake.record,
		Handle:  fake.handle,
		Unknown: okResult,
	}.Start(t)
	binary, runningMarker := fakeBinary(t, socket)
	// The socket has to exist before the binary can name it, so the marker is
	// the one field assigned once handle can already be reading it.
	fake.mu.Lock()
	fake.runningMarker = runningMarker
	fake.mu.Unlock()
	store, err := state.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &Service{
		Project:           project.Info{Root: t.TempDir(), Session: "test-session"},
		Binary:            herdr.Binary{Path: binary},
		Store:             store,
		LaunchStopCleanup: func(StopCleanupRequest) error { return nil },
	}, fake
}

func fakeBinary(t *testing.T, socket string) (string, string) {
	t.Helper()
	temp := t.TempDir()
	existsMarker := filepath.Join(temp, "exists")
	runningMarker := filepath.Join(temp, "running")
	if err := os.WriteFile(existsMarker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runningMarker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	running := fmt.Sprintf(`{"sessions":[{"name":"test-session","running":true,"socket_path":%s}]}`, strconv.Quote(socket))
	stopped := `{"sessions":[{"name":"test-session","running":false}]}`
	path := herdrtest.WriteBinary(t, temp, herdrtest.Options{
		Version: herdrtest.VersionOutput,
		Sessions: []herdrtest.SessionCase{
			{Marker: runningMarker, Payload: running},
			{Marker: existsMarker, Payload: stopped},
		},
		DeleteRemoves: existsMarker,
	})
	return path, runningMarker
}

func fakeBinarySessions(t *testing.T, sessions string) string {
	t.Helper()
	var compactSessions bytes.Buffer
	if err := json.Compact(&compactSessions, []byte(sessions)); err != nil {
		t.Fatalf("compact fake Herdr sessions JSON: %v", err)
	}
	sessions = compactSessions.String()
	return herdrtest.WriteBinary(t, t.TempDir(), herdrtest.Options{
		Version:  herdrtest.VersionOutput,
		Sessions: []herdrtest.SessionCase{{Payload: sessions}},
	})
}

func serviceSessionSocket(t *testing.T, binary herdr.Binary) string {
	t.Helper()
	sessions, err := binary.Sessions(t.Context())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("read fake sessions: %v, %#v", err, sessions)
	}
	return sessions[0].SocketPath
}

// record instruments every request the shared server receives, including the
// ones handle then drops or fails.
func (f *fakeLifecycle) record(call herdrtest.Call) {
	f.methodCalls = append(f.methodCalls, call.Method)
	switch call.Method {
	case "workspace.create":
		f.workspaceCreates++
		f.workspaceCWD = call.Text("cwd")
	case "tab.create":
		f.tabCreates++
	case "tab.rename":
		f.tabRenames++
	case "pane.rename":
		f.paneRenames++
	case "pane.split":
		f.paneSplits++
		f.splitParams = call.Params
	case "pane.focus":
		f.focusedPaneIDs = append(f.focusedPaneIDs, call.Text("pane_id"))
	case "pane.send_input":
		f.sendInputCalls++
		f.sendInputPaneID = call.Text("pane_id")
		f.sendInputText = call.Text("text")
		f.sendInputKeys = appendStrings(f.sendInputKeys[:0], call.Params["keys"])
	}
}

// handle injects the failures a test asks for and answers the agent and server
// methods that only Fledge's service drives.
func (f *fakeLifecycle) handle(conn net.Conn, call herdrtest.Call) bool {
	if call.Method == f.dropMethod {
		return true
	}
	if call.Method == f.failMethod {
		herdrtest.WriteInjectedFailure(conn, call)
		return true
	}
	switch call.Method {
	case "ping":
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "pong", "version": herdrtest.Version, "protocol": f.pongProtocol,
		})
	case "pane.process_info":
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "pane_process_info",
			"process_info": map[string]any{
				"pane_id": "p1", "shell_pid": 10,
				"foreground_processes": []map[string]any{{"pid": 10, "name": "bash"}},
			},
		})
	case "agent.start":
		f.startCalls++
		rawArgs := call.Params["args"]
		f.startArgs = appendStrings(f.startArgs, rawArgs)
		kind := "codex"
		name := call.Text("name")
		f.snapshot.Panes[0].Agent = &kind
		f.snapshot.Panes[0].AgentStatus = "idle"
		f.snapshot.Agents = []herdr.AgentInfo{{
			Agent: &kind, AgentStatus: "idle", Name: &name, PaneID: "p1", TabID: "t1", WorkspaceID: "w1",
		}}
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "agent_started", "agent": f.snapshot.Agents[0], "argv": rawArgs,
		})
	case "agent.prompt":
		f.promptTarget = call.Text("target")
		if wait, ok := call.Params["wait"].(map[string]any); ok {
			f.promptWait = wait
		}
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "agent_prompted", "agent": f.snapshot.Agents[0],
		})
	case "agent.read":
		f.readTarget = call.Text("target")
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "agent_read",
			"read": map[string]any{
				"pane_id": f.readTarget, "source": "recent_unwrapped",
				"format": "text", "text": "output", "truncated": false, "revision": 1,
			},
		})
	case "agent.send_keys":
		f.sendKeysTarget = call.Text("target")
		f.sendKeysTargets = append(f.sendKeysTargets, f.sendKeysTarget)
		if keys, ok := call.Params["keys"].([]any); ok {
			f.sendKeys = appendStrings(f.sendKeys[:0], keys)
		}
		if !f.ignoreExit {
			f.exitAgent(f.sendKeysTarget)
		}
		herdrtest.WriteResult(conn, call, okResult)
	case "agent.wait":
		f.waitTarget = call.Text("target")
		herdrtest.WriteResult(conn, call, map[string]any{"type": "agent_info"})
	case "pane.close":
		f.snapshot.Panes, f.snapshot.Agents = nil, nil
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "pane_closed", "pane_id": "p1", "workspace_id": "w1",
		})
	case "server.stop":
		if f.serverStopError != "" {
			herdrtest.WriteError(conn, call, "stop_failed", f.serverStopError)
			return true
		}
		f.serverStopped = true
		_ = os.Remove(f.runningMarker)
		if f.serverStopHook != nil {
			f.serverStopHook()
		}
		herdrtest.WriteResult(conn, call, okResult)
	default:
		return false
	}
	return true
}

func (f *fakeLifecycle) exitAgent(paneID string) {
	for i := range f.snapshot.Panes {
		if f.snapshot.Panes[i].PaneID == paneID {
			f.snapshot.Panes[i].Agent = nil
			f.snapshot.Panes[i].AgentStatus = "unknown"
		}
	}
	remaining := f.snapshot.Agents[:0]
	for _, agent := range f.snapshot.Agents {
		if agent.PaneID != paneID {
			remaining = append(remaining, agent)
		}
	}
	f.snapshot.Agents = remaining
}

func appendStrings(into []string, raw any) []string {
	values, _ := raw.([]any)
	for _, value := range values {
		text, _ := value.(string)
		into = append(into, text)
	}
	return into
}
