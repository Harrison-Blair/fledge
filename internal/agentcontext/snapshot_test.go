package agentcontext

import (
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

func TestLiveAgentsProjectsNamedAgentsWithStatusAndRef(t *testing.T) {
	t.Parallel()
	orchestrator, worker, idle := "orchestrator", "worker", "idle"
	claude, codex := "claude", "codex"
	snapshot := herdr.Snapshot{
		Agents: []herdr.Agent{
			{Name: &worker, Agent: &codex, Revision: 7, AgentStatus: "working", AgentSession: &herdr.AgentSession{Agent: "codex", Kind: "id", Value: "codex-id"}},
			{Name: &orchestrator, Agent: &claude, Revision: 3, AgentStatus: "blocked", AgentSession: &herdr.AgentSession{Agent: "claude", Kind: "id", Value: "claude-id"}},
			{Name: &idle, Agent: &claude, Revision: 1, AgentStatus: "idle"},
			{Name: nil, Agent: &codex, Revision: 9}, // anonymous pane skipped
		},
	}
	agents := LiveAgents(snapshot)
	if len(agents) != 3 {
		t.Fatalf("len = %d, want 3 (anonymous pane skipped)", len(agents))
	}

	byName := map[string]LiveAgent{}
	for _, a := range agents {
		byName[a.Name] = a
	}
	// Only "working" is in-flight; blocked and idle are not.
	if !byName["worker"].InFlight {
		t.Errorf("worker (working) should be in-flight")
	}
	if byName["orchestrator"].InFlight {
		t.Errorf("orchestrator (blocked) should not be in-flight")
	}
	if byName["idle"].InFlight {
		t.Errorf("idle should not be in-flight")
	}
	// Ref and harness come from agent_session.
	if got := byName["worker"]; got.Harness != "codex" || got.Ref.Value != "codex-id" || got.Ref.Kind != "id" {
		t.Errorf("worker projection = %+v, want codex/codex-id", got)
	}
	if got := byName["idle"]; got.Ref.Value != "" {
		t.Errorf("idle has no agent_session; ref should be empty, got %q", got.Ref.Value)
	}
}
