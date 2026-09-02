package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/profile"
	"fledge/internal/session/types"
	"fledge/internal/session/utils"
	"fledge/internal/session/workspace"
)

// LogName is the bootstrap report file written inside a session record.
const LogName = "bootstrap.log"

const (
	orchestratorName     = "orchestrator"
	orchestratorTabLabel = "fledge-orchestrator"
)

// Server is the Herder surface needed to prepare a fresh session.
type Server interface {
	Status(context.Context) (herdr.Status, error)
	Workspaces(context.Context) ([]herdr.Workspace, error)
	Panes(context.Context, string) ([]herdr.Pane, error)
	RenameWorkspace(context.Context, string, string) error
	RenameTab(context.Context, string, string) error
	CreateWorkspace(context.Context, string) (herdr.WorkspaceCreated, error)
	CloseWorkspace(context.Context, string) error
	StartAgent(context.Context, herdr.StartAgentOptions) (herdr.Agent, error)
}

// EnsureWorkspaces reconciles the requested managed-workspace roles.
type EnsureWorkspaces func(context.Context, ...workspace.Role) (map[workspace.Role]herdr.Workspace, error)

// Input describes the session being prepared.
type Input struct {
	Root                    string
	Choice                  types.AgentChoice
	ProfileInstructionsPath string
	Log                     io.Writer
	EnsureWorkspaces        EnsureWorkspaces
}

// Timing bounds the polling a fresh Herder server requires.
type Timing struct {
	Poll     time.Duration
	Deadline time.Duration
}

// DefaultTiming is the polling schedule used outside tests.
var DefaultTiming = Timing{
	Poll:     200 * time.Millisecond,
	Deadline: 30 * time.Second,
}

// Run labels a fresh session's first workspace and tab, reconciles every
// managed workspace, then starts the chosen agent in the root pane. It runs
// while Herder owns the terminal, so every step is reported to in.Log.
func Run(ctx context.Context, h Server, in Input, timing Timing) error {
	ctx, cancel := context.WithTimeout(ctx, timing.Deadline)
	defer cancel()

	logStep(in.Log, "bootstrap started")
	args, err := launchArgs(in)
	if err != nil {
		return logFail(in.Log, err)
	}
	if in.EnsureWorkspaces == nil {
		return logFail(in.Log, fmt.Errorf("ensure managed workspaces is nil"))
	}
	orchestrator, err := workspace.Lookup(workspace.Orchestrator)
	if err != nil {
		return logFail(in.Log, fmt.Errorf("load orchestrator workspace policy: %w", err))
	}
	rootLabel, err := orchestrator.Label(filepath.Base(in.Root))
	if err != nil {
		return logFail(in.Log, fmt.Errorf("label orchestrator workspace: %w", err))
	}

	// Herder reports failure until its socket exists, so errors here mean the
	// server is not up yet.
	if err := poll(ctx, timing.Poll, func(ctx context.Context) bool {
		status, err := h.Status(ctx)
		return err == nil && status.Running
	}); err != nil {
		return logFail(in.Log, fmt.Errorf("Herder server did not start: %w", err))
	}
	logStep(in.Log, "Herder server running")

	var rootWorkspace herdr.Workspace
	if err := poll(ctx, timing.Poll, func(ctx context.Context) bool {
		workspaces, err := h.Workspaces(ctx)
		if err != nil || len(workspaces) == 0 {
			return false
		}
		rootWorkspace = workspaces[0]
		return true
	}); err != nil {
		return logFail(in.Log, fmt.Errorf("list workspaces: %w", err))
	}
	tabID := rootWorkspace.ActiveTabID

	var pane herdr.Pane
	if err := poll(ctx, timing.Poll, func(ctx context.Context) bool {
		panes, err := h.Panes(ctx, rootWorkspace.ID)
		if err != nil || len(panes) == 0 {
			return false
		}
		pane = panes[0]
		for _, candidate := range panes {
			if candidate.TabID == tabID {
				pane = candidate
				break
			}
		}
		return true
	}); err != nil {
		return logFail(in.Log, fmt.Errorf("list panes: %w", err))
	}
	logStep(in.Log, "workspace %s tab %s pane %s", rootWorkspace.ID, tabID, pane.ID)

	if err := h.RenameWorkspace(ctx, rootWorkspace.ID, rootLabel); err != nil {
		return logFail(in.Log, halted(ctx, fmt.Errorf("rename workspace %s: %w", rootWorkspace.ID, err)))
	}
	logStep(in.Log, "renamed workspace to %s", rootLabel)

	if err := h.RenameTab(ctx, tabID, orchestratorTabLabel); err != nil {
		return logFail(in.Log, halted(ctx, fmt.Errorf("rename tab %s: %w", tabID, err)))
	}
	logStep(in.Log, "renamed tab to %s", orchestratorTabLabel)

	roles := workspace.Roles()
	managed, err := in.EnsureWorkspaces(ctx, roles...)
	if err != nil {
		return logFail(in.Log, halted(ctx, fmt.Errorf("ensure managed workspaces: %w", err)))
	}
	for _, role := range roles {
		ensured, ok := managed[role]
		if !ok || ensured.ID == "" {
			return logFail(in.Log, fmt.Errorf("ensure managed workspaces omitted role %q", role))
		}
		logStep(in.Log, "managed workspace %s is %s", role, ensured.ID)
	}

	if in.Choice.Harness == "" {
		logStep(in.Log, "no agent requested")
		return nil
	}
	return startAgent(ctx, h, in, pane.ID, args)
}

func launchArgs(in Input) ([]string, error) {
	if in.Choice.Harness == "" {
		return nil, nil
	}

	args := append([]string(nil), in.Choice.Args...)
	if in.Choice.Profile != nil {
		var err error
		args, err = profile.LaunchArgs(*in.Choice.Profile, in.Choice.Harness, in.ProfileInstructionsPath, args)
		if err != nil {
			return nil, fmt.Errorf("deliver pinned profile: %w", err)
		}
	}
	if in.Choice.Model != "" {
		args = append([]string{"--model", in.Choice.Model}, args...)
	}
	return args, nil
}

func startAgent(ctx context.Context, h Server, in Input, paneID string, args []string) error {
	options := herdr.StartAgentOptions{
		Name:   orchestratorName,
		Kind:   in.Choice.Harness,
		PaneID: paneID,
		Args:   args,
	}
	if _, err := h.StartAgent(ctx, options); err != nil {
		return logFail(in.Log, halted(ctx, fmt.Errorf("start %s: %w", in.Choice.Harness, err)))
	}
	logStep(in.Log, "started %s in pane %s", in.Choice.Harness, paneID)
	return nil
}

// halted reports cancellation when the context ended, so a subprocess killed
// by Herder's exit is not reported as a bootstrap failure.
func halted(ctx context.Context, err error) error {
	if err != nil && (errors.Is(err, context.Canceled) || errors.Is(herdr.ContextCause(err), context.Canceled)) && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// poll calls ready until it succeeds, reporting why the context ended first.
func poll(ctx context.Context, interval time.Duration, ready func(context.Context) bool) error {
	for {
		if ready(ctx) {
			return nil
		}
		if err := utils.Sleep(ctx, interval); err != nil {
			return err
		}
	}
}

func logStep(log io.Writer, format string, args ...any) {
	fmt.Fprintf(log, "%s %s\n", time.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func logFail(log io.Writer, err error) error {
	logStep(log, "failed: %v", err)
	return err
}
