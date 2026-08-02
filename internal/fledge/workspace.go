package fledge

import (
	"context"
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

// selectedWorkspaceID is the workspace agent operations act on: the one this
// service resolved at startup, falling back to the persisted mapping.
func (s *Service) selectedWorkspaceID(st *state.Session) string {
	if s.WorkspaceID != "" {
		return s.WorkspaceID
	}
	return st.WorkspaceID
}

// resolveWorkspaceID returns preferred when the server still knows it, and
// otherwise searches for proj's workspace by worktree metadata and then by
// label. An empty result means the workspace must be created. purpose names
// the caller in the error returned when worktree metadata is unreadable.
func resolveWorkspaceID(snapshot herdr.Snapshot, proj project.Info, preferred, purpose string) (string, error) {
	if hasWorkspace(snapshot, preferred) {
		return preferred, nil
	}
	workspaceID := ""
	if matched, found, err := matchingWorkspace(snapshot, proj.Root); err != nil {
		return "", fmt.Errorf("resolve Herdr workspace for %s: %w", purpose, err)
	} else if found {
		workspaceID = matched.WorkspaceID
	}
	if workspaceID == "" {
		if workspace, found := fallbackWorkspace(snapshot, proj.Root, proj.Session); found {
			workspaceID = workspace.WorkspaceID
		}
	}
	return workspaceID, nil
}

// createProjectWorkspace opens an unfocused workspace at cwd labelled for the
// project rooted at root. Callers wrap the error with their own context.
func createProjectWorkspace(ctx context.Context, client *herdr.Client, root, cwd string) (herdr.Result, error) {
	var created herdr.Result
	err := client.Call(ctx, "workspace.create", map[string]any{
		"cwd": cwd, "focus": false, "label": project.WorkspaceLabel(root),
	}, &created)
	return created, err
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
