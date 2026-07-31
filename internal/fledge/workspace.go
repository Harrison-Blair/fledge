package fledge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

// EnqueueOrchestratorPicker submits the spawn picker command to the saved
// primary orchestrator pane. The command itself owns picker cancellation and
// catalog warnings, so those do not affect the outer attached start.
func (s *Service) EnqueueOrchestratorPicker(ctx context.Context, socket, executable string) error {
	st, err := s.Store.Read(s.Project.Session, s.Project.Root)
	if err != nil {
		return err
	}
	if st.OrchestratorPaneID == "" {
		return errors.New("orchestrator pane is not initialized")
	}
	command := shellQuote(executable) + " agent spawn --name fledge-orchestrator"
	return (&herdr.Client{Socket: socket}).Call(ctx, "pane.send_input", map[string]any{
		"pane_id": st.OrchestratorPaneID,
		"text":    command,
		"keys":    []string{"enter"},
	}, nil)
}

func shellQuote(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n'\"\\$`!&|;()<>*?[]{}") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// EnsureAttachmentWorkspace prepares and focuses the session's dedicated
// orchestrator tab before either an interactive or detached start completes.
// Once initialized, the user's pane layout is left intact on later starts.
func (s *Service) EnsureAttachmentWorkspace(ctx context.Context, socket, cwd string) error {
	client := &herdr.Client{Socket: socket}
	var setupErr error
	var workspaceID string
	err := s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		snapshot, err := client.Snapshot(ctx)
		if err != nil {
			setupErr = fmt.Errorf("inspect Herdr session before attachment: %w", err)
			return setupErr
		}
		workspaceID, err = s.ensureOrchestratorLayout(ctx, client, st, snapshot, cwd)
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

func (s *Service) ensureOrchestratorLayout(
	ctx context.Context,
	client *herdr.Client,
	st *state.Session,
	snapshot herdr.Snapshot,
	cwd string,
) (string, error) {
	workspace, err := s.resolveOrCreateOrchestratorWorkspace(ctx, client, st, snapshot, cwd)
	if err != nil {
		return "", err
	}
	selection, err := s.selectOrCreateOrchestratorTab(ctx, client, st, snapshot, workspace, cwd)
	if err != nil {
		return "", err
	}
	if err := s.initializeOrFocusOrchestrator(ctx, client, st, workspace.id, selection, cwd); err != nil {
		return "", err
	}
	return workspace.id, nil
}

func (s *Service) resolveOrCreateOrchestratorWorkspace(
	ctx context.Context,
	client *herdr.Client,
	st *state.Session,
	snapshot herdr.Snapshot,
	cwd string,
) (orchestratorWorkspace, error) {
	// Unlike the agent path, a stale s.WorkspaceID falls back to the
	// persisted mapping before the project-wide search.
	preferred := s.WorkspaceID
	if !hasWorkspace(snapshot, preferred) {
		preferred = st.WorkspaceID
	}
	workspaceID, err := s.resolveWorkspaceID(snapshot, preferred, "orchestrator")
	if err != nil {
		return orchestratorWorkspace{}, err
	}
	if workspaceID != "" {
		return orchestratorWorkspace{id: workspaceID}, nil
	}

	created, err := s.createProjectWorkspace(ctx, client, cwd)
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

func (s *Service) selectOrCreateOrchestratorTab(
	ctx context.Context,
	client *herdr.Client,
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
	if err := client.Call(ctx, "tab.create", map[string]any{
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

func (s *Service) initializeOrFocusOrchestrator(
	ctx context.Context,
	client *herdr.Client,
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
		if err := client.Call(ctx, "pane.focus", map[string]any{"pane_id": primary.PaneID}, nil); err != nil {
			return fmt.Errorf("focus orchestrator pane %s: %w", primary.PaneID, err)
		}
		s.persistOrchestratorLayout(st, workspaceID, selection.tab.TabID, primary.PaneID)
		return nil
	}

	if err := client.Call(ctx, "tab.rename",
		map[string]any{"tab_id": selection.tab.TabID, "label": orchestratorLabel}, nil); err != nil {
		return fmt.Errorf("rename orchestrator tab %s: %w", selection.tab.TabID, err)
	}
	if err := client.Call(ctx, "pane.rename",
		map[string]any{"pane_id": primary.PaneID, "label": orchestratorLabel}, nil); err != nil {
		return fmt.Errorf("rename orchestrator pane %s: %w", primary.PaneID, err)
	}
	var split herdr.Result
	if err := client.Call(ctx, "pane.split", map[string]any{
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
	if err := client.Call(ctx, "pane.focus", map[string]any{"pane_id": primary.PaneID}, nil); err != nil {
		return fmt.Errorf("focus orchestrator pane %s: %w", primary.PaneID, err)
	}
	s.persistOrchestratorLayout(st, workspaceID, selection.tab.TabID, primary.PaneID)
	return nil
}

func (s *Service) persistOrchestratorLayout(st *state.Session, workspaceID, tabID, paneID string) {
	st.WorkspaceID = workspaceID
	st.OrchestratorTabID = tabID
	st.OrchestratorPaneID = paneID
	st.OrchestratorInitialized = true
}

// selectedWorkspaceID is the workspace agent operations act on: the one this
// service resolved at startup, falling back to the persisted mapping.
func (s *Service) selectedWorkspaceID(st *state.Session) string {
	if s.WorkspaceID != "" {
		return s.WorkspaceID
	}
	return st.WorkspaceID
}

// resolveWorkspaceID returns preferred when the server still knows it, and
// otherwise searches for this project's workspace by worktree metadata and
// then by label. An empty result means the workspace must be created. purpose
// names the caller in the error returned when worktree metadata is unreadable.
func (s *Service) resolveWorkspaceID(snapshot herdr.Snapshot, preferred, purpose string) (string, error) {
	if hasWorkspace(snapshot, preferred) {
		return preferred, nil
	}
	workspaceID := ""
	if matched, found, err := matchingWorkspace(snapshot, s.Project.Root); err != nil {
		return "", fmt.Errorf("resolve Herdr workspace for %s: %w", purpose, err)
	} else if found {
		workspaceID = matched.WorkspaceID
	}
	if workspaceID == "" {
		if workspace, found := fallbackWorkspace(snapshot, s.Project.Root, s.Project.Session); found {
			workspaceID = workspace.WorkspaceID
		}
	}
	return workspaceID, nil
}

// createProjectWorkspace opens an unfocused workspace at cwd labelled for this
// project. Callers wrap the error with their own context.
func (s *Service) createProjectWorkspace(ctx context.Context, client *herdr.Client, cwd string) (herdr.Result, error) {
	var created herdr.Result
	err := client.Call(ctx, "workspace.create", map[string]any{
		"cwd": cwd, "focus": false, "label": project.WorkspaceLabel(s.Project.Root),
	}, &created)
	return created, err
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

func hasWorkspace(snapshot herdr.Snapshot, id string) bool {
	for _, workspace := range snapshot.Workspaces {
		if workspace.WorkspaceID == id {
			return true
		}
	}
	return false
}

// fallbackWorkspace recovers layouts whose persisted IDs or worktree metadata
// are unavailable. Prefer the current project-folder label, while continuing
// to recognize the generated label used by older Fledge versions. A sole
// workspace is also safe to adopt because the deterministic session is
// dedicated to this project; its custom label is left unchanged.
func fallbackWorkspace(snapshot herdr.Snapshot, root, session string) (herdr.WorkspaceInfo, bool) {
	for _, label := range []string{
		project.WorkspaceLabel(root),
		legacyWorkspaceLabelPrefix + session,
	} {
		for _, workspace := range snapshot.Workspaces {
			if workspace.Label == label {
				return workspace, true
			}
		}
	}
	if len(snapshot.Workspaces) == 1 {
		return snapshot.Workspaces[0], true
	}
	return herdr.WorkspaceInfo{}, false
}
