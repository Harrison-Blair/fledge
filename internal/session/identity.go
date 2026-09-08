package session

import (
	"context"
	"fmt"
	"strings"

	"fledge/internal/herdr"
)

// PaneResolver is the Herder surface needed to validate a managed caller.
type PaneResolver interface {
	CurrentPane(context.Context) (herdr.Pane, error)
}

// ValidateAmbientPane verifies that injected Herder identity belongs to one of
// the current project's allowed sessions and returns its authoritative pane.
func ValidateAmbientPane(ctx context.Context, getenv func(string) string, allowed []string, scoped func(string) PaneResolver) (string, herdr.Pane, error) {
	if getenv == nil {
		return "", herdr.Pane{}, fmt.Errorf("validate Herder context: environment lookup is nil")
	}
	sessionName := getenv("HERDR_SESSION")
	paneID := getenv("HERDR_PANE_ID")
	if sessionName == "" || paneID == "" {
		return "", herdr.Pane{}, fmt.Errorf("validate Herder context: managed caller is missing HERDR_SESSION or HERDR_PANE_ID")
	}
	if !contains(allowed, sessionName) {
		return "", herdr.Pane{}, fmt.Errorf("validate Herder context: session %q does not belong to the current Fledge project (allowed: %s)", sessionName, strings.Join(allowed, ", "))
	}
	if scoped == nil {
		return "", herdr.Pane{}, fmt.Errorf("validate Herder context: scoped pane resolver is nil")
	}
	resolver := scoped(sessionName)
	if resolver == nil {
		return "", herdr.Pane{}, fmt.Errorf("validate Herder context: scoped pane resolver for session %q is nil", sessionName)
	}
	pane, err := resolver.CurrentPane(ctx)
	if err != nil {
		return "", herdr.Pane{}, fmt.Errorf("validate Herder context: resolve pane %q in session %q: %w", paneID, sessionName, err)
	}
	if pane.ID != paneID {
		return "", herdr.Pane{}, fmt.Errorf("validate Herder context: pane %q is stale for session %q (current pane is %q)", paneID, sessionName, pane.ID)
	}
	return sessionName, pane, nil
}
