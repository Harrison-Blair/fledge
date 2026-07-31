package fledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

func TestEnsureAttachmentWorkspaceCreatesOrchestratorLayout(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.Project.Root = filepath.Join(t.TempDir(), "My Project")
	if err := os.Mkdir(service.Project.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(service.Project.Root, "src", "component")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := service.EnsureAttachmentWorkspace(t.Context(), serviceSessionSocket(t, service.Binary), nested); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.workspaceCreates != 1 || fake.workspaceCWD != nested {
		t.Fatalf("workspace creates=%d cwd=%q, want %q", fake.workspaceCreates, fake.workspaceCWD, nested)
	}
	if fake.tabCreates != 0 {
		t.Fatalf("fresh workspace bootstrap tab was not reused: tab creates=%d", fake.tabCreates)
	}
	if len(fake.snapshot.Workspaces) != 1 || fake.snapshot.Workspaces[0].Label != "My Project" {
		t.Fatalf("workspace label = %#v, want %q", fake.snapshot.Workspaces, "My Project")
	}
	if len(fake.snapshot.Tabs) != 1 || fake.snapshot.Tabs[0].Label != orchestratorLabel {
		t.Fatalf("orchestrator tab was not named: %#v", fake.snapshot.Tabs)
	}
	if len(fake.snapshot.Panes) != 2 || fake.snapshot.Panes[0].Label == nil ||
		*fake.snapshot.Panes[0].Label != orchestratorLabel {
		t.Fatalf("orchestrator panes were not initialized: %#v", fake.snapshot.Panes)
	}
	if fake.snapshot.Panes[1].Label != nil {
		t.Fatalf("right pane was unexpectedly labeled: %#v", fake.snapshot.Panes[1])
	}
	for _, pane := range fake.snapshot.Panes {
		if pane.CWD == nil || *pane.CWD != nested {
			t.Fatalf("pane cwd = %#v, want %q", pane.CWD, nested)
		}
	}
	if fake.paneSplits != 1 || fake.splitParams["direction"] != "right" ||
		fake.splitParams["ratio"] != 0.5 || fake.splitParams["focus"] != false {
		t.Fatalf("unexpected split: calls=%d params=%#v", fake.paneSplits, fake.splitParams)
	}
	if len(fake.focusedPaneIDs) != 1 || fake.focusedPaneIDs[0] != "p1" {
		t.Fatalf("focused panes = %v, want primary pane p1", fake.focusedPaneIDs)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if !st.OrchestratorInitialized || st.OrchestratorTabID != "t1" || st.OrchestratorPaneID != "p1" {
		t.Fatalf("orchestrator state = %#v", st)
	}
}

func TestEnsureAttachmentWorkspaceSplitsFreshWorkspaceDespiteStaleInitialization(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.WorkspaceID = "stale-workspace"
		st.OrchestratorTabID = "stale-tab"
		st.OrchestratorPaneID = "stale-pane"
		st.OrchestratorInitialized = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if st, err := service.Store.Read(service.Project.Session, service.Project.Root); err != nil {
		t.Fatal(err)
	} else if !st.OrchestratorInitialized {
		t.Fatalf("stale initialization was not persisted: %#v", st)
	}

	if err := service.EnsureAttachmentWorkspace(
		t.Context(), serviceSessionSocket(t, service.Binary), service.Project.Root,
	); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.workspaceCreates != 1 || fake.paneSplits != 1 {
		t.Fatalf("workspace creates=%d pane splits=%d", fake.workspaceCreates, fake.paneSplits)
	}
	if len(fake.snapshot.Panes) != 2 {
		t.Fatalf("fresh workspace panes = %#v, want split layout", fake.snapshot.Panes)
	}
}

func TestEnsureAttachmentWorkspaceAdoptsFirstTab(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w-adopted"
	workspaceLabel := "User's custom workspace"
	cwd := service.Project.Root
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w-adopted", Label: workspaceLabel}}
	fake.snapshot.Tabs = []herdr.TabInfo{{TabID: "existing-tab", WorkspaceID: "w-adopted", Label: "bootstrap"}}
	fake.snapshot.Panes = []herdr.PaneInfo{{
		PaneID: "existing-pane", TabID: "existing-tab", WorkspaceID: "w-adopted", CWD: &cwd,
	}}
	fake.mu.Unlock()

	if err := service.EnsureAttachmentWorkspace(t.Context(), serviceSessionSocket(t, service.Binary), cwd); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.workspaceCreates != 0 || fake.tabCreates != 0 || fake.paneSplits != 1 {
		t.Fatalf("creates: workspace=%d tab=%d split=%d",
			fake.workspaceCreates, fake.tabCreates, fake.paneSplits)
	}
	if len(fake.snapshot.Tabs) != 1 || fake.snapshot.Tabs[0].TabID != "existing-tab" ||
		fake.snapshot.Tabs[0].Label != orchestratorLabel {
		t.Fatalf("first tab was not adopted: %#v", fake.snapshot.Tabs)
	}
	if fake.snapshot.Workspaces[0].Label != workspaceLabel {
		t.Fatalf("custom workspace label was altered: %#v", fake.snapshot.Workspaces)
	}
}

func TestEnsureAttachmentWorkspaceRecoversCurrentAndLegacyLabels(t *testing.T) {
	for _, test := range []struct {
		name  string
		label func(*Service) string
	}{
		{name: "project folder", label: func(service *Service) string {
			return project.WorkspaceLabel(service.Project.Root)
		}},
		{name: "legacy generated", label: func(service *Service) string {
			return legacyWorkspaceLabelPrefix + service.Project.Session
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, fake := newFakeLifecycle(t)
			label := test.label(service)
			cwd := service.Project.Root
			fake.mu.Lock()
			fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w-recovered", Label: label}}
			fake.snapshot.Tabs = []herdr.TabInfo{{TabID: "bootstrap-tab", WorkspaceID: "w-recovered"}}
			fake.snapshot.Panes = []herdr.PaneInfo{{
				PaneID: "bootstrap-pane", TabID: "bootstrap-tab", WorkspaceID: "w-recovered", CWD: &cwd,
			}}
			fake.mu.Unlock()

			if err := service.EnsureAttachmentWorkspace(
				t.Context(), serviceSessionSocket(t, service.Binary), service.Project.Root,
			); err != nil {
				t.Fatal(err)
			}

			fake.mu.Lock()
			defer fake.mu.Unlock()
			if fake.workspaceCreates != 0 || fake.tabCreates != 0 {
				t.Fatalf("workspace creates=%d tab creates=%d", fake.workspaceCreates, fake.tabCreates)
			}
			if len(fake.snapshot.Workspaces) != 1 || fake.snapshot.Workspaces[0].WorkspaceID != "w-recovered" {
				t.Fatalf("workspace was not recovered: %#v", fake.snapshot.Workspaces)
			}
			if len(fake.snapshot.Tabs) != 1 || fake.snapshot.Tabs[0].Label != orchestratorLabel {
				t.Fatalf("recovered bootstrap tab was not reused: %#v", fake.snapshot.Tabs)
			}
		})
	}
}

func TestWorkspaceLabelRecoveryPrefersProjectFolder(t *testing.T) {
	root := filepath.Join("/source", "My Project")
	snapshot := herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{
		{WorkspaceID: "legacy", Label: legacyWorkspaceLabelPrefix + "test-session"},
		{WorkspaceID: "current", Label: "My Project"},
	}}
	workspace, found := fallbackWorkspace(snapshot, root, "test-session")
	if !found || workspace.WorkspaceID != "current" {
		t.Fatalf("fallbackWorkspace() = %#v, %t", workspace, found)
	}
}

func TestFallbackWorkspaceAdoptsSoleCustomWorkspace(t *testing.T) {
	snapshot := herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{
		WorkspaceID: "custom", Label: "User's workspace",
	}}}
	workspace, found := fallbackWorkspace(snapshot, "/source/project", "test-session")
	if !found || workspace.WorkspaceID != "custom" || workspace.Label != "User's workspace" {
		t.Fatalf("fallbackWorkspace() = %#v, %t", workspace, found)
	}
}

