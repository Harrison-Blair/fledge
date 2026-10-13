// Package size implements task size: the review size of a task's branch
// against its target branch, judged by the limits of its delivery mode.
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

// Run measures task o.ID without writing the store or contacting Herdr. An
// over-limit size is not a refusal.
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
	var err error
	if res.Branch, err = landing.TaskBranch(ctx, s, r, "size"); err != nil {
		return res, err
	}
	if res.Target, err = landing.TaskTarget(ctx, root, r, "size"); err != nil {
		return res, err
	}
	var missing string
	if res.Base, missing, err = landing.BaseRef(ctx, root, res.Mode, res.Target, "size"); err != nil {
		return res, err
	}
	if missing != "" {
		res.BaseMissing = &missing
	}
	if r.VerifiedCommit != nil {
		res.Head = *r.VerifiedCommit
	} else if res.Head = landing.RefCommit(ctx, root, "refs/heads/"+res.Branch); res.Head == "" {
		return res, &cli.Error{Code: "size_branch_missing", Message: fmt.Sprintf("branch %s of task %s does not exist", res.Branch, r.ID)}
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
