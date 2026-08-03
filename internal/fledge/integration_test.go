//go:build linux

package fledge

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

// This test is deliberately opt-in because it starts a real detached Herdr
// server and, when Codex is installed, a real agent. It registers cleanup
// before start and verifies marker discovery, deterministic session ownership,
// and the fresh one-tab orchestrator layout.
func TestLocalHerdrLifecycle(t *testing.T) {
	herdrPath := requireIntegrationHerdr(t)
	root, session := newIntegrationRepo(t)
	registerHerdrCleanup(t, herdrPath, session)
	nested := filepath.Join(root, "src", "component")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	discovered, err := project.Discover(nested)
	if err != nil || discovered.Root != root {
		t.Fatalf("nested project discovery = %#v, %v", discovered, err)
	}
	store, err := state.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	binary := herdr.Binary{Path: herdrPath}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resolution, err := ResolveSession(ctx, discovered.Root, binary)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Session != session || resolution.Source != "derived" || resolution.WorkspaceID != "" {
		t.Fatalf("deterministic resolution failed: %#v", resolution)
	}
	discovered.Session, discovered.SessionSource = resolution.Session, resolution.Source
	installed := resolution.Installed
	service := Service{
		Project: discovered, Binary: binary, Store: store,
		Installed: &installed, WorkspaceID: resolution.WorkspaceID,
		LaunchStopCleanup: func(StopCleanupRequest) error { return nil },
	}
	started, err := service.Start(ctx, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !started.Started || started.Session != session {
		t.Fatalf("fresh lifecycle did not start deterministic session: %#v", started)
	}
	if err := service.EnsureAttachmentWorkspace(ctx, started.Socket, nested); err != nil {
		t.Fatal(err)
	}
	assertOrchestratorLayout(t, ctx, &service, started.Socket)
	status, err := service.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.ServerState != "running" || !status.ProtocolCompatible || status.SessionSource != "derived" {
		t.Fatalf("unexpected status: %#v", status)
	}
	runOptionalAgentLifecycle(t, ctx, &service)
	if _, err := service.Stop(ctx, true); err != nil {
		t.Fatal(err)
	}
	if _, found, err := binary.FindSession(ctx, session); err != nil || found {
		t.Fatalf("disposable session remains after lifecycle: found=%t err=%v", found, err)
	}
	runs, err := service.MessageRuns(0)
	if err != nil || len(runs.Runs) != 1 || runs.Runs[0].Active {
		t.Fatalf("archived message run = %#v, %v", runs, err)
	}
	assertRunIsolationAfterRestart(t, ctx, &service, runs.Runs[0].ID)
	if _, err := service.Stop(ctx, true); err != nil {
		t.Fatal(err)
	}
}

// requireIntegrationHerdr skips unless the opt-in flag is set and a real Herdr
// is installed, returning its resolved path.
func requireIntegrationHerdr(t *testing.T) string {
	t.Helper()
	if os.Getenv("FLEDGE_INTEGRATION") != "1" {
		t.Skip("set FLEDGE_INTEGRATION=1 to run against local Herdr")
	}
	herdrPath, err := exec.LookPath("herdr")
	if err != nil {
		t.Skip("herdr is not installed")
	}
	return herdrPath
}

// newIntegrationRepo initializes a project in a fresh temporary directory and
// returns its canonical root together with the session name derived from it.
func newIntegrationRepo(t *testing.T) (string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", root, "init", "-q").Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := project.Init(root); err != nil {
		t.Fatal(err)
	}
	return root, project.SessionName(root)
}

// registerHerdrCleanup tears the session down however far the test got. It
// must be registered before the server is started.
func registerHerdrCleanup(t *testing.T, herdrPath, session string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, herdrPath, "--session", session, "server", "stop").Run()
		_ = exec.CommandContext(ctx, herdrPath, "session", "stop", session).Run()
		_ = exec.CommandContext(ctx, herdrPath, "session", "delete", session, "--json").Run()
	})
}

// assertOrchestratorLayout verifies the fresh session owns exactly one
// workspace and one tab, split into a named primary pane and an unlabeled one.
func assertOrchestratorLayout(t *testing.T, ctx context.Context, service *Service, socket string) {
	t.Helper()
	snapshot, err := (&herdr.Client{Socket: socket}).Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Workspaces) != 1 || len(snapshot.Tabs) != 1 {
		t.Fatalf("orchestrator layout has workspaces=%d tabs=%d, want one of each: %#v",
			len(snapshot.Workspaces), len(snapshot.Tabs), snapshot)
	}
	persisted, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	tab, found := tabInWorkspace(snapshot, service.WorkspaceID, persisted.OrchestratorTabID)
	if !found || tab.Label != orchestratorLabel {
		t.Fatalf("orchestrator tab was not created: %#v", snapshot.Tabs)
	}
	panes := panesInTab(snapshot, tab.TabID)
	if len(panes) != 2 {
		t.Fatalf("orchestrator pane count = %d, want 2: %#v", len(panes), panes)
	}
	primary, found := orchestratorPane(panes, "")
	if !found || primary.Label == nil || *primary.Label != orchestratorLabel {
		t.Fatalf("orchestrator primary pane was not named: %#v", panes)
	}
	for _, pane := range panes {
		if pane.PaneID != primary.PaneID && pane.Label != nil {
			t.Fatalf("orchestrator right pane was unexpectedly labeled: %#v", panes)
		}
	}
}

