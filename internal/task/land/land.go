package land

import (
	"context"
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
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Options selects the task, and optionally the branch to land and the
// delivery mode, both stored on the task.
type Options struct {
	ID, Branch, Mode   string
	BranchSet, ModeSet bool
}

// Result is the task with the delivery mode used. On a refusal its land_step
// is the step reached, and Warnings is set only after a landing.
type Result struct {
	task.Record
	Mode     string   `json:"mode"`
	Warnings []string `json:"warnings"`
}

// The land steps recorded in land_step. Check covers every guard before a
// merge or a push.
const (
	stepCheck   = "check"
	stepPush    = "push"
	stepPR      = "pr"
	stepChecks  = "checks"
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
	warnings                     []string
}

// Run lands task o.ID. The caller must be a registered agent other than the
// owner, and the task verified with a bound commit. The whole command holds
// the land lock; the store lock is held only while a record is written, and
// each write first checks that the task is still verified at the same
// commit. A refusal before a pull request push changes nothing but storing
// --branch and --mode. Land never removes a checkout, stops an agent, or
// deletes a branch.
func Run(ctx context.Context, c libagent.Client, o Options) cli.Outcome {
	out := cli.NewOutcome("task.land")
	err := task.ValidateID(o.ID)
	if err == nil && o.BranchSet {
		err = landing.ValidBranch(ctx, "--branch", o.Branch)
	}
	if err == nil && o.ModeSet && o.Mode != task.ModeLocal && o.Mode != task.ModePR {
		err = cli.Invalid("--mode must be local or pr")
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	s, err := task.Existing(ctx, c.Cwd)
	var caller *identity.Record
	if err == nil && s != nil {
		caller, _, err = identity.CallerAgent(ctx, s, c)
	}
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	if caller == nil {
		out.Fail(&cli.Error{Code: "caller_unregistered", Message: "the caller has no live Fledge record, so it cannot be recorded as the lander; register with fledge agent adopt"}, "identity", false)
		return out
	}
	root, err := fledgedir.Root(ctx, c.Cwd)
	var unlock func()
	if err == nil {
		unlock, err = landing.Lock(root)
	}
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	defer unlock()
	l := &lander{s: s, root: root, caller: caller.ID, step: stepCheck}
	err = l.land(ctx, o)
	result := Result{Record: l.r, Mode: l.mode, Warnings: l.warnings}
	if err != nil {
		result.LandStep = &l.step
		result.Warnings = nil
		out.Fail(err, "land", false)
		if l.r.ID != "" {
			out.Result = result
		}
		return out
	}
	out.Effects = append(out.Effects, cli.Effect{Action: "updated", Kind: "task", ID: l.r.ID})
	out.Result = result
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
	if l.mode != task.ModeLocal && l.mode != task.ModePR {
		return refuse("task_no_delivery_mode", "task %s has delivery mode %s; pass --mode local or --mode pr", r.ID, l.mode)
	}
	if l.branch, err = l.resolveBranch(ctx); err != nil {
		return err
	}
	if err := l.guard(ctx); err != nil {
		return err
	}
	if l.mode == task.ModeLocal {
		return l.local(ctx)
	}
	return l.pr(ctx)
}

// resolveBranch returns the stored branch, else the branch checked out in
// the owner's recorded checkout, else the branch its spawn created.
func (l *lander) resolveBranch(ctx context.Context) (string, error) {
	if l.r.Branch != nil {
		return *l.r.Branch, nil
	}
	if l.r.Owner != nil {
		var owner identity.Record
		if err := l.s.Get(identity.Kind, *l.r.Owner, &owner); err == nil {
			if owner.WorktreePath != nil {
				if b := gitstatus.Branch(ctx, *owner.WorktreePath); b != nil {
					return *b, nil
				}
			}
			if owner.WorktreeBranch != nil {
				return *owner.WorktreeBranch, nil
			}
		}
	}
	return "", refuse("land_branch_unknown", "cannot tell which branch task %s is on: its owner has no recorded checkout branch; pass --branch", l.r.ID)
}

// guard runs the checks common to both modes and the mode's target check,
// then refuses a commit the target already contains.
func (l *lander) guard(ctx context.Context) error {
	if err := landing.ValidBranch(ctx, "branch", l.branch); err != nil {
		return refuse("land_branch_invalid", "%v", err)
	}
	if !landing.FullHash(l.commit) {
		return refuse("land_unverified_commit", "task %s has a malformed verified commit %q", l.r.ID, l.commit)
	}
	target, err := l.resolveTarget(ctx)
	if err != nil {
		return err
	}
	l.target = target
	if l.target == l.branch {
		return refuse("land_same_branch", "task %s would land branch %s into itself", l.r.ID, l.branch)
	}
	if head := landing.RefCommit(ctx, l.root, "refs/heads/"+l.branch); head != l.commit {
		return refuse("land_head_mismatch", "branch %s is at %s, not the verified commit %s; verify the new head again", l.branch, display(head), l.commit)
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
	targetRef := "refs/heads/" + l.target
	if l.mode == task.ModePR {
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
	if gitstatus.Merged(ctx, l.root, l.commit, targetRef) == "yes" {
		return refuse("land_already_merged", "verified commit %s is already in %s; nothing to land", l.commit, l.target)
	}
	return nil
}

// resolveTarget returns the short name of the task's base branch, else of
// the repository integration branch.
func (l *lander) resolveTarget(ctx context.Context) (string, error) {
	if l.r.BaseBranch != nil {
		return landing.ShortBranch(*l.r.BaseBranch), nil
	}
	ref, err := gitstatus.DefaultBranch(ctx, l.root)
	if err != nil {
		return "", refuse("land_target_missing", "%v", err)
	}
	if ref == "" {
		return "", refuse("land_target_missing", "no integration branch: set base_branch in .fledge/config.toml or create the task with --base-branch")
	}
	return landing.ShortBranch(ref), nil
}

// update stores mutate's changes once the task is still verified at the
// verified commit this landing read.
func (l *lander) update(mutate func(*task.Record)) error {
	r, err := task.Update(l.s, l.r.ID, func(r *task.Record) error {
		if r.Status != task.Verified || r.VerifiedCommit == nil || *r.VerifiedCommit != l.commit {
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
	if o.Error != nil {
		_, err := fmt.Fprintf(w, "Land step reached: %s.\n", *r.LandStep)
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Landed task %s: merged %s into %s (%s) as %s.\n", r.ID, *r.Branch, *r.TargetBranch, r.Mode, *r.MergeCommit)
	if r.PRURL != nil {
		fmt.Fprintf(&b, "Pull request: %s\n", *r.PRURL)
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(&b, "Warning: %s.\n", warning)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
