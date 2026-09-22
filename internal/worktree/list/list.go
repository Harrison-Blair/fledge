// Package list implements worktree list: every checkout of a repository with
// its Herdr workspace and git state.
package list

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/gitstatus"
	"github.com/Harrison-Blair/fledge/internal/lib/worktree"
)

// Options selects the repository by a directory inside it; empty means the
// process working directory.
type Options struct{ Cwd string }

// Row describes one checkout. Dirty and Merged are "yes", "no", or "unknown".
type Row struct {
	Path        string  `json:"path"`
	Branch      *string `json:"branch"`
	Primary     bool    `json:"primary"`
	WorkspaceID *string `json:"workspace_id"`
	Dirty       string  `json:"dirty"`
	Merged      string  `json:"merged"`
	Managed     bool    `json:"managed"`
	// TODO(identity follow-up): report the owning agent from the state store
	// once agent identity merges; until then Owner is always null.
	Owner *string `json:"owner"`
}
type Result struct {
	RepoRoot      string  `json:"repo_root"`
	DefaultBranch *string `json:"default_branch"`
	Worktrees     []Row   `json:"worktrees"`
}

func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "worktree.list", Status: "success", Effects: []libagent.Effect{}}
	r, err := Inspect(ctx, c, o.Cwd)
	if err != nil {
		out.Fail(err, "worktree.list", false)
		return out
	}
	out.Result = r
	return out
}

// Inspect lists the repository containing cwd (the process working directory
// when empty) and computes each checkout's git state, primary checkout first.
func Inspect(ctx context.Context, c libagent.Client, cwd string) (Result, error) {
	if cwd == "" {
		cwd = c.Cwd
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return Result{}, err
	}
	listing, err := worktree.List(ctx, c, worktree.Source{Cwd: cwd})
	if err != nil {
		return Result{}, err
	}
	root := filepath.Clean(listing.Source.RepoRoot)
	r := Result{RepoRoot: root, Worktrees: make([]Row, 0, len(listing.Worktrees))}
	target := gitstatus.DefaultBranch(ctx, root)
	if target != "" {
		short := strings.TrimPrefix(strings.TrimPrefix(target, "refs/heads/"), "refs/remotes/")
		r.DefaultBranch = &short
	}
	managed := filepath.Join(root, ".fledge", "worktrees")
	var linked []Row
	for _, w := range listing.Worktrees {
		row := Row{Path: filepath.Clean(w.Path), Branch: w.Branch, WorkspaceID: w.OpenWorkspaceID}
		row.Primary = row.Path == root
		row.Dirty = gitstatus.Dirty(ctx, row.Path)
		if row.Branch != nil {
			row.Merged = gitstatus.Merged(ctx, root, "refs/heads/"+*row.Branch, target)
		} else {
			row.Merged = gitstatus.Merged(ctx, row.Path, "HEAD", target)
		}
		rel, err := filepath.Rel(managed, row.Path)
		row.Managed = err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
		if row.Primary {
			r.Worktrees = append(r.Worktrees, row)
		} else {
			linked = append(linked, row)
		}
	}
	r.Worktrees = append(r.Worktrees, linked...)
	return r, nil
}

// Render writes a successful list outcome as a table.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "PATH\tBRANCH\tWORKSPACE\tDIRTY\tMERGED\tMANAGED")
	for _, row := range r.Worktrees {
		path, branch, workspace, managed := row.Path, "(detached)", "-", "no"
		if row.Primary {
			path += " (primary)"
		}
		if row.Branch != nil {
			branch = *row.Branch
		}
		if row.WorkspaceID != nil {
			workspace = *row.WorkspaceID
		}
		if row.Managed {
			managed = "yes"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n", path, branch, workspace, row.Dirty, row.Merged, managed)
	}
	return table.Flush()
}