// runOptionalAgentLifecycle spawns, inspects and force-stops a real agent when
// a harness is installed, and is a no-op otherwise.
func runOptionalAgentLifecycle(t *testing.T, ctx context.Context, service *Service) {
	t.Helper()
	if _, err := exec.LookPath("codex"); err != nil {
		return
	}
	// The dedicated-tab spawn re-invokes the fledge binary inside the pane,
	// and this process is a test binary, so a real one has to be built.
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fledgeBinary := filepath.Join(t.TempDir(), "fledge")
	build := exec.Command("go", "build", "-o", fledgeBinary, "./cmd/fledge")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Fledge: %v: %s", err, out)
	}
	service.FledgeExecutable = fledgeBinary
	if _, err := service.SpawnAgent(ctx, AgentStartOptions{
		Name: "integration-agent", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	agents, err := service.ListAgents(ctx)
	if err != nil || len(agents) != 1 {
		t.Fatalf("agent lifecycle inspection = %#v, %v", agents, err)
	}
	if _, err := service.StopAgent(ctx, "integration-agent", 3*time.Second, true); err != nil {
		t.Fatal(err)
	}
}

// assertRunIsolationAfterRestart restarts the stopped session and verifies it
// opens a message run distinct from the archived one.
func assertRunIsolationAfterRestart(t *testing.T, ctx context.Context, service *Service, archivedRunID string) {
	t.Helper()
	restarted, err := service.Start(ctx, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !restarted.Started {
		t.Fatalf("fresh restart = %#v", restarted)
	}
	restartedState, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if restartedState.ActiveRunID == "" || restartedState.ActiveRunID == archivedRunID {
		t.Fatalf("restart did not isolate message runs: %#v", restartedState)
	}
}

// TestLocalHerdrStopFromOrchestratorPane verifies the failure mode where
// server.stop tears down the pane that is running Fledge before the foreground
// process can clean up the stopped namespace.
func TestLocalHerdrStopFromOrchestratorPane(t *testing.T) {
	herdrPath := requireIntegrationHerdr(t)
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fledgeBinary := filepath.Join(t.TempDir(), "fledge")
	build := exec.Command("go", "build", "-o", fledgeBinary, "./cmd/fledge")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Fledge: %v: %s", err, out)
	}

	root, session := newIntegrationRepo(t)
	stateHome := t.TempDir()
	commandEnv := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "XDG_STATE_HOME=") {
			commandEnv = append(commandEnv, value)
		}
	}
	commandEnv = append(commandEnv, "XDG_STATE_HOME="+stateHome)
	registerHerdrCleanup(t, herdrPath, session)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := exec.CommandContext(ctx, fledgeBinary,
		"start", "--detach", "--json", "--herdr-bin", herdrPath)
	start.Dir, start.Env = root, commandEnv
	if out, err := start.CombinedOutput(); err != nil {
		t.Fatalf("start built Fledge: %v: %s", err, out)
	}

	binary := herdr.Binary{Path: herdrPath}
	info, found, err := binary.FindSession(ctx, session)
	if err != nil || !found || !info.Running || info.SessionDir == "" {
		t.Fatalf("started session = %#v, found=%t err=%v", info, found, err)
	}
	store, err := state.New(filepath.Join(stateHome, "fledge"))
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Read(session, root)
	if err != nil {
		t.Fatal(err)
	}
	if before.OrchestratorPaneID == "" {
		t.Fatalf("orchestrator pane was not persisted: %#v", before)
	}

	run := exec.CommandContext(ctx, herdrPath, "--session", session, "pane", "run",
		before.OrchestratorPaneID,
		"env", "-C", root, "XDG_STATE_HOME="+stateHome,
		fledgeBinary, "stop", "--json", "--herdr-bin", herdrPath)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("run stop in orchestrator pane: %v: %s", err, out)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		_, found, err = binary.FindSession(ctx, session)
		if err == nil && !found {
			break
		}
		if time.Now().After(deadline) {
			paneOut, _ := exec.Command(herdrPath, "--session", session, "pane", "read",
				before.OrchestratorPaneID, "--source", "recent-unwrapped", "--lines", "50").CombinedOutput()
			current, stateErr := store.Read(session, root)
			t.Fatalf("session remained after in-pane stop: found=%t err=%v state=%#v state_err=%v pane=%s",
				found, err, current, stateErr, paneOut)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(info.SessionDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Herdr session directory remains: %v", err)
	}
	after, err := store.Read(session, root)
	if err != nil {
		t.Fatal(err)
	}
	// The shared assertion prints only the post-stop state; this restores the
	// pre-stop context on failure without widening the helper's signature.
	t.Logf("state before in-pane stop: %#v", before)
	assertDisposableStateCleared(t, after, before.StopGeneration+1, nil)
}
