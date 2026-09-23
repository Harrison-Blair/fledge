// Package cleanup implements agent cleanup: retiring the caller's finished
// spawned workers and removing the checkouts their spawns created once those
// are clean and merged into the branch they were created from.
// cleanup.go carries out a plan by stopping workers and removing checkouts through agent stop and worktree remove;
// plan.go selects the caller's workers and checkouts read-only and decides each one;
// output.go renders a plan or its results.
package cleanup
