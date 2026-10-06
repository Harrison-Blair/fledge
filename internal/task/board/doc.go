// Package board implements a read-only keyboard task outline and details view.
//
// load.go resolves the repository once, reads independent task/worker snapshots,
// and projects the task graph.
// focus.go revalidates ownership and identity for explicit worker navigation.
// display.go formats selected details as clean terminal text.
// model.go owns keyboard state, asynchronous refresh, and the detail cache.
// view.go renders the responsive outline and details panels.
// run.go validates terminal streams and manages the Bubble Tea lifecycle.
package board
