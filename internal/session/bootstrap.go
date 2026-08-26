package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"fledge/internal/herdr"
)

const (
	bootstrapLogName = "bootstrap.log"
	orchestratorName = "orchestrator"
	workspacePrefix  = "fledge-"
)

// bootstrapInput describes the session being prepared.
type bootstrapInput struct {
	Root   string
	Choice AgentChoice
	Log    io.Writer
}

// bootstrapTiming bounds the polling a fresh Herder server requires.
type bootstrapTiming struct {
	Poll         time.Duration
	Deadline     time.Duration
	StartRetries int
	RetryDelay   time.Duration
}

var defaultBootstrapTiming = bootstrapTiming{
	Poll:         200 * time.Millisecond,
	Deadline:     30 * time.Second,
	StartRetries: 3,
	RetryDelay:   time.Second,
}

// bootstrap labels a fresh session's first workspace and tab, then starts the
// chosen agent in its root pane. It runs while Herder owns the terminal, so
// every step is reported to in.Log rather than to the user.
func bootstrap(ctx context.Context, h Bootstrapper, in bootstrapInput, t bootstrapTiming) error {
	ctx, cancel := context.WithTimeout(ctx, t.Deadline)
	defer cancel()

	logStep(in.Log, "bootstrap started")

	// Herder reports failure until its socket exists, so errors here mean the
	// server is not up yet.
	if err := poll(ctx, t.Poll, func(ctx context.Context) bool {
		status, err := h.Status(ctx)
		return err == nil && status.Running
	}); err != nil {
		return logFail(in.Log, fmt.Errorf("Herder server did not start: %w", err))
	}
	logStep(in.Log, "Herder server running")

	var workspace herdr.Workspace
	if err := poll(ctx, t.Poll, func(ctx context.Context) bool {
		workspaces, err := h.Workspaces(ctx)
		if err != nil || len(workspaces) == 0 {
			return false
		}
		workspace = workspaces[0]
		return true
	}); err != nil {
		return logFail(in.Log, fmt.Errorf("list workspaces: %w", err))
	}
	tabID := workspace.ActiveTabID

	var pane herdr.Pane
	if err := poll(ctx, t.Poll, func(ctx context.Context) bool {
		panes, err := h.Panes(ctx, workspace.ID)
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
	logStep(in.Log, "workspace %s tab %s pane %s", workspace.ID, tabID, pane.ID)

	label := workspacePrefix + filepath.Base(in.Root)
	if err := h.RenameWorkspace(ctx, workspace.ID, label); err != nil {
		return logFail(in.Log, halted(ctx, fmt.Errorf("rename workspace %s: %w", workspace.ID, err)))
	}
	logStep(in.Log, "renamed workspace to %s", label)

	if err := h.RenameTab(ctx, tabID, orchestratorName); err != nil {
		return logFail(in.Log, halted(ctx, fmt.Errorf("rename tab %s: %w", tabID, err)))
	}
	logStep(in.Log, "renamed tab to %s", orchestratorName)

	if in.Choice.Harness == "" {
		logStep(in.Log, "no agent requested")
		return nil
	}
	return startAgent(ctx, h, in, t, pane.ID)
}

// startAgent launches the chosen harness, retrying while Herder rejects the
// request because the fresh pane has not reached its shell prompt.
func startAgent(ctx context.Context, h Bootstrapper, in bootstrapInput, t bootstrapTiming, paneID string) error {
	options := herdr.StartAgentOptions{
		Name:   orchestratorName,
		Kind:   in.Choice.Harness,
		PaneID: paneID,
	}
	if in.Choice.Model != "" {
		options.Args = []string{"--model", in.Choice.Model}
	}

	for attempt := 1; ; attempt++ {
		_, err := h.StartAgent(ctx, options)
		if err == nil {
			logStep(in.Log, "started %s in pane %s", in.Choice.Harness, paneID)
			return nil
		}
		var reported *herdr.Error
		if !errors.As(err, &reported) || reported.Code != "agent_pane_busy" || attempt >= t.StartRetries {
			return logFail(in.Log, halted(ctx, fmt.Errorf("start %s: %w", in.Choice.Harness, err)))
		}
		logStep(in.Log, "start %s attempt %d failed: %v", in.Choice.Harness, attempt, err)
		if err := sleep(ctx, t.RetryDelay); err != nil {
			return logFail(in.Log, fmt.Errorf("start %s: %w", in.Choice.Harness, err))
		}
	}
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
		if err := sleep(ctx, interval); err != nil {
			return err
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func logStep(log io.Writer, format string, args ...any) {
	fmt.Fprintf(log, "%s %s\n", time.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func logFail(log io.Writer, err error) error {
	logStep(log, "failed: %v", err)
	return err
}
