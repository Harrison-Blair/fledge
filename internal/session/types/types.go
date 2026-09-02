// Package types holds the value contracts shared between the session packages.
package types

import "fledge/internal/profile"

// AgentChoice is the resolved agent to run in a fresh session's orchestrator
// pane. An empty Harness leaves the pane at a shell prompt, an empty Model
// accepts the harness default, Args are passed through to the harness, and an
// optional Profile is the immutable managed-profile snapshot for the session.
type AgentChoice struct {
	Harness string
	Model   string
	Args    []string
	Profile *profile.Profile
}
