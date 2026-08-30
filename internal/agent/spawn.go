package agent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"fledge/internal/catalog"
	"fledge/internal/herdr"
	"fledge/internal/profile"
)

const defaultSplitDirection = "right"

const cleanupTimeout = 5 * time.Second

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// SpawnOptions describes an agent launch. At most one of Workspace, Tab, and
// Pane selects where the agent lands; Workspace is "new" or a workspace ID.
// Ratio applies to the split placements only, Label defaults to Name, Profile
// is an immutable managed snapshot, and Args follow model and profile delivery.
type SpawnOptions struct {
	Name      string
	Harness   string
	Model     string
	Profile   *profile.Profile
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
	Harness     string `json:"harness"`
	Model       string `json:"model"`
	Profile     string `json:"profile"`
	WorkspaceID string `json:"workspace_id"`
	TabID       string `json:"tab_id"`
	PaneID      string `json:"pane_id"`
}

// Spawn creates a pane for the agent and starts the harness inside it. A failed
// launch closes the pane and removes any file-backed profile artifact.
func Spawn(ctx context.Context, h Herder, caller Caller, opts SpawnOptions) (result SpawnResult, err error) {
	label, err := validateSpawn(opts)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("spawn agent: %w", err)
	}
	args, cleanupArtifact, err := spawnArgs(caller, opts)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("spawn agent %q: %w", opts.Name, err)
	}
	retainArtifact := false
	if cleanupArtifact != nil {
		defer func() {
			if retainArtifact {
				return
			}
			if cleanupErr := cleanupArtifact(); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
			}
		}()
	}

	pane, err := placePane(ctx, h, caller, opts, label)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("spawn agent %q: %w", opts.Name, err)
	}

	if _, startErr := h.StartAgent(ctx, herdr.StartAgentOptions{
		Name:   opts.Name,
		Kind:   opts.Harness,
		PaneID: pane.ID,
		Args:   args,
	}); startErr != nil {
		callerErr := ctx.Err()
		cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		cleanupErr := h.ClosePane(cleanupCtx, pane.ID)
		cancelCleanup()

		failures := []error{fmt.Errorf("start in pane %q: %w", pane.ID, startErr)}
		if callerErr != nil && !errors.Is(startErr, callerErr) {
			failures = append(failures, callerErr)
		}
		if cleanupErr != nil {
			failures = append(failures, fmt.Errorf("close pane %q: %w", pane.ID, cleanupErr))
		}
		return SpawnResult{}, fmt.Errorf("spawn agent %q: %w", opts.Name, errors.Join(failures...))
	}
	retainArtifact = true

	result = SpawnResult{
		Name:        opts.Name,
		Harness:     opts.Harness,
		Model:       opts.Model,
		WorkspaceID: pane.WorkspaceID,
		TabID:       pane.TabID,
		PaneID:      pane.ID,
	}
	if opts.Profile != nil {
		result.Profile = opts.Profile.Name
	}
	return result, nil
}

func spawnArgs(caller Caller, opts SpawnOptions) ([]string, func() error, error) {
	args := append([]string(nil), opts.Args...)
	var cleanup func() error
	if opts.Profile != nil {
		// Validate the selected harness and every reserved instruction argument
		// before writing an artifact or creating a pane.
		if _, err := profile.LaunchArgs(*opts.Profile, opts.Harness, "/fledge/profile/instructions.md", args); err != nil {
			return nil, nil, fmt.Errorf("prepare profile %q: %w", opts.Profile.Name, err)
		}

		instructionPath := ""
		if opts.Harness == string(catalog.Pi) || opts.Harness == string(catalog.Claude) {
			var err error
			instructionPath, cleanup, err = createProfileArtifact(caller.RecordPath, opts.Name, opts.Profile.Instructions)
			if err != nil {
				return nil, nil, fmt.Errorf("materialize profile %q: %w", opts.Profile.Name, err)
			}
		}
		var err error
		args, err = profile.LaunchArgs(*opts.Profile, opts.Harness, instructionPath, args)
		if err != nil {
			if cleanup != nil {
				err = errors.Join(err, cleanup())
			}
			return nil, nil, fmt.Errorf("prepare profile %q: %w", opts.Profile.Name, err)
		}
	}
	if opts.Model != "" {
		args = append([]string{"--model", opts.Model}, args...)
	}
	return args, cleanup, nil
}

// validateSpawn checks the options in isolation and returns the pane label.
func validateSpawn(opts SpawnOptions) (string, error) {
	if !namePattern.MatchString(opts.Name) {
		return "", fmt.Errorf("name %q must match %s", opts.Name, namePattern)
	}
	if _, err := catalog.ParseHarness(opts.Harness); err != nil {
		return "", err
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
	if opts.Split != "" && opts.Tab == "" && opts.Pane == "" {
		return "", fmt.Errorf("split applies to tab and pane placement only")
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
	panes, err := h.Panes(ctx, "")
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
