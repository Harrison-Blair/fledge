package session

import (
	"context"
	"fmt"
	"strings"

	"fledge/internal/herdr"
	"fledge/internal/project"
)

// RunningSession returns the sole running Herder session registered by the
// project containing path.
func RunningSession(ctx context.Context, path string, list func(context.Context) ([]herdr.Session, error)) (string, error) {
	root, err := project.Find(path)
	if err != nil {
		return "", fmt.Errorf("resolve Fledge session: %w", err)
	}
	records, err := Load(root)
	if err != nil {
		return "", fmt.Errorf("resolve Fledge session: %w", err)
	}
	sessions, err := list(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve Fledge session: list Herder sessions: %w", err)
	}

	running := registeredRunningNames(records, sessions)
	switch len(running) {
	case 1:
		return running[0], nil
	case 0:
		return "", fmt.Errorf("resolve Fledge session: no running Fledge session for %q", root)
	default:
		return "", fmt.Errorf("resolve Fledge session: multiple registered sessions are running: %s", strings.Join(running, ", "))
	}
}
