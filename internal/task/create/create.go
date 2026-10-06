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
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/termtext"
)

type Options struct {
	Title, Body, File, Parent string
	After                     []string
	BodySet, FileSet          bool
}

// Run stores a new task. The brief is any nonblank, NUL-free UTF-8 text,
// stored exactly as given. The creator is the caller's live agent record, or
// null when the caller is unregistered. A Parent must exist and be neither
// verified nor cancelled, and every After prerequisite must exist; both are
// checked and the task stored under one store lock. Repeated prerequisites
// are stored once.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) cli.Outcome {
	out := cli.NewOutcome("task.create")
	var text string
	var after []string
	var err error
	for _, id := range o.After {
		if e := libagent.ValidateID("after", "task", id); e != nil {
			err = e
		}
		if !slices.Contains(after, id) {
			after = append(after, id)
		}
	}
	if err == nil && o.Parent != "" {
		err = libagent.ValidateID("parent", "task", o.Parent)
	}
	switch {
	case err != nil:
	case o.Title == "" || strings.ContainsAny(o.Title, "\r\n"):
		err = cli.Invalid("--title must be nonempty and a single line")
	default:
		text, err = cli.ReadText(in, cli.TextInput{Body: o.Body, BodyFlag: "body", BodySet: o.BodySet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Required: true, Noun: "brief"})
		if err == nil {
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
		if err := task.CheckLinks(s, o.Parent, after); err != nil {
			phase = "task"
			return err
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
	out.Effects = append(out.Effects, cli.Effect{Action: "created", Kind: "task", ID: r.ID})
	out.Result = r
	return out
}

// Render writes a successful creation.
func Render(w io.Writer, o cli.Outcome) error {
	r, ok := o.Result.(task.Record)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := fmt.Fprintf(w, "Created task %s: %s\n", r.ID, termtext.Clean(r.Title))
	return err
}
