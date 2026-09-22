// Package spawn implements agent spawn: launching an agent in a Herdr pane.
// options.go validates flags and native model arguments; spawn.go runs startup, retries, and the first prompt;
// placement.go resolves and creates the destination pane; worktree.go prepares managed checkouts;
// output.go renders results and startup recovery hints.
package spawn
