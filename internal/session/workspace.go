package session

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/session/lock"
	"fledge/internal/session/record"
	"fledge/internal/session/workspace"
)

// WorkspaceRole is a stable logical key for a managed Herder workspace.
type WorkspaceRole = workspace.Role

const (
	// OrchestratorWorkspaceRole is the pre-existing orchestrator workspace.
	OrchestratorWorkspaceRole WorkspaceRole = workspace.Orchestrator
	// AgentsWorkspaceRole is the creatable managed-worker workspace.
	AgentsWorkspaceRole WorkspaceRole = workspace.Agents
)

// WorkspaceServer is the Herder surface needed to reconcile managed
// workspaces. CreateWorkspace is contractually unfocused and preserves the
// returned workspace's root tab and pane.
type WorkspaceServer interface {
	Workspaces(context.Context) ([]herdr.Workspace, error)
	CreateWorkspace(context.Context, string) (herdr.WorkspaceCreated, error)
	CloseWorkspace(context.Context, string) error
}

// AmbiguousError reports that label-based upgrade adoption cannot choose one
// durable workspace identity. WorkspaceIDs are sorted for actionable output.
type AmbiguousError struct {
	Role         WorkspaceRole
	Label        string
	WorkspaceIDs []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("managed workspace role %q: label %q matches workspaces %s; close or rename all but one and retry",
		e.Role, e.Label, strings.Join(e.WorkspaceIDs, ", "))
}

// MissingError reports that a non-creatable managed workspace has neither a
// live stored identity nor exactly one workspace carrying its expected label.
type MissingError struct {
	Role  WorkspaceRole
	Label string
}

func (e *MissingError) Error() string {
	return fmt.Sprintf("managed workspace role %q is missing; label the intended workspace exactly %q and retry", e.Role, e.Label)
}

// RoleConflictError reports that reconciliation would collapse two logical
// roles onto one durable Herder workspace identity.
type RoleConflictError struct {
	WorkspaceID     string
	FirstRole       WorkspaceRole
	ConflictingRole WorkspaceRole
}

func (e *RoleConflictError) Error() string {
	return fmt.Sprintf("managed workspace %q resolves to both roles %q and %q; give each role a distinct workspace, repair its label or stale workspace state, and retry",
		e.WorkspaceID, e.FirstRole, e.ConflictingRole)
}

type acquireWorkspaceLock func(context.Context, string) (func() error, error)

const workspaceCleanupTimeout = 5 * time.Second

// EnsureWorkspaces returns the live Herder workspace for every requested role,
// adopting or creating missing identities and atomically publishing changes to
// the session record. All reconciliation is serialized by the project lock.
func EnsureWorkspaces(ctx context.Context, projectRoot, recordPath string, server WorkspaceServer, roles ...WorkspaceRole) (map[WorkspaceRole]herdr.Workspace, error) {
	return ensureWorkspaces(ctx, projectRoot, recordPath, server, lock.Acquire, roles...)
}

