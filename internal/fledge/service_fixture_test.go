package fledge

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	// failPromptContaining fails agent.prompt calls whose text contains this
	// substring with a definite (non-transport) error.
	failPromptContaining string
	// bootingProcessInfoCalls makes the first N pane.process_info responses
	// report a shell-less booting pane; later calls report a ready shell.
	bootingProcessInfoCalls int
	processInfoCalls        int
	// startBusyWhileBooting rejects agent.start with agent_pane_busy while the
	// pane still reports a booting shell.
	startBusyWhileBooting bool
	startBusyRejections   int
	startArgs             []string
	promptTarget     string
	promptText       string
	waitTarget       string
	readTarget       string
	sendKeysTarget   string
	sendKeys         []string
	sendKeyCalls     [][]string
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
	agentExitHook    func()
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
	service := &Service{
		Project:           project.Info{Root: t.TempDir(), Session: "test-session"},
		Binary:            herdr.Binary{Path: binary},
		Store:             store,
		LaunchStopCleanup: func(StopCleanupRequest) error { return nil },
	}
	// The default deliverer runs the bounded delivery helper synchronously
	// in-process, so tests observe its effect as soon as the spawn returns.
	service.LaunchDeliveryHelper = func(name, activationID string, timeout time.Duration) error {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return service.DeliverActivation(ctx, name, activationID, timeout)
	}
	return service, fake
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
		f.processInfoCalls++
		info := map[string]any{"pane_id": "p1"}
		if f.processInfoCalls > f.bootingProcessInfoCalls {
			info["shell_pid"] = 10
			info["foreground_processes"] = []map[string]any{{"pid": 10, "name": "bash"}}
		}
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "pane_process_info", "process_info": info,
		})
	case "agent.start":
		if f.startBusyWhileBooting && f.processInfoCalls <= f.bootingProcessInfoCalls {
			f.startBusyRejections++
			herdrtest.WriteError(conn, call, "agent_pane_busy", "pane p1 has no shell yet")
			return true
		}
		f.startCalls++
		rawArgs := call.Params["args"]
		f.startArgs = appendStrings(f.startArgs, rawArgs)
		kind := "codex"
		name := call.Text("name")
		paneID, tabID := call.Text("pane_id"), ""
		for i := range f.snapshot.Panes {
			if f.snapshot.Panes[i].PaneID == paneID {
				f.snapshot.Panes[i].Agent = &kind
				f.snapshot.Panes[i].AgentStatus = "idle"
				tabID = f.snapshot.Panes[i].TabID
			}
		}
		f.snapshot.Agents = []herdr.AgentInfo{{
			Agent: &kind, AgentStatus: "idle", Name: &name, PaneID: paneID, TabID: tabID, WorkspaceID: "w1",
			InteractiveReady: true,
		}}
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "agent_started", "agent": f.snapshot.Agents[0], "argv": rawArgs,
		})
	case "agent.prompt":
		if f.failPromptContaining != "" && strings.Contains(call.Text("text"), f.failPromptContaining) {
			herdrtest.WriteInjectedFailure(conn, call)
			return true
		}
		f.promptTarget = call.Text("target")
		f.promptText = call.Text("text")
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
		f.sendKeys = appendStrings(f.sendKeys[:0], call.Params["keys"])
		f.sendKeyCalls = append(f.sendKeyCalls, append([]string(nil), f.sendKeys...))
		if !f.ignoreExit {
			f.exitAgent(f.sendKeysTarget)
			if f.agentExitHook != nil {
				f.agentExitHook()
			}
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
	case "tab.close":
		tabID := call.Text("tab_id")
		tabs := f.snapshot.Tabs[:0]
		for _, tab := range f.snapshot.Tabs {
			if tab.TabID != tabID {
				tabs = append(tabs, tab)
			}
		}
		f.snapshot.Tabs = tabs
		panes := f.snapshot.Panes[:0]
		for _, pane := range f.snapshot.Panes {
			if pane.TabID != tabID {
				panes = append(panes, pane)
			}
		}
		f.snapshot.Panes = panes
		agents := f.snapshot.Agents[:0]
		for _, agent := range f.snapshot.Agents {
			if agent.TabID != tabID {
				agents = append(agents, agent)
			}
		}
		f.snapshot.Agents = agents
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "tab_closed", "tab_id": tabID, "workspace_id": "w1",
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
