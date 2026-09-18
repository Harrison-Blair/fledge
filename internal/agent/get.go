package agent

import (
	"context"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

// GetOptions selects one live agent by name or hosting pane.
type GetOptions struct{ Name, Pane string }

// Get inspects an agent without focusing its pane or marking output seen.
func (s *Service) Get(ctx context.Context, o GetOptions) Outcome {
	out := Outcome{Operation: "agent.get", Status: "success", Effects: []Effect{}}
	if (o.Name == "") == (o.Pane == "") {
		out.fail(invalid("exactly one of --name or --pane is required"), "validation", false)
		return out
	}
	target := o.Name
	if target == "" {
		target = o.Pane
	}
	var r herdr.AgentGetResult
	err := s.call(ctx, "agent.get", map[string]any{"target": target}, &r)
	if err == nil && (r.Type != "agent_info" || !validAgent(r.Agent.Pane)) {
		err = protocol("incomplete agent.get result")
	}
	if err != nil {
		out.fail(err, "agent.get", false)
		return out
	}
	result := GetResult{AgentRow: row(r.Agent.Pane), ForegroundCwd: r.Agent.ForegroundCwd, InteractiveReady: r.Agent.InteractiveReady, LaunchPending: r.Agent.LaunchPending, Focused: r.Agent.Focused, Title: r.Agent.Title}
	if session := r.Agent.AgentSession; session != nil {
		result.AgentSession = &SessionIdentity{Source: session.Source, Harness: session.Agent, Kind: session.Kind, Value: session.Value}
	}
	out.Result = result
	return out
}
