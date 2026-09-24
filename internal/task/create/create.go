// Package create implements task create: recording a titled brief as a new
// task in the created state, optionally as a subtask of an existing task and
// after prerequisite tasks.
package create

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/brief"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

type Options struct {
	Title, Body, File, Parent  string
	After                      []string
	BodySet, FileSet, Freeform bool
}

// Run stores a new task. The brief must follow the brief template unless
// Freeform is set. The creator is the caller's live agent record, or
// null when the caller is unregistered. A Parent must exist and be neither
// verified nor cancelled, and every After prerequisite must exist; both are
// checked and the task stored under one store lock. Repeated prerequisites
// are stored once.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) libagent.Outcome {
	out := libagent.Outcome{Operation: "task.create", Status: "success", Effects: []libagent.Effect{}}
	var text string
	var after []string
	var err error
	for _, id := range o.After {
		if task.ValidateID(id) != nil {
			err = libagent.Invalid("--after must be 8 lowercase hexadecimal characters")
		}
		if !slices.Contains(after, id) {
			after = append(after, id)
		}
	}
	switch {
	case err != nil:
	case o.Parent != "" && task.ValidateID(o.Parent) != nil:
		err = libagent.Invalid("--parent must be 8 lowercase hexadecimal characters")
	case o.Title == "" || strings.ContainsAny(o.Title, "\r\n"):
		err = libagent.Invalid("--title must be nonempty and a single line")
	default:
		text, err = libagent.ReadText(in, libagent.TextInput{Body: o.Body, BodyFlag: "body", BodySet: o.BodySet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Required: true, Noun: "brief"})
		if err == nil && !o.Freeform {
			err = brief.Validate(text)
		}
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
	err = s.Exclusive(func(tx *state.Tx) error {
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
		for _, id := range after {
			if _, err := task.Get(s, id); err != nil {
				phase = "task"
				return err
			}
		}
		_, err := tx.Create(task.Kind, func(id string) any {
			r = task.Record{ID: id, Title: o.Title, Brief: text, After: after, Status: task.Created, CreatedAt: *task.Now()}
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
