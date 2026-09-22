package agent

import (
	"context"
	"slices"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// WaitFromEnvironment is FromEnvironment for agent.wait. A zero timeout waits
// indefinitely, so the transport deadline is removed; context cancellation
// still ends the call. A finite timeout keeps FromEnvironment's longer
// transport limit so Herdr's own timeout error is what the caller sees.
func WaitFromEnvironment(timeout time.Duration) Client {
	c := FromEnvironment(timeout)
	if client, ok := c.API.(herdr.Client); ok && timeout == 0 {
		client.NoDeadline = true
		c.API = client
	}
	return c
}

// Wait blocks until target reaches a state in until, or Herdr's default settled
// set (idle, done, blocked) when until is empty. A zero timeout waits indefinitely.
func (c Client) Wait(ctx context.Context, target string, until []string, timeout time.Duration) (herdr.AgentDetails, error) {
	params := map[string]any{"target": target}
	if len(until) > 0 {
		params["until"] = until
	}
	if timeout > 0 {
		params["timeout_ms"] = timeout.Milliseconds()
	}
	matches := until
	if len(matches) == 0 {
		matches = []string{"idle", "done", "blocked"}
	}
	var r herdr.AgentResult
	err := c.Call(ctx, "agent.wait", params, &r)
	if err == nil && (r.Type != "agent_info" || !ValidAgentInfo(r.Agent) || !slices.Contains(matches, r.Agent.AgentStatus)) {
		err = Protocol("agent.wait did not return a matching agent")
	}
	if err != nil {
		return herdr.AgentDetails{}, err
	}
	return r.Agent, nil
}
