// Package verify implements task verify: a second agent accepting a completed
// task's result once its subtasks are finished, verifying a grouping parent
// whose subtasks are finished, or verifying a task again.
package verify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/gitstatus"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/landing"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Options names the task, an optional verification note, an optional
// revision to bind, and whether to override the verifier checks.
type Options struct {
	ID, Summary, File, Commit             string
	SummarySet, FileSet, CommitSet, Force bool
}

// Result is the verified task with its direct subtasks that were neither
// verified nor cancelled, which only a forced verification leaves, and any
// warnings about the bound commit.
type Result struct {
	task.Record
	OpenSubtasks []string `json:"open_subtasks"`
	Warnings     []string `json:"warnings"`
}

// Run moves a completed task to verified, or verifies a verified task again.
// A created or assigned parent can also be verified once every direct subtask
// is verified or cancelled and at least one is verified.
// The verifier must be a registered agent other than the owner, and every
// direct subtask must be verified or cancelled, unless Force is set; the
// verifier and whether Force was used are recorded. A repeat verification
// replaces the previous verifier, note, time, Force flag, and bound commit,
// keeping only the latest. Each verification also clears CarriedFrom and
// VerifiedPatchID. CommitSet resolves Commit in the caller's checkout
// and binds the commit as VerifiedCommit; a dirty checkout adds a warning,
// since the bound commit is then not exactly what was tested. The patch id
// of the bound commit is then stored. When it is unknown or the diff is
// empty, a warning replaces it. A registered verifier's live session ref is
// then stored. This is a workflow guard, not a security boundary.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) cli.Outcome {
	out := cli.NewOutcome("task.verify")
	err := task.ValidateID(o.ID)
	var note string
	if err == nil {
		note, err = cli.ReadText(in, cli.TextInput{Body: o.Summary, BodyFlag: "summary", BodySet: o.SummarySet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Noun: "summary"})
	}
	var commit *string
	warnings := []string{}
	if err == nil && o.CommitSet {
		var hash string
		hash, err = landing.ResolveCommit(ctx, c.Cwd, o.Commit)
		commit = &hash
		if err == nil && dirty(ctx, c.Cwd) {
			warnings = append(warnings, "the checkout has uncommitted changes, so the bound commit "+hash+" is not exactly what was tested")
		}
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	s, err := task.Existing(ctx, c.Cwd)
	var caller *identity.Record
	var live *herdr.AgentDetails
	if err == nil && s != nil {
		caller, live, err = identity.CallerAgent(ctx, s, c)
	}
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	open := []string{}
	r, err := task.Update(s, o.ID, func(r *task.Record) error {
		rs, err := task.List(s)
		if err != nil {
			return err
		}
		for _, child := range rs {
			if child.Parent != nil && *child.Parent == r.ID && !task.Satisfied(child.Status) {
				open = append(open, child.ID)
			}
		}
		if err := requireVerifiable(r, task.ChildProgress(r.ID, rs), open); err != nil {
			return err
		}
		switch {
		case o.Force:
		case caller == nil:
			return &cli.Error{Code: "caller_unregistered", Message: "the caller has no live Fledge record, so it cannot be recorded as the verifier; register with fledge agent adopt, or pass --force"}
		case r.Owner != nil && *r.Owner == caller.ID:
			return &cli.Error{Code: "task_self_verification", Message: fmt.Sprintf("task %s is owned by the caller (%s); another agent should verify it, or pass --force", r.ID, caller.ID)}
		case len(open) > 0:
			return &cli.Error{Code: "task_open_subtasks", Message: fmt.Sprintf("task %s has subtasks that are not verified or cancelled: %s; finish them first, or pass --force", r.ID, strings.Join(open, ", "))}
		}
		r.Status, r.VerifiedAt, r.Forced, r.Verifier, r.VerificationNote, r.VerifiedCommit = task.Verified, task.Now(), o.Force, nil, nil, commit
		r.VerifiedPatchID, r.CarriedFrom = nil, nil
		if commit != nil {
			id, err := patchID(ctx, s, c.Cwd, *r, *commit)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("no patch id is stored, so task land cannot carry this verification to a rebased head: %v", err))
			} else {
				r.VerifiedPatchID = &id
			}
		}
		if caller != nil {
			r.Verifier = &caller.ID
		}
		if note != "" {
			r.VerificationNote = &note
		}
		return nil
	})
	if err != nil {
		out.Fail(err, "task", false)
		return out
	}
	out.Effects = append(out.Effects, cli.Effect{Action: "updated", Kind: "task", ID: r.ID})
	if caller != nil {
		identity.Observe(s, observeSession, *caller, live, time.Now(), &out)
	}
	out.Result = Result{Record: r, OpenSubtasks: open, Warnings: warnings}
	return out
}

// patchID returns the patch id of commit against the base of r. The base is
// the layer base for a stack layer, else the base ref of the task target.
func patchID(ctx context.Context, s *state.Store, cwd string, r task.Record, commit string) (string, error) {
	root, err := fledgedir.Root(ctx, cwd)
	if err != nil {
		return "", err
	}
	id, err := landing.TaskPatchID(ctx, s, root, r, r.DeliveryMode, commit, "verify")
	if err == nil && id == "" {
		err = errors.New("the diff against the base is empty")
	}
	return id, err
}

// dirty reports whether the checkout containing dir has staged, unstaged, or
// untracked changes.
func dirty(ctx context.Context, dir string) bool {
	top, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	return err == nil && gitstatus.Dirty(ctx, strings.TrimSpace(string(top))) == "yes"
}

// requireVerifiable accepts a completed or verified task, or a created or
// assigned parent whose direct subtasks are all verified or cancelled with at
// least one verified. Force never widens this.
func requireVerifiable(r *task.Record, p *task.Progress, open []string) error {
	if p == nil || (r.Status != task.Created && r.Status != task.Assigned) {
		return task.Require(r, "verify", task.Completed, task.Verified)
	}
	if len(open) > 0 {
		return &cli.Error{Code: "task_open_subtasks", Message: fmt.Sprintf("task %s is %s and has subtasks that are not verified or cancelled: %s; finish them first", r.ID, r.Status, strings.Join(open, ", "))}
	}
	if p.Verified == 0 {
		return &cli.Error{Code: "task_invalid_state", Message: fmt.Sprintf("task %s is %s and all its subtasks were cancelled; cancel it instead with fledge task cancel --id %s", r.ID, r.Status, r.ID)}
	}
	return nil
}

// Render writes a successful verification.
func Render(w io.Writer, o cli.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	verifier, forced := "an unregistered caller", ""
	if r.Verifier != nil {
		verifier = *r.Verifier
	}
	if r.Forced {
		forced = " (forced)"
	}
	bound := ""
	if r.VerifiedCommit != nil {
		bound = " at commit " + *r.VerifiedCommit
	}
	if _, err := fmt.Fprintf(w, "Verified task %s as %s%s%s.\n", r.ID, verifier, forced, bound); err != nil {
		return err
	}
	for _, warning := range r.Warnings {
		if _, err := fmt.Fprintf(w, "Warning: %s.\n", warning); err != nil {
			return err
		}
	}
	if len(r.OpenSubtasks) == 0 {
		return nil
	}
	_, err := fmt.Fprintf(w, "Open subtasks: %s.\n", strings.Join(r.OpenSubtasks, ", "))
	return err
}

// observeSession is replaceable so tests can fail the session write.
var observeSession identity.Observer = identity.ObserveSession
