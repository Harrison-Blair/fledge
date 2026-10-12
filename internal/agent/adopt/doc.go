// Package adopt implements agent adopt: registering the caller's own running
// agent with a durable Fledge identity.
// adopt.go validates, registers or names the agent, and renders the result;
// owner.go verifies through Herdr and Linux /proc that the command runs
// inside that agent.
package adopt
