package cli

import (
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
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
)

type fledgeStartEnvelope struct {
	ProjectRoot   string `json:"project_root"`
	Session       string `json:"session"`
	SessionSource string `json:"session_source"`
	Socket        string `json:"socket"`
	Started       bool   `json:"started"`
	Version       string `json:"herdr_version"`
	Protocol      int    `json:"protocol"`
}

func initializedProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, err := project.Init(root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	return root
}

func fakeStartBinary(
	t *testing.T,
	root, session string,
	running, workspacePresent bool,
	attachExit int,
	setupFailure ...string,
) (string, string, string) {
	t.Helper()
	temp := t.TempDir()
	runningMarker := filepath.Join(temp, "running")
	attachLog := filepath.Join(temp, "attach.log")
	workspaceLog := filepath.Join(temp, "workspace.log")
	pidFile := filepath.Join(temp, "server.pid")
	socket := fakeStartSocket(t, root, workspacePresent, workspaceLog, setupFailure...)
	if running {
		if err := os.WriteFile(runningMarker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(string(data))
		if err == nil && pid > 0 {
			_ = syscall.Kill(-pid, syscall.SIGTERM)
		}
	})

	methods := make([]string, 0, len(herdr.RequiredMethods))
	for _, method := range herdr.RequiredMethods {
		methods = append(methods, fmt.Sprintf(`{"method":{"const":%s}}`, strconv.Quote(method)))
	}
	schema := fmt.Sprintf(`{"protocol":17,"requests":[%s]}`, strings.Join(methods, ","))
	sessions := fmt.Sprintf(`{"sessions":[{"name":%s,"running":true,"socket_path":%s}]}`,
		strconv.Quote(session), strconv.Quote(socket))
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "herdr 0.7.5"
elif [ "$1" = "api" ] && [ "$2" = "schema" ]; then
  printf '%%s\n' %s
elif [ "$1" = "session" ] && [ "$2" = "list" ]; then
  if [ -f %s ]; then
    printf '%%s\n' %s
  else
    printf '%%s\n' '{"sessions":[]}'
  fi
elif [ "$1" = "session" ] && [ "$2" = "attach" ]; then
  printf '%%s|%%s\n' "$PWD" "$*" >> %s
  exit %d
elif [ "$1" = "--session" ] && [ "$3" = "server" ]; then
  printf '%%s' "$$" > %s
  touch %s
  trap 'exit 0' TERM INT
  while :; do sleep 1; done
else
  exit 2
fi
`, strconv.Quote(schema), strconv.Quote(runningMarker), strconv.Quote(sessions),
		strconv.Quote(attachLog), attachExit, strconv.Quote(pidFile), strconv.Quote(runningMarker))
	binary := filepath.Join(temp, "herdr-fake")
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary, attachLog, workspaceLog
}

func fakeCoordinatedStopBinary(t *testing.T, root, session string) (binary, attached, running string) {
	t.Helper()
	temp := t.TempDir()
	running = filepath.Join(temp, "running")
	exists := filepath.Join(temp, "exists")
	attached = filepath.Join(temp, "attached")
	for _, marker := range []string{running, exists} {
		if err := os.WriteFile(marker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	socket := filepath.Join(temp, "herdr.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		if os.IsPermission(err) || errors.Is(err, syscall.EPERM) {
			t.Skip("sandbox does not permit Unix-domain listeners")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	snapshot := herdr.Snapshot{
		Version: "0.7.5", Protocol: 17,
		Workspaces: []herdr.WorkspaceInfo{{
			WorkspaceID: "workspace-1",
			Worktree:    &herdr.WorkspaceWorktreeInfo{CheckoutPath: root, RepoRoot: root},
		}},
		Tabs: []herdr.TabInfo{{
			TabID: "orchestrator-tab", WorkspaceID: "workspace-1", Label: "orchestrator",
		}},
		Panes: []herdr.PaneInfo{
			{
				PaneID: "orchestrator-left", TabID: "orchestrator-tab",
				WorkspaceID: "workspace-1", Label: stringPointer("orchestrator"), CWD: &root,
			},
			{
				PaneID: "orchestrator-right", TabID: "orchestrator-tab",
				WorkspaceID: "workspace-1", CWD: &root,
			},
		},
		Agents: []herdr.AgentInfo{},
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				var request struct {
					ID     string `json:"id"`
					Method string `json:"method"`
				}
				if json.NewDecoder(conn).Decode(&request) != nil {
					return
				}
				var result any
				switch request.Method {
				case "ping":
					result = herdr.Pong{Type: "pong", Version: "0.7.5", Protocol: 17}
				case "session.snapshot":
					result = herdr.Result{Type: "session_snapshot", Snapshot: snapshot}
				case "pane.focus", "workspace.focus":
					result = map[string]any{"type": "focused"}
				case "server.stop":
					result = map[string]any{"type": "ok"}
				default:
					return
				}
				if json.NewEncoder(conn).Encode(map[string]any{"id": request.ID, "result": result}) != nil {
					return
				}
				if request.Method == "server.stop" {
					_ = os.Remove(running)
				}
			}()
		}
	}()

	methods := make([]string, 0, len(herdr.RequiredMethods))
	for _, method := range herdr.RequiredMethods {
		methods = append(methods, fmt.Sprintf(`{"method":{"const":%s}}`, strconv.Quote(method)))
	}
	schema := fmt.Sprintf(`{"protocol":17,"requests":[%s]}`, strings.Join(methods, ","))
	sessions := fmt.Sprintf(`{"sessions":[{"name":%s,"running":true,"socket_path":%s}]}`,
		strconv.Quote(session), strconv.Quote(socket))
	stoppedSessions := fmt.Sprintf(`{"sessions":[{"name":%s,"running":false}]}`, strconv.Quote(session))
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
elif [ "$1" = "session" ] && [ "$2" = "attach" ]; then
  touch %s
  while [ -f %s ]; do sleep 0.02; done
  exit 9
else
  exit 2
fi
`, strconv.Quote(schema), strconv.Quote(running), strconv.Quote(sessions),
		strconv.Quote(exists), strconv.Quote(stoppedSessions), strconv.Quote(exists),
		strconv.Quote(attached), strconv.Quote(running))
	binary = filepath.Join(temp, "herdr-fake")
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary, attached, running
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func fakeStartSocket(
	t *testing.T,
	root string,
	workspacePresent bool,
	workspaceLog string,
	setupFailure ...string,
) string {
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
	snapshot := herdr.Snapshot{
		Version:  "0.7.5",
		Protocol: 17,
		Tabs:     []herdr.TabInfo{},
		Panes:    []herdr.PaneInfo{},
		Agents:   []herdr.AgentInfo{},
	}
	if workspacePresent {
		snapshot.Workspaces = []herdr.WorkspaceInfo{{
			WorkspaceID: "workspace-1",
			Worktree:    &herdr.WorkspaceWorktreeInfo{CheckoutPath: root, RepoRoot: root},
		}}
	} else {
		snapshot.Workspaces = []herdr.WorkspaceInfo{}
	}
	var mu sync.Mutex
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				var request struct {
					ID     string         `json:"id"`
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				if json.NewDecoder(conn).Decode(&request) != nil {
					return
				}
				mu.Lock()
				defer mu.Unlock()
				if len(setupFailure) > 0 && request.Method == setupFailure[0] {
					_ = json.NewEncoder(conn).Encode(map[string]any{
						"id": request.ID,
						"error": map[string]any{
							"code": "injected_failure", "message": "injected " + request.Method + " failure",
						},
					})
					return
				}
				var result any
				switch request.Method {
				case "ping":
					result = herdr.Pong{Type: "pong", Version: "0.7.5", Protocol: 17}
				case "pane.focus", "workspace.focus":
					result = map[string]any{"type": "focused"}
				case "session.snapshot":
					result = herdr.Result{
						Type:     "session_snapshot",
						Snapshot: snapshot,
					}
				case "workspace.create":
					cwd, _ := request.Params["cwd"].(string)
					if err := os.WriteFile(workspaceLog, []byte(cwd+"\n"), 0o600); err != nil {
						return
					}
					workspace := herdr.WorkspaceInfo{WorkspaceID: "workspace-created", Label: "fledge:test"}
					tab := herdr.TabInfo{TabID: "tab-created", WorkspaceID: workspace.WorkspaceID}
					pane := herdr.PaneInfo{
						PaneID: "pane-created", TabID: tab.TabID, WorkspaceID: workspace.WorkspaceID, CWD: &cwd,
					}
					snapshot.Workspaces = []herdr.WorkspaceInfo{workspace}
					snapshot.Tabs = []herdr.TabInfo{tab}
					snapshot.Panes = []herdr.PaneInfo{pane}
					result = herdr.Result{
						Type: "workspace_created", Workspace: workspace, Tab: tab, RootPane: pane,
					}
				case "tab.create":
					cwd, _ := request.Params["cwd"].(string)
					label, _ := request.Params["label"].(string)
					workspaceID, _ := request.Params["workspace_id"].(string)
					tab := herdr.TabInfo{
						TabID: "tab-created", WorkspaceID: workspaceID, Label: label,
					}
					pane := herdr.PaneInfo{
						PaneID: "pane-created", TabID: tab.TabID, WorkspaceID: workspaceID, CWD: &cwd,
					}
					snapshot.Tabs = append(snapshot.Tabs, tab)
					snapshot.Panes = append(snapshot.Panes, pane)
					result = herdr.Result{Type: "tab_created", Tab: tab, RootPane: pane}
				case "tab.rename":
					tabID, _ := request.Params["tab_id"].(string)
					label, _ := request.Params["label"].(string)
					for i := range snapshot.Tabs {
						if snapshot.Tabs[i].TabID == tabID {
							snapshot.Tabs[i].Label = label
							result = herdr.Result{Type: "tab_info", Tab: snapshot.Tabs[i]}
							break
						}
					}
				case "pane.rename":
					paneID, _ := request.Params["pane_id"].(string)
					label, _ := request.Params["label"].(string)
					for i := range snapshot.Panes {
						if snapshot.Panes[i].PaneID == paneID {
							snapshot.Panes[i].Label = &label
							result = map[string]any{"type": "pane_info", "pane": snapshot.Panes[i]}
							break
						}
					}
				case "pane.split":
					targetID, _ := request.Params["target_pane_id"].(string)
					cwd, _ := request.Params["cwd"].(string)
					var target herdr.PaneInfo
					for _, pane := range snapshot.Panes {
						if pane.PaneID == targetID {
							target = pane
							break
						}
					}
					pane := herdr.PaneInfo{
						PaneID: "pane-right", TabID: target.TabID,
						WorkspaceID: target.WorkspaceID, CWD: &cwd,
					}
					snapshot.Panes = append(snapshot.Panes, pane)
					result = map[string]any{"type": "pane_created", "pane": pane}
				case "pane.send_input":
					paneID, paneOK := request.Params["pane_id"].(string)
					text, textOK := request.Params["text"].(string)
					keys, keysOK := request.Params["keys"].([]any)
					if len(request.Params) != 3 || !paneOK || !textOK || !keysOK ||
						len(keys) != 1 || keys[0] != "enter" {
						_ = json.NewEncoder(conn).Encode(map[string]any{
							"id": request.ID,
							"error": map[string]any{
								"code":    "invalid_params",
								"message": "pane.send_input requires pane_id, text, and keys: [\"enter\"]",
							},
						})
						return
					}
					pickerLog, err := os.OpenFile(workspaceLog+".picker",
						os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
					if err != nil {
						return
					}
					_, writeErr := fmt.Fprintf(pickerLog, "%s|%s|%s|%s\n",
						request.Method, paneID, text, keys[0])
					closeErr := pickerLog.Close()
					if writeErr != nil || closeErr != nil {
						return
					}
					result = map[string]any{"type": "input_sent"}
				default:
					return
				}
				_ = json.NewEncoder(conn).Encode(map[string]any{"id": request.ID, "result": result})
			}()
		}
	}()
	return socket
}

func stringPointer(value string) *string {
	return &value
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
