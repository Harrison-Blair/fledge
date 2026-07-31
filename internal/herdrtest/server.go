package herdrtest

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

// Call is one decoded request a fake herdr server received.
type Call struct {
	ID     string         `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

// Text returns the string parameter named key, or "" when it is missing or of
// another type.
func (c Call) Text(key string) string {
	value, _ := c.Params[key].(string)
	return value
}

// Listen serves a Unix-domain socket in a temporary directory, handing every
// connection and its decoded request to handle, and returns the socket path.
//
// A sandbox refuses the listener either with EPERM or with a permission error
// of another number; the fixtures this replaces had drifted onto one check
// each, so both are skipped here.
func Listen(t *testing.T, handle func(net.Conn, Call)) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "herdr.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		if os.IsPermission(err) || errors.Is(err, syscall.EPERM) {
			t.Skip("sandbox does not permit Unix-domain listeners")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				var call Call
				if json.NewDecoder(conn).Decode(&call) == nil {
					handle(conn, call)
				}
			}()
		}
	}()
	return socket
}

// IDs names the objects a Server's creation methods synthesize. Fixtures
// assert on these names, so every server picks its own.
type IDs struct {
	Workspace     string // workspace.create's workspace
	WorkspaceTab  string // workspace.create's tab
	WorkspacePane string // workspace.create's root pane
	Tab           string // tab.create's tab
	TabPane       string // tab.create's root pane
	SplitPane     string // pane.split's new pane
}

// Server is a fake herdr JSON-RPC server. It answers the session methods
// Fledge drives from a caller-owned snapshot, mutating it the way Herdr would,
// and leaves everything a fixture needs to vary to Observe and Handle.
type Server struct {
	// Snapshot is the session state to answer from and mutate.
	Snapshot *herdr.Snapshot
	// Mutex guards Snapshot for the whole of a request. Start allocates one
	// when it is nil, but only for its own use: a fixture whose assertions read
	// Snapshot has to supply the lock it takes.
	Mutex *sync.Mutex
	// IDs names the objects the creation methods synthesize.
	IDs IDs
	// Observe records every request, ahead of Handle and the built-in methods.
	// It therefore sees requests that are about to be dropped, failed, or
	// rejected as invalid, and cannot report what a call was answered with.
	Observe func(Call)
	// Handle answers a request no built-in method owns, or takes over one that
	// a built-in owns; returning true means Handle wrote the response, or
	// deliberately wrote none at all.
	Handle func(net.Conn, Call) bool
	// Unknown answers methods nothing else owns. A nil Unknown closes the
	// connection unanswered.
	Unknown any
}

// Start serves s on a Unix-domain socket and returns the socket path.
func (s Server) Start(t *testing.T) string {
	t.Helper()
	if s.Mutex == nil {
		s.Mutex = &sync.Mutex{}
	}
	return Listen(t, func(conn net.Conn, call Call) {
		s.Mutex.Lock()
		defer s.Mutex.Unlock()
		if s.Observe != nil {
			s.Observe(call)
		}
		if s.Handle != nil && s.Handle(conn, call) {
			return
		}
		if s.dispatch(conn, call) || s.Unknown == nil {
			return
		}
		WriteResult(conn, call, s.Unknown)
	})
}

// dispatch answers the built-in methods, reporting whether it owned the call.
func (s Server) dispatch(conn net.Conn, call Call) bool {
	switch call.Method {
	case "ping":
		WriteResult(conn, call, herdr.Pong{Type: "pong", Version: Version, Protocol: Protocol})
	case "session.snapshot":
		WriteResult(conn, call, map[string]any{"type": "session_snapshot", "snapshot": *s.Snapshot})
	case "pane.focus", "workspace.focus":
		WriteResult(conn, call, map[string]any{"type": "focused"})
	case "server.stop":
		WriteResult(conn, call, okResult)
	case "workspace.create":
		WriteResult(conn, call, s.createWorkspace(call))
	case "tab.create":
		WriteResult(conn, call, s.createTab(call))
	case "tab.rename":
		WriteResult(conn, call, s.renameTab(call))
	case "pane.rename":
		WriteResult(conn, call, s.renamePane(call))
	case "pane.split":
		WriteResult(conn, call, s.splitPane(call))
	case "pane.send_input":
		if !validSendInput(call) {
			WriteError(conn, call, "invalid_params",
				`pane.send_input requires pane_id, text, and keys: ["enter"]`)
			return true
		}
		WriteResult(conn, call, map[string]any{"type": "input_sent"})
	default:
		return false
	}
	return true
}

var okResult = map[string]any{"type": "ok"}

func (s Server) createWorkspace(call Call) any {
	cwd := call.Text("cwd")
	workspace := herdr.WorkspaceInfo{WorkspaceID: s.IDs.Workspace, Label: call.Text("label")}
	tab := herdr.TabInfo{TabID: s.IDs.WorkspaceTab, WorkspaceID: workspace.WorkspaceID}
	pane := herdr.PaneInfo{
		PaneID: s.IDs.WorkspacePane, TabID: tab.TabID, WorkspaceID: workspace.WorkspaceID,
		CWD: &cwd, AgentStatus: "unknown",
	}
	s.Snapshot.Workspaces = []herdr.WorkspaceInfo{workspace}
	s.Snapshot.Tabs = []herdr.TabInfo{tab}
	s.Snapshot.Panes = []herdr.PaneInfo{pane}
	return map[string]any{
		"type": "workspace_created", "workspace": workspace, "tab": tab, "root_pane": pane,
	}
}

func (s Server) createTab(call Call) any {
	cwd := call.Text("cwd")
	tab := herdr.TabInfo{
		TabID: s.IDs.Tab, WorkspaceID: call.Text("workspace_id"), Label: call.Text("label"),
	}
	pane := herdr.PaneInfo{
		PaneID: s.IDs.TabPane, TabID: tab.TabID, WorkspaceID: tab.WorkspaceID,
		CWD: &cwd, AgentStatus: "unknown",
	}
	s.Snapshot.Tabs = append(s.Snapshot.Tabs, tab)
	s.Snapshot.Panes = append(s.Snapshot.Panes, pane)
	return map[string]any{"type": "tab_created", "tab": tab, "root_pane": pane}
}

func (s Server) renameTab(call Call) any {
	tabID := call.Text("tab_id")
	for i := range s.Snapshot.Tabs {
		if s.Snapshot.Tabs[i].TabID == tabID {
			s.Snapshot.Tabs[i].Label = call.Text("label")
			return map[string]any{"type": "tab_info", "tab": s.Snapshot.Tabs[i]}
		}
	}
	return okResult
}

func (s Server) renamePane(call Call) any {
	paneID := call.Text("pane_id")
	label := call.Text("label")
	for i := range s.Snapshot.Panes {
		if s.Snapshot.Panes[i].PaneID == paneID {
			s.Snapshot.Panes[i].Label = &label
			return map[string]any{"type": "pane_info", "pane": s.Snapshot.Panes[i]}
		}
	}
	return okResult
}

func (s Server) splitPane(call Call) any {
	targetID := call.Text("target_pane_id")
	var target herdr.PaneInfo
	for _, pane := range s.Snapshot.Panes {
		if pane.PaneID == targetID {
			target = pane
			break
		}
	}
	cwd := call.Text("cwd")
	pane := herdr.PaneInfo{
		PaneID: s.IDs.SplitPane, TabID: target.TabID, WorkspaceID: target.WorkspaceID,
		CWD: &cwd, AgentStatus: "unknown",
	}
	s.Snapshot.Panes = append(s.Snapshot.Panes, pane)
	return map[string]any{"type": "pane_created", "pane": pane}
}

// validSendInput holds Herdr's caller to exactly the three parameters Fledge is
// allowed to send, with the single "enter" key it always submits.
func validSendInput(call Call) bool {
	if len(call.Params) != 3 {
		return false
	}
	if _, ok := call.Params["pane_id"].(string); !ok {
		return false
	}
	if _, ok := call.Params["text"].(string); !ok {
		return false
	}
	keys, ok := call.Params["keys"].([]any)
	return ok && len(keys) == 1 && keys[0] == "enter"
}

// EmptySnapshot is a running session with no workspaces, tabs, panes or agents.
func EmptySnapshot() herdr.Snapshot {
	return herdr.Snapshot{
		Version: Version, Protocol: Protocol,
		Workspaces: []herdr.WorkspaceInfo{}, Tabs: []herdr.TabInfo{},
		Panes: []herdr.PaneInfo{}, Agents: []herdr.AgentInfo{},
	}
}

// WriteResult answers call with a result envelope.
func WriteResult(conn net.Conn, call Call, result any) {
	_ = json.NewEncoder(conn).Encode(map[string]any{"id": call.ID, "result": result})
}

// WriteError answers call with an error envelope.
func WriteError(conn net.Conn, call Call, code, message string) {
	_ = json.NewEncoder(conn).Encode(map[string]any{
		"id": call.ID, "error": map[string]any{"code": code, "message": message},
	})
}

// WriteInjectedFailure answers call with the envelope fixtures inject to make
// one method fail.
func WriteInjectedFailure(conn net.Conn, call Call) {
	WriteError(conn, call, "injected_failure", "injected "+call.Method+" failure")
}
