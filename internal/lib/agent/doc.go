// Package agent is the shared foundation for agent operations.
// outcome.go defines the result envelope, failure classification, and generic rendering;
// client.go performs validated Herdr requests;
// read.go adds agent.read;
// wait.go adds agent.wait with wait's transport deadline policy;
// input.go reads inline, file, or stdin text;
// harness.go holds canonical harness membership;
// name.go holds the agent-name rule;
// header.go attributes delivered prompts to their sender.
package agent
