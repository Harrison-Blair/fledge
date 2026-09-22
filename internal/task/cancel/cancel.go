// Package cancel implements task cancel: closing a task that is not yet
// verified or cancelled.
package cancel

import (
	"context"
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Options names the task and an optional reason, stored when nonempty.
type Options struct{ ID, Reason string }

// Run cancels a created, assigned, or completed task. Anyone may cancel.
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
	r, err := task.Update(s, o.ID, func(r *task.Record) error {
		if err := task.Require(r, "cancel", task.Created, task.Assigned, task.Completed); err != nil {
			return err
		}
		r.Status, r.CancelledAt = task.Cancelled, task.Now()
		if o.Reason != "" {
			r.CancelReason = &o.Reason
		}
		return nil
	})
	if err != nil {
		out.Fail(err, "task", false)
		return out
	}
	out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "task", ID: r.ID})
	out.Result = r
	return out
}

// Render writes a successful cancellation.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(task.Record)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := fmt.Fprintf(w, "Cancelled task %s.\n", r.ID)
	return err
}
