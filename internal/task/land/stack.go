package land

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/gitstatus"
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
// stack run. Then the push step and the pull request steps run.
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
	layers, err := l.pushLayers(ctx)
	if err != nil {
		return err
	}
	return l.openLayers(ctx, layers)
}

// pushLayers checks each layer and pushes the layer branches to origin,
// bottom first. Each layer must be verified with a commit. The local guards
// run for all layers before the first push. The target check of a layer runs
// after the push of the layer below, because that branch is its target.
// The current head of a layer with a pull request is the pull request head.
// openLayers checks that head, so the local guards skip the head check. A
// push without force cannot replace a remote head that GitHub rebased, so
// pushLayers does not push over it.
func (l *lander) pushLayers(ctx context.Context) ([]*lander, error) {
	records, err := landing.StackLayers(l.s, l.r, "land")
	if err != nil {
		return nil, err
	}
	var layers []*lander
	for _, r := range records {
		if r.Status != task.Verified || r.VerifiedCommit == nil {
			return nil, refuse("land_stack_layer_unverified", "layer %s of stack %s is %s and has no verified commit. Verify it with fledge task verify --id %s --commit <rev>", r.ID, l.r.ID, r.Status, r.ID)
		}
		layer := &lander{s: l.s, root: l.root, caller: l.caller, r: r, mode: task.ModePR, commit: *r.VerifiedCommit, step: stepCheck}
		if layer.branch, err = landing.TaskBranch(ctx, l.s, r, "land"); err != nil {
			return nil, err
		}
		if err := landing.ValidBranch(ctx, "branch", layer.branch); err != nil {
			return nil, refuse("land_branch_invalid", "%v", err)
		}
		layers = append(layers, layer)
	}
	if err := l.fetch(ctx, layers); err != nil {
		return nil, err
	}
	for _, layer := range layers {
		head := landing.RefCommit(ctx, l.root, "refs/heads/"+layer.branch)
		if layer.r.PRURL != nil {
			head = layer.commit
		}
		if err := layer.guard(ctx, head); err != nil {
			return nil, err
		}
	}
	if err := l.reachParent(stepPush); err != nil {
		return nil, err
	}
	for _, layer := range layers {
		if err := layer.checkTarget(ctx); err != nil {
			return nil, err
		}
		if err := layer.reach(stepPush, nil); err != nil {
			return nil, err
		}
		remote := "refs/remotes/origin/" + layer.branch
		if layer.r.PRURL != nil && gitstatus.Merged(ctx, l.root, remote, layer.commit) == "no" {
			continue
		}
		if err := layer.push(ctx); err != nil {
			return nil, err
		}
	}
	return layers, nil
}

// openLayers finds or opens the pull request of each layer into its layer
// base, bottom first, and stores its pr_url. A layer with a stored pr_url
// uses that pull request and never opens another. Then it checks each pull
// request. Then every layer and the parent store the review step.
func (l *lander) openLayers(ctx context.Context, layers []*lander) error {
	gh := landing.GH{Dir: l.root}
	if err := l.reachParent(stepPR); err != nil {
		return err
	}
	prs := make([]landing.PR, len(layers))
	for i, layer := range layers {
		if err := layer.reach(stepPR, nil); err != nil {
			return err
		}
		var pr landing.PR
		var err error
		if layer.r.PRURL != nil {
			pr, err = layer.pullHead(ctx, gh)
		} else {
			pr, err = layer.pullRequest(ctx, gh, layer.branch, layer.target)
		}
		if err != nil {
			return err
		}
		if err := layer.reach(stepChecks, &pr.URL); err != nil {
			return err
		}
		prs[i] = pr
	}
	if err := l.reachParent(stepChecks); err != nil {
		return err
	}
	for i, layer := range layers {
		c, err := gh.Checks(ctx, prs[i].Number)
		if err != nil {
			return err
		}
		if err := layer.mergeable(prs[i], c); err != nil {
			return err
		}
	}
	for i, layer := range layers {
		if err := layer.reach(stepReview, nil); err != nil {
			return err
		}
		l.layers = append(l.layers, Layer{ID: layer.r.ID, Branch: layer.branch, Head: layer.commit, PRURL: prs[i].URL, LandStep: layer.step})
	}
	l.review = true
	return l.reachParent(stepReview)
}

// pullHead reads the stored pull request of the layer through the REST API
// and returns it. A pull request closed without a merge is land_pr_closed.
// Its head is the current head of the layer, and its base is the patch id
// base. A head with the verified patch id gets the verification. Any other
// head is land_head_mismatch.
func (l *lander) pullHead(ctx context.Context, gh landing.GH) (landing.PR, error) {
	url := *l.r.PRURL
	n, err := strconv.Atoi(path.Base(url))
	if err != nil {
		return landing.PR{}, refuse("land_gh_output_invalid", "pull request URL %s of branch %s has no number", url, l.branch)
	}
	p, err := gh.Pull(ctx, n)
	if err != nil {
		return landing.PR{}, err
	}
	if p.State == "closed" && !p.Merged {
		return landing.PR{}, refuse("land_pr_closed", "%s of branch %s was closed without a merge; reopen it, then run task land again", url, l.branch)
	}
	pr := landing.PR{Number: p.Number, URL: url, HeadRefOid: p.HeadSHA}
	if p.HeadSHA == l.commit {
		return pr, nil
	}
	id, err := landing.PatchID(ctx, l.root, p.BaseSHA, p.HeadSHA)
	if err != nil || id == "" || l.r.VerifiedPatchID == nil || id != *l.r.VerifiedPatchID {
		return landing.PR{}, refuse("land_head_mismatch", "pull request %s of branch %s has head %s, not the verified commit %s, and its patch id against base %s is not the verified patch id; verify the new head again", url, l.branch, p.HeadSHA, l.commit, p.BaseSHA)
	}
	return pr, l.carryTo(p.HeadSHA)
}

// reachParent stores step as the land step that the stack parent reached.
func (l *lander) reachParent(step string) error {
	l.step = step
	return l.update(func(r *task.Record) { r.LandStep = &l.step })
}

// fetch runs git fetch for the stack base and the layer branches that exist
// on origin. A git fetch of a branch that is not on origin fails. fetch removes
// the local tracking ref of each layer branch that origin lacks.
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
	// A stale tracking ref of a missing layer branch would stop its push.
	for _, layer := range layers {
		if !slices.Contains(remote, layer.branch) {
			if output, err := l.git(ctx, "update-ref", "-d", "refs/remotes/origin/"+layer.branch).CombinedOutput(); err != nil {
				return fmt.Errorf("git update-ref -d refs/remotes/origin/%s failed: %s", layer.branch, strings.TrimSpace(string(output)))
			}
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
