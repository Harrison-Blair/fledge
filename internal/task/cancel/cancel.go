// Package cancel implements task cancel: closing a task that is not yet
// verified or cancelled and reporting the dependents it left ready.
package cancel

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Options names the task and an optional reason, stored when nonempty.
type Options struct{ ID, Reason string }

// Result is the cancelled task with the created tasks it left ready: those
// that ran after it and have no other unmet prerequisite.
type Result struct {
	task.Record
	Unblocked []string `json:"unblocked"`
}

// Run cancels a created, assigned, or completed task. Anyone may cancel.
// Subtasks are left as they are. Dependents keep the cancelled prerequisite,
// which now counts as satisfied; those left ready are found under the same
// store lock.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "task.cancel", Status: "success", Effects: []libagent.Effect{}}
	if err := task.ValidateID(o.ID); err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	s, err := task.Existing(ctx, c.Cwd)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	unblocked := []string{}
	r, err := task.Update(s, o.ID, func(r *task.Record) error {
		if err := task.Require(r, "cancel", task.Created, task.Assigned, task.Completed); err != nil {
			return err
		}
		r.Status, r.CancelledAt = task.Cancelled, task.Now()
		if o.Reason != "" {
			r.CancelReason = &o.Reason
		}
		rs, err := task.List(s)
		if err != nil {
			return err
		}
		byID := task.Index(rs)
		byID[r.ID] = *r
		for _, d := range rs {
			if d.Status == task.Created && slices.Contains(d.After, r.ID) && len(task.Unmet(d, byID)) == 0 {
				unblocked = append(unblocked, d.ID)
			}
		}
		return nil
	})
	if err != nil {
		out.Fail(err, "task", false)
		return out
	}
	out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "task", ID: r.ID})
	out.Result = Result{Record: r, Unblocked: unblocked}
	return out
}

// Render writes a successful cancellation.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	if _, err := fmt.Fprintf(w, "Cancelled task %s.\n", r.ID); err != nil || len(r.Unblocked) == 0 {
		return err
	}
	_, err := fmt.Fprintf(w, "Now ready: %s.\n", strings.Join(r.Unblocked, ", "))
	return err
}
