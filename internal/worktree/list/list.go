// Package list implements worktree list: every checkout of a repository with
// its Herdr workspace, git state, and owning agent.
package list

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/gitstatus"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
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
	// Owner is the earliest registered live agent whose recorded worktree is
	// this checkout; OwnerCount counts every such agent.
	Owner      *Owner `json:"owner"`
	OwnerCount int    `json:"owner_count"`
}

// Owner identifies a registered agent by its record id, name, and pane.
type Owner struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
	Pane string  `json:"pane"`
}

// Result names the integration branch that Merged is checked against, or why
// it could not be chosen.
type Result struct {
	RepoRoot           string  `json:"repo_root"`
	DefaultBranch      *string `json:"default_branch"`
	DefaultBranchError *string `json:"default_branch_error"`
	Worktrees          []Row   `json:"worktrees"`
}

func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "worktree.list", Status: "success", Effects: []libagent.Effect{}}
	r, err := inspectAll(ctx, c, o.Cwd)
	if err != nil {
		out.Fail(err, "worktree.list", false)
		return out
	}
	addOwners(ctx, c, r)
	out.Result = r
	return out
}

// inspectAll lists the repository containing cwd and computes each
// checkout's git state, primary checkout first.
func inspectAll(ctx context.Context, c libagent.Client, cwd string) (Result, error) {
	listing, err := worktree.ListCheckouts(ctx, c, cwd)
	if err != nil {
		return Result{}, err
	}
	root := listing.Root
	r := Result{RepoRoot: root, Worktrees: make([]Row, 0, len(listing.Worktrees))}
	target, err := gitstatus.DefaultBranch(ctx, root)
	if err != nil {
		reason := err.Error()
		r.DefaultBranchError = &reason
	}
	if target != "" {
		short := strings.TrimPrefix(strings.TrimPrefix(target, "refs/heads/"), "refs/remotes/")
		r.DefaultBranch = &short
	}
	managed := filepath.Join(root, ".fledge", "worktrees")
	// Rows are independent, so their git checks run concurrently, a few at a time.
	rows := make([]Row, len(listing.Worktrees))
	limit := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, w := range listing.Worktrees {
		wg.Go(func() {
			limit <- struct{}{}
			defer func() { <-limit }()
			rows[i] = inspect(ctx, root, managed, target, w)
		})
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	var linked []Row
	for _, row := range rows {
		if row.Primary {
			r.Worktrees = append(r.Worktrees, row)
		} else {
			linked = append(linked, row)
		}
	}
	r.Worktrees = append(r.Worktrees, linked...)
	return r, nil
}

// inspect computes the row for checkout w of the repository at root.
func inspect(ctx context.Context, root, managed, target string, w herdr.Worktree) Row {
	row := Row{Path: w.Path, Branch: w.Branch, WorkspaceID: w.OpenWorkspaceID, Primary: w.Path == root}
	row.Dirty, row.Merged = worktree.State(ctx, root, target, w)
	rel, err := filepath.Rel(managed, row.Path)
	row.Managed = err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	return row
}

// addOwners fills each row's owner from live agent records. Owners are
// best effort: a missing or unreadable store, or an unavailable or incomplete
// agent.list, leaves them null without creating anything.
func addOwners(ctx context.Context, c libagent.Client, r Result) {
	s, err := identity.Existing(ctx, r.RepoRoot)
	if err != nil || s == nil {
		return
	}
	records, err := identity.LiveByTerminal(s)
	if err != nil || len(records) == 0 {
		return
	}
	live, err := c.List(ctx)
	if err != nil {
		return
	}
	byPath := map[string][]identity.Record{}
	for _, a := range live {
		rec, ok := identity.Attributed(records, a)
		if ok && rec.WorktreePath != nil {
			p := worktree.Canonical(*rec.WorktreePath)
			byPath[p] = append(byPath[p], rec)
		}
	}
	for i := range r.Worktrees {
		owners := byPath[r.Worktrees[i].Path]
		if len(owners) == 0 {
			continue
		}
		sort.SliceStable(owners, func(a, b int) bool { return owners[a].RegisteredAt < owners[b].RegisteredAt })
		first := owners[0]
		r.Worktrees[i].Owner = &Owner{ID: first.ID, Name: first.Name, Pane: first.Pane}
		r.Worktrees[i].OwnerCount = len(owners)
	}
}

// Render writes a successful list outcome as a table.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "PATH\tBRANCH\tWORKSPACE\tDIRTY\tMERGED\tMANAGED\tOWNER")
	for _, row := range r.Worktrees {
		path, branch, workspace, managed, owner := row.Path, "(detached)", "-", "no", "-"
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
		if row.Owner != nil {
			owner = row.Owner.ID
			if row.Owner.Name != nil && *row.Owner.Name != "" {
				owner = *row.Owner.Name + " (" + row.Owner.ID + ")"
			}
			if row.OwnerCount > 1 {
				owner += fmt.Sprintf(" +%d", row.OwnerCount-1)
			}
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", path, branch, workspace, row.Dirty, row.Merged, managed, owner)
	}
	if err := table.Flush(); err != nil {
		return err
	}
	if r.DefaultBranchError != nil {
		_, err := fmt.Fprintf(w, "MERGED is unknown: %s.\n", *r.DefaultBranchError)
		return err
	}
	return nil
}
