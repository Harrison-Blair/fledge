// Package create implements worktree create: a new managed checkout opened
// as a Herdr workspace, with no agent.
package create

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/worktree"
)

// Options names the new branch, an optional base ref, and a directory inside
// the repository; an empty Cwd means the process working directory.
type Options struct{ Branch, Base, Cwd string }
type Result struct {
	Path        string `json:"path"`
	Branch      string `json:"branch"`
	WorkspaceID string `json:"workspace_id"`
}

func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "worktree.create", Status: "success", Effects: []libagent.Effect{}}
	if o.Branch == "" {
		out.Fail(libagent.Invalid("--branch is required"), "validation", false)
		return out
	}
	cwd := o.Cwd
	if cwd == "" {
		cwd = c.Cwd
	}
	cwd, err := filepath.Abs(cwd)
	if err == nil {
		cwd, err = fledgedir.Root(ctx, cwd)
	}
	if err != nil {
		out.Fail(err, "preflight", false)
		return out
	}
	src := worktree.Source{Cwd: cwd}
	listing, err := worktree.List(ctx, c, src)
	if err != nil {
		out.Fail(err, "worktree.list", false)
		return out
	}
	path, err := worktree.Prepare(ctx, cwd, o.Branch, &out)
	if err != nil {
		out.Fail(err, "preflight", false)
		return out
	}
	r, err := worktree.Create(ctx, c, src, listing, o.Branch, o.Base, path)
	if err != nil {
		out.Fail(err, "worktree.create", true)
		return out
	}
	out.Effects = append(out.Effects,
		libagent.Effect{Action: "created", Kind: "worktree", Path: r.Worktree.Path},
		libagent.Effect{Action: "created", Kind: "workspace", ID: r.Workspace.ID})
	out.Result = Result{Path: r.Worktree.Path, Branch: o.Branch, WorkspaceID: r.Workspace.ID}
	if _, err = fledgedir.Ensure(r.Worktree.Path, &out); err != nil {
		out.Fail(fmt.Errorf("prepare .fledge in new worktree %s: %w", r.Worktree.Path, err), "worktree.ignore", false)
	}
	return out
}

// Render writes a successful create outcome.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := fmt.Fprintf(w, "Created worktree %s on branch %s in workspace %s.\n", r.Path, r.Branch, r.WorkspaceID)
	return err
}
