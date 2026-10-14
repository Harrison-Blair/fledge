package land

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/gitstatus"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/landing"
	"github.com/Harrison-Blair/fledge/internal/lib/reviewsize"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Options selects the task, and optionally the branch to land and the
// delivery mode, both stored on the task. Merge merges a pr task that
// stopped for review.
type Options struct {
	ID, Branch, Mode   string
	BranchSet, ModeSet bool
	Merge              bool
}

// Result is the task with the delivery mode used. On a refusal its land_step
// is the step reached, and Warnings is set only after a landing. Size is the
// advisory review size, set once the head check passed.
type Result struct {
	task.Record
	Mode          string             `json:"mode"`
	Warnings      []string           `json:"warnings"`
	ReviewPending bool               `json:"review_pending"`
	Size          *reviewsize.Report `json:"size"`
	Layers        []Layer            `json:"layers"`
	Stack         int                `json:"stack"`
	summary       []string
	target        string
	base          string
}

// Layer is one layer of a stack landing.
type Layer struct {
	ID       string `json:"id"`
	Branch   string `json:"branch"`
	Head     string `json:"head"`
	PRURL    string `json:"pr_url"`
	LandStep string `json:"land_step"`
}

// errMergeMode refuses --merge outside pr mode.
var errMergeMode = cli.Invalid("--merge applies only to pr mode")

// The land steps recorded in land_step. Check covers every guard before a
// merge or a push. Review is a pr landing stopped with its pull request open.
const (
	stepCheck   = "check"
	stepPush    = "push"
	stepPR      = "pr"
	stepChecks  = "checks"
	stepReview  = "review"
	stepMerge   = "merge"
	stepConfirm = "confirm"
)

// lander is one landing in progress. r is the task as last read or stored.
type lander struct {
	s                            *state.Store
	root, caller                 string
	r                            task.Record
	mode, branch, target, commit string
	step                         string
	merge, review                bool
	warnings                     []string
	size                         *reviewsize.Report
	base                         string // the size base ref, and the missing ref of the mode
	layers                       []Layer
	stackNumber                  int                     // the GitHub stack number of the layer pull requests
	pulls                        map[string]landing.Pull // the layer pull requests that the reconcile read, by layer id
	summary                      []string                // the human lines of a stack run that ends after a record
}

// Run lands task o.ID. The caller must be a registered agent other than the
// owner, and the task verified with a bound commit. The whole command holds
// the land lock; the store lock is held only while a record is written, and
// each write first checks that the task is still verified at the same
// commit. A refusal before a pull request push changes nothing but storing
// --branch and --mode, and a carry-forward to a head with an equal patch id.
// Land never removes a checkout, stops an agent, or
// deletes a branch.
func Run(ctx context.Context, c libagent.Client, o Options) cli.Outcome {
	out := cli.NewOutcome("task.land")
	l := &lander{step: stepCheck, merge: o.Merge}
	// fail refuses with the record as last read and the step reached.
	fail := func(err error, phase string) cli.Outcome {
		out.Fail(err, phase, false)
		result := Result{Record: l.r, Mode: l.mode, Size: l.size, base: l.base}
		result.LandStep = &l.step
		out.Result = result
		return out
	}
	err := task.ValidateID(o.ID)
	if err == nil && o.BranchSet {
		err = landing.ValidBranch(ctx, "--branch", o.Branch)
	}
	if err == nil && o.ModeSet && o.Mode != task.ModeLocal && o.Mode != task.ModePR && o.Mode != task.ModeAuto {
		err = cli.Invalid("--mode must be local, pr, or auto")
	}
	if err == nil && o.Merge && o.ModeSet && o.Mode != task.ModePR {
		err = errMergeMode
	}
	if err != nil {
		return fail(err, "validation")
	}
	s, err := task.Existing(ctx, c.Cwd)
	var caller *identity.Record
	if err == nil && s != nil {
		caller, _, err = identity.CallerAgent(ctx, s, c)
	}
	if err != nil {
		return fail(err, "state")
	}
	if caller == nil {
		return fail(&cli.Error{Code: "caller_unregistered", Message: "the caller has no live Fledge record, so it cannot be recorded as the lander; register with fledge agent adopt"}, "identity")
	}
	root, err := fledgedir.Root(ctx, c.Cwd)
	var unlock func()
	if err == nil {
		unlock, err = landing.Lock(root)
	}
	if err != nil {
		return fail(err, "state")
	}
	defer unlock()
	l.s, l.root, l.caller = s, root, caller.ID
	if err := l.land(ctx, o); err != nil {
		return fail(err, "land")
	}
	out.Effects = append(out.Effects, cli.Effect{Action: "updated", Kind: "task", ID: l.r.ID})
	if l.size != nil && !l.review {
		l.warnings = append(l.warnings, sizeWarnings(*l.size)...)
	}
	out.Result = Result{Record: l.r, Mode: l.mode, Warnings: l.warnings, ReviewPending: l.review, Size: l.size, Layers: l.layers, Stack: l.stackNumber, summary: l.summary, target: l.target, base: l.base}
	return out
}

