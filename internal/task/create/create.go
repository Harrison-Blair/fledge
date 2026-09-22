// Package create implements task create: recording a titled brief as a new
// task in the created state, optionally as a subtask of an existing task.
package create

import (
	"context"
	"fmt"
	"io"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

type Options struct {
	Title, Body, File, Parent string
	BodySet, FileSet          bool
}

// Run stores a new task. The creator is the caller's live agent record, or
// null when the caller is unregistered. A Parent must exist and be neither
// verified nor cancelled; it is checked and the task stored under one store
// lock.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) libagent.Outcome {
	out := libagent.Outcome{Operation: "task.create", Status: "success", Effects: []libagent.Effect{}}
	var brief string
	err := libagent.Invalid("--title must be nonempty and a single line")
	switch {
	case o.Parent != "" && task.ValidateID(o.Parent) != nil:
		err = libagent.Invalid("--parent must be 8 lowercase hexadecimal characters")
	case o.Title != "" && !strings.ContainsAny(o.Title, "\r\n"):
		brief, err = libagent.ReadText(in, libagent.TextInput{Body: o.Body, BodyFlag: "body", BodySet: o.BodySet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Required: true, Noun: "brief"})
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	s, err := identity.OpenStore(ctx, c.Cwd, &out)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	caller, err := identity.Caller(ctx, s, c)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	var r task.Record
	phase := "state"
	err = s.Exclusive(func() error {
		if o.Parent != "" {
			parent, err := task.Get(s, o.Parent)
			if err == nil {
				err = task.Require(&parent, "adding a subtask", task.Created, task.Assigned, task.Completed)
			}
			if err != nil {
				phase = "task"
				return err
			}
		}
		_, err := s.Create(task.Kind, func(id string) any {
			r = task.Record{ID: id, Title: o.Title, Brief: brief, Status: task.Created, CreatedAt: *task.Now()}
			if o.Parent != "" {
				r.Parent = &o.Parent
			}
			if caller != nil {
				r.CreatedBy = &caller.ID
			}
			return r
		})
		return err
	})
	if err != nil {
		out.Fail(err, phase, false)
		return out
	}
	out.Effects = append(out.Effects, libagent.Effect{Action: "created", Kind: "task", ID: r.ID})
	out.Result = r
	return out
}

// Render writes a successful creation.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(task.Record)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := fmt.Fprintf(w, "Created task %s: %s\n", r.ID, r.Title)
	return err
}
