// Package agent is the shared foundation for agent operations.
// outcome.go defines the result envelope, failure classification, and generic rendering;
// fanout.go summarizes a fan-out's failed targets as one failure;
// client.go performs validated Herdr requests;
// read.go adds agent.read;
// wait.go adds agent.wait with wait's transport deadline policy;
// input.go reads inline, file, or stdin text;
// status.go holds live agent_status membership and the settled subset;
// harness.go validates --harness values against lib/harness;
// id.go validates record id flag values;
// name.go holds the agent-name rule;
// label.go renames an agent and labels its pane and, when alone, its tab;
// header.go attributes delivered prompts to their sender;
// prompt_wait.go adds agent.prompt with a wait for confirmed activity.
package agent
