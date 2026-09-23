// Package worktree manages Fledge's checkouts and the Herdr worktree requests
// that create and open them.
// checkouts.go lists a repository's checkouts with canonical paths and reports each one's git state and live users;
// managed.go validates and prepares managed checkout paths under .fledge/worktrees;
// request.go builds and validates worktree.list, worktree.create, and worktree.open.
package worktree
