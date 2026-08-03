package fledge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	if st.SpawnSelectionGeneration != 1 {
		t.Fatalf("spawn selection generation = %d, want 1", st.SpawnSelectionGeneration)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.tabRenames != 0 || fake.paneRenames != 1 ||
		fake.snapshot.Tabs[0].Label != "orchestrator" ||
		fake.snapshot.Panes[0].Label == nil || *fake.snapshot.Panes[0].Label != "worker" {
		t.Fatalf("layout = tabs %#v panes %#v", fake.snapshot.Tabs, fake.snapshot.Panes)
	}
}

func TestCurrentPaneDirectExecKeepsControlBytesInNativeArgv(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	seedTakeoverPane(t, service, fake)
	want := "line\nwith\ttty controls"
	var got []string
	service.ExecAgent = func(_ string, argv, _ []string) error {
		got = append([]string(nil), argv...)
		return nil
	}
	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", CurrentPaneID: "p1", Executable: "/usr/bin/codex",
		Timeout: 30 * time.Second, Args: []string{want},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1] != want {
		t.Fatalf("direct exec argv = %#v", got)
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
	if st.SpawnSelectionGeneration != 2 {
		t.Fatalf("rollback generation = %d, want 2", st.SpawnSelectionGeneration)
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

func TestDedicatedChildClaimsOnlyExactReservationAndMarksExecFailure(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	seedTakeoverPane(t, service, fake)
	reserved := state.Agent{
		Name: "worker", Kind: "codex", Placement: "tab", CWD: service.Project.Root,
		TabID: "t1", PaneID: "p1", LaunchID: "launch_exact", LaunchPhase: launchReserved,
	}
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.Agents["worker"] = reserved
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service.ExecAgent = func(string, []string, []string) error { return errors.New("exec failed") }
	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", CurrentPaneID: "p1", Executable: "/usr/bin/codex",
		LaunchID: "launch_exact", Timeout: 30 * time.Second,
	})
	if translated := Translate(err); translated == nil || translated.Code != "agent_exec_failed" {
		t.Fatalf("exec failure = %v", err)
	}
	st, readErr := service.Store.Read(service.Project.Session, service.Project.Root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	managed := st.Agents["worker"]
	if managed.LaunchID != "launch_exact" || managed.LaunchPhase != launchFailed ||
		managed.LaunchPID != os.Getpid() || managed.LaunchExecutable != "/usr/bin/codex" {
		t.Fatalf("failed launch ledger = %#v", managed)
	}
}

func TestDedicatedChildRejectsMismatchedReservationAndOrdinaryTakeover(t *testing.T) {
	for _, test := range []struct {
		name, launchID string
	}{
		{name: "wrong launch id", launchID: "launch_wrong"},
		{name: "ordinary takeover", launchID: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, fake := newFakeLifecycle(t)
			seedTakeoverPane(t, service, fake)
			if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
				st.Agents["worker"] = state.Agent{
					Name: "worker", Kind: "codex", TabID: "t1", PaneID: "p1",
					LaunchID: "launch_exact", LaunchPhase: launchReserved,
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			service.ExecAgent = func(string, []string, []string) error {
				t.Fatal("exec reached after reservation mismatch")
				return nil
			}
			_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
				Name: "worker", Kind: "codex", CurrentPaneID: "p1", Executable: "/usr/bin/codex",
				LaunchID: test.launchID, Timeout: 30 * time.Second,
			})
			if translated := Translate(err); translated == nil ||
				(translated.Code != "agent_launch_unconfirmed" && translated.Code != "agent_spawn_in_progress") {
				t.Fatalf("reservation mismatch error = %v", err)
			}
		})
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
	if managed := st.Agents["worker"]; managed.LaunchID != "" || managed.LaunchPhase != "" || managed.LaunchPID != 0 || managed.LaunchExecutable != "" {
		t.Fatalf("confirmed launch ledger was not finalized: %#v", managed)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.snapshot.Tabs) != 1 || fake.snapshot.Tabs[0].Label != "worker" ||
		len(fake.snapshot.Panes) != 1 || fake.snapshot.Panes[0].Label == nil ||
		*fake.snapshot.Panes[0].Label != "worker" {
		t.Fatalf("raw labels not applied: tabs=%#v panes=%#v", fake.snapshot.Tabs, fake.snapshot.Panes)
	}
}

