package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"fledge/internal/herdr"
)

const defaultSplitDirection = "right"

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// SpawnOptions describes an agent launch. At most one of Workspace, Tab, and
// Pane selects where the agent lands; Workspace is "new" or a workspace ID.
// Ratio applies to the split placements only, Label defaults to Name, and Args
// are extra harness arguments appended after the model selection.
type SpawnOptions struct {
	Name      string
	Kind      string
	Model     string
	Workspace string
	Tab       string
	Pane      string
	Split     string
	Ratio     *float64
	Label     string
	Args      []string
}

// SpawnResult describes the agent that was started and where it landed.
type SpawnResult struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Model       string `json:"model"`
	WorkspaceID string `json:"workspace_id"`
	TabID       string `json:"tab_id"`
	PaneID      string `json:"pane_id"`
}

// Spawn creates a pane for the agent and starts the harness inside it. The
// pane is closed again when the harness fails to start.
func Spawn(ctx context.Context, h Herder, caller Caller, opts SpawnOptions) (SpawnResult, error) {
	label, err := validateSpawn(opts)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("spawn agent: %w", err)
	}

	pane, err := placePane(ctx, h, caller, opts, label)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("spawn agent %q: %w", opts.Name, err)
	}

	args := opts.Args
	if opts.Model != "" {
		args = append([]string{"--model", opts.Model}, opts.Args...)
	}
	if _, err := h.StartAgent(ctx, herdr.StartAgentOptions{
		Name:   opts.Name,
		Kind:   opts.Kind,
		PaneID: pane.ID,
		Args:   args,
	}); err != nil {
		_ = h.ClosePane(ctx, pane.ID)
		return SpawnResult{}, fmt.Errorf("spawn agent %q: start in pane %q: %w", opts.Name, pane.ID, err)
	}

	return SpawnResult{
		Name:        opts.Name,
		Kind:        opts.Kind,
		Model:       opts.Model,
		WorkspaceID: pane.WorkspaceID,
		TabID:       pane.TabID,
		PaneID:      pane.ID,
	}, nil
}

// validateSpawn checks the options in isolation and returns the pane label.
func validateSpawn(opts SpawnOptions) (string, error) {
	if !namePattern.MatchString(opts.Name) {
		return "", fmt.Errorf("name %q must match %s", opts.Name, namePattern)
	}
	if opts.Kind == "" {
		return "", fmt.Errorf("agent kind is required")
	}

	placements := 0
	for _, placement := range []string{opts.Workspace, opts.Tab, opts.Pane} {
		if placement != "" {
			placements++
		}
	}
	if placements > 1 {
		return "", fmt.Errorf("at most one of workspace, tab, and pane may be set")
	}
	if opts.Split != "" && opts.Split != defaultSplitDirection && opts.Split != "down" {
		return "", fmt.Errorf("split %q must be right or down", opts.Split)
	}
	if opts.Ratio != nil && opts.Tab == "" && opts.Pane == "" {
		return "", fmt.Errorf("ratio applies to tab and pane placement only")
	}

	if opts.Label != "" {
		return opts.Label, nil
	}
	return opts.Name, nil
}

// placePane creates or splits the pane the agent will run in.
func placePane(ctx context.Context, h Herder, caller Caller, opts SpawnOptions, label string) (herdr.Pane, error) {
	switch {
	case opts.Pane != "":
		return h.SplitPane(ctx, splitOptions(opts, opts.Pane))
	case opts.Tab != "":
		host, err := tabHostPane(ctx, h, opts.Tab)
		if err != nil {
			return herdr.Pane{}, err
		}
		return h.SplitPane(ctx, splitOptions(opts, host.ID))
	case opts.Workspace == "new":
		created, err := h.CreateWorkspace(ctx, label)
		if err != nil {
			return herdr.Pane{}, err
		}
		return created.RootPane, nil
	case opts.Workspace != "":
		return rootPaneOfNewTab(ctx, h, opts.Workspace, label)
	default:
		workspace := caller.WorkspaceID
		if workspace == "" {
			focused, err := focusedWorkspace(ctx, h)
			if err != nil {
				return herdr.Pane{}, err
			}
			workspace = focused
		}
		return rootPaneOfNewTab(ctx, h, workspace, label)
	}
}

func splitOptions(opts SpawnOptions, paneID string) herdr.SplitOptions {
	direction := opts.Split
	if direction == "" {
		direction = defaultSplitDirection
	}
	return herdr.SplitOptions{PaneID: paneID, Direction: direction, Ratio: opts.Ratio}
}

func rootPaneOfNewTab(ctx context.Context, h Herder, workspaceID, label string) (herdr.Pane, error) {
	created, err := h.CreateTab(ctx, workspaceID, label)
	if err != nil {
		return herdr.Pane{}, err
	}
	return created.RootPane, nil
}

// tabHostPane returns the pane to split for a tab, preferring its focused pane.
func tabHostPane(ctx context.Context, h Herder, tabID string) (herdr.Pane, error) {
	workspaceID, _, _ := strings.Cut(tabID, ":")
	panes, err := h.Panes(ctx, workspaceID)
	if err != nil {
		return herdr.Pane{}, err
	}

	var first *herdr.Pane
	for i, pane := range panes {
		if pane.TabID != tabID {
			continue
		}
		if pane.Focused {
			return pane, nil
		}
		if first == nil {
			first = &panes[i]
		}
	}
	if first == nil {
		return herdr.Pane{}, fmt.Errorf("tab %q has no panes", tabID)
	}
	return *first, nil
}

func focusedWorkspace(ctx context.Context, h Herder) (string, error) {
	workspaces, err := h.Workspaces(ctx)
	if err != nil {
		return "", err
	}
	for _, workspace := range workspaces {
		if workspace.Focused {
			return workspace.ID, nil
		}
	}
	return "", fmt.Errorf("no focused workspace")
}
