package board

import (
	"context"
	"fmt"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// FocusOwner is the only navigation boundary. Ownership and terminal identity
// are revalidated using read-only lookups under one total five-second budget.
// Focus and revalidation cannot be atomic: a mismatched reply is reported, not retried.
func FocusOwner(ctx context.Context, c libagent.Client, id, expectedOwner string) error {
	ctx, cancel := context.WithTimeout(ctx, requestBudget)
	defer cancel()
	s, err := task.Existing(ctx, c.Cwd)
	if err != nil {
		return err
	}
	r, err := task.Get(s, id)
	if err != nil {
		return err
	}
	if r.Owner == nil || *r.Owner != expectedOwner {
		return fmt.Errorf("task owner changed or is absent; refresh before visiting")
	}
	matches, err := selector.Resolve(ctx, c, selector.Filter{Tasks: []string{id}})
	if err != nil {
		return err
	}
	if len(matches) != 1 || matches[0].Record == nil || matches[0].Record.ID != expectedOwner {
		return fmt.Errorf("task owner is not live or its identity changed")
	}
	m := matches[0]
	// Re-read ownership after selection too, so an assignment during agent.list
	// cannot redirect a visit to the old worker.
	current, err := task.Get(s, id)
	if err != nil {
		return err
	}
	if current.Owner == nil || *current.Owner != expectedOwner {
		return fmt.Errorf("task owner changed during navigation")
	}
	var reply herdr.AgentResult
	if err := c.Call(ctx, "agent.focus", map[string]any{"target": m.Agent.PaneID}, &reply); err != nil {
		return err
	}
	if reply.Type != "agent_info" || !libagent.ValidAgentInfo(reply.Agent) || reply.Agent.TerminalID != m.Record.TerminalID || reply.Agent.PaneID != m.Agent.PaneID || identity.Mismatched(*m.Record, reply.Agent) {
		return fmt.Errorf("post-focus identity mismatch or incomplete response; focus may have changed")
	}
	return nil
}
