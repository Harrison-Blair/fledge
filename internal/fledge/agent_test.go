package fledge

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

func TestSpawnAgentInCurrentPaneRenamesOnlyPaneAndPersistsSelection(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	seedTakeoverPane(t, service, fake)
	var execPath string
	var execArgv []string
	service.ExecAgent = func(path string, argv, _ []string) error {
		execPath, execArgv = path, append([]string(nil), argv...)
		return nil
	}

	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Model: "custom/model", Profile: "reviewer", CurrentPaneID: "p1",
		Executable: "/usr/bin/codex", Timeout: 30 * time.Second, Args: []string{"--sandbox", "read-only"},
		RememberSelection: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Agent.Placement != "pane" || result.Agent.Model != "custom/model" || result.Agent.Profile != "reviewer" ||
		execPath != "/usr/bin/codex" ||
		strings.Join(execArgv, " ") != "/usr/bin/codex --model custom/model --sandbox read-only" {
		t.Fatalf("result=%#v path=%q argv=%v", result, execPath, execArgv)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if st.Agents["worker"].PaneID != "p1" || st.Agents["worker"].Placement != "pane" ||
		st.Agents["worker"].Profile != "reviewer" {
		t.Fatalf("state = %#v", st)
	}
	if st.LastSpawnSelection == nil || *st.LastSpawnSelection !=
		(state.SpawnSelection{Harness: "codex", Model: "custom/model"}) {
		t.Fatalf("remembered selection = %#v", st.LastSpawnSelection)
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
	label := seedTakeoverPane(t, service, fake)
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.LastSpawnSelection = &state.SpawnSelection{Harness: "claude", Model: "sonnet"}
		st.Agents["prior"] = state.Agent{Name: "prior", Profile: "original", PaneID: "p1", TabID: "t1"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service.ExecAgent = func(string, []string, []string) error { return errors.New("exec exploded") }

	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Model: "gpt-5.6", Profile: "replacement", CurrentPaneID: "p1",
		Executable: "/usr/bin/codex", Timeout: 30 * time.Second,
		RememberSelection: true,
	})
	if Translate(err).Code != "agent_exec_failed" {
		t.Fatalf("error = %v", err)
	}
	st, readErr := service.Store.Read(service.Project.Session, service.Project.Root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(st.Agents) != 1 || st.Agents["prior"].Profile != "original" {
		t.Fatalf("mapping was not rolled back: %#v", st.Agents)
	}
	if st.LastSpawnSelection == nil || *st.LastSpawnSelection !=
		(state.SpawnSelection{Harness: "claude", Model: "sonnet"}) {
		t.Fatalf("selection was not rolled back: %#v", st.LastSpawnSelection)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.snapshot.Panes[0].Label == nil || *fake.snapshot.Panes[0].Label != label {
		t.Fatalf("label was not restored: %#v", fake.snapshot.Panes[0])
	}
}

func TestSpawnAgentInCurrentPaneRollsBackSelectionOnActivationFailure(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	seedTakeoverPane(t, service, fake)
	want := state.SpawnSelection{Harness: "claude", Model: "sonnet"}
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		selection := want
		st.LastSpawnSelection = &selection
		st.Agents["prior"] = state.Agent{Name: "prior", Profile: "original", PaneID: "p1", TabID: "t1"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service.LaunchDeliveryHelper = func(string, string, time.Duration) error {
		return errors.New("helper exploded")
	}
	service.ExecAgent = func(string, []string, []string) error {
		t.Fatal("exec called after activation failure")
		return nil
	}

	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Model: "gpt-5.6", Profile: "replacement", CurrentPaneID: "p1",
		Executable: "/usr/bin/codex", Timeout: 30 * time.Second, RememberSelection: true,
	})
	if Translate(err).Code != "agent_exec_failed" {
		t.Fatalf("error = %v", err)
	}
	st, readErr := service.Store.Read(service.Project.Session, service.Project.Root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if st.LastSpawnSelection == nil || *st.LastSpawnSelection != want || len(st.Agents) != 1 ||
		st.Agents["prior"].Profile != "original" {
		t.Fatalf("rollback state = %#v", st)
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
		Name: "worker", Kind: "claude", Model: "sonnet", Profile: "reviewer", NewTab: true,
		Executable: "/usr/bin/claude", Timeout: 30 * time.Second, RememberSelection: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Agent.Model != "sonnet" || result.Agent.Profile != "reviewer" || result.Agent.Placement != "tab" {
		t.Fatalf("result = %#v", result)
	}
	spawnJSON, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(spawnJSON), `"profile":"reviewer"`) {
		t.Fatalf("spawn JSON = %s, %v", spawnJSON, err)
	}
	agents, err := service.ListAgents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].Model != "sonnet" || agents[0].Profile != "reviewer" ||
		agents[0].Placement != "tab" {
		t.Fatalf("listed agents = %#v", agents)
	}
	status, err := service.AgentStatus(t.Context(), "worker")
	if err != nil || len(status) != 1 || status[0].Profile != "reviewer" {
		t.Fatalf("agent status = %#v, %v", status, err)
	}
	projectStatus, err := service.Status(t.Context())
	if err != nil || len(projectStatus.Agents) != 1 || projectStatus.Agents[0].Profile != "reviewer" {
		t.Fatalf("project status agents = %#v, %v", projectStatus.Agents, err)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastSpawnSelection == nil || *st.LastSpawnSelection !=
		(state.SpawnSelection{Harness: "claude", Model: "sonnet"}) {
		t.Fatalf("remembered selection = %#v", st.LastSpawnSelection)
	}
	if st.Agents["worker"].Profile != "reviewer" {
		t.Fatalf("persisted profile = %#v", st.Agents["worker"])
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.snapshot.Tabs) != 1 || fake.snapshot.Tabs[0].Label != "worker" ||
		len(fake.snapshot.Panes) != 1 || fake.snapshot.Panes[0].Label == nil ||
		*fake.snapshot.Panes[0].Label != "worker" {
		t.Fatalf("raw labels not applied: tabs=%#v panes=%#v", fake.snapshot.Tabs, fake.snapshot.Panes)
	}
}

func TestDedicatedRespawnReplacesAndClearsProfileOnRetainedPane(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	first, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Profile: "reviewer", NewTab: true, Timeout: 30 * time.Second,
	})
	if err != nil || first.Agent.Profile != "reviewer" {
		t.Fatalf("profiled spawn = %#v, %v", first, err)
	}
	fake.mu.Lock()
	fake.exitAgent(first.Agent.PaneID)
	fake.mu.Unlock()

	adhoc, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	})
	if err != nil || adhoc.Agent.Profile != "" || adhoc.Agent.PaneID != first.Agent.PaneID {
		t.Fatalf("ad-hoc respawn = %#v, %v", adhoc, err)
	}
	adhocJSON, err := json.Marshal(adhoc)
	if err != nil || strings.Contains(string(adhocJSON), `"profile"`) {
		t.Fatalf("ad-hoc spawn JSON = %s, %v", adhocJSON, err)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil || st.Agents["worker"].Profile != "" {
		t.Fatalf("ad-hoc persisted state = %#v, %v", st.Agents["worker"], err)
	}

	fake.mu.Lock()
	fake.exitAgent(adhoc.Agent.PaneID)
	fake.mu.Unlock()
	profiled, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Profile: "builder", NewTab: true, Timeout: 30 * time.Second,
	})
	if err != nil || profiled.Agent.Profile != "builder" || profiled.Agent.PaneID != first.Agent.PaneID {
		t.Fatalf("profiled respawn = %#v, %v", profiled, err)
	}
	st, err = service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil || st.Agents["worker"].Profile != "builder" {
		t.Fatalf("profiled persisted state = %#v, %v", st.Agents["worker"], err)
	}
}