// land checks the task and every guard, then delivers it in its mode.
func (l *lander) land(ctx context.Context, o Options) error {
	r, err := task.Get(l.s, o.ID)
	if err != nil {
		return err
	}
	l.r = r
	switch {
	case r.Owner != nil && *r.Owner == l.caller:
		return refuse("task_self_landing", "task %s is owned by the caller (%s); another agent must land it", r.ID, l.caller)
	}
	if p, ok, err := stackParent(l.s, r); err != nil || ok {
		if err != nil {
			return err
		}
		return l.stack(ctx, o, p)
	}
	if err := task.Require(&r, "land", task.Verified); err != nil {
		return err
	}
	if r.VerifiedCommit == nil {
		return refuse("land_unverified_commit", "task %s was verified without --commit; verify it again with fledge task verify --id %s --commit <rev>", r.ID, r.ID)
	}
	l.commit, l.mode = *r.VerifiedCommit, r.DeliveryMode
	if o.ModeSet {
		l.mode = o.Mode
	}
	if o.Merge && l.mode != task.ModePR {
		return errMergeMode
	}
	if o.Merge && r.PRURL == nil {
		return refuse("land_review_not_open", "task %s has no open pull request to merge; run fledge task land --id %s first", r.ID, r.ID)
	}
	if o.BranchSet || o.ModeSet {
		if err := l.update(func(r *task.Record) {
			if o.BranchSet {
				r.Branch = &o.Branch
			}
			if o.ModeSet {
				r.DeliveryMode = o.Mode
			}
		}); err != nil {
			return err
		}
	}
	if l.mode != task.ModeLocal && l.mode != task.ModePR && l.mode != task.ModeAuto {
		return refuse("task_no_delivery_mode", "task %s has delivery mode %s; pass --mode local, --mode pr, or --mode auto", r.ID, l.mode)
	}
	if l.branch, err = landing.TaskBranch(ctx, l.s, l.r, "land"); err != nil {
		var coded *cli.Error
		if errors.As(err, &coded) {
			coded.Message += "; pass --branch"
		}
		return err
	}
	if err := l.guard(ctx, landing.RefCommit(ctx, l.root, "refs/heads/"+l.branch)); err != nil {
		return err
	}
	if err := l.checkTarget(ctx); err != nil {
		return err
	}
	if l.mode == task.ModeLocal {
		return l.local(ctx)
	}
	return l.pr(ctx)
}

// guard runs the local checks common to both modes. It checks that head, the
// current head of the branch, is the verified commit or has its patch id.
// The target is the layer base.
func (l *lander) guard(ctx context.Context, head string) error {
	if err := landing.ValidBranch(ctx, "branch", l.branch); err != nil {
		return refuse("land_branch_invalid", "%v", err)
	}
	if !landing.FullHash(l.commit) {
		return refuse("land_unverified_commit", "task %s has a malformed verified commit %q", l.r.ID, l.commit)
	}
	target, err := landing.LayerBase(ctx, l.s, l.root, l.r, "land")
	if err != nil {
		return err
	}
	l.target = target
	if l.target == l.branch {
		return refuse("land_same_branch", "task %s would land branch %s into itself", l.r.ID, l.branch)
	}
	if head != l.commit {
		if err := l.carry(ctx, head); err != nil {
			return err
		}
	}
	if err := l.measure(ctx); err != nil {
		return err
	}
	paths, err := landing.Checkouts(ctx, l.root, l.branch)
	if err != nil {
		return err
	}
	for _, path := range paths {
		changes, err := landing.Changes(ctx, path)
		if err != nil {
			return err
		}
		if len(changes) > 0 {
			return refuse("land_dirty", "checkout %s of branch %s has uncommitted changes: %s", path, l.branch, strings.Join(changes, ", "))
		}
	}
	return nil
}

