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
	if os.Getenv("FLEDGE_INTEGRATION") != "1" {
		t.Skip("set FLEDGE_INTEGRATION=1 to run against local Herdr")
	}
	if _, err := exec.LookPath("herdr"); err != nil {
		t.Skip("herdr is not installed")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session := project.SessionName(root)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "herdr", "--session", session, "server", "stop").Run()
		_ = exec.CommandContext(ctx, "herdr", "session", "stop", session).Run()
		_ = exec.CommandContext(ctx, "herdr", "session", "delete", session, "--json").Run()
	})
	if err := exec.Command("git", "-C", root, "init", "-q").Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := project.Init(root); err != nil {
		t.Fatal(err)
	}
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
	binary := herdr.Binary{Path: "herdr"}
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
	snapshot, err := (&herdr.Client{Socket: started.Socket}).Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Workspaces) != 1 || len(snapshot.Tabs) != 1 {
		t.Fatalf("orchestrator layout has workspaces=%d tabs=%d, want one of each: %#v",
			len(snapshot.Workspaces), len(snapshot.Tabs), snapshot)
	}
	tab, found := orchestratorTab(snapshot, service.WorkspaceID, "")
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
	status, err := service.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.ServerState != "running" || !status.ProtocolCompatible || status.SessionSource != "derived" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if _, err := exec.LookPath("codex"); err == nil {
		if _, err := service.StartAgent(ctx, AgentStartOptions{
			Name: "integration-agent", Kind: "codex", Timeout: 30 * time.Second,
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
	firstRunID := runs.Runs[0].ID
	restarted, err := service.Start(ctx, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !restarted.Started {
		t.Fatalf("fresh restart = %#v", restarted)
	}
	restartedState, err := store.Read(session, root)
	if err != nil {
		t.Fatal(err)
	}
	if restartedState.ActiveRunID == "" || restartedState.ActiveRunID == firstRunID {
		t.Fatalf("restart did not isolate message runs: %#v", restartedState)
	}
	if _, err := service.Stop(ctx, true); err != nil {
		t.Fatal(err)
	}
}

// TestLocalHerdrStopFromOrchestratorPane verifies the failure mode where
// server.stop tears down the pane that is running Fledge before the foreground
// process can clean up the stopped namespace.
func TestLocalHerdrStopFromOrchestratorPane(t *testing.T) {
	if os.Getenv("FLEDGE_INTEGRATION") != "1" {
		t.Skip("set FLEDGE_INTEGRATION=1 to run against local Herdr")
	}
	herdrPath, err := exec.LookPath("herdr")
	if err != nil {
		t.Skip("herdr is not installed")
	}
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
	session := project.SessionName(root)
	stateHome := t.TempDir()
	commandEnv := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "XDG_STATE_HOME=") {
			commandEnv = append(commandEnv, value)
		}
	}
	commandEnv = append(commandEnv, "XDG_STATE_HOME="+stateHome)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanupCtx, herdrPath, "--session", session, "server", "stop").Run()
		_ = exec.CommandContext(cleanupCtx, herdrPath, "session", "stop", session).Run()
		_ = exec.CommandContext(cleanupCtx, herdrPath, "session", "delete", session, "--json").Run()
	})

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
	if after.StopGeneration != before.StopGeneration+1 ||
		after.Socket != "" || after.WorkspaceID != "" ||
		after.OrchestratorTabID != "" || after.OrchestratorPaneID != "" ||
		after.OrchestratorInitialized || len(after.Agents) != 0 {
		t.Fatalf("in-pane cleanup left stale state: before=%#v after=%#v", before, after)
	}
}
