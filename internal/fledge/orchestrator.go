package fledge

import (
	"context"
	"errors"
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

// EnqueueOrchestratorSpawn submits a spawn command to the saved primary
// orchestrator pane. An empty profile opens the ad-hoc picker; a nonempty
// profile launches through that profile's unset-field pickers.
func (s *Service) EnqueueOrchestratorSpawn(ctx context.Context, socket, executable, profile string) error {
	argv := []string{executable, "agent", "spawn"}
	if profile != "" {
		argv = append(argv, profile)
	}
	argv = append(argv, "--name", "fledge-orchestrator")
	command, err := renderPTYCommand(argv)
	if err != nil {
		return err
	}
	st, err := s.Store.Read(s.Project.Session, s.Project.Root)
	if err != nil {
		return err
	}
	if st.OrchestratorPaneID == "" {
		return errors.New("orchestrator pane is not initialized")
	}
	return (&herdr.Client{Socket: socket}).Call(ctx, "pane.send_input", map[string]any{
		"pane_id": st.OrchestratorPaneID,
		"text":    command,
		"keys":    []string{"enter"},
	}, nil)
}

// EnqueueOrchestratorPicker preserves the ad-hoc picker entry point for
// callers that do not select a managed profile.
func (s *Service) EnqueueOrchestratorPicker(ctx context.Context, socket, executable string) error {
	return s.EnqueueOrchestratorSpawn(ctx, socket, executable, "")
}

// EnsureAttachmentWorkspace prepares and focuses the session's dedicated
// orchestrator tab before either an interactive or detached start completes.
// Once initialized, the user's pane layout is left intact on later starts.
func (s *Service) EnsureAttachmentWorkspace(ctx context.Context, socket, cwd string) error {
	client := &herdr.Client{Socket: socket}
	layout := &orchestratorLayout{project: s.Project, client: client, preferred: s.WorkspaceID}
	var setupErr error
	var workspaceID string
	err := s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		snapshot, err := client.Snapshot(ctx)
		if err != nil {
			setupErr = fmt.Errorf("inspect Herdr session before attachment: %w", err)
			return setupErr
		}
		workspaceID, err = layout.ensure(ctx, st, snapshot, cwd)
		if err != nil {
			setupErr = err
			return setupErr
		}
		st.Socket = socket
		return nil
	})
	if err != nil {
		if setupErr != nil {
			return Wrap("session_setup_failed", setupErr.Error(), setupErr)
		}
		return Wrap("state_persist_failed",
			fmt.Sprintf("orchestrator layout was prepared but its state could not be persisted: %v", err), err)
	}
	s.WorkspaceID = workspaceID
	return nil
}

// orchestratorLayout owns the session's dedicated orchestrator tab: locating
// or creating the project workspace, choosing the tab, naming and splitting
// its primary pane, and recording the resulting IDs in session state. It is
// driven under the state lock, so every method takes the locked session.
type orchestratorLayout struct {
	project   project.Info
	client    *herdr.Client
	preferred string // the workspace the service resolved, possibly stale
}

type orchestratorWorkspace struct {
	id          string
	initialTab  herdr.TabInfo
	initialPane herdr.PaneInfo
	created     bool
}

type orchestratorTabSelection struct {
	tab           herdr.TabInfo
	panes         []herdr.PaneInfo
	initialized   bool
	trustedPaneID string
}

// ensure returns the ID of the workspace the orchestrator tab now lives in.
func (o *orchestratorLayout) ensure(
	ctx context.Context,
	st *state.Session,
	snapshot herdr.Snapshot,
	cwd string,
) (string, error) {
	workspace, err := o.resolveOrCreateWorkspace(ctx, st, snapshot, cwd)
	if err != nil {
		return "", err
	}
	selection, err := o.selectOrCreateTab(ctx, st, snapshot, workspace, cwd)
	if err != nil {
		return "", err
	}
	if err := o.initializeOrFocus(ctx, st, workspace.id, selection, cwd); err != nil {
		return "", err
	}
	return workspace.id, nil
}

