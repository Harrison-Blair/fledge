package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/fledge"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/herdrtest"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

func TestStopJSONDeletesStoppedDeterministicSession(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	session := project.SessionName(root)
	binary, deleteLog := fakeStoppedStopBinary(t, session, true)

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"stop", "--json", "--herdr-bin", binary},
		bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Session string `json:"session"`
			Stopped bool   `json:"stopped"`
			Deleted bool   `json:"deleted"`
			Forced  bool   `json:"forced"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data.Session != session || envelope.Data.Stopped ||
		!envelope.Data.Deleted || envelope.Data.Forced {
		t.Fatalf("unexpected stop envelope: %#v", envelope)
	}
	if got := readFile(t, deleteLog); got != "session delete "+session+" --json\n" {
		t.Fatalf("delete invocation = %q", got)
	}
}

func TestStopTTYConfirmsStoppedSessionAndCancellationIsReadOnly(t *testing.T) {
	for _, test := range []struct {
		name       string
		answer     string
		wantDelete bool
	}{
		{name: "short yes", answer: "y\n", wantDelete: true},
		{name: "full yes case insensitive", answer: "YES\n", wantDelete: true},
		{name: "empty", answer: "\n", wantDelete: false},
		{name: "eof", answer: "", wantDelete: false},
		{name: "other", answer: "no\n", wantDelete: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := initializedProject(t)
			session := project.SessionName(root)
			binary, deleteLog := fakeStoppedStopBinary(t, session, true)
			if foundSession, found, findErr := (herdr.Binary{Path: binary}).FindSession(t.Context(), session); findErr != nil || !found {
				t.Fatalf("fake stopped session unavailable: session=%q found=%t value=%#v err=%v",
					session, found, foundSession, findErr)
			}
			stateDir := t.TempDir()
			store, err := state.New(stateDir)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.WithLocked(session, root, func(st *state.Session) error {
				st.StopGeneration = 9
				st.Socket = "/saved/socket"
				st.Agents["worker"] = state.Agent{Name: "worker", PaneID: "pane"}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before, err := store.Read(session, root)
			if err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			env := &environment{
				in: strings.NewReader(test.answer), out: &stdout, errOut: &stderr,
				cwd: root, stateDir: stateDir, herdrBin: binary,
				stdinTTY: func() bool { return true },
			}
			if err := executeStopCommand(t.Context(), env); err != nil {
				t.Fatalf("execute: %v stderr=%s", err, stderr.String())
			}
			output := stdout.String()
			if strings.Count(output, "[y/N]") != 1 ||
				!strings.Contains(output, "Shut down and delete Fledge session "+session+"?") {
				t.Fatalf("unexpected confirmation: %s", output)
			}
			_, deleteErr := os.Stat(deleteLog)
			deleted := deleteErr == nil
			if deleted != test.wantDelete {
				t.Fatalf("deleted=%t want=%t output=%s", deleted, test.wantDelete, output)
			}
			after, err := store.Read(session, root)
			if err != nil {
				t.Fatal(err)
			}
			if !test.wantDelete {
				if !reflect.DeepEqual(after, before) {
					t.Fatalf("cancellation mutated state: before=%#v after=%#v", before, after)
				}
				if !strings.Contains(output, "Cancelled") {
					t.Fatalf("missing cancellation status: %s", output)
				}
			}
		})
	}
}

func TestStopTTYShowsEveryLiveAgentInStableOrderBeforeConfirmation(t *testing.T) {
	root := initializedProject(t)
	session := project.SessionName(root)
	codex, claude, gemini := "codex", "claude", "gemini"
	stateDir := t.TempDir()
	store, err := state.New(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithLocked(session, root, func(st *state.Session) error {
		st.Agents["alpha"] = state.Agent{Name: "alpha", PaneID: "pane-a"}
		st.Agents["zeta"] = state.Agent{Name: "zeta", PaneID: "pane-z"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	snapshot := herdr.Snapshot{
		Panes: []herdr.PaneInfo{
			{WorkspaceID: "workspace-z", PaneID: "pane-z", Agent: &claude, AgentStatus: "working"},
			{WorkspaceID: "workspace-u", PaneID: "pane-unmanaged", Agent: &gemini, AgentStatus: "blocked"},
			{WorkspaceID: "workspace-a", PaneID: "pane-a", Agent: &codex, AgentStatus: "idle"},
		},
		Agents: []herdr.AgentInfo{
			{WorkspaceID: "workspace-z", Agent: &claude, AgentStatus: "working", PaneID: "pane-z"},
			{WorkspaceID: "workspace-a", Agent: &codex, AgentStatus: "idle", PaneID: "pane-a"},
		},
	}
	binary, serverStops := fakeLiveStopBinary(t, session, snapshot)
	var stdout, stderr bytes.Buffer
	env := &environment{
		in: strings.NewReader("no\n"), out: &stdout, errOut: &stderr,
		cwd: root, stateDir: stateDir, herdrBin: binary,
		stdinTTY: func() bool { return true },
	}
	if err := executeStopCommand(t.Context(), env); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, stderr.String())
	}
	output := stdout.String()
	for _, value := range []string{
		"NAME", "HARNESS", "STATE", "WORKSPACE", "PANE",
		"alpha", "codex", "idle", "workspace-a", "pane-a",
		"pane-unmanaged", "gemini", "blocked", "workspace-u",
		"zeta", "claude", "working", "workspace-z", "pane-z",
		"Running agents will be shut down. Are you sure you want to shut down Fledge session " + session + "? [y/N]",
	} {
		if !strings.Contains(output, value) {
			t.Fatalf("output missing %q:\n%s", value, output)
		}
	}
	if strings.Index(output, "alpha") > strings.Index(output, "pane-unmanaged") ||
		strings.Index(output, "pane-unmanaged") > strings.Index(output, "zeta") ||
		strings.Index(output, "zeta") > strings.Index(output, "Running agents will be shut down") {
		t.Fatalf("agents or prompt are out of order:\n%s", output)
	}
	if strings.Count(output, "[y/N]") != 1 || serverStops.Load() != 0 {
		t.Fatalf("prompt count/server stops = %d/%d:\n%s",
			strings.Count(output, "[y/N]"), serverStops.Load(), output)
	}
}

func TestStopForceSkipsTTYConfirmation(t *testing.T) {
	root := initializedProject(t)
	session := project.SessionName(root)
	binary, deleteLog := fakeStoppedStopBinary(t, session, true)
	if foundSession, found, findErr := (herdr.Binary{Path: binary}).FindSession(t.Context(), session); findErr != nil || !found {
		t.Fatalf("fake stopped session unavailable: session=%q found=%t value=%#v err=%v",
			session, found, foundSession, findErr)
	}
	var stdout, stderr bytes.Buffer
	env := &environment{
		in: strings.NewReader(""), out: &stdout, errOut: &stderr,
		cwd: root, stateDir: t.TempDir(), herdrBin: binary,
		stdinTTY: func() bool { return true },
	}
	if err := executeStopCommand(t.Context(), env, "--force"); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, stderr.String())
	}
	if strings.Contains(stdout.String(), "[y/N]") {
		t.Fatalf("--force prompted: %s", stdout.String())
	}
	if _, err := os.Stat(deleteLog); err != nil {
		t.Fatalf("--force did not stop session: %v", err)
	}
}

func TestStopTTYConfirmsMissingSessionBeforeClearingState(t *testing.T) {
	root := initializedProject(t)
	session := project.SessionName(root)
	binary, _ := fakeStoppedStopBinary(t, session, false)
	stateDir := t.TempDir()
	store, err := state.New(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithLocked(session, root, func(st *state.Session) error {
		st.StopGeneration = 5
		st.Socket = "/stale/socket"
		st.Agents["worker"] = state.Agent{Name: "worker", PaneID: "pane"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	env := &environment{
		in: strings.NewReader("yes\n"), out: &stdout, errOut: &stderr,
		cwd: root, stateDir: stateDir, herdrBin: binary,
		stdinTTY: func() bool { return true },
	}
	if err := executeStopCommand(t.Context(), env); err != nil {
		t.Fatalf("execute: %v stderr=%s", err, stderr.String())
	}
	if strings.Count(stdout.String(), "[y/N]") != 1 ||
		!strings.Contains(stdout.String(), "does not exist") {
		t.Fatalf("unexpected missing-session output: %s", stdout.String())
	}
	st, err := store.Read(session, root)
	if err != nil {
		t.Fatal(err)
	}
	if st.StopGeneration != 5 || st.Socket != "" || len(st.Agents) != 0 {
		t.Fatalf("missing-session cleanup left stale state: %#v", st)
	}
}

func TestStopHumanOutputReportsAgentsRequiringSessionShutdown(t *testing.T) {
	var output bytes.Buffer
	printStopResult(&output, fledge.StopResult{
		Session:      "fledge-test",
		Stopped:      true,
		Deleted:      true,
		ForcedAgents: []string{"alpha", "zeta"},
	})
	if !strings.Contains(output.String(), "Agents requiring session shutdown: alpha, zeta\n") {
		t.Fatalf("forced agents not reported: %s", output.String())
	}
}

func executeStopCommand(ctx context.Context, env *environment, args ...string) error {
	herdrBin := env.herdrBin
	root := newRoot(env)
	commandArgs := append([]string{"stop", "--herdr-bin", herdrBin}, args...)
	root.SetArgs(commandArgs)
	root.SetIn(env.in)
	root.SetOut(env.out)
	root.SetErr(env.errOut)
	return root.ExecuteContext(ctx)
}

func fakeStoppedStopBinary(t *testing.T, session string, exists bool) (string, string) {
	t.Helper()
	temp := t.TempDir()
	existsMarker := filepath.Join(temp, "exists")
	deleteLog := filepath.Join(temp, "delete.log")
	if exists {
		if err := os.WriteFile(existsMarker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sessions := fmt.Sprintf(`{"sessions":[{"name":%s,"running":false}]}`, strconv.Quote(session))
	binary := herdrtest.WriteBinary(t, temp, herdrtest.Options{
		Version:       herdrtest.VersionOutput,
		Sessions:      []herdrtest.SessionCase{{Marker: existsMarker, Payload: sessions}},
		DeleteRemoves: existsMarker,
		DeleteLog:     deleteLog,
	})
	return binary, deleteLog
}

func fakeLiveStopBinary(
	t *testing.T,
	session string,
	snapshot herdr.Snapshot,
) (string, *atomic.Int32) {
	t.Helper()
	temp := t.TempDir()
	socket := filepath.Join(temp, "herdr.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Skipf("sandbox does not permit Unix-domain listeners: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	var serverStops atomic.Int32
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
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
				case "server.stop":
					serverStops.Add(1)
					result = map[string]any{"type": "ok"}
				default:
					return
				}
				_ = json.NewEncoder(conn).Encode(map[string]any{"id": request.ID, "result": result})
			}()
		}
	}()
	sessions := fmt.Sprintf(`{"sessions":[{"name":%s,"running":true,"socket_path":%s}]}`,
		strconv.Quote(session), strconv.Quote(socket))
	binary := herdrtest.WriteBinary(t, temp, herdrtest.Options{
		Version:  herdrtest.VersionOutput,
		Sessions: []herdrtest.SessionCase{{Payload: sessions}},
	})
	return binary, &serverStops
}

func TestInternalStopCleanupFinalizesExactSessionAndState(t *testing.T) {
	root := t.TempDir()
	session := "fledge-test-cleanup"
	temp := t.TempDir()
	stateDir := filepath.Join(temp, "state")
	exists := filepath.Join(temp, "exists")
	if err := os.WriteFile(exists, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := state.New(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithLocked(session, root, func(st *state.Session) error {
		st.StopGeneration = 6
		st.Socket = "/old/socket"
		st.WorkspaceID = "workspace"
		st.OrchestratorTabID = "tab"
		st.OrchestratorPaneID = "pane"
		st.OrchestratorInitialized = true
		st.Agents["worker"] = state.Agent{Name: "worker", PaneID: "agent-pane"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sessions := fmt.Sprintf(`{"sessions":[{"name":%s,"running":false}]}`, strconv.Quote(session))
	binary := herdrtest.WriteBinary(t, temp, herdrtest.Options{
		Sessions:      []herdrtest.SessionCase{{Marker: exists, Payload: sessions}},
		DeleteRemoves: exists,
	})

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		stopCleanupCommand,
		"--project-root", root,
		"--session", session,
		"--state-dir", stateDir,
		"--herdr-bin", binary,
		"--generation", "6",
		"--timeout", "1s",
	}, bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("internal worker emitted output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if _, err := os.Stat(exists); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session namespace remains: %v", err)
	}
	st, err := store.Read(session, root)
	if err != nil {
		t.Fatal(err)
	}
	if st.StopGeneration != 7 || st.Socket != "" || st.WorkspaceID != "" ||
		st.OrchestratorTabID != "" || st.OrchestratorPaneID != "" ||
		st.OrchestratorInitialized || len(st.Agents) != 0 {
		t.Fatalf("internal cleanup left stale state: %#v", st)
	}
}

func TestInitJSONAndIdempotency(t *testing.T) {
	root := t.TempDir()
	for i, wantInitialized := range []bool{true, false} {
		var stdout, stderr bytes.Buffer
		code := Execute(context.Background(), []string{"init", root, "--json"}, bytes.NewBuffer(nil), &stdout, &stderr)
		if code != 0 {
			t.Fatalf("run %d: exit=%d stderr=%s", i, code, stderr.String())
		}
		var envelope struct {
			SchemaVersion int  `json:"schema_version"`
			OK            bool `json:"ok"`
			Data          struct {
				ProjectRoot string `json:"project_root"`
				MarkerPath  string `json:"marker_path"`
				Initialized bool   `json:"initialized"`
			} `json:"data"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.SchemaVersion != 1 || !envelope.OK ||
			envelope.Data.Initialized != wantInitialized ||
			envelope.Data.MarkerPath != filepath.Join(envelope.Data.ProjectRoot, ".fledge", "config.json") {
			t.Fatalf("unexpected envelope: %#v", envelope)
		}
	}
}

