package agent

import (
	"context"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

// GetOptions selects one live agent by name or hosting pane.
type GetOptions struct{ Name, Pane string }

// resolveTitle prefers the agent-reported title, else the stripped terminal title, else the raw terminal title.
func resolveTitle(a herdr.AgentDetails) *string {
	for _, t := range []*string{a.Title, a.TerminalTitleStripped, a.TerminalTitle} {
		if t != nil && *t != "" {
			return t
		}
	}
	return nil
}

// Get inspects an agent without focusing its pane or marking output seen.
func (s *Service) Get(ctx context.Context, o GetOptions) Outcome {
	out := Outcome{Operation: "agent.get", Status: "success", Effects: []Effect{}}
	target, err := resolveTarget(o.Name, o.Pane)
	if err != nil {
		out.fail(err, "validation", false)
		return out
	}
	a, err := s.lookup(ctx, target, &out)
	if err != nil {
		return out
	}
	result := GetResult{AgentRow: row(a.Pane), ForegroundCwd: a.ForegroundCwd, InteractiveReady: a.InteractiveReady, LaunchPending: a.LaunchPending, Focused: a.Focused, Title: resolveTitle(a)}
	if session := a.AgentSession; session != nil {
		result.AgentSession = &SessionIdentity{Source: session.Source, Harness: session.Agent, Kind: session.Kind, Value: session.Value}
	}
	out.Result = result
	return out
}
