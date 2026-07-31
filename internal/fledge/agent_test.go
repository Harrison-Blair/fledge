package fledge

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/state"
)

func TestSpawnAgentInCurrentPaneRenamesOnlyPaneAndPersistsSelection(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w1"
	label := "orchestrator"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}
	fake.snapshot.Tabs = []herdr.TabInfo{{TabID: "t1", WorkspaceID: "w1", Label: "orchestrator"}}
	fake.snapshot.Panes = []herdr.PaneInfo{{
		PaneID: "p1", TabID: "t1", WorkspaceID: "w1", Label: &label,
	}}
	fake.mu.Unlock()
	var execPath string
	var execArgv []string
	service.ExecAgent = func(path string, argv, _ []string) error {
		execPath, execArgv = path, append([]string(nil), argv...)
		return nil
	}

	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Model: "custom/model", CurrentPaneID: "p1",
		Executable: "/usr/bin/codex", Timeout: 30 * time.Second, Args: []string{"--sandbox", "read-only"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Agent.Placement != "pane" || result.Agent.Model != "custom/model" ||
		execPath != "/usr/bin/codex" ||
		strings.Join(execArgv, " ") != "/usr/bin/codex --model custom/model --sandbox read-only" {
		t.Fatalf("result=%#v path=%q argv=%v", result, execPath, execArgv)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if st.Agents["worker"].PaneID != "p1" || st.Agents["worker"].Placement != "pane" {
		t.Fatalf("state = %#v", st)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.tabRenames != 0 || fake.paneRenames != 1 ||
		fake.snapshot.Tabs[0].Label != "orchestrator" ||
		fake.snapshot.Panes[0].Label == nil || *fake.snapshot.Panes[0].Label != "worker" {
		t.Fatalf("layout = tabs %#v panes %#v", fake.snapshot.Tabs, fake.snapshot.Panes)
	}
}

func TestSpawnAgentInCurrentPaneRollsBackMappingAndLabelOnExecFailure(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w1"
	label := "orchestrator"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}
	fake.snapshot.Tabs = []herdr.TabInfo{{TabID: "t1", WorkspaceID: "w1", Label: label}}
	fake.snapshot.Panes = []herdr.PaneInfo{{
		PaneID: "p1", TabID: "t1", WorkspaceID: "w1", Label: &label,
	}}
	fake.mu.Unlock()
	service.ExecAgent = func(string, []string, []string) error { return errors.New("exec exploded") }

	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", CurrentPaneID: "p1",
		Executable: "/usr/bin/codex", Timeout: 30 * time.Second,
	})
	if Translate(err).Code != "agent_exec_failed" {
		t.Fatalf("error = %v", err)
	}
	st, readErr := service.Store.Read(service.Project.Session, service.Project.Root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(st.Agents) != 0 {
		t.Fatalf("mapping was not rolled back: %#v", st.Agents)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.snapshot.Panes[0].Label == nil || *fake.snapshot.Panes[0].Label != label {
		t.Fatalf("label was not restored: %#v", fake.snapshot.Panes[0])
	}
}

func TestSpawnAgentRejectsPaneOutsideCurrentWorkspace(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w-current"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w-current"}, {WorkspaceID: "w-other"}}
	fake.snapshot.Panes = []herdr.PaneInfo{{PaneID: "p-other", WorkspaceID: "w-other"}}
	fake.mu.Unlock()
	service.ExecAgent = func(string, []string, []string) error { return nil }

	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", CurrentPaneID: "p-other",
		Executable: "/usr/bin/codex", Timeout: 30 * time.Second,
	})
	if Translate(err).Code != "invalid_herdr_pane" {
		t.Fatalf("error = %v", err)
	}
}

func TestSpawnAgentReassignsStoppedMappingOnCurrentPane(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w1"
	oldLabel := "old"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}
	fake.snapshot.Panes = []herdr.PaneInfo{{
		PaneID: "p1", TabID: "t1", WorkspaceID: "w1", Label: &oldLabel,
	}}
	fake.mu.Unlock()
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.WorkspaceID = "w1"
		st.Agents["old"] = state.Agent{Name: "old", PaneID: "p1", TabID: "t1"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service.ExecAgent = func(string, []string, []string) error { return nil }
	if _, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "new", Kind: "pi", CurrentPaneID: "p1", Executable: "/usr/bin/pi",
		Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := st.Agents["old"]; exists || st.Agents["new"].PaneID != "p1" {
		t.Fatalf("state = %#v", st.Agents)
	}
}