// checkTarget runs the target check of the mode. Then it refuses a commit
// that the target already contains, except for a pr task with a pull
// request.
func (l *lander) checkTarget(ctx context.Context) error {
	targetRef := "refs/heads/" + l.target
	if l.mode != task.ModeLocal {
		if _, err := exec.LookPath("gh"); err != nil {
			return refuse("land_gh_missing", "pr mode needs the GitHub CLI gh on PATH")
		}
		if exec.CommandContext(ctx, "git", "-C", l.root, "remote", "get-url", "origin").Run() != nil {
			return refuse("land_target_missing", "pr mode needs an origin remote")
		}
		targetRef = "refs/remotes/origin/" + l.target
	}
	if landing.RefCommit(ctx, l.root, targetRef) == "" {
		return refuse("land_target_missing", "target branch %s does not exist as %s", l.target, targetRef)
	}
	// A pr task with a pull request may have been merged in GitHub; pr
	// checks that pull request instead.
	if gitstatus.Merged(ctx, l.root, l.commit, targetRef) == "yes" && (l.mode != task.ModePR || l.r.PRURL == nil) {
		return refuse("land_already_merged", "verified commit %s is already in %s; nothing to land", l.commit, l.target)
	}
	return nil
}

// carry moves the verification to head when head has the verified patch id.
// It stores head as the verified commit and the old commit as carried_from.
// Any other head is land_head_mismatch.
func (l *lander) carry(ctx context.Context, head string) error {
	if head == "" {
		return refuse("land_head_mismatch", "branch %s is at %s, not the verified commit %s; verify the new head again", l.branch, display(head), l.commit)
	}
	if err := landing.CheckCarry(ctx, l.s, l.root, l.r, l.mode, head, "land"); err != nil {
		return err
	}
	return l.carryTo(head)
}

// carryTo stores head as the verified commit and the old commit as
// carried_from.
func (l *lander) carryTo(head string) error {
	old := l.commit
	if err := l.update(func(r *task.Record) { r.VerifiedCommit, r.CarriedFrom = &head, &old }); err != nil {
		return err
	}
	l.commit = head
	return nil
}

// measure sets the advisory review size of the verified commit against the
// base ref of the mode for the layer base. Task size uses the same base. It
// returns only the land_branch_unknown and land_stack_fork refusals of the
// layer base. With any other error, no base ref, or a failed diff the size
// stays unset.
func (l *lander) measure(ctx context.Context) error {
	target, err := landing.LayerBase(ctx, l.s, l.root, l.r, "land")
	if err != nil {
		var coded *cli.Error
		if errors.As(err, &coded) && (coded.Code == "land_branch_unknown" || coded.Code == "land_stack_fork") {
			return err
		}
		return nil
	}
	base, missing, err := landing.BaseRef(ctx, l.root, l.mode, target, "land")
	if err != nil {
		return nil
	}
	d, err := reviewsize.Measure(ctx, l.root, base, l.commit)
	if err != nil {
		return nil
	}
	kind := reviewsize.KindTask
	if l.mode != task.ModeLocal {
		kind = reviewsize.KindPR
	}
	r := reviewsize.Compare(d, kind)
	l.size, l.base = &r, base
	if missing != "" {
		l.base += " (" + missing + " is missing)"
	}
	return nil
}

