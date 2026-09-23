package spawn

import (
	"context"
	"fmt"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// readyPoll paces agent.get while a settled launch finishes becoming promptable.
const readyPoll = 100 * time.Millisecond

// ready polls the started agent from a, its lifecycle wait result, until Herdr
// admits prompts or a blocked startup dialog ends the wait. agent.wait matches
// status alone, and an agent can report idle while its launch is still pending.
// Each observation must be the started agent: same pane and terminal, spawn's
// name, and no different harness. Polling stops at deadline.
func (s *spawner) ready(ctx context.Context, o Options, start, a herdr.AgentDetails, deadline time.Time) (herdr.AgentDetails, error) {
	for {
		if !samePane(a.Pane, start.Pane) || a.TerminalID != start.TerminalID || a.Name == nil || *a.Name != o.Name || (a.Agent != nil && *a.Agent != o.Harness) {
			return a, libagent.Protocol(fmt.Sprintf("a different agent answered for %s in %s", o.Name, start.PaneID))
		}
		settled := a.AgentStatus == "idle" || a.AgentStatus == "done"
		if a.AgentStatus == "blocked" || settled && a.InteractiveReady != nil && *a.InteractiveReady && (a.LaunchPending == nil || !*a.LaunchPending) {
			return a, nil
		}
		remaining := deadline.Sub(s.now())
		if remaining <= 0 {
			return a, &herdr.Error{Code: "timeout", Message: fmt.Sprintf("agent %s was not ready for input within %s", o.Name, o.Timeout)}
		}
		if err := s.wait(ctx, min(readyPoll, remaining)); err != nil {
			return a, err
		}
		var err error
		if a, err = s.Get(ctx, start.PaneID); err != nil {
			return a, err
		}
	}
}
