// Package types holds the value contracts shared between the session packages.
// It depends on nothing else in Fledge so any session package may import it.
package types

// AgentChoice is the agent to run in a fresh session's orchestrator pane. An
// empty Harness leaves the pane at a shell prompt, and an empty Model accepts
// the harness default.
type AgentChoice struct {
	Harness string
	Model   string
}
