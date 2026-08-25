package session

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// StartDependencies contains the external operations used by Start.
type StartDependencies struct {
	Herder  Herder
	Entropy io.Reader
	Now     func() time.Time
	Getenv  func(string) string
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
		unavailable := make(map[string]struct{}, len(sessions))
		for _, listed := range sessions {
			unavailable[listed.Name] = struct{}{}
		}
		record, err := Create(root, unavailable, deps.Entropy, deps.Now())
		if err != nil {
			return fmt.Errorf("start Fledge session: %w", err)
		}
		if err := deps.Herder.Launch(ctx, root, record.HerdrSessionName); err != nil {
			return fmt.Errorf("start Fledge session: launch %q: %w", record.HerdrSessionName, err)
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
