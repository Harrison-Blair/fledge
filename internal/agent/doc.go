// Package agent spawns and drives the Herder agents of a Fledge session.
//
// Files:
//   - agent.go  Herder client interface, caller resolution, and messaging,
//     listing, and stopping agents
//   - spawn.go  agent placement and Spawn, which creates a pane and starts
//     the harness inside it
package agent