func TestSpawnAgentRejectsNameOwnedByAnotherPane(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w1"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}
	fake.snapshot.Panes = []herdr.PaneInfo{
		{PaneID: "p1", TabID: "t1", WorkspaceID: "w1"},
		{PaneID: "p2", TabID: "t2", WorkspaceID: "w1"},
	}
	fake.mu.Unlock()
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.WorkspaceID = "w1"
		st.Agents["worker"] = state.Agent{Name: "worker", PaneID: "p2", TabID: "t2"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service.ExecAgent = func(string, []string, []string) error { return nil }
	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "pi", CurrentPaneID: "p1", Executable: "/usr/bin/pi",
		Timeout: 30 * time.Second,
	})
	if Translate(err).Code != "agent_name_conflict" {
		t.Fatalf("error = %v", err)
	}
}

func TestDedicatedSpawnUsesRawLabelsAndPersistsModel(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "claude", Model: "sonnet", NewTab: true,
		Executable: "/usr/bin/claude", Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Agent.Model != "sonnet" || result.Agent.Placement != "tab" {
		t.Fatalf("result = %#v", result)
	}
	agents, err := service.ListAgents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].Model != "sonnet" || agents[0].Placement != "tab" {
		t.Fatalf("listed agents = %#v", agents)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.snapshot.Tabs) != 1 || fake.snapshot.Tabs[0].Label != "worker" ||
		len(fake.snapshot.Panes) != 1 || fake.snapshot.Panes[0].Label == nil ||
		*fake.snapshot.Panes[0].Label != "worker" {
		t.Fatalf("raw labels not applied: tabs=%#v panes=%#v", fake.snapshot.Tabs, fake.snapshot.Panes)
	}
}

func TestDedicatedSpawnDoesNotAdoptUntrustedRawLabel(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w1"
	raw := "worker"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}
	fake.snapshot.Tabs = []herdr.TabInfo{{TabID: "user-tab", WorkspaceID: "w1", Label: raw}}
	fake.snapshot.Panes = []herdr.PaneInfo{{
		PaneID: "user-pane", TabID: "user-tab", WorkspaceID: "w1", Label: &raw,
	}}
	fake.mu.Unlock()
	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Agent.PaneID != "p-new" {
		t.Fatalf("unrelated raw label was adopted: %#v", result)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.tabCreates != 1 {
		t.Fatalf("tab creates = %d", fake.tabCreates)
	}
}

func TestLegacyPrefixedLabelIsRecoveredAndRenamed(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w1"
	legacy := "fledge-agent:worker"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}
	fake.snapshot.Tabs = []herdr.TabInfo{{TabID: "legacy-tab", WorkspaceID: "w1", Label: legacy}}
	fake.snapshot.Panes = []herdr.PaneInfo{{
		PaneID: "legacy-pane", TabID: "legacy-tab", WorkspaceID: "w1", Label: &legacy,
	}}
	fake.mu.Unlock()
	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Agent.PaneID != "legacy-pane" {
		t.Fatalf("legacy pane not reused: %#v", result)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.snapshot.Tabs[0].Label != "worker" || fake.snapshot.Panes[0].Label == nil ||
		*fake.snapshot.Panes[0].Label != "worker" {
		t.Fatalf("legacy labels not renamed: tabs=%#v panes=%#v", fake.snapshot.Tabs, fake.snapshot.Panes)
	}
}

func TestModelArgumentsGeneratedForEveryHarness(t *testing.T) {
	for _, harness := range []string{"claude", "codex", "pi", "opencode"} {
		t.Run(harness, func(t *testing.T) {
			got := modelArgs("provider/model", []string{"--native"})
			if strings.Join(got, " ") != "--model provider/model --native" {
				t.Fatalf("args = %v", got)
			}
		})
	}
}