func TestDedicatedRespawnAlwaysReplacesStoppedPaneAndClearsProfile(t *testing.T) {
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
	if err != nil || adhoc.Agent.Profile != "" || adhoc.Agent.PaneID == first.Agent.PaneID {
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
	if err != nil || profiled.Agent.Profile != "builder" {
		t.Fatalf("profiled respawn = %#v, %v", profiled, err)
	}
	st, err = service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil || st.Agents["worker"].Profile != "builder" {
		t.Fatalf("profiled persisted state = %#v, %v", st.Agents["worker"], err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.tabCloses != 2 || fake.tabCreates != 2 {
		t.Fatalf("fresh respawns closed/created tabs = %d/%d, want 2/2", fake.tabCloses, fake.tabCreates)
	}
}

func TestDedicatedRespawnClosesOnlyManagedPaneInSharedTab(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	workerLabel, otherLabel := "worker", "notes"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}
	fake.snapshot.Tabs = []herdr.TabInfo{{TabID: "t-shared", WorkspaceID: "w1", Label: workerLabel}}
	fake.snapshot.Panes = []herdr.PaneInfo{
		{PaneID: "p-old", TabID: "t-shared", WorkspaceID: "w1", Label: &workerLabel},
		{PaneID: "p-notes", TabID: "t-shared", WorkspaceID: "w1", Label: &otherLabel},
	}
	fake.mu.Unlock()
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.WorkspaceID = "w1"
		st.Agents["worker"] = state.Agent{Name: "worker", Kind: "codex", Placement: "tab", TabID: "t-shared", PaneID: "p-old"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if result.Agent.PaneID == "p-old" || fake.paneCloses != 1 || fake.tabCloses != 0 {
		t.Fatalf("shared-tab replacement = %#v pane closes=%d tab closes=%d", result, fake.paneCloses, fake.tabCloses)
	}
	foundNotes := false
	for _, pane := range fake.snapshot.Panes {
		foundNotes = foundNotes || pane.PaneID == "p-notes"
	}
	if !foundNotes {
		t.Fatal("unmanaged pane in shared tab was closed")
	}
}

func TestDedicatedRespawnPreservesOccupiedStateOwnedPane(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	label := "worker"
	argv0 := "/usr/bin/vim"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}
	fake.snapshot.Tabs = []herdr.TabInfo{{TabID: "t-old", WorkspaceID: "w1", Label: label}}
	fake.snapshot.Panes = []herdr.PaneInfo{{PaneID: "p-old", TabID: "t-old", WorkspaceID: "w1", Label: &label}}
	fake.foregroundByPane = map[string][]herdr.Process{"p-old": {{PID: 42, Name: "vim", Argv0: &argv0, Argv: []string{argv0}}}}
	fake.mu.Unlock()
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.WorkspaceID = "w1"
		st.Agents["worker"] = state.Agent{Name: "worker", Kind: "codex", Placement: "tab", TabID: "t-old", PaneID: "p-old"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second})
	if translated := Translate(err); translated == nil || translated.Code != "pane_occupied" {
		t.Fatalf("occupied pane error = %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.tabCreates != 0 || fake.tabCloses != 0 || fake.paneCloses != 0 {
		t.Fatalf("occupied pane was mutated: creates=%d tab closes=%d pane closes=%d", fake.tabCreates, fake.tabCloses, fake.paneCloses)
	}
}

func TestDedicatedSpawnDropsMissingStatePaneBeforeFreshAllocation(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}
	fake.mu.Unlock()
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.WorkspaceID = "w1"
		st.Agents["worker"] = state.Agent{Name: "worker", Kind: "codex", Placement: "tab", TabID: "t-missing", PaneID: "p-missing"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second})
	if err != nil || result.Agent.PaneID == "p-missing" {
		t.Fatalf("fresh allocation after stale state = %#v, %v", result, err)
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

	fake.failMethod = "pane.send_input"
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
	fake.mu.Unlock()

	if _, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.sendInputCalls != 1 || fake.shellProbesAtSendInput < 3 {
		t.Fatalf("bootstrap was injected before the shell appeared: injections=%d shell probes=%d",
			fake.sendInputCalls, fake.shellProbesAtSendInput)
	}
	if fake.workspaceCreates != 1 || fake.tabCreates != 0 {
		t.Fatalf("boot race duplicated layout: workspace creates=%d tab creates=%d",
			fake.workspaceCreates, fake.tabCreates)
	}
}

func TestDedicatedSpawnInjectsQuotedBootstrapCommandWithoutAgentStart(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.FledgeExecutable = "/opt/Fledge Tools/fledge"
	root, err := project.Canonical(service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "claude", Model: "claude-opus-4-8", NewTab: true,
		Timeout: 30 * time.Second, Args: []string{"--append-system-prompt", "stay focused"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "'/opt/Fledge Tools/fledge' agent spawn --name worker --harness claude" +
		" --model claude-opus-4-8 --cwd " + shellQuote(root) +
		" --timeout 30s --no-pickers --launch-id launch_"
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.sendInputCalls != 1 || fake.sendInputPaneID != result.Agent.PaneID ||
		!strings.HasPrefix(fake.sendInputText, want) ||
		!strings.HasSuffix(fake.sendInputText, " -- --append-system-prompt 'stay focused'") ||
		len(fake.sendInputKeys) != 1 || fake.sendInputKeys[0] != "enter" {
		t.Fatalf("injections=%d pane=%q command=%q, want %q keys=%v",
			fake.sendInputCalls, fake.sendInputPaneID, fake.sendInputText, want, fake.sendInputKeys)
	}
	for _, method := range fake.methodCalls {
		if method == "agent.start" {
			t.Fatal("dedicated-tab spawn still calls agent.start")
		}
	}
}

func TestDedicatedProfileSpawnBootstrapOmitsLockedAndProfileOwnedFlags(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.FledgeExecutable = "/usr/bin/fledge"
	if _, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "reviewer", Kind: "claude", Model: "sonnet", Profile: "review-profile",
		ProfileLocksHarness: true, NewTab: true, Timeout: 30 * time.Second,
		Args: []string{"--append-system-prompt", "profile instructions"},
	}); err != nil {
		t.Fatal(err)
	}
	// The harness flag is locked by the profile; cwd and native args are
	// always profile-owned. The unlocked model still travels.
	want := "/usr/bin/fledge agent spawn review-profile --name reviewer --model sonnet --timeout 30s --no-pickers --launch-id launch_"
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if !strings.HasPrefix(fake.sendInputText, want) {
		t.Fatalf("profile bootstrap = %q, want %q", fake.sendInputText, want)
	}
}

func TestDedicatedSpawnWaitsForTheInjectedCommandToTakeThePane(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	fake.mu.Lock()
	fake.spawnAppearsAfterPolls = 3
	fake.staleAgentCaches = true
	fake.mu.Unlock()

	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Agent.State != StateUnknown {
		t.Fatalf("state after PID-confirmed launch = %q, want %q", result.Agent.State, StateUnknown)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if !fake.childExeced || len(fake.snapshot.Agents) != 0 {
		t.Fatalf("PID launch did not complete independently of stale caches: execed=%v agents=%#v",
			fake.childExeced, fake.snapshot.Agents)
	}
}

func TestStaleHerdrCachesCannotStopPIDConfirmedHarness(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	fake.mu.Lock()
	fake.staleAgentCaches = true
	fake.ignoreExit = true
	fake.mu.Unlock()
	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.StopAgent(t.Context(), "worker", 250*time.Millisecond, false)
	if translated := Translate(err); translated == nil || translated.Code != "agent_stop_timeout" {
		t.Fatalf("stale-cache stop = %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if result.Agent.PaneID == "" || fake.tabCloses != 0 || fake.paneCloses != 0 || len(fake.snapshot.Panes) != 1 {
		t.Fatalf("live PID-confirmed pane was closed: result=%#v panes=%#v", result, fake.snapshot.Panes)
	}
}

func TestStaleHerdrCachesCannotDeactivatePIDConfirmedHarness(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	runID := startTestMessageRun(t, service)
	fake.mu.Lock()
	fake.staleAgentCaches = true
	fake.mu.Unlock()
	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.messageStore().Append(runID, messaging.Event{
		Type: messaging.EventAgentActivated, Agent: "worker",
		ActivationID: "act-live", PaneID: result.Agent.PaneID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		managed := st.Agents["worker"]
		managed.ActivationID = "act-live"
		st.Agents["worker"] = managed
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListAgents(t.Context()); err != nil {
		t.Fatal(err)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil || st.Agents["worker"].ActivationID != "act-live" {
		t.Fatalf("live activation was retired: %#v, %v", st.Agents["worker"], err)
	}
	run, err := service.messageStore().ReadRun(runID)
	if err != nil || run.Activations["act-live"].DeactivatedAt != nil {
		t.Fatalf("live activation log was retired: %#v, %v", run.Activations["act-live"], err)
	}
}

// This is the phantom-success regression test: the injected bootstrap runs
// (a non-shell process is visible in the pane's foreground) but its harness
// never comes up — a profile failure, a missing harness, or a blocked picker.
// The spawn must report failure and roll everything back, not report success
// because "some process appeared".
func TestDedicatedSpawnUnconfirmedLaunchRollsBackTheCreatedTab(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	want := state.SpawnSelection{Harness: "claude", Model: "sonnet"}
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		selection := want
		st.LastSpawnSelection = &selection
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.spawnAppearsAfterPolls = -1
	fake.phantomAgentCache = true
	fake.mu.Unlock()

	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Model: "gpt-5.6", NewTab: true,
		Timeout: 600 * time.Millisecond, RememberSelection: true,
	})
	if translated := Translate(err); translated == nil || translated.Code != "agent_launch_unconfirmed" {
		t.Fatalf("error = %v", err)
	}

	fake.mu.Lock()
	if fake.sendInputCalls != 1 || len(fake.snapshot.Tabs) != 0 || len(fake.snapshot.Panes) != 0 {
		fake.mu.Unlock()
		t.Fatalf("unconfirmed spawn left layout behind: injections=%d tabs=%#v panes=%#v",
			fake.sendInputCalls, fake.snapshot.Tabs, fake.snapshot.Panes)
	}
	fake.mu.Unlock()
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, recorded := st.Agents["worker"]; recorded {
		t.Fatalf("unconfirmed spawn persisted its mapping: %#v", st.Agents)
	}
	if st.LastSpawnSelection == nil || *st.LastSpawnSelection != want {
		t.Fatalf("unconfirmed spawn kept its remembered selection: %#v", st.LastSpawnSelection)
	}
	if st.SpawnSelectionGeneration != 2 {
		t.Fatalf("unconfirmed rollback generation = %d, want 2", st.SpawnSelectionGeneration)
	}
}

func TestDedicatedSpawnPreservesLedgerWhenProcessInspectionFails(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	fake.mu.Lock()
	fake.spawnAppearsAfterPolls = -1
	fake.dropProcessInfoAfter = 2
	fake.mu.Unlock()
	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 600 * time.Millisecond,
	})
	if translated := Translate(err); translated == nil || translated.Code != "agent_launch_unconfirmed" {
		t.Fatalf("inspection failure = %v", err)
	}
	st, readErr := service.Store.Read(service.Project.Session, service.Project.Root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	managed, recorded := st.Agents["worker"]
	if !recorded || managed.LaunchID == "" {
		t.Fatalf("unverifiable launch ledger was removed: %#v", st.Agents)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.snapshot.Panes) != 1 || fake.tabCloses != 0 || fake.paneCloses != 0 {
		t.Fatalf("unverifiable pane was closed: panes=%#v tab closes=%d pane closes=%d", fake.snapshot.Panes, fake.tabCloses, fake.paneCloses)
	}
}

func TestDedicatedSpawnRollsBackWhenTheBootstrapCannotBeSent(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	fake.mu.Lock()
	fake.failMethod = "pane.send_input"
	fake.mu.Unlock()

	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	})
	if err == nil {
		t.Fatal("expected the send_input failure to fail the spawn")
	}

	fake.mu.Lock()
	if len(fake.snapshot.Tabs) != 0 || len(fake.snapshot.Panes) != 0 {
		fake.mu.Unlock()
		t.Fatalf("failed injection left layout behind: tabs=%#v panes=%#v",
			fake.snapshot.Tabs, fake.snapshot.Panes)
	}
	fake.mu.Unlock()
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, recorded := st.Agents["worker"]; recorded {
		t.Fatalf("failed injection persisted its mapping: %#v", st.Agents)
	}
}

func TestDedicatedSpawnPreservesReservationOnBootstrapTransportFailure(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	fake.mu.Lock()
	fake.dropMethod = "pane.send_input"
	fake.mu.Unlock()
	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 600 * time.Millisecond,
	})
	if translated := Translate(err); translated == nil || translated.Code != "agent_launch_unconfirmed" {
		t.Fatalf("transport failure = %v", err)
	}
	st, readErr := service.Store.Read(service.Project.Session, service.Project.Root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if managed, ok := st.Agents["worker"]; !ok || managed.LaunchPhase != launchFailed || managed.LaunchID == "" {
		t.Fatalf("transport failure removed reservation: %#v", st.Agents)
	}
	fake.mu.Lock()
	if len(fake.snapshot.Panes) != 1 || fake.tabCloses != 0 || fake.paneCloses != 0 {
		fake.mu.Unlock()
		t.Fatalf("transport failure closed unverifiable pane: %#v", fake.snapshot.Panes)
	}
	fake.dropMethod = ""
	fake.mu.Unlock()
	recovered, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	})
	if err != nil || recovered.Agent.PaneID == "p1" {
		t.Fatalf("failed reservation was not reconciled into a fresh pane: %#v, %v", recovered, err)
	}
}

