package land

import (
	"context"
	"slices"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/landing"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// stackParent returns the stack parent of r: r when r is a stack parent, or
// the parent of r when that parent is a stack parent. ok is false for any
// other task.
func stackParent(s *state.Store, r task.Record) (p task.Record, ok bool, err error) {
	if r.Shape == task.ShapeStack {
		return r, true, nil
	}
	if r.Parent == nil {
		return task.Record{}, false, nil
	}
	if p, err = task.Get(s, *r.Parent); err != nil {
		return task.Record{}, false, err
	}
	return p, p.Shape == task.ShapeStack, nil
}

// stack lands stack parent p. The parent and the layers store the branches
// and the delivery mode, so --branch and --mode are invalid input. A layer
// lands only after the parent reached review. The parent guards run on each
// stack run. Then the push step runs.
func (l *lander) stack(ctx context.Context, o Options, p task.Record) error {
	if o.BranchSet || o.ModeSet {
		return cli.Invalid("--branch and --mode do not apply to a stack task: the stack records store the branches and the delivery mode")
	}
	if o.Merge || l.r.ID != p.ID && (p.LandStep == nil || *p.LandStep != stepReview) {
		return refuse("land_stack_not_open", "stack %s has no open pull requests. Land the stack with fledge task land --id %s", p.ID, p.ID)
	}
	l.r, l.mode, l.commit = p, p.DeliveryMode, commit(&p)
	if err := task.Require(&p, "land", task.Verified); err != nil {
		return err
	}
	if p.DeliveryMode != task.ModePR {
		return refuse("land_stack_mode", "stack %s has delivery mode %s. A stack lands only in pr mode", p.ID, p.DeliveryMode)
	}
	return l.pushLayers(ctx)
}

// pushLayers checks each layer and pushes the layer branches to origin,
// bottom first. Each layer must be verified with a commit. The local guards
// run for all layers before the first push. The target check of a layer runs
// after the push of the layer below, because that branch is its target.
func (l *lander) pushLayers(ctx context.Context) error {
	records, err := landing.StackLayers(l.s, l.r, "land")
	if err != nil {
		return err
	}
	var layers []*lander
	for _, r := range records {
		if r.Status != task.Verified || r.VerifiedCommit == nil {
			return refuse("land_stack_layer_unverified", "layer %s of stack %s is %s and has no verified commit. Verify it with fledge task verify --id %s --commit <rev>", r.ID, l.r.ID, r.Status, r.ID)
		}
		layer := &lander{s: l.s, root: l.root, caller: l.caller, r: r, mode: task.ModePR, commit: *r.VerifiedCommit, step: stepCheck}
		if layer.branch, err = landing.TaskBranch(ctx, l.s, r, "land"); err != nil {
			return err
		}
		if err := landing.ValidBranch(ctx, "branch", layer.branch); err != nil {
			return refuse("land_branch_invalid", "%v", err)
		}
		layers = append(layers, layer)
	}
	if err := l.fetch(ctx, layers); err != nil {
		return err
	}
	for _, layer := range layers {
		if err := layer.guard(ctx, landing.RefCommit(ctx, l.root, "refs/heads/"+layer.branch)); err != nil {
			return err
		}
	}
	l.step = stepPush
	if err := l.update(func(r *task.Record) { r.LandStep = &l.step }); err != nil {
		return err
	}
	for _, layer := range layers {
		if err := layer.checkTarget(ctx); err != nil {
			return err
		}
		if err := layer.reach(stepPush, nil); err != nil {
			return err
		}
		if err := layer.push(ctx); err != nil {
			return err
		}
		l.layers = append(l.layers, Layer{ID: layer.r.ID, Branch: layer.branch, Head: layer.commit, LandStep: layer.step})
	}
	return nil
}

// fetch runs git fetch for the stack base and the layer branches that exist
// on origin. A git fetch of a branch that is not on origin fails.
func (l *lander) fetch(ctx context.Context, layers []*lander) error {
	base, err := landing.TaskTarget(ctx, l.root, l.r, "land")
	if err != nil {
		return err
	}
	names := []string{base}
	for _, layer := range layers {
		names = append(names, layer.branch)
	}
	output, err := l.git(ctx, append([]string{"ls-remote", "--heads", "origin"}, names...)...).Output()
	if err != nil {
		return refuse("land_target_missing", "git ls-remote origin failed: %v", err)
	}
	var remote []string
	for _, line := range strings.Split(string(output), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 {
			remote = append(remote, strings.TrimPrefix(fields[1], "refs/heads/"))
		}
	}
	names = slices.DeleteFunc(names, func(name string) bool { return !slices.Contains(remote, name) })
	if len(names) == 0 {
		return nil
	}
	if output, err := l.git(ctx, append([]string{"fetch", "origin"}, names...)...).CombinedOutput(); err != nil {
		return refuse("land_target_missing", "git fetch origin %s failed: %s", strings.Join(names, " "), strings.TrimSpace(string(output)))
	}
	return nil
}