// sizeWarnings names each measure of r over a limit.
func sizeWarnings(r reviewsize.Report) []string {
	var warnings []string
	for _, m := range []struct {
		name string
		c    reviewsize.Check
	}{{"source lines", r.SourceLines}, {"files", r.Files}, {"code domains", r.CodeDomains}, {"total read", r.TotalLines}} {
		switch {
		case m.c.Status == reviewsize.Within:
		case m.c.Target == m.c.Ceiling:
			warnings = append(warnings, fmt.Sprintf("review size: %s %d is over limit %d", m.name, m.c.Value, m.c.Ceiling))
		case m.c.Status == reviewsize.OverCeiling:
			warnings = append(warnings, fmt.Sprintf("review size: %s %d is over ceiling %d", m.name, m.c.Value, m.c.Ceiling))
		default:
			warnings = append(warnings, fmt.Sprintf("review size: %s %d is over target %d", m.name, m.c.Value, m.c.Target))
		}
	}
	return warnings
}

// sizeText is the review size report against base.
func sizeText(r reviewsize.Report, base string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Review size against %s:\n", base)
	reviewsize.Render(&b, r)
	return b.String()
}

// update stores mutate's changes once the task is still verified at the
// verified commit this landing read.
func (l *lander) update(mutate func(*task.Record)) error {
	r, err := task.Update(l.s, l.r.ID, func(r *task.Record) error {
		if r.Status != task.Verified || commit(r) != l.commit {
			return refuse("task_state_changed", "task %s changed while it was being landed; inspect it with fledge task get --id %s", r.ID, r.ID)
		}
		mutate(r)
		return nil
	})
	if err == nil {
		l.r = r
	}
	return err
}

// record stores the landing and moves the task to landed.
func (l *lander) record(merge string, prURL *string) error {
	return l.update(func(r *task.Record) {
		r.Status, r.MergeCommit, r.TargetBranch, r.Branch = task.Landed, &merge, &l.target, &l.branch
		r.LandedAt, r.LandedBy, r.LandStep = task.Now(), &l.caller, &l.step
		if prURL != nil {
			r.PRURL = prURL
		}
	})
}

// commit is the verified commit of r, or "" when r has none.
func commit(r *task.Record) string {
	if r.VerifiedCommit == nil {
		return ""
	}
	return *r.VerifiedCommit
}

func refuse(code, format string, args ...any) error {
	return &cli.Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func display(s string) string {
	if s == "" {
		return "nothing (it does not exist)"
	}
	return s
}

// Render writes a landing, or the step a refused landing reached.
func Render(w io.Writer, o cli.Outcome) error {
	r, ok := o.Result.(Result)
	if !ok {
		return nil
	}
	var b strings.Builder
	switch {
	case o.Error != nil:
		fmt.Fprintf(&b, "Land step reached: %s.\n", *r.LandStep)
	case len(r.summary) > 0:
		b.WriteString(strings.Join(r.summary, "\n") + "\n")
	case len(r.Layers) > 0:
		for _, layer := range r.Layers {
			fmt.Fprintf(&b, "Layer %s: branch %s at %s, pull request %s, land step %s.\n", layer.ID, layer.Branch, layer.Head, layer.PRURL, layer.LandStep)
		}
		fmt.Fprintf(&b, "The layer pull requests form GitHub stack %d.\n", r.Stack)
		fmt.Fprintf(&b, "Merge the stack only on the owner's word: fledge task land --id %s --merge\n", r.ID)
	case r.ReviewPending:
		fmt.Fprintf(&b, "Task %s is ready for review: pull request %s merges %s into %s (%s), and its checks passed.\n", r.ID, *r.PRURL, *r.Branch, r.target, r.Mode)
		fmt.Fprintf(&b, "Merge it only on the owner's word: fledge task land --id %s --merge\n", r.ID)
	default:
		fmt.Fprintf(&b, "Landed task %s: merged %s into %s (%s) as %s.\n", r.ID, *r.Branch, *r.TargetBranch, r.Mode, *r.MergeCommit)
		if r.PRURL != nil {
			fmt.Fprintf(&b, "Pull request: %s\n", *r.PRURL)
		}
		for _, warning := range r.Warnings {
			fmt.Fprintf(&b, "Warning: %s.\n", warning)
		}
	}
	if r.Size != nil {
		b.WriteString(sizeText(*r.Size, r.base))
	}
	_, err := io.WriteString(w, b.String())
	return err
}