func TestStartAgentAllocatesOnceAndForwardsNativeArgs(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	result, err := service.StartAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Args: []string{"--model", "gpt-5"}, Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Agent.PaneID != "p1" || strings.Join(result.Argv, " ") != "--model gpt-5" {
		t.Fatalf("unexpected result: %#v", result)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.startCalls != 1 || strings.Join(fake.startArgs, " ") != "--model gpt-5" {
		t.Fatalf("start calls=%d args=%v", fake.startCalls, fake.startArgs)
	}
}

func TestAgentWorkspaceCreationUsesProjectFolderLabel(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.Project.Root = filepath.Join(t.TempDir(), "My Project")
	if err := os.Mkdir(service.Project.Root, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := service.StartAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.workspaceCreates != 1 || len(fake.snapshot.Workspaces) != 1 ||
		fake.snapshot.Workspaces[0].Label != "My Project" {
		t.Fatalf("created workspaces = %#v, calls = %d", fake.snapshot.Workspaces, fake.workspaceCreates)
	}
}

func TestConcurrentDuplicateAgentStartAllocatesOnlyOnce(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.StartAgent(t.Context(), AgentStartOptions{Name: "same", Kind: "codex", Timeout: 30 * time.Second})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	failures := 0
	for err := range errs {
		if err != nil {
			failures++
			if Translate(err).Code != "agent_already_running" {
				t.Fatalf("unexpected error: %v", err)
			}
		}
	}
	fake.mu.Lock()
	starts := fake.startCalls
	fake.mu.Unlock()
	if failures != 1 || starts != 1 {
		t.Fatalf("failures=%d start calls=%d", failures, starts)
	}
}

func TestStartAgentAddsTabToAdoptedWorkspaceWithoutCreatingWorkspace(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w-adopted"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w-adopted", Label: "independent"}}
	fake.mu.Unlock()

	result, err := service.StartAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.workspaceCreates != 0 || fake.tabCreates != 1 {
		t.Fatalf("workspace creates=%d tab creates=%d", fake.workspaceCreates, fake.tabCreates)
	}
	if result.Agent.TabID != "t-new" || result.Agent.PaneID != "p-new" {
		t.Fatalf("agent was not placed in adopted workspace: %#v", result)
	}
}

func TestStartAgentReusesLabeledPaneInAdoptedWorkspace(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w-adopted"
	label := "fledge-agent:worker"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w-adopted"}}
	fake.snapshot.Tabs = []herdr.TabInfo{{TabID: "t-existing", WorkspaceID: "w-adopted", Label: label}}
	fake.snapshot.Panes = []herdr.PaneInfo{{
		PaneID: "p-existing", TabID: "t-existing", WorkspaceID: "w-adopted", Label: &label, AgentStatus: "unknown",
	}}
	fake.mu.Unlock()

	result, err := service.StartAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.workspaceCreates != 0 || fake.tabCreates != 0 {
		t.Fatalf("retained pane reuse mutated layout: workspace creates=%d tab creates=%d",
			fake.workspaceCreates, fake.tabCreates)
	}
	if result.Agent.PaneID != "p-existing" || result.Agent.TabID != "t-existing" {
		t.Fatalf("labeled pane was not reused: %#v", result)
	}
}

