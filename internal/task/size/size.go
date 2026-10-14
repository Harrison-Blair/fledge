// Package size implements task size: the review size of a task's branch
// against its target branch, judged by the limits of its delivery mode. It
// compares a stack layer with the layer below it, and a stack parent from
// the stack base to the head of its top layer.
package size

import (
	"context"
	"fmt"
	"io"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/landing"
	"github.com/Harrison-Blair/fledge/internal/lib/reviewsize"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/termtext"
)

type Options struct{ ID string }

// Result is the measured diff base...head. BaseMissing names the ref of the
// mode that did not exist when Base is the other one.
type Result struct {
	Branch      string            `json:"branch"`
	Target      string            `json:"target"`
	Base        string            `json:"base"`
	BaseMissing *string           `json:"base_missing"`
	Head        string            `json:"head"`
	Mode        string            `json:"mode"`
	Size        reviewsize.Report `json:"size"`
}

// Run measures task o.ID without contacting Herdr. It writes the store only
// for a carry-forward. An over-limit size is not a refusal.
func Run(ctx context.Context, c libagent.Client, o Options) cli.Outcome {
	out := cli.NewOutcome("task.size")
	if err := task.ValidateID(o.ID); err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	s, err := task.Existing(ctx, c.Cwd)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	r, err := task.Get(s, o.ID)
	if err != nil {
		out.Fail(err, "task", false)
		return out
	}
	root, err := fledgedir.Root(ctx, c.Cwd)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	result, err := measure(ctx, s, root, r)
	if err != nil {
		out.Fail(err, "size", false)
		return out
	}
	out.Result = result
	return out
}

func measure(ctx context.Context, s *state.Store, root string, r task.Record) (Result, error) {
	res := Result{Mode: r.DeliveryMode}
	if res.Mode == "" {
		res.Mode = task.ModeNone
	}
	// Measure a stack parent at the head of its top layer.
	top := r
	if r.Shape == task.ShapeStack {
		layers, err := landing.StackLayers(s, r, "size")
		if err != nil {
			return res, err
		}
		if len(layers) > 0 {
			top = layers[len(layers)-1]
		}
	}
	var err error
	if res.Branch, err = landing.TaskBranch(ctx, s, top, "size"); err != nil {
		return res, err
	}
	if res.Target, err = landing.LayerBase(ctx, s, root, r, "size"); err != nil {
		return res, err
	}
	var missing string
	if res.Base, missing, err = landing.BaseRef(ctx, root, res.Mode, res.Target, "size"); err != nil {
		return res, err
	}
	if missing != "" {
		res.BaseMissing = &missing
	}
	res.Head = landing.RefCommit(ctx, root, "refs/heads/"+res.Branch)
	switch {
	case top.VerifiedCommit == nil && res.Head == "":
		return res, &cli.Error{Code: "size_branch_missing", Message: fmt.Sprintf("branch %s of task %s does not exist", res.Branch, top.ID)}
	case top.VerifiedCommit == nil:
	case top.Status != task.Verified || res.Head == "" || res.Head == *top.VerifiedCommit:
		res.Head = *top.VerifiedCommit
	default:
		if err := carry(ctx, s, root, top, res.Head); err != nil {
			return res, err
		}
	}
	d, err := reviewsize.Measure(ctx, root, res.Base, res.Head)
	if err != nil {
		return res, err
	}
	kind := reviewsize.KindTask
	if res.Mode == task.ModePR || res.Mode == task.ModeAuto {
		kind = reviewsize.KindPR
	}
	res.Size = reviewsize.Compare(d, kind)
	return res, nil
}

// carry moves the verification of r to head when head has the verified patch
// id. It stores head as the verified commit and the old commit as
// carried_from, once r is still verified at the old commit.
func carry(ctx context.Context, s *state.Store, root string, r task.Record, head string) error {
	if err := landing.CheckCarry(ctx, s, root, r, r.DeliveryMode, head, "size"); err != nil {
		return err
	}
	_, err := task.Update(s, r.ID, func(cur *task.Record) error {
		if cur.Status != task.Verified || cur.VerifiedCommit == nil || *cur.VerifiedCommit != *r.VerifiedCommit {
			return &cli.Error{Code: "task_state_changed", Message: fmt.Sprintf("task %s changed while it was being measured; inspect it with fledge task get --id %s", r.ID, r.ID)}
		}
		cur.VerifiedCommit, cur.CarriedFrom = &head, r.VerifiedCommit
		return nil
	})
	return err
}

// Render writes what was measured, then the size report.
func Render(w io.Writer, o cli.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	limits := "task limits"
	if r.Size.Kind == reviewsize.KindPR {
		limits = "pull request target and ceiling"
	}
	base := r.Base
	if r.BaseMissing != nil {
		base += " (" + *r.BaseMissing + " is missing)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "branch: %s\ntarget: %s\nmode: %s (%s)\nbase: %s\nhead: %s\n", termtext.Clean(r.Branch), r.Target, r.Mode, limits, base, termtext.Clean(r.Head))
	reviewsize.Render(&b, r.Size)
	_, err := io.WriteString(w, b.String())
	return err
}
