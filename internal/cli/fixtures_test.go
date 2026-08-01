package cli

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/herdrtest"
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
	TempDir       string `json:"temp_dir"`
	TempCleaned   bool   `json:"temp_cleaned"`
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

// startOptions varies the fake herdr built by fakeStartBinary: whether the
// session is already Running, whether the workspace is already present, the
// status the "session attach" branch exits with, and the socket method whose
// failure is injected during session setup.
type startOptions struct {
	Running          bool
	WorkspacePresent bool
	AttachExit       int
	SetupFailure     string
}

func fakeStartBinary(t *testing.T, root, session string, opts startOptions) (string, string, string) {
	t.Helper()
	temp := t.TempDir()
	runningMarker := filepath.Join(temp, "running")
	attachLog := filepath.Join(temp, "attach.log")
	workspaceLog := filepath.Join(temp, "workspace.log")
	pidFile := filepath.Join(temp, "server.pid")
	socket := fakeStartSocket(t, root, workspaceLog, opts)
	if opts.Running {
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

	sessions := fmt.Sprintf(`{"sessions":[{"name":%s,"running":true,"socket_path":%s}]}`,
		strconv.Quote(session), strconv.Quote(socket))
	attachBody := fmt.Sprintf(`printf '%%s|%%s\n' "$PWD" "$*" >> %s
exit %d
`, strconv.Quote(attachLog), opts.AttachExit)
	serverBody := fmt.Sprintf(`printf '%%s' "$$" > %s
touch %s
trap 'exit 0' TERM INT
while :; do sleep 1; done
`, strconv.Quote(pidFile), strconv.Quote(runningMarker))
	binary := herdrtest.WriteBinary(t, temp, herdrtest.Options{
		Version:  herdrtest.VersionOutput,
		Sessions: []herdrtest.SessionCase{{Marker: runningMarker, Payload: sessions}},
		Branches: []herdrtest.Branch{
			{Condition: `[ "$1" = "session" ] && [ "$2" = "attach" ]`, Body: attachBody},
			{Condition: `[ "$1" = "--session" ] && [ "$3" = "server" ]`, Body: serverBody},
		},
	})
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
	snapshot := herdrtest.EmptySnapshot()
	snapshot.Workspaces = []herdr.WorkspaceInfo{{
		WorkspaceID: "workspace-1",
		Worktree:    &herdr.WorkspaceWorktreeInfo{CheckoutPath: root, RepoRoot: root},
	}}
	snapshot.Tabs = []herdr.TabInfo{{
		TabID: "orchestrator-tab", WorkspaceID: "workspace-1", Label: "orchestrator",
	}}
	snapshot.Panes = []herdr.PaneInfo{
		{
			PaneID: "orchestrator-left", TabID: "orchestrator-tab",
			WorkspaceID: "workspace-1", Label: stringPointer("orchestrator"), CWD: &root,
		},
		{
			PaneID: "orchestrator-right", TabID: "orchestrator-tab",
			WorkspaceID: "workspace-1", CWD: &root,
		},
	}
	// The marker disappears just before the stop is answered, so the fake
	// "session attach" branch can notice the shutdown as soon as it happens.
	socket := herdrtest.Server{
		Snapshot: &snapshot,
		Observe: func(call herdrtest.Call) {
			if call.Method == "server.stop" {
				_ = os.Remove(running)
			}
		},
	}.Start(t)

	sessions := fmt.Sprintf(`{"sessions":[{"name":%s,"running":true,"socket_path":%s}]}`,
		strconv.Quote(session), strconv.Quote(socket))
	stoppedSessions := fmt.Sprintf(`{"sessions":[{"name":%s,"running":false}]}`, strconv.Quote(session))
	attachBody := fmt.Sprintf(`touch %s
while [ -f %s ]; do sleep 0.02; done
exit 9
`, strconv.Quote(attached), strconv.Quote(running))
	binary = herdrtest.WriteBinary(t, temp, herdrtest.Options{
		Version: herdrtest.VersionOutput,
		Sessions: []herdrtest.SessionCase{
			{Marker: running, Payload: sessions},
			{Marker: exists, Payload: stoppedSessions},
		},
		DeleteRemoves: exists,
		Branches: []herdrtest.Branch{
			{Condition: `[ "$1" = "session" ] && [ "$2" = "attach" ]`, Body: attachBody},
		},
	})
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

func fakeStartSocket(t *testing.T, root, workspaceLog string, opts startOptions) string {
	t.Helper()
	snapshot := herdrtest.EmptySnapshot()
	if opts.WorkspacePresent {
		snapshot.Workspaces = []herdr.WorkspaceInfo{{
			WorkspaceID: "workspace-1",
			Worktree:    &herdr.WorkspaceWorktreeInfo{CheckoutPath: root, RepoRoot: root},
		}}
	}
	return herdrtest.Server{
		Snapshot: &snapshot,
		IDs: herdrtest.IDs{
			Workspace: "workspace-created", WorkspaceTab: "tab-created", WorkspacePane: "pane-created",
			Tab: "tab-created", TabPane: "pane-created", SplitPane: "pane-right",
		},
		Observe: func(call herdrtest.Call) { logStartCall(workspaceLog, call) },
		Handle: func(conn net.Conn, call herdrtest.Call) bool {
			if call.Method != opts.SetupFailure {
				return false
			}
			herdrtest.WriteInjectedFailure(conn, call)
			return true
		},
	}.Start(t)
}

// logStartCall records the two session-setup requests start's tests read back
// from disk.
func logStartCall(workspaceLog string, call herdrtest.Call) {
	switch call.Method {
	case "workspace.create":
		_ = os.WriteFile(workspaceLog, []byte(call.Text("cwd")+"\n"), 0o600)
	case "pane.send_input":
		keys, _ := call.Params["keys"].([]any)
		if len(keys) != 1 {
			return
		}
		appendLine(workspaceLog+".picker", fmt.Sprintf("%s|%s|%s|%s\n",
			call.Method, call.Text("pane_id"), call.Text("text"), keys[0]))
	}
}

func appendLine(path, line string) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	_, _ = file.WriteString(line)
	_ = file.Close()
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
