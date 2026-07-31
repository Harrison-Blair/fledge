package fledge

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

func TestFailedServerStopDoesNotAdvanceGeneration(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	fake.mu.Lock()
	fake.serverStopError = "refused by server"
	fake.mu.Unlock()
	workerDone := make(chan error, 1)
	service.LaunchStopCleanup = func(request StopCleanupRequest) error {
		workerStore, err := state.New(request.StateDir)
		if err != nil {
			return err
		}
		worker := &Service{
			Project: project.Info{Root: request.ProjectRoot, Session: request.Session},
			Binary:  herdr.Binary{Path: request.HerdrBinary},
			Store:   workerStore,
		}
		go func() {
			workerDone <- worker.FinalizeStop(context.Background(), request.BaseGeneration, 150*time.Millisecond)
		}()
		return nil
	}

	if _, err := service.Stop(t.Context(), false); err == nil {
		t.Fatal("expected server stop failure")
	}
	select {
	case err := <-workerDone:
		if translated := Translate(err); translated.Code != "session_stop_timeout" {
			t.Fatalf("worker error after rejected stop = %#v", translated)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup worker did not stop polling after rejected shutdown")
	}
	if generation, err := service.StopGeneration(); err != nil || generation != 0 {
		t.Fatalf("generation after failed stop = %d, %v", generation, err)
	}
	if _, found, err := service.Binary.FindSession(t.Context(), service.Project.Session); err != nil || !found {
		t.Fatalf("rejected stop removed the running session: found=%t err=%v", found, err)
	}
}

func TestCleanupWorkerLaunchFailureLeavesServerRunning(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.LaunchStopCleanup = func(StopCleanupRequest) error {
		return errors.New("injected launch failure")
	}

	_, err := service.Stop(t.Context(), false)
	if translated := Translate(err); translated.Code != "cleanup_worker_launch_failed" {
		t.Fatalf("unexpected launch error: %#v", translated)
	}
	fake.mu.Lock()
	stopped := fake.serverStopped
	fake.mu.Unlock()
	if stopped {
		t.Fatal("server.stop was sent after cleanup worker launch failed")
	}
	if generation, err := service.StopGeneration(); err != nil || generation != 0 {
		t.Fatalf("generation after launch failure = %d, %v", generation, err)
	}
}

func TestDetachedCleanupSurvivesCallerDisappearingAfterServerStop(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.StopGeneration = 4
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

	workerDone := make(chan error, 1)
	service.LaunchStopCleanup = func(request StopCleanupRequest) error {
		workerStore, err := state.New(request.StateDir)
		if err != nil {
			return err
		}
		worker := &Service{
			Project: project.Info{Root: request.ProjectRoot, Session: request.Session},
			Binary:  herdr.Binary{Path: request.HerdrBinary},
			Store:   workerStore,
		}
		go func() {
			workerDone <- worker.FinalizeStop(context.Background(), request.BaseGeneration, request.Timeout)
		}()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake.mu.Lock()
	fake.serverStopHook = func() {
		cancel()
		time.Sleep(20 * time.Millisecond)
	}
	fake.mu.Unlock()

	if _, err := service.Stop(ctx, false); err == nil {
		t.Fatal("foreground stop unexpectedly survived its canceled connection")
	}
	select {
	case err := <-workerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("detached cleanup did not finish")
	}
	if _, found, err := service.Binary.FindSession(context.Background(), service.Project.Session); err != nil || found {
		t.Fatalf("session remains after detached cleanup: found=%t err=%v", found, err)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if st.StopGeneration != 5 || st.Socket != "" || st.WorkspaceID != "" ||
		st.OrchestratorTabID != "" || st.OrchestratorPaneID != "" ||
		st.OrchestratorInitialized || len(st.Agents) != 0 {
		t.Fatalf("detached cleanup left stale state: %#v", st)
	}
}

func TestServerStopStillDeletesSessionWhenGenerationCannotBePersisted(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	if _, err := service.Store.Read(service.Project.Session, service.Project.Root); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(service.Store.Root, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("state files = %v, %v", files, err)
	}
	service.LaunchStopCleanup = func(StopCleanupRequest) error {
		return os.WriteFile(files[0], []byte("{broken\n"), 0o600)
	}

	_, err = service.Stop(t.Context(), false)
	if translated := Translate(err); translated.Code != "state_persist_failed" ||
		!strings.Contains(translated.Message, "session was deleted") {
		t.Fatalf("unexpected stop error: %#v", translated)
	}
	if _, found, findErr := service.Binary.FindSession(t.Context(), service.Project.Session); findErr != nil || found {
		t.Fatalf("session remains after state failure: found=%t err=%v", found, findErr)
	}
}

func TestForcedServerStopDeletesSession(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	if _, err := service.StartAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := service.Stop(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Stopped || !result.Deleted || !result.Forced ||
		strings.Join(result.Agents, ",") != "worker" ||
		strings.Join(result.GracefullyStoppedAgents, ",") != "worker" ||
		len(result.ForcedAgents) != 0 {
		t.Fatalf("unexpected forced stop result: %#v", result)
	}
}

func TestInspectStopIncludesSortedManagedAndUnmanagedLiveAgents(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	codex, claude := "codex", "claude"
	alpha, zeta := "alpha", "zeta"
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.Agents["saved-alpha"] = state.Agent{Name: "saved-alpha", PaneID: "p-alpha"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.snapshot.Panes = []herdr.PaneInfo{
		{WorkspaceID: "workspace-z", PaneID: "p-zeta", Agent: &claude, AgentStatus: "working"},
		{WorkspaceID: "workspace-u", PaneID: "p-unmanaged", Agent: &codex, AgentStatus: "blocked"},
		{WorkspaceID: "workspace-a", PaneID: "p-alpha", Agent: &codex, AgentStatus: "idle"},
	}
	fake.snapshot.Agents = []herdr.AgentInfo{
		{Name: &zeta, Agent: &claude, AgentStatus: "working", WorkspaceID: "workspace-z", PaneID: "p-zeta"},
		{Name: &alpha, Agent: &codex, AgentStatus: "idle", WorkspaceID: "workspace-a", PaneID: "p-alpha"},
	}
	fake.mu.Unlock()

	inspection, err := service.InspectStop(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.Exists || !inspection.Running || inspection.Session != service.Project.Session {
		t.Fatalf("unexpected session inspection: %#v", inspection)
	}
	want := []StopAgentInspection{
		{Name: "alpha", Harness: "codex", State: "idle", WorkspaceID: "workspace-a", PaneID: "p-alpha"},
		{Name: "p-unmanaged", Harness: "codex", State: "blocked", WorkspaceID: "workspace-u", PaneID: "p-unmanaged"},
		{Name: "zeta", Harness: "claude", State: "working", WorkspaceID: "workspace-z", PaneID: "p-zeta"},
	}
	if !reflect.DeepEqual(inspection.LiveAgents, want) {
		t.Fatalf("live agents = %#v, want %#v", inspection.LiveAgents, want)
	}
}

func TestStopRecoversSavedNamesWithoutMutatingState(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	codex := "codex"
	alphaLabel, orchestratorLabel := "fledge-agent:alpha", "fledge-agent:fledge-orchestrator"
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.WorkspaceID = "old-workspace"
		st.Agents["alpha"] = state.Agent{Name: "alpha", PaneID: "old-alpha", TabID: "old-tab"}
		st.Agents["fledge-orchestrator"] = state.Agent{
			Name: "fledge-orchestrator", PaneID: "w1:p1", TabID: "old-tab",
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	stateFiles, err := filepath.Glob(filepath.Join(service.Store.Root, "*.json"))
	if err != nil || len(stateFiles) != 1 {
		t.Fatalf("state files = %v, err=%v", stateFiles, err)
	}
	stateBefore, err := os.ReadFile(stateFiles[0])
	if err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "workspace-new"}}
	fake.snapshot.Panes = []herdr.PaneInfo{
		{
			WorkspaceID: "workspace-new", TabID: "tab-new", PaneID: "pane-alpha",
			Label: &alphaLabel, Agent: &codex, AgentStatus: "idle",
		},
		{
			WorkspaceID: "workspace-new", TabID: "tab-new", PaneID: "w1:p1",
			Label: &orchestratorLabel, Agent: &codex, AgentStatus: "working",
		},
	}
	fake.snapshot.Agents = []herdr.AgentInfo{
		{WorkspaceID: "workspace-new", TabID: "tab-new", PaneID: "pane-alpha", Agent: &codex, AgentStatus: "idle"},
		{WorkspaceID: "workspace-new", TabID: "tab-new", PaneID: "w1:p1", Agent: &codex, AgentStatus: "working"},
	}
	fake.mu.Unlock()

	inspection, err := service.InspectStop(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []StopAgentInspection{
		{Name: "alpha", Harness: "codex", State: "idle", WorkspaceID: "workspace-new", PaneID: "pane-alpha"},
		{Name: "fledge-orchestrator", Harness: "codex", State: "working", WorkspaceID: "workspace-new", PaneID: "w1:p1"},
	}
	if !reflect.DeepEqual(inspection.LiveAgents, want) {
		t.Fatalf("live agents = %#v, want %#v", inspection.LiveAgents, want)
	}
	stateAfterInspection, err := os.ReadFile(stateFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stateAfterInspection, stateBefore) {
		t.Fatal("stop inspection persisted in-memory mapping reconciliation")
	}

	_, err = service.Stop(t.Context(), false)
	translated := Translate(err)
	if translated.Code != "live_agents" {
		t.Fatalf("stop error = %#v", translated)
	}
	details, ok := translated.Details.(map[string]any)
	if !ok || !reflect.DeepEqual(details["agents"], []string{"alpha", "fledge-orchestrator"}) {
		t.Fatalf("live-agent details = %#v", translated.Details)
	}
	stateAfterConflict, err := os.ReadFile(stateFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stateAfterConflict, stateBefore) {
		t.Fatal("rejected stop mutated saved state")
	}

	result, err := service.Stop(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Agents, []string{"alpha", "fledge-orchestrator"}) ||
		!reflect.DeepEqual(result.GracefullyStoppedAgents, []string{"alpha", "fledge-orchestrator"}) ||
		len(result.ForcedAgents) != 0 {
		t.Fatalf("stop result names = %#v", result)
	}
}

func TestInspectStopIgnoresMalformedOptionalNameMappings(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	codex := "codex"
	fake.mu.Lock()
	fake.snapshot.Panes = []herdr.PaneInfo{{
		WorkspaceID: "workspace", PaneID: "w1:p1", Agent: &codex, AgentStatus: "idle",
	}}
	fake.snapshot.Agents = []herdr.AgentInfo{{
		WorkspaceID: "workspace", PaneID: "w1:p1", Agent: &codex, AgentStatus: "idle",
	}}
	fake.mu.Unlock()
	if _, err := service.Store.Read(service.Project.Session, service.Project.Root); err != nil {
		t.Fatal(err)
	}
	stateFiles, err := filepath.Glob(filepath.Join(service.Store.Root, "*.json"))
	if err != nil || len(stateFiles) != 1 {
		t.Fatalf("state files = %v, err=%v", stateFiles, err)
	}
	malformed := []byte("{broken\n")
	if err := os.WriteFile(stateFiles[0], malformed, 0o600); err != nil {
		t.Fatal(err)
	}

	inspection, err := service.InspectStop(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []StopAgentInspection{{
		Name: "w1:p1", Harness: "codex", State: "idle", WorkspaceID: "workspace", PaneID: "w1:p1",
	}}
	if !reflect.DeepEqual(inspection.LiveAgents, want) {
		t.Fatalf("live agents = %#v, want %#v", inspection.LiveAgents, want)
	}
	after, err := os.ReadFile(stateFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, malformed) {
		t.Fatal("best-effort name lookup rewrote malformed state")
	}
}

func TestInspectStopUsesPaneIDWhenOptionalNameMappingsAreMissing(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	codex, arbitraryLabel := "codex", "friendly-but-unmanaged"
	fake.mu.Lock()
	fake.snapshot.Panes = []herdr.PaneInfo{{
		WorkspaceID: "workspace", PaneID: "w1:p1", Label: &arbitraryLabel,
		Agent: &codex, AgentStatus: "idle",
	}}
	fake.snapshot.Agents = []herdr.AgentInfo{{
		WorkspaceID: "workspace", PaneID: "w1:p1", Agent: &codex, AgentStatus: "idle",
	}}
	fake.mu.Unlock()

	inspection, err := service.InspectStop(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []StopAgentInspection{{
		Name: "w1:p1", Harness: "codex", State: "idle", WorkspaceID: "workspace", PaneID: "w1:p1",
	}}
	if !reflect.DeepEqual(inspection.LiveAgents, want) {
		t.Fatalf("live agents = %#v, want %#v", inspection.LiveAgents, want)
	}
	entries, err := os.ReadDir(service.Store.Root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("inspection created state files for missing optional mappings: %v", entries)
	}
}

func TestCoordinatedStopGracefullySignalsEveryLiveAgentBeforeServerStop(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	codex, claude := "codex", "claude"
	alpha, zeta := "alpha", "zeta"
	fake.mu.Lock()
	fake.snapshot.Panes = []herdr.PaneInfo{
		{PaneID: "p-zeta", Agent: &claude, AgentStatus: "working"},
		{PaneID: "p-alpha", Agent: &codex, AgentStatus: "idle"},
	}
	fake.snapshot.Agents = []herdr.AgentInfo{
		{Name: &zeta, Agent: &claude, AgentStatus: "working", PaneID: "p-zeta"},
		{Name: &alpha, Agent: &codex, AgentStatus: "idle", PaneID: "p-alpha"},
	}
	fake.mu.Unlock()

	result, err := service.Stop(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(result.GracefullyStoppedAgents, ",") != "alpha,zeta" ||
		len(result.ForcedAgents) != 0 {
		t.Fatalf("unexpected agent outcomes: %#v", result)
	}
	fake.mu.Lock()
	targets := append([]string(nil), fake.sendKeysTargets...)
	methods := append([]string(nil), fake.methodCalls...)
	fake.mu.Unlock()
	sort.Strings(targets)
	if strings.Join(targets, ",") != "p-alpha,p-zeta" {
		t.Fatalf("graceful targets = %v", targets)
	}
	serverStopIndex := slices.Index(methods, "server.stop")
	for _, targetMethod := range []string{"agent.send_keys"} {
		if index := slices.Index(methods, targetMethod); index < 0 || index > serverStopIndex {
			t.Fatalf("method ordering = %v", methods)
		}
	}
}

func TestGracefulStopAgentsClassifiesSharedBudgetTimeouts(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	codex := "codex"
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.Agents["beta"] = state.Agent{Name: "beta", PaneID: "p-beta"}
		st.Agents["alpha"] = state.Agent{Name: "alpha", PaneID: "p-alpha"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.ignoreExit = true
	fake.snapshot.Panes = []herdr.PaneInfo{
		{WorkspaceID: "workspace", PaneID: "p-beta", Agent: &codex, AgentStatus: "working"},
		{WorkspaceID: "workspace", PaneID: "p-alpha", Agent: &codex, AgentStatus: "idle"},
	}
	fake.mu.Unlock()
	client := &herdr.Client{Socket: serviceSessionSocket(t, service.Binary)}
	live, err := service.collectLiveAgentDetails(t.Context(), client)
	if err != nil {
		t.Fatal(err)
	}
	if names := liveAgentNames(live); !reflect.DeepEqual(names, []string{"alpha", "beta"}) {
		t.Fatalf("recovered live names = %v", names)
	}

	start := time.Now()
	graceful, forced := service.gracefullyStopAgents(t.Context(), client, live, 40*time.Millisecond)
	if len(graceful) != 0 || strings.Join(forced, ",") != "alpha,beta" {
		t.Fatalf("graceful=%v forced=%v", graceful, forced)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("agents did not share one shutdown budget: %s", elapsed)
	}
}

func TestGracefulAgentStopRetainsPane(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	if _, err := service.StartAgent(t.Context(), AgentStartOptions{Name: "worker", Kind: "codex", Timeout: 30 * time.Second}); err != nil {
		t.Fatal(err)
	}
	result, err := service.StopAgent(t.Context(), "worker", time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	paneCount := len(fake.snapshot.Panes)
	fake.mu.Unlock()
	if result.Forced || result.Agent.State != "stopped" || paneCount != 1 {
		t.Fatalf("unexpected retained stop: result=%#v panes=%d", result, paneCount)
	}
}

func TestAgentStopTimeoutPreservesPane(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	if _, err := service.StartAgent(t.Context(), AgentStartOptions{Name: "worker", Kind: "codex", Timeout: 30 * time.Second}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.ignoreExit = true
	fake.mu.Unlock()
	_, err := service.StopAgent(t.Context(), "worker", 30*time.Millisecond, false)
	if Translate(err).Code != "agent_stop_timeout" {
		t.Fatalf("unexpected error: %v", err)
	}
	fake.mu.Lock()
	paneCount := len(fake.snapshot.Panes)
	fake.mu.Unlock()
	if paneCount != 1 {
		t.Fatal("timed-out stop did not preserve pane")
	}
}