func (o *orchestratorLayout) resolveOrCreateWorkspace(
	ctx context.Context,
	st *state.Session,
	snapshot herdr.Snapshot,
	cwd string,
) (orchestratorWorkspace, error) {
	// Unlike the agent path, a stale preferred workspace falls back to the
	// persisted mapping before the project-wide search.
	preferred := o.preferred
	if !hasWorkspace(snapshot, preferred) {
		preferred = st.WorkspaceID
	}
	workspaceID, err := resolveWorkspaceID(snapshot, o.project, preferred, "orchestrator")
	if err != nil {
		return orchestratorWorkspace{}, err
	}
	if workspaceID != "" {
		return orchestratorWorkspace{id: workspaceID}, nil
	}

	created, err := createProjectWorkspace(ctx, o.client, o.project.Root, cwd)
	if err != nil {
		return orchestratorWorkspace{}, fmt.Errorf("create Herdr workspace at %s: %w", cwd, err)
	}
	workspace := orchestratorWorkspace{
		id: created.Workspace.WorkspaceID, initialTab: created.Tab,
		initialPane: created.RootPane, created: true,
	}
	if workspace.id == "" || workspace.initialTab.TabID == "" || workspace.initialPane.PaneID == "" {
		return orchestratorWorkspace{}, errors.New("Herdr created a workspace without returning its workspace, tab, and primary pane IDs")
	}
	return workspace, nil
}

func (o *orchestratorLayout) selectOrCreateTab(
	ctx context.Context,
	st *state.Session,
	snapshot herdr.Snapshot,
	workspace orchestratorWorkspace,
	cwd string,
) (orchestratorTabSelection, error) {
	var selection orchestratorTabSelection
	if workspace.created {
		selection.tab = workspace.initialTab
		selection.panes = []herdr.PaneInfo{workspace.initialPane}
	} else {
		persistedTab, persistedTabFound := tabInWorkspace(snapshot, workspace.id, st.OrchestratorTabID)
		selection.initialized = st.OrchestratorInitialized &&
			st.WorkspaceID == workspace.id &&
			persistedTabFound
		if selection.initialized {
			selection.tab = persistedTab
			selection.trustedPaneID = st.OrchestratorPaneID
		} else if !(st.OrchestratorInitialized &&
			st.WorkspaceID == workspace.id &&
			st.OrchestratorTabID != "" &&
			!persistedTabFound) {
			selection.tab, _ = firstTabInWorkspace(snapshot, workspace.id)
		}
		selection.panes = panesInTab(snapshot, selection.tab.TabID)
	}

	if selection.tab.TabID != "" && !selection.initialized &&
		selection.tab.Label == orchestratorLabel && len(selection.panes) > 1 {
		// A completed setup whose state write failed (or legacy schema-v1
		// state) is safely recoverable from its stable labels and pane count.
		selection.initialized = true
	}
	if selection.tab.TabID != "" {
		return selection, nil
	}

	var created herdr.Result
	if err := o.client.Call(ctx, "tab.create", map[string]any{
		"workspace_id": workspace.id, "cwd": cwd, "focus": false, "label": orchestratorLabel,
	}, &created); err != nil {
		return orchestratorTabSelection{}, fmt.Errorf("create orchestrator tab: %w", err)
	}
	if created.Tab.TabID == "" || created.RootPane.PaneID == "" {
		return orchestratorTabSelection{}, errors.New("Herdr created an orchestrator tab without returning its tab and primary pane IDs")
	}
	selection.tab = created.Tab
	selection.panes = []herdr.PaneInfo{created.RootPane}
	return selection, nil
}