func TestDedicatedSpawnFailureAndExplicitSpawnDoNotChangeSelection(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	want := state.SpawnSelection{Harness: "claude", Model: "sonnet"}
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		selection := want
		st.LastSpawnSelection = &selection
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	fake.failMethod = "agent.start"
	if _, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "failed", Kind: "codex", Model: "gpt-5.6", NewTab: true,
		Timeout: 30 * time.Second, RememberSelection: true,
	}); err == nil {
		t.Fatal("expected launch failure")
	}
	fake.mu.Lock()
	if len(fake.snapshot.Tabs) != 0 || len(fake.snapshot.Panes) != 0 {
		fake.mu.Unlock()
		t.Fatalf("failed spawn left its freshly created tab open: %#v", fake.snapshot.Tabs)
	}
	fake.mu.Unlock()
	fake.failMethod = ""
	if _, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "explicit", Kind: "codex", Model: "gpt-5.6", NewTab: true,
		Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastSpawnSelection == nil || *st.LastSpawnSelection != want {
		t.Fatalf("selection changed after failed or explicit spawn: %#v", st.LastSpawnSelection)
	}
}

func TestDedicatedSpawnWaitsForBootingPaneShell(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	fake.mu.Lock()
	fake.bootingProcessInfoCalls = 2
	fake.startBusyWhileBooting = true
	fake.mu.Unlock()

	if _, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.startBusyRejections != 0 || fake.startCalls != 1 || fake.processInfoCalls < 3 {
		t.Fatalf("busy rejections=%d starts=%d shell probes=%d",
			fake.startBusyRejections, fake.startCalls, fake.processInfoCalls)
	}
	if fake.workspaceCreates != 1 || fake.tabCreates != 0 {
		t.Fatalf("boot race duplicated layout: workspace creates=%d tab creates=%d",
			fake.workspaceCreates, fake.tabCreates)
	}
}

