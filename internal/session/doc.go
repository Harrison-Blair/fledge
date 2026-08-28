// Package session starts and stops the Herder sessions owned by a Fledge
// project checkout, coordinating the record store, the project lock, and the
// bootstrap of a freshly launched session.
//
// Files:
//   - lifecycle.go     Start and Stop plus their dependency seams
//   - resolve.go       RunningSession, the sole running registered session
//   - identity.go      PaneResolver and ValidateAmbientPane for managed callers
//   - confirmation.go  TerminalConfirmer, the interactive stop prompt
package session