func TestEnsureAttachmentWorkspaceReusesAndPreservesInitializedLayout(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w-adopted"
	cwd := service.Project.Root
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w-adopted"}}
	fake.mu.Unlock()
	socket := serviceSessionSocket(t, service.Binary)
	if err := service.EnsureAttachmentWorkspace(t.Context(), socket, cwd); err != nil {
		t.Fatal(err)
	}

	extraCWD := t.TempDir()
	fake.mu.Lock()
	fake.snapshot.Tabs[0].Label = "user-renamed-tab"
	fake.snapshot.Panes[0].Label = nil
	fake.snapshot.Panes = append(fake.snapshot.Panes, herdr.PaneInfo{
		PaneID: "p-extra", TabID: "t-new", WorkspaceID: "w-adopted", CWD: &extraCWD,
	})
	creates, splits, tabRenames, paneRenames := fake.tabCreates, fake.paneSplits, fake.tabRenames, fake.paneRenames
	fake.mu.Unlock()

	if err := service.EnsureAttachmentWorkspace(t.Context(), socket, cwd); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.tabCreates != creates || fake.paneSplits != splits ||
		fake.tabRenames != tabRenames || fake.paneRenames != paneRenames {
		t.Fatalf("repeated setup changed layout: tabs=%d splits=%d tab renames=%d pane renames=%d",
			fake.tabCreates, fake.paneSplits, fake.tabRenames, fake.paneRenames)
	}
	if len(fake.snapshot.Panes) != 3 || fake.snapshot.Tabs[0].Label != "user-renamed-tab" {
		t.Fatalf("user layout edits were not preserved: tabs=%#v panes=%#v",
			fake.snapshot.Tabs, fake.snapshot.Panes)
	}
	if got := fake.focusedPaneIDs[len(fake.focusedPaneIDs)-1]; got != "p-new" {
		t.Fatalf("focused pane = %s, want persisted primary p-new", got)
	}
}

