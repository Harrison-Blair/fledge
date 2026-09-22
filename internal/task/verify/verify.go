// Package verify implements task verify: a second agent accepting a completed
// task's result once its subtasks are finished.
package verify

import (
	"context"
	"fmt"
	"io"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Options names the task, an optional verification note, and whether to
// override the verifier checks.
type Options struct {
	ID, Summary, File          string
	SummarySet, FileSet, Force bool
}

// Result is the verified task with its direct subtasks that were neither
// verified nor cancelled, which only a forced verification leaves.
type Result struct {
	task.Record
	OpenSubtasks []string `json:"open_subtasks"`
}

// Run moves a completed task to verified. The verifier must be a registered
// agent other than the owner, and every direct subtask must be verified or
// cancelled, unless Force is set; the verifier and whether Force was used are
// recorded. This is a workflow guard, not a security boundary.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) libagent.Outcome {
	out := libagent.Outcome{Operation: "task.verify", Status: "success", Effects: []libagent.Effect{}}
	err := task.ValidateID(o.ID)
	var note string
	if err == nil {
		note, err = libagent.ReadText(in, libagent.TextInput{Body: o.Summary, BodyFlag: "summary", BodySet: o.SummarySet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Noun: "summary"})
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	s, err := task.Existing(ctx, c.Cwd)
	var caller *identity.Record
	if err == nil && s != nil {
		caller, err = identity.Caller(ctx, s, c)
	}
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	open := []string{}
	r, err := task.Update(s, o.ID, func(r *task.Record) error {
		if err := task.Require(r, "verify", task.Completed); err != nil {
			return err
		}
		rs, err := task.List(s)
		if err != nil {
			return err
		}
		for _, child := range rs {
			if child.Parent != nil && *child.Parent == r.ID && child.Status != task.Verified && child.Status != task.Cancelled {
				open = append(open, child.ID)
			}
		}
		switch {
		case o.Force:
		case caller == nil:
			return &herdr.Error{Code: "caller_unregistered", Message: "the caller has no live Fledge record, so it cannot be recorded as the verifier; register with fledge agent adopt, or pass --force"}
		case r.Owner != nil && *r.Owner == caller.ID:
			return &herdr.Error{Code: "task_self_verification", Message: fmt.Sprintf("task %s is owned by the caller (%s); another agent should verify it, or pass --force", r.ID, caller.ID)}
		case len(open) > 0:
			return &herdr.Error{Code: "task_open_subtasks", Message: fmt.Sprintf("task %s has subtasks that are not verified or cancelled: %s; finish them first, or pass --force", r.ID, strings.Join(open, ", "))}
		}
		r.Status, r.VerifiedAt, r.Forced = task.Verified, task.Now(), o.Force
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
	out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "task", ID: r.ID})
	out.Result = Result{Record: r, OpenSubtasks: open}
	return out
}

// Render writes a successful verification.
func Render(w io.Writer, o libagent.Outcome) error {
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
	if _, err := fmt.Fprintf(w, "Verified task %s as %s%s.\n", r.ID, verifier, forced); err != nil || len(r.OpenSubtasks) == 0 {
		return err
	}
	_, err := fmt.Fprintf(w, "Open subtasks: %s.\n", strings.Join(r.OpenSubtasks, ", "))
	return err
}
