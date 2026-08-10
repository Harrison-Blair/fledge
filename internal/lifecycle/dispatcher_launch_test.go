package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/watchproc"
)

// Coordination has no polling fallback, so an old Herdr has to be refused up
// front, with the user-controlled upgrade sequence spelled out. Starting
// against it would produce a session whose wakes are never delivered.
func TestStartRejectsAnOlderHerdrProtocolWithRestartGuidance(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	client := &fakeHerdr{protocol: watchproc.RequiredHerdrProtocol - 1}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness
	err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout})
	if err == nil {
		t.Fatal("Start() accepted an unsupported Herdr protocol")
	}
	for _, want := range []string{"protocol 18", "protocol 19", "fledge stop", "fledge start"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q is missing %q", err, want)
		}
	}
	if slicesContain(client.calls, "create-workspace") || slicesContain(client.calls, "start-agent") {
		t.Fatalf("calls = %v, want nothing created", client.calls)
	}
}

// A command that appends a wake has to make sure something is running to
// deliver it. The launch is idempotent, so doing it every time costs nothing
// when a dispatcher is already up and rescues the session when it has died.
func TestMutatingCommandsEnsureADispatcherIsRunning(t *testing.T) {
	t.Parallel()

	newLaunchManager := func(t *testing.T) (*Manager, *int, string, *fakeHerdr) {
		t.Helper()
		root := t.TempDir()
		writeTestRecord(t, root)
		client := &fakeHerdr{sessions: []herdr.Session{{Name: testSessionName, Running: true}}}
		manager, _ := newTestManager(client, &fakeConfirmer{})
		manager.getenv = func(string) string { return "" }
		launches := 0
		manager.watchLauncher = func(string) error { launches++; return nil }
		store := messaging.New(root, testSessionName)
		sessionID, err := store.Initialize()
		if err != nil {
			t.Fatal(err)
		}
		if err := writeRecordSessionBinding(root, testSessionName, sessionID, true); err != nil {
			t.Fatal(err)
		}
		for _, params := range []messaging.RegisterParams{
			{Name: "orchestrator", PaneID: "p-orchestrator", Harness: "codex", Caller: "user", CanDelegate: true},
			{Name: "worker", PaneID: "p-worker", Harness: "codex", Caller: "orchestrator"},
		} {
			if _, _, err := store.RegisterAgent(params); err != nil {
				t.Fatal(err)
			}
		}
		return manager, &launches, root, client
	}

	ctx := context.Background()
	t.Run("send", func(t *testing.T) {
		manager, launches, root, client := newLaunchManager(t)
		if _, err := manager.SendMessage(ctx, root, "worker", "go"); err != nil {
			t.Fatal(err)
		}
		if *launches != 1 {
			t.Fatalf("dispatcher launches = %d, want 1", *launches)
		}
		if len(client.calls) != 0 {
			t.Fatalf("Herdr calls = %v, want none", client.calls)
		}
	})

	t.Run("assign and transition", func(t *testing.T) {
		manager, launches, root, client := newLaunchManager(t)
		// Invoke from a nested directory: coordinationStore resolves the project
		// root once, and the same canonical root must reach the launcher for both
		// the assign and the transition — never the nested working directory.
		nested := filepath.Join(root, "sub", "dir")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		wantRoot, err := project.Find(nested)
		if err != nil {
			t.Fatal(err)
		}
		var launchedRoots []string
		manager.watchLauncher = func(gotRoot string) error {
			*launches++
			launchedRoots = append(launchedRoots, gotRoot)
			return nil
		}
		task, err := manager.TaskAssign(ctx, nested, "worker", "", "do the thing")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.TaskTransition(ctx, nested, task.ID, messaging.TaskCanceled, ""); err != nil {
			t.Fatal(err)
		}
		if *launches != 2 {
			t.Fatalf("dispatcher launches = %d, want 2", *launches)
		}
		for _, got := range launchedRoots {
			if got != wantRoot {
				t.Fatalf("launcher root = %q, want canonical project root %q", got, wantRoot)
			}
		}
		// Task commands validate, append, and launch. A wedged pane must not be
		// able to stall them, so they wait on no Herdr call at all.
		if len(client.calls) != 0 {
			t.Fatalf("Herdr calls = %v, want none", client.calls)
		}
	})

	// Reads append nothing, so they have nothing to deliver.
	t.Run("reads do not launch", func(t *testing.T) {
		manager, launches, root, _ := newLaunchManager(t)
		if _, err := manager.AgentList(ctx, root); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.TaskList(ctx, root); err != nil {
			t.Fatal(err)
		}
		if *launches != 0 {
			t.Fatalf("dispatcher launches = %d, want 0", *launches)
		}
	})
}
