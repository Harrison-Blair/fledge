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
	"github.com/Harrison-Blair/fledge/internal/lib/landing"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/termtext"
)

type Options struct {
	Title, Body, File, Parent, DeliveryMode, BaseBranch, Shape, Branch    string
	After                                                                 []string
	BodySet, FileSet, DeliveryModeSet, BaseBranchSet, ShapeSet, BranchSet bool
}

// Run stores a new task. The brief is any nonblank, NUL-free UTF-8 text,
// stored exactly as given. The creator is the caller's live agent record, or
// null when the caller is unregistered. A Parent must exist and be neither
// verified nor cancelled, and every After prerequisite must exist; both are
// checked and the task stored under one store lock. Repeated prerequisites
// are stored once. The delivery mode is DeliveryMode when set, else the
// repository's configured delivery_mode, else none; BaseBranch, when set, is
// stored as the task's landing target. Shape, when set, is one of
// task.Shapes. Branch, when set, is stored as the task's branch, and shape
// feature requires it. Under a stack Parent, the new task must be after the
// top layer and no other layer. Under a feature Parent, see featureMember.
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
	if err == nil && o.DeliveryModeSet {
		err = landing.ValidMode("--delivery-mode", o.DeliveryMode)
	}
	if err == nil && o.BaseBranchSet {
		err = landing.ValidTarget(ctx, "--base-branch", o.BaseBranch)
	}
	if err == nil && o.ShapeSet && !slices.Contains(task.Shapes, o.Shape) {
		err = cli.Invalid("--shape must be one of %s", strings.Join(task.Shapes, ", "))
	}
	if err == nil && o.BranchSet {
		err = landing.ValidBranch(ctx, "--branch", o.Branch)
	}
	if err == nil && o.Shape == task.ShapeFeature && !o.BranchSet {
		err = cli.Invalid("--shape feature needs --branch")
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	mode := o.DeliveryMode
	if !o.DeliveryModeSet {
		if mode, err = landing.DefaultMode(ctx, c.Cwd); err != nil {
			out.Fail(err, "config", false)
			return out
		}
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
		if err := checkLayer(s, o.Parent, after); err != nil {
			phase = "task"
			return err
		}
		if err := featureMember(s, &o, &mode); err != nil {
			phase = "task"
			return err
		}
		_, err := tx.Create(task.Kind, func(id string) any {
			r = task.Record{ID: id, Title: o.Title, Brief: text, After: after, Status: task.Created, CreatedAt: *task.Now(), DeliveryMode: mode, Shape: o.Shape}
			if o.BranchSet {
				r.Branch = &o.Branch
			}
			if o.BaseBranchSet {
				r.BaseBranch = &o.BaseBranch
			}
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

// checkLayer applies only when parent has shape stack. It fails with
// task_stack_fork when the layers do not form one after chain. It also fails
// when the new task is not after the top layer and no other layer. The first
// layer is after no layer.
func checkLayer(s *state.Store, parent string, after []string) error {
	if parent == "" {
		return nil
	}
	p, err := task.Get(s, parent)
	if err != nil || p.Shape != task.ShapeStack {
		return err
	}
	rs, err := task.List(s)
	if err != nil {
		return err
	}
	layers, err := task.Layers(parent, rs)
	if err != nil {
		return &cli.Error{Code: "task_stack_fork", Message: err.Error()}
	}
	var below, want []string
	for _, l := range layers {
		if slices.Contains(after, l.ID) {
			below = append(below, l.ID)
		}
	}
	if len(layers) > 0 {
		want = []string{layers[len(layers)-1].ID}
	}
	if !slices.Equal(below, want) {
		return &cli.Error{Code: "task_stack_fork", Message: fmt.Sprintf("a new layer of stack %s must be after its top layer %s and no other layer", parent, want[0])}
	}
	return nil
}

// featureMember applies only when o.Parent has shape feature. The member gets
// the delivery mode local and, when the parent has a branch, that branch as
// its base branch. Another explicit mode fails with task_feature_member_mode,
// and another explicit base branch with task_feature_member_base.
func featureMember(s *state.Store, o *Options, mode *string) error {
	if o.Parent == "" {
		return nil
	}
	p, err := task.Get(s, o.Parent)
	if err != nil || p.Shape != task.ShapeFeature {
		return err
	}
	if o.DeliveryModeSet && o.DeliveryMode != task.ModeLocal {
		return &cli.Error{Code: "task_feature_member_mode", Message: fmt.Sprintf("a member of feature parent %s needs delivery mode local, not %q", o.Parent, o.DeliveryMode)}
	}
	*mode = task.ModeLocal
	if p.Branch == nil {
		return nil
	}
	if o.BaseBranchSet && o.BaseBranch != *p.Branch {
		return &cli.Error{Code: "task_feature_member_base", Message: fmt.Sprintf("a member of feature parent %s needs base branch %q, not %q", o.Parent, *p.Branch, o.BaseBranch)}
	}
	o.BaseBranch, o.BaseBranchSet = *p.Branch, true
	return nil
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