func TestDedicatedSpawnClosesCreatedTabWhenShellNeverAppears(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	fake.mu.Lock()
	fake.bootingProcessInfoCalls = 1 << 30
	fake.mu.Unlock()

	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 300 * time.Millisecond,
	})
	if translated := Translate(err); translated == nil || translated.Code != "agent_pane_unready" {
		t.Fatalf("error = %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.startCalls != 0 || len(fake.snapshot.Tabs) != 0 || len(fake.snapshot.Panes) != 0 {
		t.Fatalf("failed spawn left layout behind: starts=%d tabs=%#v panes=%#v",
			fake.startCalls, fake.snapshot.Tabs, fake.snapshot.Panes)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, recorded := st.Agents["worker"]; recorded {
		t.Fatalf("failed spawn persisted its mapping: %#v", st.Agents)
	}
}

func TestDedicatedSpawnRollbackSurfacesTabCloseFailure(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	fake.mu.Lock()
	fake.bootingProcessInfoCalls = 1 << 30
	fake.failMethod = "tab.close"
	fake.mu.Unlock()

	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 300 * time.Millisecond,
	})
	translated := Translate(err)
	if translated == nil || translated.Code != "agent_tab_close_failed" ||
		!strings.Contains(translated.Message, `agent "worker" failed to start`) ||
		!strings.Contains(translated.Message, "could not be closed") {
		t.Fatalf("error = %#v", translated)
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

// modelArgs is harness-independent, so one assertion covers every harness.
func TestModelArgumentsPrecedeNativeArguments(t *testing.T) {
	got := modelArgs("provider/model", []string{"--native"})
	if strings.Join(got, " ") != "--model provider/model --native" {
		t.Fatalf("args = %v", got)
	}
}

func TestDedicatedSpawnAllocatesOnceAndForwardsNativeArgs(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true,
		Args: []string{"--model", "gpt-5"}, Timeout: 30 * time.Second,
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

	mustStartAgent(t, service, "worker")

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
			_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
				Name: "same", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
			})
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

func TestDedicatedSpawnAddsTabToAdoptedWorkspaceWithoutCreatingWorkspace(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w-adopted"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w-adopted", Label: "independent"}}
	fake.mu.Unlock()

	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
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

