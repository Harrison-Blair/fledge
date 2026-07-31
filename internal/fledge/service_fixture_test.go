package fledge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

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
	socket := filepath.Join(t.TempDir(), "herdr.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		if os.IsPermission(err) || errors.Is(err, syscall.EPERM) {
			t.Skip("sandbox does not permit Unix-domain listeners")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	binary, runningMarker := fakeBinary(t, socket)
	fake := &fakeLifecycle{snapshot: herdr.Snapshot{
		Version: "0.7.5", Protocol: 17, Workspaces: []herdr.WorkspaceInfo{},
		Tabs: []herdr.TabInfo{}, Panes: []herdr.PaneInfo{}, Agents: []herdr.AgentInfo{},
	}, pongProtocol: 17, runningMarker: runningMarker}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go fake.serve(conn)
		}
	}()
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
	methods := make([]string, 0, len(herdr.RequiredMethods))
	for _, method := range herdr.RequiredMethods {
		methods = append(methods, fmt.Sprintf(`{"method":{"const":%s}}`, strconv.Quote(method)))
	}
	schema := fmt.Sprintf(`{"protocol":17,"requests":[%s]}`, strings.Join(methods, ","))
	running := fmt.Sprintf(`{"sessions":[{"name":"test-session","running":true,"socket_path":%s}]}`, strconv.Quote(socket))
	stopped := `{"sessions":[{"name":"test-session","running":false}]}`
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "herdr 0.7.5"
elif [ "$1" = "api" ] && [ "$2" = "schema" ]; then
  printf '%%s\n' %s
elif [ "$1" = "session" ] && [ "$2" = "list" ]; then
  if [ -f %s ]; then
    printf '%%s\n' %s
  elif [ -f %s ]; then
    printf '%%s\n' %s
  else
    printf '%%s\n' '{"sessions":[]}'
  fi
elif [ "$1" = "session" ] && [ "$2" = "delete" ]; then
  rm -f %s
  printf '%%s\n' '{"deleted":true}'
else
  exit 2
fi
`, strconv.Quote(schema), strconv.Quote(runningMarker), strconv.Quote(running),
		strconv.Quote(existsMarker), strconv.Quote(stopped), strconv.Quote(existsMarker))
	path := filepath.Join(temp, "herdr-fake")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path, runningMarker
}

func fakeBinarySessions(t *testing.T, sessions string) string {
	t.Helper()
	var compactSessions bytes.Buffer
	if err := json.Compact(&compactSessions, []byte(sessions)); err != nil {
		t.Fatalf("compact fake Herdr sessions JSON: %v", err)
	}
	sessions = compactSessions.String()
	methods := make([]string, 0, len(herdr.RequiredMethods))
	for _, method := range herdr.RequiredMethods {
		methods = append(methods, fmt.Sprintf(`{"method":{"const":%s}}`, strconv.Quote(method)))
	}
	schema := fmt.Sprintf(`{"protocol":17,"requests":[%s]}`, strings.Join(methods, ","))
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "herdr 0.7.5"
elif [ "$1" = "api" ] && [ "$2" = "schema" ]; then
  printf '%%s\n' %s
elif [ "$1" = "session" ] && [ "$2" = "list" ]; then
  printf '%%s\n' %s
else
  exit 2
fi
`, strconv.Quote(schema), strconv.Quote(sessions))
	path := filepath.Join(t.TempDir(), "herdr-fake")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func serviceSessionSocket(t *testing.T, binary herdr.Binary) string {
	t.Helper()
	sessions, err := binary.Sessions(t.Context())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("read fake sessions: %v, %#v", err, sessions)
	}
	return sessions[0].SocketPath
}

