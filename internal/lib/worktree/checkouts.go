package worktree

import (
	"context"
	"path/filepath"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/gitstatus"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
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

// Canonical cleans p and resolves symlinks in its longest existing ancestor,
// keeping any missing tail, so a removed checkout still compares by its real path.
func Canonical(p string) string {
	p = filepath.Clean(p)
	dir, tail := p, ""
	for {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(resolved, tail)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return p
		}
		dir, tail = parent, filepath.Join(filepath.Base(dir), tail)
	}
}

// User describes the first of agents using checkout row, or returns "" when
// none does: an agent is in the checkout's open workspace, has its cwd at or
// inside the checkout, or is registered, in records (a LiveByTerminal map),
// with the checkout as its worktree.
func User(agents []herdr.AgentDetails, records map[string]identity.Record, row herdr.Worktree) string {
	for _, a := range agents {
		var where string
		rec, registered := identity.Attributed(records, a)
		switch {
		case row.OpenWorkspaceID != nil && a.WorkspaceID == *row.OpenWorkspaceID:
			where = "is in workspace " + a.WorkspaceID
		case a.Cwd != nil && inside(row.Path, Canonical(*a.Cwd)):
			where = "is working in " + *a.Cwd
		case registered && rec.WorktreePath != nil && inside(row.Path, Canonical(*rec.WorktreePath)):
			where = "is registered to " + row.Path
		default:
			continue
		}
		who := a.PaneID
		if a.Name != nil && *a.Name != "" {
			who = *a.Name + " (" + a.PaneID + ")"
		} else if registered && rec.Name != nil {
			who = *rec.Name + " (" + a.PaneID + ")"
		}
		return "live agent " + who + " " + where
	}
	return ""
}

// inside reports whether p is dir or below it.
func inside(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