func TestDedicatedSpawnReusesLabeledPaneInAdoptedWorkspace(t *testing.T) {
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

	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
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

func TestControlOperationsUsePersistedPaneID(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	mustStartAgent(t, service, "worker")
	if _, err := service.ReadAgent(t.Context(), "worker", "recent-unwrapped", 10, false); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.readTarget != "p1" {
		t.Fatalf("read target=%q, want pane ID", fake.readTarget)
	}
}

func TestForceStopRejectsSavedOrchestratorPane(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	mustStartAgent(t, service, "fledge-orchestrator")
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

func TestEnqueueOrchestratorSpawnIncludesManagedProfile(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.OrchestratorPaneID = "p-left"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.EnqueueOrchestratorSpawn(
		t.Context(), serviceSessionSocket(t, service.Binary), "/usr/bin/fledge", "orchestrator",
	); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.sendInputText != "/usr/bin/fledge agent spawn orchestrator --name fledge-orchestrator" {
		t.Fatalf("profile command = %q", fake.sendInputText)
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
	mustStartAgent(t, service, "worker")
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

// seedTakeoverPane gives the fake a single workspace holding one labeled pane,
// the layout an in-pane spawn takes over. It returns the pane's original label
// so callers can assert it was renamed or restored.
func seedTakeoverPane(t *testing.T, service *Service, fake *fakeLifecycle) string {
	t.Helper()
	service.WorkspaceID = "w1"
	label := "orchestrator"
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}
	fake.snapshot.Tabs = []herdr.TabInfo{{TabID: "t1", WorkspaceID: "w1", Label: label}}
	fake.snapshot.Panes = []herdr.PaneInfo{{PaneID: "p1", TabID: "t1", WorkspaceID: "w1", Label: &label}}
	return label
}

func TestInPaneSpawnLaunchesDeliveryHelperForTheActiveRun(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	seedTakeoverPane(t, service, fake)
	var launches int
	var launchedName, launchedActivation string
	var launchedTimeout time.Duration
	service.LaunchDeliveryHelper = func(name, activationID string, timeout time.Duration) error {
		launches++
		launchedName, launchedActivation, launchedTimeout = name, activationID, timeout
		return nil
	}
	service.ExecAgent = func(string, []string, []string) error { return nil }

	if _, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", CurrentPaneID: "p1",
		Executable: "/usr/bin/codex", Timeout: 7 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}

	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if launches != 1 || launchedName != "worker" || launchedTimeout != 7*time.Second ||
		launchedActivation == "" || launchedActivation != st.Agents["worker"].ActivationID {
		t.Fatalf("launches=%d name=%q activation=%q timeout=%s persisted=%#v",
			launches, launchedName, launchedActivation, launchedTimeout, st.Agents["worker"])
	}
}

func TestNewTabSpawnLaunchesDeliveryHelperForTheActiveRun(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	var launches int
	var launchedName, launchedActivation string
	var launchedTimeout time.Duration
	service.LaunchDeliveryHelper = func(name, activationID string, timeout time.Duration) error {
		launches++
		launchedName, launchedActivation, launchedTimeout = name, activationID, timeout
		return nil
	}

	if _, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 7 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}

	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if launches != 1 || launchedName != "worker" || launchedTimeout != 7*time.Second ||
		launchedActivation == "" || launchedActivation != st.Agents["worker"].ActivationID {
		t.Fatalf("launches=%d name=%q activation=%q timeout=%s persisted=%#v",
			launches, launchedName, launchedActivation, launchedTimeout, st.Agents["worker"])
	}
}

func TestInPaneSpawnWithoutAnActiveRunSkipsTheDeliveryHelper(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	seedTakeoverPane(t, service, fake)
	launches := 0
	service.LaunchDeliveryHelper = func(string, string, time.Duration) error {
		launches++
		return nil
	}
	service.ExecAgent = func(string, []string, []string) error { return nil }

	if _, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", CurrentPaneID: "p1",
		Executable: "/usr/bin/codex", Timeout: 7 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}

	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if launches != 0 || st.Agents["worker"].ActivationID != "" {
		t.Fatalf("launches=%d persisted=%#v", launches, st.Agents["worker"])
	}
}