func (o *orchestratorLayout) initializeOrFocus(
	ctx context.Context,
	st *state.Session,
	workspaceID string,
	selection orchestratorTabSelection,
	cwd string,
) error {
	primary, ok := orchestratorPane(selection.panes, selection.trustedPaneID)
	if !ok {
		if selection.initialized {
			return fmt.Errorf("orchestrator tab %s has no pane to focus", selection.tab.TabID)
		}
		return fmt.Errorf("orchestrator tab %s has no primary pane", selection.tab.TabID)
	}
	if selection.initialized {
		if err := o.client.Call(ctx, "pane.focus", map[string]any{"pane_id": primary.PaneID}, nil); err != nil {
			return fmt.Errorf("focus orchestrator pane %s: %w", primary.PaneID, err)
		}
		persistOrchestratorLayout(st, workspaceID, selection.tab.TabID, primary.PaneID)
		return nil
	}

	if err := o.client.Call(ctx, "tab.rename",
		map[string]any{"tab_id": selection.tab.TabID, "label": orchestratorLabel}, nil); err != nil {
		return fmt.Errorf("rename orchestrator tab %s: %w", selection.tab.TabID, err)
	}
	if err := o.client.Call(ctx, "pane.rename",
		map[string]any{"pane_id": primary.PaneID, "label": orchestratorLabel}, nil); err != nil {
		return fmt.Errorf("rename orchestrator pane %s: %w", primary.PaneID, err)
	}
	var split herdr.Result
	if err := o.client.Call(ctx, "pane.split", map[string]any{
		"target_pane_id": primary.PaneID,
		"workspace_id":   workspaceID,
		"direction":      "right",
		"ratio":          0.5,
		"cwd":            cwd,
		"focus":          false,
	}, &split); err != nil {
		return fmt.Errorf("split orchestrator pane %s: %w", primary.PaneID, err)
	}
	if split.Pane.PaneID == "" {
		return errors.New("Herdr split the orchestrator pane without returning the new pane ID")
	}
	if err := o.client.Call(ctx, "pane.focus", map[string]any{"pane_id": primary.PaneID}, nil); err != nil {
		return fmt.Errorf("focus orchestrator pane %s: %w", primary.PaneID, err)
	}
	persistOrchestratorLayout(st, workspaceID, selection.tab.TabID, primary.PaneID)
	return nil
}

func persistOrchestratorLayout(st *state.Session, workspaceID, tabID, paneID string) {
	st.WorkspaceID = workspaceID
	st.OrchestratorTabID = tabID
	st.OrchestratorPaneID = paneID
	st.OrchestratorInitialized = true
}

func tabInWorkspace(snapshot herdr.Snapshot, workspaceID, tabID string) (herdr.TabInfo, bool) {
	if tabID == "" {
		return herdr.TabInfo{}, false
	}
	for _, tab := range snapshot.Tabs {
		if tab.WorkspaceID == workspaceID && tab.TabID == tabID {
			return tab, true
		}
	}
	return herdr.TabInfo{}, false
}

func firstTabInWorkspace(snapshot herdr.Snapshot, workspaceID string) (herdr.TabInfo, bool) {
	for _, tab := range snapshot.Tabs {
		if tab.WorkspaceID == workspaceID {
			return tab, true
		}
	}
	return herdr.TabInfo{}, false
}

func panesInTab(snapshot herdr.Snapshot, tabID string) []herdr.PaneInfo {
	panes := make([]herdr.PaneInfo, 0)
	for _, pane := range snapshot.Panes {
		if pane.TabID == tabID {
			panes = append(panes, pane)
		}
	}
	return panes
}

func orchestratorPane(panes []herdr.PaneInfo, persistedID string) (herdr.PaneInfo, bool) {
	for _, pane := range panes {
		if pane.PaneID == persistedID {
			return pane, true
		}
	}
	for _, pane := range panes {
		if pane.Label != nil && *pane.Label == orchestratorLabel {
			return pane, true
		}
	}
	if len(panes) > 0 {
		return panes[0], true
	}
	return herdr.PaneInfo{}, false
}
