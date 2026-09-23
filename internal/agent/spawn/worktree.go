package spawn

import (
	"context"
	"fmt"
	"path/filepath"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/gitstatus"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/worktree"
)

func (s *spawner) worktreePlacement(ctx context.Context, o Options, snap *herdr.Snapshot, out *libagent.Outcome) (herdr.Pane, error) {
	source, err := workspace(snap, o.Workspace, o.WorkspaceID)
	if err != nil {
		return herdr.Pane{}, err
	}
	if o.Workspace != "" && source == "" {
		return herdr.Pane{}, libagent.Invalid("source workspace %q does not exist", o.Workspace)
	}
	src := worktree.Source{WorkspaceID: source}
	if source == "" {
		cwd := o.Cwd
		if cwd == "" {
			if o.Worktree == "new" {
				cwd = s.Cwd
			} else {
				cwd = o.Worktree
			}
		}
		if cwd == "" {
			return herdr.Pane{}, fmt.Errorf("cannot resolve source working directory")
		}
		cwd, err = filepath.Abs(cwd)
		if err != nil {
			return herdr.Pane{}, err
		}
		src.Cwd = cwd
	}
	listing, err := worktree.List(ctx, s, src)
	if err != nil {
		return herdr.Pane{}, err
	}
	method := "worktree.open"
	path := o.Worktree
	branch := o.Branch
	if o.Worktree == "new" {
		if branch == "" {
			branch = o.Name
		}
		path, err = worktree.Prepare(ctx, listing.Source.RepoRoot, branch, out)
		if err != nil {
			return herdr.Pane{}, err
		}
		method = "worktree.create"
	} else {
		path, err = filepath.Abs(path)
		if err != nil {
			return herdr.Pane{}, err
		}
		// Resolve known named-tab conflicts before opening a workspace.
		for _, w := range listing.Worktrees {
			if filepath.Clean(w.Path) == path && w.OpenWorkspaceID != nil && o.Tab != "" {
				t, err := tab(snap, *w.OpenWorkspaceID, o.Tab, "")
				if err != nil {
					return herdr.Pane{}, err
				}
				if t != nil {
					if _, err = anchor(snap, *t); err != nil {
						return herdr.Pane{}, err
					}
				}
			}
		}
	}
	out.Result.(*Result).WorktreePath = &path
	var r herdr.CreatedResult
	var base *string
	if method == "worktree.create" {
		// Herdr creates from the primary checkout's HEAD when no base is given.
		base = libagent.Pointer(o.Base)
		if base == nil {
			base = gitstatus.Branch(ctx, listing.Source.RepoRoot)
		}
		r, err = worktree.Create(ctx, s, src, listing, branch, o.Base, path)
	} else {
		r, err = worktree.Open(ctx, s, src, listing, path)
	}
	if err != nil {
		out.Fail(err, method, true)
		return herdr.Pane{}, err
	}
	path = r.Worktree.Path
	out.Result.(*Result).WorktreePath = &path
	o.Cwd = path
	s.checkout = &identity.Checkout{Path: path}
	alreadyOpen := r.AlreadyOpen != nil && *r.AlreadyOpen
	if !alreadyOpen {
		if method == "worktree.create" {
			out.Effects = append(out.Effects, libagent.Effect{Action: "created", Kind: "worktree", Path: path})
			s.checkout.Created, s.checkout.Base = true, base
		}
		recordCreated(out, r, true)
		return s.initialTab(ctx, o, r, out)
	}
	out.Effects = append(out.Effects, libagent.Effect{Action: "reused", Kind: "workspace", ID: r.Workspace.ID})
	return s.placeInWorkspace(ctx, o, r.Workspace.ID, snap, out)
}
