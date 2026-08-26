package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/project"
)

// Herder is the Herder CLI surface needed to manage Fledge sessions.
type Herder interface {
	List(context.Context) ([]herdr.Session, error)
	Launch(context.Context, string, string) error
	Stop(context.Context, string) error
}

// AgentChoice is the agent to run in a fresh session's orchestrator pane. An
// empty Harness leaves the pane at a shell prompt, and an empty Model accepts
// the harness default.
type AgentChoice struct {
	Harness string
	Model   string
}

// Chooser obtains the agent to run in a fresh session.
type Chooser interface {
	Choose(context.Context) (AgentChoice, error)
}

// Bootstrapper is the Herder surface needed to prepare a fresh session.
type Bootstrapper interface {
	Status(context.Context) (herdr.Status, error)
	Workspaces(context.Context) ([]herdr.Workspace, error)
	Panes(context.Context, string) ([]herdr.Pane, error)
	RenameWorkspace(context.Context, string, string) error
	RenameTab(context.Context, string, string) error
	StartAgent(context.Context, herdr.StartAgentOptions) (herdr.Agent, error)
}

// StartDependencies contains the external operations used by Start.
type StartDependencies struct {
	Herder  Herder
	Entropy io.Reader
	Now     func() time.Time
	Getenv  func(string) string
	Chooser Chooser
	// Scoped addresses the Herder server of one session by name.
	Scoped func(sessionName string) Bootstrapper
	// Diagnostics receives the bootstrap report written after Herder exits.
	Diagnostics io.Writer
}

// Start attaches to the sole running session registered by the nearest Fledge
// project, or publishes and launches a fresh session when none is running.
func Start(ctx context.Context, path string, deps StartDependencies) error {
	if deps.Herder == nil {
		return fmt.Errorf("start Fledge session: Herder client is nil")
	}
	if deps.Now == nil {
		return fmt.Errorf("start Fledge session: clock is nil")
	}
	if deps.Getenv == nil {
		return fmt.Errorf("start Fledge session: environment lookup is nil")
	}
	if deps.Getenv("HERDR_ENV") == "1" {
		return fmt.Errorf("start Fledge session: cannot start Herder from inside Herder")
	}

	root, err := project.Find(path)
	if err != nil {
		return fmt.Errorf("start Fledge session: %w", err)
	}
	records, err := Load(root)
	if err != nil {
		return fmt.Errorf("start Fledge session: %w", err)
	}
	sessions, err := deps.Herder.List(ctx)
	if err != nil {
		return fmt.Errorf("start Fledge session: list Herder sessions: %w", err)
	}

	running := registeredRunningNames(records, sessions)
	switch len(running) {
	case 0:
		if deps.Chooser == nil {
			return fmt.Errorf("start Fledge session: chooser is nil")
		}
		if deps.Scoped == nil {
			return fmt.Errorf("start Fledge session: scoped Herder client is nil")
		}
		if deps.Diagnostics == nil {
			return fmt.Errorf("start Fledge session: diagnostics is nil")
		}
		unavailable := make(map[string]struct{}, len(sessions))
		for _, listed := range sessions {
			unavailable[listed.Name] = struct{}{}
		}
		record, err := Create(root, unavailable, deps.Entropy, deps.Now())
		if err != nil {
			return fmt.Errorf("start Fledge session: %w", err)
		}
		choice, err := deps.Chooser.Choose(ctx)
		if err != nil {
			return fmt.Errorf("start Fledge session: choose agent: %w", err)
		}

		logPath := filepath.Join(record.Path, bootstrapLogName)
		logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		var log io.Writer = logFile
		if err != nil {
			log = io.Discard
			fmt.Fprintf(deps.Diagnostics, "fledge: cannot write %s: %v\n", logPath, err)
		} else {
			defer logFile.Close()
		}

		// Herder needs its own terminal, so the session is prepared through the
		// CLI while its TUI holds the foreground.
		bootCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			done <- bootstrap(bootCtx, deps.Scoped(record.HerdrSessionName), bootstrapInput{
				Root:   root,
				Choice: choice,
				Log:    log,
			}, defaultBootstrapTiming)
		}()

		launchErr := deps.Herder.Launch(ctx, root, record.HerdrSessionName)
		cancel()
		bootErr := <-done

		if launchErr != nil {
			return fmt.Errorf("start Fledge session: launch %q: %w", record.HerdrSessionName, launchErr)
		}
		// Quitting Herder before the bootstrap finishes cancels it, which is a
		// choice rather than a failure.
		if bootErr != nil && !errors.Is(bootErr, context.Canceled) {
			fmt.Fprintf(deps.Diagnostics, "fledge: session bootstrap failed (see %s): %v\n", logPath, bootErr)
			return fmt.Errorf("start Fledge session: bootstrap: %w", bootErr)
		}
		return nil
	case 1:
		if err := deps.Herder.Launch(ctx, root, running[0]); err != nil {
			return fmt.Errorf("start Fledge session: launch %q: %w", running[0], err)
		}
		return nil
	default:
		return fmt.Errorf("start Fledge session: multiple registered sessions are running: %s", strings.Join(running, ", "))
	}
}