func TestEnsureAttachmentWorkspaceAdoptsRemainingPaneWithoutSplitting(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w-adopted"
	socket := serviceSessionSocket(t, service.Binary)
	if err := service.EnsureAttachmentWorkspace(t.Context(), socket, service.Project.Root); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.snapshot.Panes = append([]herdr.PaneInfo(nil), fake.snapshot.Panes[1:]...)
	splits := fake.paneSplits
	fake.mu.Unlock()

	if err := service.EnsureAttachmentWorkspace(t.Context(), socket, service.Project.Root); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	if fake.paneSplits != splits {
		t.Fatalf("closed primary caused another split: %d -> %d", splits, fake.paneSplits)
	}
	fake.mu.Unlock()
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if st.OrchestratorPaneID != "p-right" {
		t.Fatalf("remaining pane was not adopted: %#v", st)
	}
}

func TestEnsureAttachmentWorkspaceRecreatesClosedTab(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w-adopted"
	socket := serviceSessionSocket(t, service.Binary)
	if err := service.EnsureAttachmentWorkspace(t.Context(), socket, service.Project.Root); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	cwd := service.Project.Root
	fake.snapshot.Tabs = []herdr.TabInfo{{
		TabID: "unrelated-tab", WorkspaceID: "w-adopted", Label: "user-work",
	}}
	fake.snapshot.Panes = []herdr.PaneInfo{{
		PaneID: "unrelated-pane", TabID: "unrelated-tab", WorkspaceID: "w-adopted", CWD: &cwd,
	}}
	fake.mu.Unlock()

	if err := service.EnsureAttachmentWorkspace(t.Context(), socket, service.Project.Root); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.tabCreates != 1 || fake.paneSplits != 2 {
		t.Fatalf("closed tab was not recreated: tab creates=%d splits=%d",
			fake.tabCreates, fake.paneSplits)
	}
	if len(fake.snapshot.Tabs) != 2 || fake.snapshot.Tabs[0].Label != "user-work" ||
		fake.snapshot.Tabs[1].Label != orchestratorLabel {
		t.Fatalf("unrelated tab was repurposed: %#v", fake.snapshot.Tabs)
	}
}

