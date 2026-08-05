package agentcontext

import "github.com/Harrison-Blair/fledge/internal/herdr"

// inFlightStatus is the Herdr agent_status that marks an agent as actively
// working, i.e. mid-turn. Only this status suppresses the figure as
// agent_working; idle, blocked, and done agents are read normally.
const inFlightStatus = "working"

// LiveAgents projects a Herdr snapshot onto the minimal view the report needs.
// Only named agents are included — the orchestrator among them — so anonymous
// split panes are skipped. The harness and native session ref come from Herdr's
// agent_session correlation, never from cwd or pane heuristics, and InFlight is
// taken from the authoritative agent_status.
//
// This adapter is the one place the package touches the herdr types; the Build
// core stays decoupled from them.
func LiveAgents(snapshot herdr.Snapshot) []LiveAgent {
	agents := make([]LiveAgent, 0, len(snapshot.Agents))
	for _, agent := range snapshot.Agents {
		if agent.Name == nil {
			continue
		}
		live := LiveAgent{
			Name:     *agent.Name,
			Revision: agent.Revision,
			InFlight: agent.AgentStatus == inFlightStatus,
		}
		if agent.Agent != nil {
			live.Harness = *agent.Agent
		}
		if agent.AgentSession != nil {
			if agent.AgentSession.Agent != "" {
				live.Harness = agent.AgentSession.Agent
			}
			live.Ref = Ref{Kind: agent.AgentSession.Kind, Value: agent.AgentSession.Value}
		}
		agents = append(agents, live)
	}
	return agents
}
