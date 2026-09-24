// Package taskimport implements task import: validating a proposal file and
// creating its parent task, tasks, and prerequisites under one store lock.
package taskimport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/proposal"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

type Options struct {
	File, Parent    string
	FileSet, DryRun bool
}

// Task is one imported task. ID is null on a dry run; After holds the file's
// keys and ids on a dry run and resolved ids otherwise.
type Task struct {
	Key   string   `json:"key"`
	ID    *string  `json:"id"`
	Title string   `json:"title"`
	After []string `json:"after"`
}

// Result lists the tasks in creation order under Parent, the existing or
// created parent id, or null.
type Result struct {
	Parent *string `json:"parent"`
	Tasks  []Task  `json:"tasks"`
	DryRun bool    `json:"dry_run"`
	// newParent is the title of the file's [parent], created or to create.
	newParent string
}

// Run validates the proposal before opening the store, then, unless DryRun,
// creates the parent (if the file has one) and the tasks in topological
// order under one store lock. An existing Parent must be neither verified
// nor cancelled and every existing prerequisite id must exist; a dry run
// checks the same without creating anything. The creator is the caller's
// live agent record, or null when the caller is unregistered.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) libagent.Outcome {
	out := libagent.Outcome{Operation: "task.import", Status: "success", Effects: []libagent.Effect{}}
	p, order, err := validate(o, in)
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	r := Result{Tasks: []Task{}, DryRun: o.DryRun}
	if o.Parent != "" {
		r.Parent = &o.Parent
	}
	if p.Parent != nil {
		r.newParent = p.Parent.Title
	}
	var existing []string
	for _, t := range order {
		after := []string{}
		for _, dep := range t.After {
			after = append(after, dep)
			// Keys never look like ids, so an id names an existing task.
			if state.ValidID(dep) {
				existing = append(existing, dep)
			}
		}
		r.Tasks = append(r.Tasks, Task{Key: t.Key, Title: t.Title, After: after})
	}
	if o.DryRun {
		if o.Parent != "" || len(existing) > 0 {
			s, err := task.Existing(ctx, c.Cwd)
			if err != nil {
				out.Fail(err, "state", false)
				return out
			}
			if err := check(s, o.Parent, existing); err != nil {
				out.Fail(err, "task", false)
				return out
			}
		}
		out.Result = r
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
	phase := "state"
	err = s.Exclusive(func(tx *state.Tx) error {
		if err := check(s, o.Parent, existing); err != nil {
			phase = "task"
			return err
		}
		create := func(rec task.Record) (string, error) {
			rec.Status, rec.CreatedAt = task.Created, *task.Now()
			if caller != nil {
				rec.CreatedBy = &caller.ID
			}
			id, err := tx.Create(task.Kind, func(id string) any { rec.ID = id; return rec })
			if err == nil {
				out.Effects = append(out.Effects, libagent.Effect{Action: "created", Kind: "task", ID: id})
			}
			return id, err
		}
		if p.Parent != nil {
			id, err := create(task.Record{Title: p.Parent.Title, Brief: p.Parent.Brief})
			if err != nil {
				return err
			}
			r.Parent = &id
		}
		ids := map[string]string{}
		for i, t := range order {
			var after []string
			for _, dep := range t.After {
				if id, ok := ids[dep]; ok {
					dep = id
				}
				after = append(after, dep)
			}
			id, err := create(task.Record{Title: t.Title, Brief: t.Brief, Parent: r.Parent, After: after})
			if err != nil {
				return err
			}
			ids[t.Key] = id
			r.Tasks[i].ID = &id
			if after != nil {
				r.Tasks[i].After = after
			}
		}
		return nil
	})
	if err != nil {
		out.Fail(err, phase, false)
		return out
	}
	out.Result = r
	return out
}

// validate reads and decodes the proposal and checks the flags against it.
// File and schema problems are input errors; a brief off the template keeps
// its task_brief_incomplete code.
func validate(o Options, in io.Reader) (proposal.Proposal, []proposal.Task, error) {
	switch {
	case !o.FileSet:
		return proposal.Proposal{}, nil, libagent.Invalid("--file is required")
	case o.Parent != "" && task.ValidateID(o.Parent) != nil:
		return proposal.Proposal{}, nil, libagent.Invalid("--parent must be 8 lowercase hexadecimal characters")
	}
	text, err := libagent.ReadText(in, libagent.TextInput{File: o.File, FileFlag: "file", FileSet: true, Noun: "proposal"})
	if err != nil {
		return proposal.Proposal{}, nil, err
	}
	p, err := proposal.Decode([]byte(text))
	if err != nil {
		var coded *herdr.Error
		if !errors.As(err, &coded) {
			err = libagent.Invalid("%v", err)
		}
		return proposal.Proposal{}, nil, err
	}
	if p.Parent != nil && o.Parent != "" {
		return proposal.Proposal{}, nil, libagent.Invalid("--parent conflicts with the file's [parent]; use one")
	}
	order, err := p.Order()
	return p, order, err
}

// check requires parent, when set, to accept subtasks and every existing
// prerequisite to exist.
func check(s *state.Store, parent string, existing []string) error {
	if parent != "" {
		r, err := task.Get(s, parent)
		if err == nil {
			err = task.Require(&r, "adding a subtask", task.Created, task.Assigned, task.Completed)
		}
		if err != nil {
			return err
		}
	}
	for _, id := range existing {
		if _, err := task.Get(s, id); err != nil {
			return err
		}
	}
	return nil
}

// Render writes the tasks in creation order: a plan on a dry run, one line
// per created record otherwise.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	var b strings.Builder
	n := fmt.Sprintf("%d task", len(r.Tasks))
	if len(r.Tasks) != 1 {
		n += "s"
	}
	if r.DryRun {
		switch {
		case r.newParent != "":
			fmt.Fprintf(&b, "Would create parent task: %s\nWould create %s under it:\n", r.newParent, n)
		case r.Parent != nil:
			fmt.Fprintf(&b, "Would create %s under %s:\n", n, *r.Parent)
		default:
			fmt.Fprintf(&b, "Would create %s:\n", n)
		}
		width := 0
		for _, t := range r.Tasks {
			width = max(width, len(t.Key))
		}
		for _, t := range r.Tasks {
			fmt.Fprintf(&b, "  %-*s  %s", width, t.Key, t.Title)
			if len(t.After) > 0 {
				fmt.Fprintf(&b, "  (after: %s)", strings.Join(t.After, ", "))
			}
			b.WriteString("\n")
		}
	} else {
		if r.newParent != "" {
			fmt.Fprintf(&b, "Created task %s: %s\n", *r.Parent, r.newParent)
		}
		for _, t := range r.Tasks {
			fmt.Fprintf(&b, "Created task %s (%s): %s\n", *t.ID, t.Key, t.Title)
		}
		if r.newParent != "" {
			fmt.Fprintf(&b, "Created %s under parent %s\n", n, *r.Parent)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
