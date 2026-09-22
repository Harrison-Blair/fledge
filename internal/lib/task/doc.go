// Package task stores durable task records: a brief, the agent that owns it,
// its outcome, and who verified it. Records live in the repository's state
// store, so they outlive their agents' panes. No status is ever derived from
// Herdr idle or done.
//
//	task.go   records, statuses, and locked store access
//	graph.go  subtask progress derived from parent links
package task