func TestUninitializedProjectReturnsStableActionableError(t *testing.T) {
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	var stderr bytes.Buffer
	code := Execute(context.Background(), []string{"status", "--json"}, bytes.NewBuffer(nil), &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "project_not_initialized" ||
		!bytes.Contains([]byte(envelope.Error.Message), []byte("fledge init")) {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
}

func TestStatusFromNestedDirectoryUsesMarkerAndReportsSessionSource(t *testing.T) {
	root := t.TempDir()
	if _, err := project.Init(root); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "src", "component")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	legacyAssociation := filepath.Join(stateHome, "fledge", "associations.json")
	if err := os.MkdirAll(filepath.Dir(legacyAssociation), 0o700); err != nil {
		t.Fatal(err)
	}
	legacyContents := []byte("{legacy association state is intentionally ignored\n")
	if err := os.WriteFile(legacyAssociation, legacyContents, 0o600); err != nil {
		t.Fatal(err)
	}

	unrelated := `{"sessions":[{"name":"unrelated-session","running":true,` +
		`"socket_path":"/missing/unrelated.sock"}]}`
	binary := herdrtest.WriteBinary(t, t.TempDir(), herdrtest.Options{
		Version:  "herdr test",
		Sessions: []herdrtest.SessionCase{{Payload: unrelated}},
	})

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"status", "--json", "--herdr-bin", binary},
		bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var envelope struct {
		Data struct {
			ProjectRoot   string `json:"project_root"`
			SessionSource string `json:"session_source"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	canonical, _ := filepath.EvalSymlinks(root)
	if envelope.Data.ProjectRoot != canonical || envelope.Data.SessionSource != "derived" {
		t.Fatalf("unexpected nested status: %#v", envelope)
	}
	after, err := os.ReadFile(legacyAssociation)
	if err != nil || !bytes.Equal(after, legacyContents) {
		t.Fatalf("legacy association file was read or rewritten: %q, %v", after, err)
	}
}