// ensureWorkspaces is the same-package dependency seam used by lifecycle tests
// that already substitute project-lock acquisition.
func ensureWorkspaces(ctx context.Context, projectRoot, recordPath string, server WorkspaceServer, acquire acquireWorkspaceLock, roles ...WorkspaceRole) (ensured map[WorkspaceRole]herdr.Workspace, err error) {
	if ctx == nil {
		return nil, fmt.Errorf("ensure managed workspaces: context is nil")
	}
	if server == nil {
		return nil, fmt.Errorf("ensure managed workspaces: Herder server is nil")
	}
	if err := validateWorkspacePaths(projectRoot, recordPath); err != nil {
		return nil, fmt.Errorf("ensure managed workspaces: %w", err)
	}
	ordered, err := requestedWorkspaceSpecs(roles)
	if err != nil {
		return nil, fmt.Errorf("ensure managed workspaces: %w", err)
	}
	if len(ordered) == 0 {
		return map[WorkspaceRole]herdr.Workspace{}, nil
	}
	if acquire == nil {
		return nil, fmt.Errorf("ensure managed workspaces: project lock acquisition is nil")
	}
	release, err := acquire(ctx, filepath.Join(projectRoot, ".fledge"))
	if err != nil {
		return nil, fmt.Errorf("ensure managed workspaces: lock project: %w", err)
	}
	if release == nil {
		return nil, fmt.Errorf("ensure managed workspaces: lock project returned a nil release")
	}
	var created []herdr.Workspace
	createdPublished := false
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			releaseFailure := error(fmt.Errorf("ensure managed workspaces: release project lock: %w", releaseErr))
			// Once the new IDs are atomically published, cleanup would leave
			// durable state pointing at a workspace this call just closed.
			if len(created) != 0 && !createdPublished {
				releaseFailure = cleanupCreatedWorkspaces(ctx, server, created, releaseFailure)
				created = nil
				ensured = nil
			}
			err = errors.Join(err, releaseFailure)
		}
	}()

	ids, err := record.ReadWorkspaces(recordPath)
	if err != nil {
		return nil, fmt.Errorf("ensure managed workspaces: %w", err)
	}
	live, err := server.Workspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("ensure managed workspaces: list Herder workspaces: %w", err)
	}
	byID := make(map[string]herdr.Workspace, len(live))
	for _, listed := range live {
		if listed.ID == "" {
			return nil, fmt.Errorf("ensure managed workspaces: Herder listed a workspace with an empty ID")
		}
		if _, duplicate := byID[listed.ID]; duplicate {
			return nil, fmt.Errorf("ensure managed workspaces: Herder listed duplicate workspace ID %q", listed.ID)
		}
		byID[listed.ID] = listed
	}

	result := make(map[WorkspaceRole]herdr.Workspace, len(ordered))
	created = make([]herdr.Workspace, 0, len(ordered))
	changed := false
	fail := func(cause error) (map[WorkspaceRole]herdr.Workspace, error) {
		if len(created) == 0 {
			return nil, cause
		}
		cleanup := created
		created = nil
		return nil, cleanupCreatedWorkspaces(ctx, server, cleanup, cause)
	}
	assigned := make(map[string]WorkspaceRole, len(ordered))
	assign := func(role WorkspaceRole, selected herdr.Workspace) error {
		if first, duplicate := assigned[selected.ID]; duplicate {
			return &RoleConflictError{WorkspaceID: selected.ID, FirstRole: first, ConflictingRole: role}
		}
		assigned[selected.ID] = role
		result[role] = selected
		return nil
	}

	projectBase := filepath.Base(projectRoot)
	for _, spec := range ordered {
		role := spec.Role()
		if storedID := ids[string(role)]; storedID != "" {
			if stored, ok := byID[storedID]; ok {
				if assignErr := assign(role, stored); assignErr != nil {
					return fail(assignErr)
				}
				continue
			}
		}

		label, labelErr := spec.Label(projectBase)
		if labelErr != nil {
			return fail(fmt.Errorf("ensure managed workspaces: role %q: %w", role, labelErr))
		}
		matches := workspacesWithLabel(live, label)
		switch len(matches) {
		case 0:
			if !spec.Creatable() {
				return fail(&MissingError{Role: role, Label: label})
			}
			createdWorkspace, createErr := server.CreateWorkspace(ctx, label)
			if createErr != nil {
				return fail(fmt.Errorf("ensure managed workspaces: create role %q with label %q: %w", role, label, createErr))
			}
			if createdWorkspace.Workspace.ID == "" {
				return fail(fmt.Errorf("ensure managed workspaces: create role %q returned an empty workspace ID", role))
			}
			selected := createdWorkspace.Workspace
			created = append(created, selected)
			if assignErr := assign(role, selected); assignErr != nil {
				return fail(assignErr)
			}
			ids[string(role)] = selected.ID
			changed = true
		case 1:
			if assignErr := assign(role, matches[0]); assignErr != nil {
				return fail(assignErr)
			}
			ids[string(role)] = matches[0].ID
			changed = true
		default:
			workspaceIDs := make([]string, len(matches))
			for i, match := range matches {
				workspaceIDs[i] = match.ID
			}
			sort.Strings(workspaceIDs)
			return fail(&AmbiguousError{Role: role, Label: label, WorkspaceIDs: workspaceIDs})
		}
	}
	if changed {
		if err := record.WriteWorkspaces(recordPath, ids); err != nil {
			return fail(fmt.Errorf("ensure managed workspaces: %w", err))
		}
		createdPublished = true
	}
	return result, nil
}

func cleanupCreatedWorkspaces(ctx context.Context, server WorkspaceServer, created []herdr.Workspace, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workspaceCleanupTimeout)
	defer cancel()
	joined := cause
	for i := len(created) - 1; i >= 0; i-- {
		if closeErr := server.CloseWorkspace(cleanupCtx, created[i].ID); closeErr != nil {
			joined = errors.Join(joined, fmt.Errorf("clean up created workspace %q: %w", created[i].ID, closeErr))
		}
	}
	return joined
}

func requestedWorkspaceSpecs(requested []WorkspaceRole) ([]workspace.Spec, error) {
	selected := make(map[WorkspaceRole]struct{}, len(requested))
	for _, role := range requested {
		if _, err := workspace.Lookup(role); err != nil {
			return nil, err
		}
		if _, duplicate := selected[role]; duplicate {
			return nil, fmt.Errorf("duplicate managed workspace role %q", role)
		}
		selected[role] = struct{}{}
	}
	ordered := make([]workspace.Spec, 0, len(selected))
	for _, role := range workspace.Roles() {
		if _, requested := selected[role]; !requested {
			continue
		}
		spec, _ := workspace.Lookup(role)
		ordered = append(ordered, spec)
	}
	return ordered, nil
}

func workspacesWithLabel(live []herdr.Workspace, label string) []herdr.Workspace {
	var matches []herdr.Workspace
	for _, listed := range live {
		if listed.Label == label {
			matches = append(matches, listed)
		}
	}
	return matches
}

func validateWorkspacePaths(projectRoot, recordPath string) error {
	for _, value := range []struct {
		name string
		path string
	}{
		{name: "project root", path: projectRoot},
		{name: "record path", path: recordPath},
	} {
		if value.path == "" {
			return fmt.Errorf("%s is empty", value.name)
		}
		if strings.IndexByte(value.path, 0) >= 0 || !filepath.IsAbs(value.path) || filepath.Clean(value.path) != value.path {
			return fmt.Errorf("%s %q is not a clean absolute path", value.name, value.path)
		}
	}
	recordsRoot := filepath.Join(projectRoot, ".fledge", "sessions")
	relative, err := filepath.Rel(recordsRoot, recordPath)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("record path %q is not a session record beneath %q", recordPath, recordsRoot)
	}
	return nil
}
