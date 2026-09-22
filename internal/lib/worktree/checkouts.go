package worktree

import (
	"context"
	"path/filepath"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/gitstatus"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// Checkouts is a repository's checkouts as Herdr lists them, with Root (the
// primary checkout) and every checkout path canonical, so paths compare equal
// however the directory used to reach the repository was spelled.
type Checkouts struct {
	Root      string
	Worktrees []herdr.Worktree
}

// ListCheckouts lists the repository containing cwd, or the process working
// directory of c when cwd is empty.
func ListCheckouts(ctx context.Context, c libagent.Client, cwd string) (Checkouts, error) {
	if cwd == "" {
		cwd = c.Cwd
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return Checkouts{}, err
	}
	listing, err := List(ctx, c, Source{Cwd: cwd})
	if err != nil {
		return Checkouts{}, err
	}
	r := Checkouts{Root: Canonical(listing.Source.RepoRoot), Worktrees: listing.Worktrees}
	for i := range r.Worktrees {
		r.Worktrees[i].Path = Canonical(r.Worktrees[i].Path)
	}
	return r, nil
}

// State reports whether checkout w of the repository at root is dirty, and
// whether its branch, or its HEAD when detached, is merged into target.
func State(ctx context.Context, root, target string, w herdr.Worktree) (dirty, merged string) {
	dirty = gitstatus.Dirty(ctx, w.Path)
	if w.Branch != nil {
		return dirty, gitstatus.Merged(ctx, root, "refs/heads/"+*w.Branch, target)
	}
	return dirty, gitstatus.Merged(ctx, w.Path, "HEAD", target)
}

// Canonical cleans p and resolves its symlinks when it exists.
func Canonical(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}
