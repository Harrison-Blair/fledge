package agent

import (
	"context"
	"slices"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// confirmStates are the agent.prompt wait matches for PromptConfirm. From a
// non-working start Herdr first requires observed post-send working or blocked
// activity, so any match there confirms activity, even a turn already finished.
var confirmStates = []string{"working", "done", "idle", "blocked"}

// PromptConfirm submits text like Prompt, then has Herdr wait up to timeout
// for observed activity. The wait tracks lifecycle state, not this prompt: an
// agent already working satisfies it immediately. Herdr raises
// agent_prompt_stalled only after submission, so callers must not resend.
func (c Client) PromptConfirm(ctx context.Context, target, text string, timeout time.Duration) (herdr.AgentDetails, error) {
	var r herdr.AgentResult
	wait := map[string]any{"until": confirmStates, "timeout_ms": timeout.Milliseconds()}
	err := c.Call(ctx, "agent.prompt", map[string]any{"target": target, "text": text, "wait": wait}, &r)
	if err == nil && (r.Type != "agent_prompted" || !ValidAgent(r.Agent.Pane) || !slices.Contains(confirmStates, r.Agent.AgentStatus)) {
		err = Protocol("agent.prompt did not return a confirming agent")
	}
	if err != nil {
		return herdr.AgentDetails{}, err
	}
	return r.Agent, nil
}
