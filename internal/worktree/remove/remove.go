// Package remove implements worktree remove: deleting a linked checkout while
// keeping its branch, guarded against live agents and unsaved work.
package remove

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/gitstatus"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/worktree"
)

// Options names the checkout by exactly one of Path or Branch. Force permits
// removing dirty or unmerged checkouts; it never overrides the live-agent guard.
// Base, when set, is the ref the merged check uses instead of the repository
// integration branch, as for a checkout created from that ref.
type Options struct {
	Path, Branch, Cwd, Base string
	Force                   bool
}
type Result struct {
	Path              string  `json:"path"`
	Branch            *string `json:"branch"`
	ClosedWorkspaceID *string `json:"closed_workspace_id"`
	Forced            bool    `json:"forced"`
}

func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "worktree.remove", Status: "success", Effects: []libagent.Effect{}}
	if (o.Path == "") == (o.Branch == "") {
		out.Fail(libagent.Invalid("exactly one of --path or --branch is required"), "validation", false)
		return out
	}
	listing, err := worktree.ListCheckouts(ctx, c, o.Cwd)
	if err != nil {
		out.Fail(err, "worktree.list", false)
		return out
	}
	row, err := target(listing, o)
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	if row.Path == listing.Root {
		out.Fail(libagent.Invalid("%s is the primary checkout, which is never removed", row.Path), "guard", false)
		return out
	}
	if err = checkAgents(ctx, c, listing.Root, row); err != nil {
		out.Fail(err, "guard", false)
		return out
	}
	if !o.Force {
		target, targetErr := o.Base, error(nil)
		if target == "" {
			target, targetErr = gitstatus.DefaultBranch(ctx, listing.Root)
		}
		dirty, merged := worktree.State(ctx, listing.Root, target, row)
		var reasons []string
		if dirty != "no" {
			reasons = append(reasons, "dirty: "+dirty)
		}
		if merged != "yes" {
			reason := "merged: " + merged
			if targetErr != nil {
				reason += " (" + targetErr.Error() + ")"
			}
			reasons = append(reasons, reason)
		}
		if len(reasons) > 0 {
			out.Fail(libagent.Invalid("worktree %s is not known to be clean and merged (%s); pass --force to remove it anyway", row.Path, strings.Join(reasons, ", ")), "guard", false)
			return out
		}
	}
	// Recheck immediately before removal: an agent may have started since.
	if err = checkAgents(ctx, c, listing.Root, row); err != nil {
		out.Fail(err, "guard", false)
		return out
	}
	result := Result{Path: row.Path, Branch: row.Branch, Forced: o.Force}
	if row.OpenWorkspaceID == nil {
		args := []string{"-C", listing.Root, "worktree", "remove"}
		if o.Force {
			args = append(args, "--force")
		}
		if b, err := exec.CommandContext(ctx, "git", append(args, "--", row.Path)...).CombinedOutput(); err != nil {
			out.Fail(fmt.Errorf("git worktree remove: %v: %s", err, strings.TrimSpace(string(b))), "git worktree remove", false)
			return out
		}
		out.Effects = append(out.Effects, libagent.Effect{Action: "removed", Kind: "worktree", Path: row.Path})
		out.Result = result
		return out
	}
	var r struct {
		Type        string `json:"type"`
		WorkspaceID string `json:"workspace_id"`
		Path        string `json:"path"`
	}
	err = c.Call(ctx, "worktree.remove", map[string]any{"workspace_id": *row.OpenWorkspaceID, "force": o.Force}, &r)
	if err == nil && (r.Type != "worktree_removed" || r.Path == "") {
		err = libagent.Protocol("incomplete worktree.remove result")
	}
	if err != nil {
		out.Fail(err, "worktree.remove", true)
		return out
	}
	result.Path = filepath.Clean(r.Path)
	result.ClosedWorkspaceID = row.OpenWorkspaceID
	out.Effects = append(out.Effects,
		libagent.Effect{Action: "removed", Kind: "worktree", Path: result.Path},
		libagent.Effect{Action: "closed", Kind: "workspace", ID: *row.OpenWorkspaceID})
	out.Result = result
	return out
}

// target selects the checkout named by o, comparing canonical paths.
func target(r worktree.Checkouts, o Options) (herdr.Worktree, error) {
	path := o.Path
	if path != "" {
		abs, err := filepath.Abs(path)
		if err != nil {
			return herdr.Worktree{}, err
		}
		path = worktree.Canonical(abs)
	}
	for _, row := range r.Worktrees {
		if (path != "" && row.Path == path) || (o.Branch != "" && row.Branch != nil && *row.Branch == o.Branch) {
			return row, nil
		}
	}
	if path != "" {
		return herdr.Worktree{}, libagent.Invalid("no worktree of %s at %s", r.Root, path)
	}
	return herdr.Worktree{}, libagent.Invalid("no worktree of %s has branch %s checked out", r.Root, o.Branch)
}

// checkAgents refuses when any live agent in the connected Herdr session is in
// the checkout's workspace, has its cwd at or inside the checkout, or is
// registered with the checkout as its worktree. Unreadable agent records and
// incomplete agent.list entries fail closed.
func checkAgents(ctx context.Context, c libagent.Client, repo string, row herdr.Worktree) error {
	agents, err := c.List(ctx)
	if err != nil {
		return err
	}
	records := map[string]identity.Record{}
	s, err := identity.Existing(ctx, repo)
	if err == nil && s != nil {
		records, err = identity.LiveByTerminal(s)
	}
	if err != nil {
		return fmt.Errorf("read agent records: %w; repair or remove the bad record under .fledge/state", err)
	}
	if user := worktree.User(agents, records, row); user != "" {
		return libagent.Invalid("%s; stop it first (--force does not override this)", user)
	}
	return nil
}

// Render writes a successful remove outcome.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	line := "Removed worktree " + r.Path
	if r.ClosedWorkspaceID != nil {
		line += " and closed workspace " + *r.ClosedWorkspaceID
	}
	if r.Branch != nil {
		line += "; kept branch " + *r.Branch + "."
	} else {
		line += " (detached HEAD)."
	}
	_, err := fmt.Fprintln(w, line)
	return err
}
