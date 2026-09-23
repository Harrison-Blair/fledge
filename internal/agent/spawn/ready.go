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

// budget is spawn's --timeout, shared from agent.start through the first
// prompt. ctx cuts off a request still running at the deadline; expired also
// rejects a reply that arrived after it.
type budget struct {
	ctx, parent context.Context
	deadline    time.Time
	now         func() time.Time
	timeout     time.Duration
}

// expired reports the budget ran out, not that the caller cancelled.
func (b budget) expired() bool {
	return b.parent.Err() == nil && (b.ctx.Err() != nil || !b.now().Before(b.deadline))
}
func (b budget) exhausted(name, what string) error {
	return &herdr.Error{Code: "timeout", Message: fmt.Sprintf("agent %s %s within --timeout %s", name, what, b.timeout)}
}

// ready polls the started agent from a, its lifecycle wait result, until Herdr
// admits prompts or a blocked startup dialog ends the wait. agent.wait matches
// status alone, and an agent can report idle while its launch is still pending.
// Each observation must be the started agent: same pane and terminal, spawn's
// name, and no different harness. An observation after the deadline is not
// readiness.
func (s *spawner) ready(b budget, o Options, start, a herdr.AgentDetails) (herdr.AgentDetails, error) {
	notReady := func() error { return b.exhausted(o.Name, "was not ready for input") }
	for {
		if !samePane(a.Pane, start.Pane) || a.TerminalID != start.TerminalID || a.Name == nil || *a.Name != o.Name || (a.Agent != nil && *a.Agent != o.Harness) {
			return a, libagent.Protocol(fmt.Sprintf("a different agent answered for %s in %s", o.Name, start.PaneID))
		}
		if b.expired() {
			return a, notReady()
		}
		settled := a.AgentStatus == "idle" || a.AgentStatus == "done"
		if a.AgentStatus == "blocked" || settled && a.InteractiveReady != nil && *a.InteractiveReady && (a.LaunchPending == nil || !*a.LaunchPending) {
			return a, nil
		}
		if err := s.wait(b.ctx, min(readyPoll, b.deadline.Sub(b.now()))); err != nil {
			if b.expired() {
				return a, notReady()
			}
			return a, err
		}
		var err error
		if a, err = s.Get(b.ctx, start.PaneID); err != nil {
			if b.expired() {
				return a, notReady()
			}
			return a, err
		}
	}
}