func TestInPaneSpawnExecsFromTheAgentWorkingDirectoryAndRestoresIt(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	seedTakeoverPane(t, service, fake)
	nested := filepath.Join(service.Project.Root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var execCWD string
	service.ExecAgent = func(string, []string, []string) error {
		execCWD, err = os.Getwd()
		return err
	}

	if _, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", CWD: nested, CurrentPaneID: "p1",
		Executable: "/usr/bin/codex", Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}

	want, err := project.Canonical(nested)
	if err != nil {
		t.Fatal(err)
	}
	if execCWD != want {
		t.Fatalf("harness exec cwd = %q, want %q", execCWD, want)
	}
	after, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("working directory was not restored: %q, want %q", after, before)
	}
}

func TestHarnessExecDoesNotInheritNoColor(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "fledge-term")
	t.Setenv("COLORTERM", "fledge-truecolor")
	t.Setenv("FLEDGE_UNRELATED", "unchanged")
	t.Setenv("TMPDIR", "/inherited/tmp")

	var captured []string
	service := Service{
		ExecAgent: func(_ string, _, environ []string) error {
			captured = append([]string(nil), environ...)
			return nil
		},
	}
	service.Project.Root = root
	if err := service.execIntoHarness("/usr/bin/codex", nil, root); err != nil {
		t.Fatal(err)
	}
	environ := make(map[string]string, len(captured))
	for _, entry := range captured {
		name, value, found := strings.Cut(entry, "=")
		if found {
			environ[name] = value
		}
	}
	if _, found := environ["NO_COLOR"]; found {
		t.Fatalf("NO_COLOR was forwarded: %q", environ["NO_COLOR"])
	}
	for name, want := range map[string]string{
		"TERM": "fledge-term", "COLORTERM": "fledge-truecolor",
		"FLEDGE_UNRELATED": "unchanged", "TMPDIR": project.TempDir(root),
	} {
		if got := environ[name]; got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestResolveAgentStatePrefersTheAgentRecordOverThePane(t *testing.T) {
	codex := "codex"
	for _, testCase := range []struct {
		name  string
		panes map[string]herdr.PaneInfo
		live  map[string]herdr.AgentInfo
		want  string
	}{
		{
			name:  "missing pane is unknown",
			panes: map[string]herdr.PaneInfo{},
			want:  StateUnknown,
		},
		{
			name:  "pane without a harness is stopped",
			panes: map[string]herdr.PaneInfo{"p1": {PaneID: "p1", AgentStatus: StateIdle}},
			want:  StateStopped,
		},
		{
			name:  "agent record overrides the pane status",
			panes: map[string]herdr.PaneInfo{"p1": {PaneID: "p1", Agent: &codex, AgentStatus: StateIdle}},
			live:  map[string]herdr.AgentInfo{"p1": {PaneID: "p1", Agent: &codex, AgentStatus: StateWorking}},
			want:  StateWorking,
		},
		{
			name:  "agent record without a harness is stopped",
			panes: map[string]herdr.PaneInfo{"p1": {PaneID: "p1", Agent: &codex, AgentStatus: StateWorking}},
			live:  map[string]herdr.AgentInfo{"p1": {PaneID: "p1", AgentStatus: StateIdle}},
			want:  StateStopped,
		},
		{
			name:  "an empty live status does not override the pane",
			panes: map[string]herdr.PaneInfo{"p1": {PaneID: "p1", Agent: &codex, AgentStatus: StateWorking}},
			live:  map[string]herdr.AgentInfo{"p1": {PaneID: "p1", Agent: &codex}},
			want:  StateWorking,
		},
		{
			// Herdr always reports a status for a live harness, so this
			// pins today's passthrough rather than a chosen fallback.
			name:  "an empty status on a live pane is reported verbatim",
			panes: map[string]herdr.PaneInfo{"p1": {PaneID: "p1", Agent: &codex}},
			live:  map[string]herdr.AgentInfo{"p1": {PaneID: "p1", Agent: &codex}},
			want:  "",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := resolveAgentState(testCase.panes, testCase.live, "p1"); got != testCase.want {
				t.Fatalf("state = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestEvictPaneOwnersRefusesAPaneWithARunningHarness(t *testing.T) {
	codex := "codex"
	st := state.Session{Agents: map[string]state.Agent{
		"other":     {Name: "other", PaneID: "p1"},
		"elsewhere": {Name: "elsewhere", PaneID: "p2"},
	}}

	err := evictPaneOwners(&st, herdr.PaneInfo{PaneID: "p1", Agent: &codex}, "new")
	if translated := Translate(err); translated.Code != "pane_occupied" ||
		translated.Message != `pane p1 is still owned by running agent "other"` {
		t.Fatalf("occupied error = %#v", translated)
	}
	if _, retained := st.Agents["other"]; !retained {
		t.Fatalf("refused eviction still dropped the owner: %#v", st.Agents)
	}

	if err := evictPaneOwners(&st, herdr.PaneInfo{PaneID: "p1"}, "new"); err != nil {
		t.Fatal(err)
	}
	if _, evicted := st.Agents["other"]; evicted {
		t.Fatalf("vacant pane owner was not evicted: %#v", st.Agents)
	}
	if _, retained := st.Agents["elsewhere"]; !retained {
		t.Fatalf("eviction reached another pane: %#v", st.Agents)
	}

	// The incoming name owns the pane it is claiming, so it must survive.
	claimant := state.Session{Agents: map[string]state.Agent{"new": {Name: "new", PaneID: "p1"}}}
	if err := evictPaneOwners(&claimant, herdr.PaneInfo{PaneID: "p1"}, "new"); err != nil {
		t.Fatal(err)
	}
	if _, retained := claimant.Agents["new"]; !retained {
		t.Fatalf("eviction dropped the claiming agent itself: %#v", claimant.Agents)
	}
}

func TestListAgentsRetiresActivationsWhoseHarnessExited(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	mustStartAgent(t, service, "worker")
	sent, err := service.SendMessage(t.Context(), "worker", "hello")
	if err != nil || sent.Message.Status != messaging.StatusAwaitingAck {
		t.Fatalf("sent = %#v, %v", sent, err)
	}
	fake.mu.Lock()
	fake.snapshot.Agents = nil
	fake.snapshot.Panes[0].Agent = nil
	fake.mu.Unlock()

	if _, err := service.ListAgents(t.Context()); err != nil {
		t.Fatal(err)
	}

	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if st.Agents["worker"].ActivationID != "" {
		t.Fatalf("activation was not retired: %#v", st.Agents["worker"])
	}
	message, err := service.ShowMessage(sent.Message.ID)
	if err != nil || message.Status != messaging.StatusFailed {
		t.Fatalf("undelivered message = %#v, %v", message, err)
	}
}

func TestListAgentsPersistsTheReconciledPaneMapping(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	label := agentLabelPrefix + "worker"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}
	fake.snapshot.Panes = []herdr.PaneInfo{{
		PaneID: "new-pane", TabID: "new-tab", WorkspaceID: "w1", Label: &label,
	}}
	fake.mu.Unlock()
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.WorkspaceID = "w1"
		st.Agents["worker"] = state.Agent{Name: "worker", PaneID: "old-pane", TabID: "old-tab"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	agents, err := service.ListAgents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].PaneID != "new-pane" || agents[0].TabID != "new-tab" {
		t.Fatalf("listed agents = %#v", agents)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if st.Agents["worker"].PaneID != "new-pane" || st.Agents["worker"].TabID != "new-tab" {
		t.Fatalf("reconciliation was not persisted: %#v", st.Agents["worker"])
	}
}
