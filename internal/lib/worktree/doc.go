// Package worktree manages Fledge's checkouts and the Herdr worktree requests
// that create and open them.
// managed.go validates and prepares managed checkout paths under .fledge/worktrees;
// request.go builds and validates worktree.list, worktree.create, and worktree.open.
package worktree