func (f *fakeLifecycle) serve(conn net.Conn) {
	defer conn.Close()
	var request struct {
		ID     string         `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if json.NewDecoder(conn).Decode(&request) != nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.methodCalls = append(f.methodCalls, request.Method)
	if request.Method == f.dropMethod {
		return
	}
	if request.Method == f.failMethod {
		_ = json.NewEncoder(conn).Encode(map[string]any{
			"id": request.ID,
			"error": map[string]any{
				"code": "injected_failure", "message": "injected " + request.Method + " failure",
			},
		})
		return
	}
	result := map[string]any{"type": "ok"}
	switch request.Method {
	case "ping":
		result = map[string]any{"type": "pong", "version": "0.7.5", "protocol": f.pongProtocol}
	case "session.snapshot":
		result = map[string]any{"type": "session_snapshot", "snapshot": f.snapshot}
	case "workspace.create":
		f.workspaceCreates++
		label, _ := request.Params["label"].(string)
		f.workspaceCWD, _ = request.Params["cwd"].(string)
		f.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: label}}
		f.snapshot.Tabs = []herdr.TabInfo{{TabID: "t1", WorkspaceID: "w1"}}
		cwd := f.workspaceCWD
		f.snapshot.Panes = []herdr.PaneInfo{{
			PaneID: "p1", TabID: "t1", WorkspaceID: "w1", CWD: &cwd, AgentStatus: "unknown",
		}}
		result = map[string]any{
			"type": "workspace_created", "workspace": f.snapshot.Workspaces[0],
			"tab": f.snapshot.Tabs[0], "root_pane": f.snapshot.Panes[0],
		}
	case "tab.create":
		f.tabCreates++
		label, _ := request.Params["label"].(string)
		workspaceID, _ := request.Params["workspace_id"].(string)
		tab := herdr.TabInfo{TabID: "t-new", WorkspaceID: workspaceID, Label: label}
		cwd, _ := request.Params["cwd"].(string)
		pane := herdr.PaneInfo{
			PaneID: "p-new", TabID: tab.TabID, WorkspaceID: workspaceID, CWD: &cwd, AgentStatus: "unknown",
		}
		f.snapshot.Tabs = append(f.snapshot.Tabs, tab)
		f.snapshot.Panes = append(f.snapshot.Panes, pane)
		result = map[string]any{"type": "tab_created", "tab": tab, "root_pane": pane}
	case "tab.rename":
		f.tabRenames++
		tabID, _ := request.Params["tab_id"].(string)
		for i := range f.snapshot.Tabs {
			if f.snapshot.Tabs[i].TabID == tabID {
				f.snapshot.Tabs[i].Label, _ = request.Params["label"].(string)
				result = map[string]any{"type": "tab_info", "tab": f.snapshot.Tabs[i]}
				break
			}
		}
	case "pane.rename":
		f.paneRenames++
		paneID, _ := request.Params["pane_id"].(string)
		label, _ := request.Params["label"].(string)
		for i := range f.snapshot.Panes {
			if f.snapshot.Panes[i].PaneID == paneID {
				f.snapshot.Panes[i].Label = &label
				result = map[string]any{"type": "pane_info", "pane": f.snapshot.Panes[i]}
				break
			}
		}
	case "pane.split":
		f.paneSplits++
		f.splitParams = request.Params
		targetID, _ := request.Params["target_pane_id"].(string)
		var target herdr.PaneInfo
		for _, pane := range f.snapshot.Panes {
			if pane.PaneID == targetID {
				target = pane
				break
			}
		}
		cwd, _ := request.Params["cwd"].(string)
		pane := herdr.PaneInfo{
			PaneID: "p-right", TabID: target.TabID, WorkspaceID: target.WorkspaceID,
			CWD: &cwd, AgentStatus: "unknown",
		}
		f.snapshot.Panes = append(f.snapshot.Panes, pane)
		result = map[string]any{"type": "pane_created", "pane": pane}
	case "pane.focus":
		paneID, _ := request.Params["pane_id"].(string)
		f.focusedPaneIDs = append(f.focusedPaneIDs, paneID)
	case "pane.process_info":
		result = map[string]any{
			"type": "pane_process_info",
			"process_info": map[string]any{
				"pane_id": "p1", "shell_pid": 10,
				"foreground_processes": []map[string]any{{"pid": 10, "name": "bash"}},
			},
		}
	case "agent.start":
		f.startCalls++
		rawArgs, present := request.Params["args"].([]any)
		if present {
			for _, arg := range rawArgs {
				f.startArgs = append(f.startArgs, arg.(string))
			}
		}
		kind := "codex"
		name, _ := request.Params["name"].(string)
		f.snapshot.Panes[0].Agent = &kind
		f.snapshot.Panes[0].AgentStatus = "idle"
		f.snapshot.Agents = []herdr.AgentInfo{{
			Agent: &kind, AgentStatus: "idle", Name: &name, PaneID: "p1", TabID: "t1", WorkspaceID: "w1",
		}}
		result = map[string]any{"type": "agent_started", "agent": f.snapshot.Agents[0], "argv": rawArgs}
	case "agent.prompt":
		f.promptTarget, _ = request.Params["target"].(string)
		if wait, ok := request.Params["wait"].(map[string]any); ok {
			f.promptWait = wait
		}
		result = map[string]any{"type": "agent_prompted", "agent": f.snapshot.Agents[0]}
	case "agent.read":
		f.readTarget, _ = request.Params["target"].(string)
		result = map[string]any{
			"type": "agent_read",
			"read": map[string]any{
				"pane_id": f.readTarget, "source": "recent_unwrapped",
				"format": "text", "text": "output", "truncated": false, "revision": 1,
			},
		}
	case "agent.send_keys":
		f.sendKeysTarget, _ = request.Params["target"].(string)
		f.sendKeysTargets = append(f.sendKeysTargets, f.sendKeysTarget)
		if raw, ok := request.Params["keys"].([]any); ok {
			f.sendKeys = f.sendKeys[:0]
			for _, key := range raw {
				f.sendKeys = append(f.sendKeys, key.(string))
			}
		}
		if !f.ignoreExit {
			for i := range f.snapshot.Panes {
				if f.snapshot.Panes[i].PaneID == f.sendKeysTarget {
					f.snapshot.Panes[i].Agent = nil
					f.snapshot.Panes[i].AgentStatus = "unknown"
				}
			}
			remaining := f.snapshot.Agents[:0]
			for _, agent := range f.snapshot.Agents {
				if agent.PaneID != f.sendKeysTarget {
					remaining = append(remaining, agent)
				}
			}
			f.snapshot.Agents = remaining
		}
	case "pane.send_input":
		f.sendInputCalls++
		var valid bool
		f.sendInputPaneID, valid = request.Params["pane_id"].(string)
		if !valid || len(request.Params) != 3 {
			f.writeInvalidParams(conn, request.ID, request.Method)
			return
		}
		f.sendInputText, valid = request.Params["text"].(string)
		if !valid {
			f.writeInvalidParams(conn, request.ID, request.Method)
			return
		}
		rawKeys, valid := request.Params["keys"].([]any)
		if !valid || len(rawKeys) != 1 {
			f.writeInvalidParams(conn, request.ID, request.Method)
			return
		}
		f.sendInputKeys = f.sendInputKeys[:0]
		for _, rawKey := range rawKeys {
			key, ok := rawKey.(string)
			if !ok {
				f.writeInvalidParams(conn, request.ID, request.Method)
				return
			}
			f.sendInputKeys = append(f.sendInputKeys, key)
		}
	case "agent.wait":
		f.waitTarget, _ = request.Params["target"].(string)
		result = map[string]any{"type": "agent_info"}
	case "pane.close":
		f.snapshot.Panes, f.snapshot.Agents = nil, nil
		result = map[string]any{"type": "pane_closed", "pane_id": "p1", "workspace_id": "w1"}
	case "server.stop":
		if f.serverStopError != "" {
			_ = json.NewEncoder(conn).Encode(map[string]any{
				"id": request.ID,
				"error": map[string]any{
					"code": "stop_failed", "message": f.serverStopError,
				},
			})
			return
		}
		f.serverStopped = true
		_ = os.Remove(f.runningMarker)
		if f.serverStopHook != nil {
			f.serverStopHook()
		}
	}
	_ = json.NewEncoder(conn).Encode(map[string]any{"id": request.ID, "result": result})
}

func (f *fakeLifecycle) writeInvalidParams(conn net.Conn, id, method string) {
	_ = json.NewEncoder(conn).Encode(map[string]any{
		"id": id,
		"error": map[string]any{
			"code": "invalid_params", "message": "invalid parameters for " + method,
		},
	})
}
