// Package agent spawns and drives the Herder agents of a Fledge session.
//
// Files:
//   - agent.go            Herder client interface, caller resolution, messaging,
//     listing, and stopping agents
//   - spawn.go             agent placement and Spawn, which creates a pane and
//     starts the harness inside it
//   - profile_artifact.go file-backed profile snapshots for spawned agents
package agent