// Confirmer obtains approval to stop an immutable session snapshot.
type Confirmer interface {
	Confirm(projectRoot string, names []string, selfStop bool) (bool, error)
}

// StopDependencies contains the external operations used by Stop.
type StopDependencies struct {
	Herder    Herder
	Confirmer Confirmer
	Output    io.Writer
	Getenv    func(string) string
}

// Stop confirms and stops all running sessions registered by the nearest
// Fledge project. Local records are retained.
func Stop(ctx context.Context, path string, deps StopDependencies) error {
	if deps.Herder == nil {
		return fmt.Errorf("stop Fledge sessions: Herder client is nil")
	}
	if deps.Output == nil {
		return fmt.Errorf("stop Fledge sessions: output is nil")
	}
	if deps.Getenv == nil {
		return fmt.Errorf("stop Fledge sessions: environment lookup is nil")
	}

	root, err := project.Find(path)
	if err != nil {
		return fmt.Errorf("stop Fledge sessions: %w", err)
	}
	records, err := Load(root)
	if err != nil {
		return fmt.Errorf("stop Fledge sessions: %w", err)
	}
	sessions, err := deps.Herder.List(ctx)
	if err != nil {
		return fmt.Errorf("stop Fledge sessions: list Herder sessions: %w", err)
	}

	targets := registeredRunningNames(records, sessions)
	if len(targets) == 0 {
		if _, err := fmt.Fprintf(deps.Output, "No running Fledge sessions for %q.\n", root); err != nil {
			return fmt.Errorf("stop Fledge sessions: report status: %w", err)
		}
		return nil
	}
	if deps.Confirmer == nil {
		return fmt.Errorf("stop Fledge sessions: confirmer is nil")
	}

	current := deps.Getenv("HERDR_SESSION")
	selfStop := contains(targets, current)
	confirmed, err := deps.Confirmer.Confirm(root, append([]string(nil), targets...), selfStop)
	if err != nil {
		return fmt.Errorf("stop Fledge sessions: confirm: %w", err)
	}
	if !confirmed {
		return nil
	}

	stopOrder := append([]string(nil), targets...)
	if selfStop {
		stopOrder = moveLast(stopOrder, current)
	}
	var failures []error
	for _, name := range stopOrder {
		if err := deps.Herder.Stop(ctx, name); err != nil {
			failures = append(failures, fmt.Errorf("stop %q: %w", name, err))
		}
	}
	if len(failures) != 0 {
		return fmt.Errorf("stop Fledge sessions: %w", errors.Join(failures...))
	}
	return nil
}

func registeredRunningNames(records []Record, sessions []herdr.Session) []string {
	registered := make(map[string]struct{}, len(records))
	for _, record := range records {
		registered[record.HerdrSessionName] = struct{}{}
	}

	running := make(map[string]struct{})
	for _, listed := range sessions {
		if !listed.Running {
			continue
		}
		if _, ok := registered[listed.Name]; ok {
			running[listed.Name] = struct{}{}
		}
	}

	names := make([]string, 0, len(running))
	for name := range running {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func contains(names []string, target string) bool {
	if target == "" {
		return false
	}
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

func moveLast(names []string, target string) []string {
	for i, name := range names {
		if name != target {
			continue
		}
		copy(names[i:], names[i+1:])
		names[len(names)-1] = target
		break
	}
	return names
}
