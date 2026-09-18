package agent

import "context"

// GetOptions selects one live agent by name or hosting pane.
type GetOptions struct{ Name, Pane string }

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
	result := GetResult{AgentRow: row(a.Pane), ForegroundCwd: a.ForegroundCwd, InteractiveReady: a.InteractiveReady, LaunchPending: a.LaunchPending, Focused: a.Focused, Title: a.Title}
	if session := a.AgentSession; session != nil {
		result.AgentSession = &SessionIdentity{Source: session.Source, Harness: session.Agent, Kind: session.Kind, Value: session.Value}
	}
	out.Result = result
	return out
}