func TestDedicatedSpawnRejectsInvalidTerminalInputBeforeTabInputOrStateMutation(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	priorSelection := state.SpawnSelection{Harness: "claude", Model: "sonnet"}
	priorAgent := state.Agent{Name: "prior", Kind: "claude", PaneID: "p-prior", TabID: "t-prior"}
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.LastSpawnSelection = &priorSelection
		st.SpawnSelectionGeneration = 41
		st.Agents["prior"] = priorAgent
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	base := AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
		RememberSelection: true,
	}
	for _, test := range []struct {
		name   string
		unsafe string
		edit   func(*AgentStartOptions)
	}{
		{"name", "worker\nsecond", func(opts *AgentStartOptions) { opts.Name = "worker\nsecond" }},
		{"harness", "codex\tsecond", func(opts *AgentStartOptions) { opts.Kind = "codex\tsecond" }},
		{"model", "model\x7fsecond", func(opts *AgentStartOptions) { opts.Model = "model\x7fsecond" }},
		{"cwd", "dir\rsecond", func(opts *AgentStartOptions) { opts.CWD = "dir\rsecond" }},
		{"native arg", "native\x03argument", func(opts *AgentStartOptions) { opts.Args = []string{"native\x03argument"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			opts := base
			test.edit(&opts)
			_, err := service.SpawnAgent(t.Context(), opts)
			if translated := Translate(err); translated == nil || translated.Code != "invalid_terminal_input" {
				t.Fatalf("error = %v", err)
			}
			if strings.Contains(err.Error(), test.unsafe) {
				t.Fatalf("unsafe input was echoed: %q", err)
			}
		})
	}
	service.FledgeExecutable = "fledge\nsecond"
	if _, err := service.SpawnAgent(t.Context(), base); Translate(err) == nil || Translate(err).Code != "invalid_terminal_input" {
		t.Fatalf("invalid bootstrap executable error = %v", err)
	}
	fake.mu.Lock()
	tabCreates, sendInputCalls := fake.tabCreates, fake.sendInputCalls
	fake.mu.Unlock()
	if tabCreates != 0 || sendInputCalls != 0 {
		t.Fatalf("invalid input caused terminal mutation: tab creates=%d sends=%d", tabCreates, sendInputCalls)
	}
	st, readErr := service.Store.Read(service.Project.Session, service.Project.Root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if st.SpawnSelectionGeneration != 41 || st.LastSpawnSelection == nil ||
		*st.LastSpawnSelection != priorSelection || len(st.Agents) != 1 || st.Agents["prior"] != priorAgent {
		t.Fatalf("invalid input changed state: %#v", st)
	}
}

func TestSpawnSelectionRollbackIsOwnedByMonotonicGeneration(t *testing.T) {
	for _, test := range []struct {
		name   string
		first  state.SpawnSelection
		second state.SpawnSelection
	}{
		{
			name:   "distinct selections",
			first:  state.SpawnSelection{Harness: "codex", Model: "gpt-a"},
			second: state.SpawnSelection{Harness: "claude", Model: "sonnet"},
		},
		{
			name:   "identical selections",
			first:  state.SpawnSelection{Harness: "codex", Model: "same"},
			second: state.SpawnSelection{Harness: "codex", Model: "same"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			prior := &state.SpawnSelection{Harness: "pi", Model: "prior"}
			st := state.Session{LastSpawnSelection: cloneSpawnSelection(prior), SpawnSelectionGeneration: 7}
			firstGeneration, err := rememberSpawnSelection(&st, AgentStartOptions{
				Kind: test.first.Harness, Model: test.first.Model, RememberSelection: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = rememberSpawnSelection(&st, AgentStartOptions{
				Kind: test.second.Harness, Model: test.second.Model, RememberSelection: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := restoreSpawnSelectionIfOwned(&st, firstGeneration, prior); err != nil {
				t.Fatal(err)
			}
			if st.SpawnSelectionGeneration != 9 || st.LastSpawnSelection == nil || *st.LastSpawnSelection != test.second {
				t.Fatalf("stale rollback changed newer selection: %#v", st)
			}
		})
	}
}

// The in-pane child of a dedicated-tab spawn claims a pane the parent already
// labelled with the child's own name and recorded provisionally; the claim
// must succeed and keep the dedicated-tab placement.
func TestInPaneClaimOfItsOwnPreparedTabKeepsDedicatedPlacement(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w1"
	label := "worker"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}
	fake.snapshot.Tabs = []herdr.TabInfo{{TabID: "t-agent", WorkspaceID: "w1", Label: label}}
	fake.snapshot.Panes = []herdr.PaneInfo{{PaneID: "p-agent", TabID: "t-agent", WorkspaceID: "w1", Label: &label}}
	fake.mu.Unlock()
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.WorkspaceID = "w1"
		st.Agents["worker"] = state.Agent{
			Name: "worker", Kind: "codex", Placement: "tab",
			CWD: service.Project.Root, TabID: "t-agent", PaneID: "p-agent",
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service.ExecAgent = func(string, []string, []string) error { return nil }

	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", CurrentPaneID: "p-agent",
		Executable: "/usr/bin/codex", Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Agent.Placement != "tab" || result.Agent.PaneID != "p-agent" {
		t.Fatalf("claim result = %#v", result.Agent)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if st.Agents["worker"].Placement != "tab" || st.Agents["worker"].PaneID != "p-agent" ||
		st.Agents["worker"].TabID != "t-agent" {
		t.Fatalf("claimed state = %#v", st.Agents["worker"])
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
	if fake.sendInputCalls != 0 || len(fake.snapshot.Tabs) != 0 || len(fake.snapshot.Panes) != 0 {
		t.Fatalf("failed spawn left layout behind: injections=%d tabs=%#v panes=%#v",
			fake.sendInputCalls, fake.snapshot.Tabs, fake.snapshot.Panes)
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
		!strings.Contains(translated.Message, "t1") {
		t.Fatalf("error = %#v", translated)
	}
}

func TestDedicatedSpawnClosesCreatedTabWhenRenameFails(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	fake.mu.Lock()
	fake.failMethod = "tab.rename"
	fake.mu.Unlock()

	_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	})
	if err == nil {
		t.Fatal("expected the tab.rename failure to fail the spawn")
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.sendInputCalls != 0 || len(fake.snapshot.Tabs) != 0 || len(fake.snapshot.Panes) != 0 {
		t.Fatalf("rename failure left layout behind: injections=%d tabs=%#v panes=%#v",
			fake.sendInputCalls, fake.snapshot.Tabs, fake.snapshot.Panes)
	}
}

func TestDedicatedSpawnCancellationStillClosesCreatedTab(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	fake.mu.Lock()
	fake.bootingProcessInfoCalls = 1 << 30
	fake.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Cancel once the shell poll has issued its first probe, i.e. mid-wait.
	go func() {
		for {
			fake.mu.Lock()
			probes := fake.processInfoCalls
			fake.mu.Unlock()
			if probes >= 1 {
				cancel()
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	_, err := service.SpawnAgent(ctx, AgentStartOptions{
		Name: "worker", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	})
	translated := Translate(err)
	if translated == nil || translated.Code != "agent_pane_unready" ||
		!strings.Contains(translated.Message, "cancelled") {
		t.Fatalf("error = %#v", translated)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.sendInputCalls != 0 || len(fake.snapshot.Tabs) != 0 || len(fake.snapshot.Panes) != 0 {
		t.Fatalf("cancelled spawn left layout behind: injections=%d tabs=%#v panes=%#v",
			fake.sendInputCalls, fake.snapshot.Tabs, fake.snapshot.Panes)
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

func TestLegacyPrefixedLabelWithoutStateIsNotAdoptedOrDestroyed(t *testing.T) {
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
	if result.Agent.PaneID == "legacy-pane" {
		t.Fatalf("unowned legacy pane was reused: %#v", result)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.tabCreates != 1 || len(fake.snapshot.Panes) != 2 || fake.snapshot.Panes[0].PaneID != "legacy-pane" {
		t.Fatalf("unowned legacy pane was mutated: tabs=%#v panes=%#v", fake.snapshot.Tabs, fake.snapshot.Panes)
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
	if fake.sendInputCalls != 1 || !strings.HasSuffix(fake.sendInputText, " -- --model gpt-5") {
		t.Fatalf("injections=%d command=%q", fake.sendInputCalls, fake.sendInputText)
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
	claimEntered, releaseClaim := make(chan struct{}), make(chan struct{})
	fake.mu.Lock()
	originalClaim := fake.childClaim
	fake.childClaim = func(paneID string) string {
		close(claimEntered)
		<-releaseClaim
		return originalClaim(paneID)
	}
	fake.mu.Unlock()
	firstErr := make(chan error, 1)
	go func() {
		_, err := service.SpawnAgent(t.Context(), AgentStartOptions{
			Name: "same", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
		})
		firstErr <- err
	}()
	<-claimEntered
	_, duplicateErr := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: "same", Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	})
	close(releaseClaim)
	if err := <-firstErr; err != nil {
		t.Fatalf("reservation owner failed: %v", err)
	}
	if translated := Translate(duplicateErr); translated == nil || translated.Code != "agent_spawn_in_progress" {
		t.Fatalf("duplicate error = %v", duplicateErr)
	}
	fake.mu.Lock()
	injections := fake.sendInputCalls
	fake.mu.Unlock()
	if injections != 1 {
		t.Fatalf("bootstrap injections=%d, want 1", injections)
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

func TestDedicatedSpawnDoesNotAdoptHumanLabeledPane(t *testing.T) {
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
	if fake.workspaceCreates != 0 || fake.tabCreates != 1 {
		t.Fatalf("fresh allocation counts: workspace creates=%d tab creates=%d",
			fake.workspaceCreates, fake.tabCreates)
	}
	if result.Agent.PaneID == "p-existing" || result.Agent.TabID == "t-existing" || len(fake.snapshot.Panes) != 2 {
		t.Fatalf("human-labeled pane was adopted or destroyed: %#v panes=%#v", result, fake.snapshot.Panes)
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

func TestEnqueueOrchestratorSpawnRejectsInvalidInputBeforePaneOrStateMutation(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.OrchestratorPaneID = "p-left"
		st.SpawnSelectionGeneration = 12
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	unsafe := "orchestrator\rsecond-command"
	err := service.EnqueueOrchestratorSpawn(
		t.Context(), serviceSessionSocket(t, service.Binary), "/usr/bin/fledge", unsafe,
	)
	if translated := Translate(err); translated == nil || translated.Code != "invalid_terminal_input" {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), unsafe) {
		t.Fatalf("unsafe input was echoed: %q", err)
	}
	fake.mu.Lock()
	sends := fake.sendInputCalls
	fake.mu.Unlock()
	if sends != 0 {
		t.Fatalf("invalid input caused %d pane sends", sends)
	}
	st, readErr := service.Store.Read(service.Project.Session, service.Project.Root)
	if readErr != nil || st.OrchestratorPaneID != "p-left" || st.SpawnSelectionGeneration != 12 {
		t.Fatalf("invalid input changed state: %#v, %v", st, readErr)
	}
}

func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	if got := shellQuote("/opt/user's tools/fledge"); got != "'/opt/user'\"'\"'s tools/fledge'" {
		t.Fatalf("quoted path = %q", got)
	}
}

// The unquoted-safe set is a conservative allowlist: '#' would comment out
// the rest of the injected command and '~' would expand in the shell.
func TestShellQuoteQuotesBeyondClassicMetacharacters(t *testing.T) {
	for _, test := range []struct{ in, want string }{
		{"#tag", "'#tag'"},
		{"~", "'~'"},
		{"雪だるま☃", "'雪だるま☃'"},
		{"safe-value_1.2:,@%+=/x", "safe-value_1.2:,@%+=/x"},
	} {
		if got := shellQuote(test.in); got != test.want {
			t.Fatalf("shellQuote(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func TestRenderPTYCommandRejectsInvalidUTF8AndAllTerminalControls(t *testing.T) {
	inputs := []string{string([]byte{0xff})}
	for value := 0; value <= 0x1f; value++ {
		inputs = append(inputs, string(rune(value)))
	}
	for value := 0x7f; value <= 0x9f; value++ {
		inputs = append(inputs, string(rune(value)))
	}
	for _, input := range inputs {
		_, err := renderPTYCommand([]string{"fledge", input})
		translated := Translate(err)
		if translated == nil || translated.Code != "invalid_terminal_input" {
			t.Fatalf("input bytes %x: error = %v", []byte(input), err)
		}
		if strings.Contains(err.Error(), input) {
			t.Fatalf("input bytes %x were echoed in error %q", []byte(input), err)
		}
	}
}

func TestRenderPTYCommandRoundTripsPrintableUnicodeAndShellMetacharacters(t *testing.T) {
	value := "雪だるま 'quoted' $HOME; #literal ~ & | < > (ok)"
	command, err := renderPTYCommand([]string{"printf", "%s", value})
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("execute %q: %v", command, err)
	}
	if string(output) != value {
		t.Fatalf("round trip = %q, want %q (command %q)", output, value, command)
	}
}

// Shell metacharacters reach the bootstrap through model, cwd, and native
// args; each must arrive quoted so it cannot expand or split the command.
func TestSpawnCommandQuotesHostileSelectionValues(t *testing.T) {
	command, err := spawnCommand("/usr/bin/fledge", AgentStartOptions{
		Name: "worker", Kind: "codex", Model: "#5", Timeout: 30 * time.Second,
		Args: []string{"--prompt", "~home; echo nope"}, LaunchID: "launch_safe-123",
	}, "/tmp/agent cwd")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		" --model '#5' ",
		" --cwd '/tmp/agent cwd' ",
		" --launch-id launch_safe-123 ",
		" -- --prompt '~home; echo nope'",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("command %q does not contain %q", command, want)
		}
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
	if paneHasNonShellForegroundProcess(herdr.ProcessInfo{
		ShellPID: &shell, ForegroundProcesses: []herdr.Process{{PID: 20, Name: "bash"}},
	}) {
		t.Fatal("idle shell was treated as a non-shell foreground process")
	}
	if !paneHasNonShellForegroundProcess(herdr.ProcessInfo{
		ShellPID: &shell, ForegroundProcesses: []herdr.Process{{PID: 21, Name: "vim"}},
	}) {
		t.Fatal("non-shell foreground process was not detected")
	}
}

func TestLaunchHarnessProcessRecognizesInterpreterBackedHarness(t *testing.T) {
	node := "/usr/bin/node"
	managed := state.Agent{LaunchPID: 42, LaunchExecutable: "/usr/bin/pi"}
	info := herdr.ProcessInfo{ForegroundProcesses: []herdr.Process{{
		PID: 42, Name: "node", Argv0: &node, Argv: []string{"node", "/usr/bin/pi", "--mode", "interactive"},
	}}}
	if launchHarnessProcess(managed, info) == nil {
		t.Fatal("exact PID exec through an interpreter was not confirmed")
	}
	info.ForegroundProcesses[0].PID = 43
	if launchHarnessProcess(managed, info) != nil {
		t.Fatal("script argv from the wrong PID confirmed a launch")
	}
}

func TestReconcileMappingsDoesNotClaimPaneByLabel(t *testing.T) {
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
	if st.WorkspaceID != "new-workspace" || st.Agents["worker"].PaneID != "old-pane" || st.Agents["worker"].TabID != "old-tab" {
		t.Fatalf("label changed durable pane ownership: %#v", st)
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

// The parent's dedicated-tab spawn only prepares the tab and injects the
// bootstrap command; messaging activation and the delivery helper belong to
// the in-pane child that command starts.
func TestNewTabSpawnDefersMessagingActivationToTheInPaneChild(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	launches := 0
	service.LaunchDeliveryHelper = func(string, string, time.Duration) error {
		launches++
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
	if launches != 0 || st.Agents["worker"].ActivationID != "" {
		t.Fatalf("parent activated messaging itself: launches=%d persisted=%#v", launches, st.Agents["worker"])
	}
}

func TestNewTabSpawnChildClaimLaunchesDeliveryHelperForTheActiveRun(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	var launches int
	var launchedName, launchedActivation string
	service.LaunchDeliveryHelper = func(name, activationID string, _ time.Duration) error {
		launches++
		launchedName, launchedActivation = name, activationID
		return nil
	}

	mustStartAgent(t, service, "worker")

	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if launches != 1 || launchedName != "worker" ||
		launchedActivation == "" || launchedActivation != st.Agents["worker"].ActivationID {
		t.Fatalf("launches=%d name=%q activation=%q persisted=%#v",
			launches, launchedName, launchedActivation, st.Agents["worker"])
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
	fake.exitAgent("p1")
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

func TestListAgentsDoesNotReconcileOwnershipFromLabel(t *testing.T) {
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
	if len(agents) != 1 || agents[0].PaneID != "old-pane" || agents[0].TabID != "old-tab" {
		t.Fatalf("listed agents = %#v", agents)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if st.Agents["worker"].PaneID != "old-pane" || st.Agents["worker"].TabID != "old-tab" {
		t.Fatalf("label changed durable ownership: %#v", st.Agents["worker"])
	}
}
