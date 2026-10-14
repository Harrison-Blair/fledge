// Package taskimport implements task import: validating a proposal file and
// creating its parent task, tasks, and prerequisites under one store lock.
package taskimport

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/landing"
	"github.com/Harrison-Blair/fledge/internal/lib/proposal"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/termtext"
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
// nor cancelled and every existing prerequisite id must exist. When the
// Parent has shape stack, its layers and the new tasks must form one after
// chain. A dry run checks the same without creating anything. The creator is
// the caller's live agent record, or null when the caller is unregistered.
// Repeated prerequisites are stored once. A task or the file's parent without
// delivery_mode takes the repository's configured delivery_mode, else none.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) cli.Outcome {
	out := cli.NewOutcome("task.import")
	p, order, err := validate(ctx, o, in)
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
			if slices.Contains(after, dep) {
				continue
			}
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
			if err := checkLinks(s, o.Parent, existing, p, order); err != nil {
				out.Fail(err, "task", false)
				return out
			}
		}
		out.Result = r
		return out
	}
	mode, err := landing.DefaultMode(ctx, c.Cwd)
	if err != nil {
		out.Fail(err, "config", false)
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
		if err := checkLinks(s, o.Parent, existing, p, order); err != nil {
			phase = "task"
			return err
		}
		create := func(rec task.Record) (string, error) {
			rec.Status, rec.CreatedAt = task.Created, *task.Now()
			if rec.DeliveryMode == "" {
				rec.DeliveryMode = mode
			}
			if caller != nil {
				rec.CreatedBy = &caller.ID
			}
			id, err := tx.Create(task.Kind, func(id string) any { rec.ID = id; return rec })
			if err == nil {
				out.Effects = append(out.Effects, cli.Effect{Action: "created", Kind: "task", ID: id})
			}
			return id, err
		}
		if p.Parent != nil {
			rec := task.Record{Title: p.Parent.Title, Brief: p.Parent.Brief, Branch: p.Parent.Branch, BaseBranch: p.Parent.BaseBranch}
			if p.Parent.Shape != nil {
				rec.Shape = *p.Parent.Shape
			}
			if p.Parent.DeliveryMode != nil {
				rec.DeliveryMode = *p.Parent.DeliveryMode
			}
			id, err := create(rec)
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
				if !slices.Contains(after, dep) {
					after = append(after, dep)
				}
			}
			rec := task.Record{Title: t.Title, Brief: t.Brief, Parent: r.Parent, After: after, BaseBranch: t.BaseBranch, Branch: t.Branch}
			if t.DeliveryMode != nil {
				rec.DeliveryMode = *t.DeliveryMode
			}
			id, err := create(rec)
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

// checkLinks runs task.CheckLinks. When parent has shape stack, its layers
// and the proposal's tasks must also form one after chain. A fork is invalid
// input. When parent has shape feature, proposal.FeatureMembers applies to
// order with the branch of parent. A conflict is invalid input.
func checkLinks(s *state.Store, parent string, existing []string, p proposal.Proposal, order []proposal.Task) error {
	if err := task.CheckLinks(s, parent, existing); err != nil || parent == "" {
		return err
	}
	r, err := task.Get(s, parent)
	if err == nil && r.Shape == task.ShapeFeature {
		if err := proposal.FeatureMembers(order, r.Branch); err != nil {
			return cli.Invalid("%v", err)
		}
	}
	if err != nil || r.Shape != task.ShapeStack {
		return err
	}
	rs, err := task.List(s)
	if err != nil {
		return err
	}
	if err := p.CheckStack(parent, rs); err != nil {
		return cli.Invalid("%v", err)
	}
	return nil
}

// validate reads and decodes the proposal, checks each branch and
// base_branch, and checks the flags against it. Every problem is an input error.
func validate(ctx context.Context, o Options, in io.Reader) (proposal.Proposal, []proposal.Task, error) {
	switch {
	case !o.FileSet:
		return proposal.Proposal{}, nil, cli.Invalid("--file is required")
	case o.Parent != "":
		if err := libagent.ValidateID("parent", "task", o.Parent); err != nil {
			return proposal.Proposal{}, nil, err
		}
	}
	text, err := cli.ReadText(in, cli.TextInput{File: o.File, FileFlag: "file", FileSet: true, Noun: "proposal"})
	if err != nil {
		return proposal.Proposal{}, nil, err
	}
	p, order, err := proposal.DecodeOrdered([]byte(text))
	if err != nil {
		return proposal.Proposal{}, nil, cli.Invalid("%v", err)
	}
	if p.Parent != nil {
		if err := validBranches(ctx, "[parent]:", p.Parent.Branch, p.Parent.BaseBranch); err != nil {
			return proposal.Proposal{}, nil, err
		}
	}
	for _, t := range order {
		if err := validBranches(ctx, fmt.Sprintf("task %q:", t.Key), t.Branch, t.BaseBranch); err != nil {
			return proposal.Proposal{}, nil, err
		}
	}
	if p.Parent != nil && o.Parent != "" {
		return proposal.Proposal{}, nil, cli.Invalid("--parent conflicts with the file's [parent]; use one")
	}
	return p, order, nil
}

// validBranches checks branch as task create checks --branch, and base as it
// checks --base-branch. A nil value is not checked.
func validBranches(ctx context.Context, owner string, branch, base *string) error {
	if branch != nil {
		if err := landing.ValidBranch(ctx, owner+" branch", *branch); err != nil {
			return err
		}
	}
	if base != nil {
		return landing.ValidTarget(ctx, owner+" base_branch", *base)
	}
	return nil
}

// Render writes the tasks in creation order: a plan on a dry run, one line
// per created record otherwise.
func Render(w io.Writer, o cli.Outcome) error {
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
			fmt.Fprintf(&b, "Would create parent task: %s\nWould create %s under it:\n", termtext.Clean(r.newParent), n)
		case r.Parent != nil:
			fmt.Fprintf(&b, "Would create %s under %s:\n", n, *r.Parent)
		default:
			fmt.Fprintf(&b, "Would create %s:\n", n)
		}
		width := 0
		for _, t := range r.Tasks {
			width = max(width, len(termtext.Clean(t.Key)))
		}
		for _, t := range r.Tasks {
			fmt.Fprintf(&b, "  %-*s  %s", width, termtext.Clean(t.Key), termtext.Clean(t.Title))
			if len(t.After) > 0 {
				fmt.Fprintf(&b, "  (after: %s)", termtext.Clean(strings.Join(t.After, ", ")))
			}
			b.WriteString("\n")
		}
	} else {
		if r.newParent != "" {
			fmt.Fprintf(&b, "Created task %s: %s\n", *r.Parent, termtext.Clean(r.newParent))
		}
		for _, t := range r.Tasks {
			fmt.Fprintf(&b, "Created task %s (%s): %s\n", *t.ID, termtext.Clean(t.Key), termtext.Clean(t.Title))
		}
		if r.newParent != "" {
			fmt.Fprintf(&b, "Created %s under parent %s\n", n, *r.Parent)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
