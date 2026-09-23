// Package verify implements task verify: a second agent accepting a completed
// task's result once its subtasks are finished, verifying a grouping parent
// whose subtasks are finished, or verifying a task again.
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

// Run moves a completed task to verified, or verifies a verified task again.
// A created or assigned parent can also be verified once every direct subtask
// is verified or cancelled and at least one is verified.
// The verifier must be a registered agent other than the owner, and every
// direct subtask must be verified or cancelled, unless Force is set; the
// verifier and whether Force was used are recorded. A repeat verification
// replaces the previous verifier, note, time, and Force flag, keeping only the
// latest. This is a workflow guard, not a security boundary.
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
		rs, err := task.List(s)
		if err != nil {
			return err
		}
		children, verified := 0, 0
		for _, child := range rs {
			if child.Parent == nil || *child.Parent != r.ID {
				continue
			}
			children++
			switch child.Status {
			case task.Verified:
				verified++
			case task.Cancelled:
			default:
				open = append(open, child.ID)
			}
		}
		if err := requireVerifiable(r, children, verified, open); err != nil {
			return err
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
		r.Status, r.VerifiedAt, r.Forced, r.Verifier, r.VerificationNote = task.Verified, task.Now(), o.Force, nil, nil
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

// requireVerifiable accepts a completed or verified task, or a created or
// assigned parent whose direct subtasks are all verified or cancelled with at
// least one verified. Force never widens this.
func requireVerifiable(r *task.Record, children, verified int, open []string) error {
	if children == 0 || (r.Status != task.Created && r.Status != task.Assigned) {
		return task.Require(r, "verify", task.Completed, task.Verified)
	}
	if len(open) > 0 {
		return &herdr.Error{Code: "task_open_subtasks", Message: fmt.Sprintf("task %s is %s and has subtasks that are not verified or cancelled: %s; finish them first", r.ID, r.Status, strings.Join(open, ", "))}
	}
	if verified == 0 {
		return &herdr.Error{Code: "task_invalid_state", Message: fmt.Sprintf("task %s is %s and all its subtasks were cancelled; cancel it instead with fledge task cancel --id %s", r.ID, r.Status, r.ID)}
	}
	return nil
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
