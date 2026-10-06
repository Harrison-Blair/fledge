// Package agent performs Herdr agent requests for agent operations.
// outcome.go holds temporary aliases to the lib/cli result envelope;
// row.go reports a live pane's agent fields;
// fanout.go summarizes a fan-out's failed targets as one failure;
// client.go performs validated Herdr requests;
// read.go adds agent.read;
// wait.go adds agent.wait with wait's transport deadline policy;
// status.go holds live agent_status membership and the settled subset;
// harness.go validates --harness values against lib/harness;
// id.go validates record id flag values;
// name.go holds the agent-name rule;
// label.go renames an agent and labels its pane and, when alone, its tab;
// header.go attributes delivered prompts to their sender;
// prompt_wait.go adds agent.prompt with a wait for confirmed activity.
package agent
