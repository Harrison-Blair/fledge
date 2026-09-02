// Package session manages the Herder sessions and workspaces owned by a Fledge
// project checkout, coordinating the record store, project lock, and bootstrap
// of a freshly launched session.
//
// Files:
//   - lifecycle.go     Start and Stop plus their dependency seams
//   - workspace.go     lock-protected managed-workspace reconciliation facade
//   - resolve.go       RunningSession, the sole running registered session
//   - identity.go      PaneResolver and ValidateAmbientPane for managed callers
//   - confirmation.go  TerminalConfirmer, the interactive stop prompt
package session