func TestPromptDefaultWaitStatesRemainServerOwned(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	if _, err := service.StartAgent(t.Context(), AgentStartOptions{Name: "worker", Kind: "codex", Timeout: 30 * time.Second}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Prompt(t.Context(), PromptOptions{Name: "worker", Text: "hello", Wait: true}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.promptWait == nil {
		t.Fatal("prompt did not request atomic wait")
	}
	if _, present := fake.promptWait["until"]; present {
		t.Fatalf("client overrode Herdr default settled states: %#v", fake.promptWait)
	}
	if fake.promptTarget != "p1" {
		t.Fatalf("prompt target = %q, want pane ID", fake.promptTarget)
	}
}

func TestControlOperationsUsePersistedPaneID(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	if _, err := service.StartAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Wait(t.Context(), "worker", nil, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadAgent(t.Context(), "worker", "recent-unwrapped", 10, false); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.waitTarget != "p1" || fake.readTarget != "p1" {
		t.Fatalf("wait target=%q read target=%q", fake.waitTarget, fake.readTarget)
	}
}

func TestForceStopRejectsSavedOrchestratorPane(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	if _, err := service.StartAgent(t.Context(), AgentStartOptions{
		Name: "fledge-orchestrator", Kind: "codex", Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.OrchestratorPaneID = st.Agents["fledge-orchestrator"].PaneID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err := service.StopAgent(t.Context(), "fledge-orchestrator", time.Second, true)
	if Translate(err).Code != "orchestrator_force_stop_forbidden" {
		t.Fatalf("error = %v", err)
	}
}

func TestEnqueueOrchestratorPickerTargetsSavedPaneAndQuotesExecutable(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.OrchestratorPaneID = "p-left"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	socket := serviceSessionSocket(t, service.Binary)
	if err := service.EnqueueOrchestratorPicker(t.Context(), socket, "/opt/Fledge Tools/fledge"); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.sendInputCalls != 1 ||
		fake.sendInputPaneID != "p-left" ||
		fake.sendInputText != "'/opt/Fledge Tools/fledge' agent spawn --name fledge-orchestrator" ||
		len(fake.sendInputKeys) != 1 || fake.sendInputKeys[0] != "enter" {
		t.Fatalf("calls=%d pane_id=%q text=%q keys=%v",
			fake.sendInputCalls, fake.sendInputPaneID, fake.sendInputText, fake.sendInputKeys)
	}
	if fake.sendKeysTarget != "" || len(fake.sendKeys) != 0 {
		t.Fatalf("picker used agent.send_keys: target=%q keys=%v", fake.sendKeysTarget, fake.sendKeys)
	}
}

func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	if got := shellQuote("/opt/user's tools/fledge"); got != "'/opt/user'\"'\"'s tools/fledge'" {
		t.Fatalf("quoted path = %q", got)
	}
}

func TestServerStopRefusesLiveAgentAndForceStopClosesPane(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	cleanupLaunches := 0
	service.LaunchStopCleanup = func(StopCleanupRequest) error {
		cleanupLaunches++
		return nil
	}
	if _, err := service.StartAgent(t.Context(), AgentStartOptions{Name: "worker", Kind: "codex", Timeout: 30 * time.Second}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Stop(t.Context(), false); Translate(err).Code != "live_agents" {
		t.Fatalf("unexpected stop error: %v", err)
	}
	if cleanupLaunches != 0 {
		t.Fatalf("cleanup worker launched before live-agent safety validation: %d", cleanupLaunches)
	}
	if generation, err := service.StopGeneration(); err != nil || generation != 0 {
		t.Fatalf("generation after refused stop = %d, %v", generation, err)
	}
	if _, err := service.StopAgent(t.Context(), "worker", time.Second, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Stop(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if cleanupLaunches != 1 {
		t.Fatalf("cleanup worker launches after successful safety validation = %d, want 1", cleanupLaunches)
	}
	if generation, err := service.StopGeneration(); err != nil || generation != 1 {
		t.Fatalf("generation after successful stop = %d, %v", generation, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if !fake.serverStopped {
		t.Fatal("server.stop was not sent")
	}
}

func TestIdleShellIsNotAnOccupiedPane(t *testing.T) {
	shell := 20
	if paneHasUnrelatedForegroundProcess(herdr.ProcessInfo{
		ShellPID: &shell, ForegroundProcesses: []herdr.Process{{PID: 20, Name: "bash"}},
	}) {
		t.Fatal("idle shell was treated as unrelated")
	}
	if !paneHasUnrelatedForegroundProcess(herdr.ProcessInfo{
		ShellPID: &shell, ForegroundProcesses: []herdr.Process{{PID: 21, Name: "vim"}},
	}) {
		t.Fatal("unrelated foreground process was not detected")
	}
}

func TestReconcileMappingsReclaimsRestoredLabeledPane(t *testing.T) {
	label := "fledge-agent:worker"
	st := state.Session{
		WorkspaceID: "old-workspace",
		Agents: map[string]state.Agent{
			"worker": {Name: "worker", PaneID: "old-pane", TabID: "old-tab"},
		},
	}
	snapshot := herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "new-workspace", Label: "fledge:test-session"}},
		Panes:      []herdr.PaneInfo{{PaneID: "new-pane", TabID: "new-tab", WorkspaceID: "new-workspace", Label: &label}},
	}
	reconcileMappings(&st, snapshot, "/source/test", "test-session", "")
	if st.WorkspaceID != "new-workspace" || st.Agents["worker"].PaneID != "new-pane" || st.Agents["worker"].TabID != "new-tab" {
		t.Fatalf("mapping was not reclaimed: %#v", st)
	}
}