func TestEnsureAttachmentWorkspaceDoesNotTrustTabFromAnotherWorkspace(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "current-workspace"
	cwd := service.Project.Root
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.WorkspaceID = "old-workspace"
		st.OrchestratorTabID = "current-first-tab"
		st.OrchestratorPaneID = "stale-second-pane"
		st.OrchestratorInitialized = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "current-workspace"}}
	fake.snapshot.Tabs = []herdr.TabInfo{{
		TabID: "current-first-tab", WorkspaceID: "current-workspace", Label: "bootstrap",
	}}
	fake.snapshot.Panes = []herdr.PaneInfo{
		{
			PaneID: "current-first-pane", TabID: "current-first-tab", WorkspaceID: "current-workspace", CWD: &cwd,
		},
		{
			PaneID: "stale-second-pane", TabID: "current-first-tab", WorkspaceID: "current-workspace", CWD: &cwd,
		},
	}
	fake.mu.Unlock()

	if err := service.EnsureAttachmentWorkspace(t.Context(), serviceSessionSocket(t, service.Binary), cwd); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.tabCreates != 0 || fake.paneSplits != 1 ||
		fake.snapshot.Tabs[0].Label != orchestratorLabel {
		t.Fatalf("stale initialization was trusted: tabs=%#v creates=%d splits=%d",
			fake.snapshot.Tabs, fake.tabCreates, fake.paneSplits)
	}
	if fake.snapshot.Panes[0].Label == nil || *fake.snapshot.Panes[0].Label != orchestratorLabel ||
		fake.snapshot.Panes[1].Label != nil {
		t.Fatalf("stale pane mapping was trusted: %#v", fake.snapshot.Panes)
	}
}

func TestEnsureAttachmentWorkspaceRecoversSchemaV1Labels(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w-adopted"
	label := orchestratorLabel
	cwd := service.Project.Root
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w-adopted"}}
	fake.snapshot.Tabs = []herdr.TabInfo{{TabID: "restored-tab", WorkspaceID: "w-adopted", Label: label}}
	fake.snapshot.Panes = []herdr.PaneInfo{
		{PaneID: "restored-left", TabID: "restored-tab", WorkspaceID: "w-adopted", Label: &label, CWD: &cwd},
		{PaneID: "restored-right", TabID: "restored-tab", WorkspaceID: "w-adopted", CWD: &cwd},
	}
	fake.mu.Unlock()

	if err := service.EnsureAttachmentWorkspace(t.Context(), serviceSessionSocket(t, service.Binary), cwd); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	if fake.tabCreates != 0 || fake.paneSplits != 0 ||
		fake.focusedPaneIDs[len(fake.focusedPaneIDs)-1] != "restored-left" {
		t.Fatalf("legacy labels were not recovered: creates=%d splits=%d focuses=%v",
			fake.tabCreates, fake.paneSplits, fake.focusedPaneIDs)
	}
	fake.mu.Unlock()
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if !st.OrchestratorInitialized || st.OrchestratorTabID != "restored-tab" ||
		st.OrchestratorPaneID != "restored-left" {
		t.Fatalf("recovered state = %#v", st)
	}
}

func TestEnsureAttachmentWorkspaceSetupFailuresAreActionable(t *testing.T) {
	for _, method := range []string{"tab.rename", "pane.rename", "pane.split", "pane.focus"} {
		t.Run(method, func(t *testing.T) {
			service, fake := newFakeLifecycle(t)
			service.WorkspaceID = "w-adopted"
			fake.mu.Lock()
			fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w-adopted"}}
			fake.failMethod = method
			fake.mu.Unlock()
			err := service.EnsureAttachmentWorkspace(
				t.Context(), serviceSessionSocket(t, service.Binary), service.Project.Root,
			)
			if Translate(err).Code != "session_setup_failed" || !strings.Contains(err.Error(), method) {
				t.Fatalf("error = %v", err)
			}
			st, readErr := service.Store.Read(service.Project.Session, service.Project.Root)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if st.OrchestratorInitialized {
				t.Fatalf("failed setup was persisted: %#v", st)
			}
		})
	}
}

func TestEnsureAttachmentWorkspacePersistenceFailureIsActionable(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	service.WorkspaceID = "w-adopted"
	fake.mu.Lock()
	fake.snapshot.Workspaces = []herdr.WorkspaceInfo{{WorkspaceID: "w-adopted"}}
	fake.mu.Unlock()
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root,
		func(*state.Session) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(service.Store.Root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(service.Store.Root, 0o700) })

	err := service.EnsureAttachmentWorkspace(
		t.Context(), serviceSessionSocket(t, service.Binary), service.Project.Root,
	)
	if Translate(err).Code != "state_persist_failed" ||
		!strings.Contains(err.Error(), "orchestrator layout was prepared") {
		t.Fatalf("error = %v", err)
	}
	if err := os.Chmod(service.Store.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	st, readErr := service.Store.Read(service.Project.Session, service.Project.Root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if st.OrchestratorInitialized {
		t.Fatalf("failed state write unexpectedly persisted: %#v", st)
	}
}
