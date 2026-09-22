// Package complete implements task complete: the owner recording its result.
package complete

import (
	"context"
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

type Options struct {
	ID, Summary, File          string
	SummarySet, FileSet, Force bool
}

// Run moves an assigned task to completed with the summary as its result.
// Only the owner, identified by the caller's live record, may complete it
// unless Force is set.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) libagent.Outcome {
	out := libagent.Outcome{Operation: "task.complete", Status: "success", Effects: []libagent.Effect{}}
	err := task.ValidateID(o.ID)
	var summary string
	if err == nil {
		summary, err = libagent.ReadText(in, libagent.TextInput{Body: o.Summary, BodyFlag: "summary", BodySet: o.SummarySet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Required: true, Noun: "summary"})
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
	r, err := task.Update(s, o.ID, func(r *task.Record) error {
		if err := task.Require(r, "complete", task.Assigned); err != nil {
			return err
		}
		if !o.Force && (caller == nil || r.Owner == nil || *r.Owner != caller.ID) {
			return &herdr.Error{Code: "task_not_owner", Message: fmt.Sprintf("task %s is owned by %s and only its owner may complete it; pass --force to override", r.ID, display(r.Owner))}
		}
		r.Status, r.Result, r.CompletedAt = task.Completed, &summary, task.Now()
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

// Render writes a successful completion.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(task.Record)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := fmt.Fprintf(w, "Completed task %s.\n", r.ID)
	return err
}

func display(s *string) string {
	if s == nil {
		return "no one"
	}
	return *s
}
