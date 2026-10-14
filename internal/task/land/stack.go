package land

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/gitstatus"
	"github.com/Harrison-Blair/fledge/internal/lib/landing"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// The poll of a stack merge request reads its status every pollInterval for
// at most pollTimeout. Tests change them.
var pollInterval, pollTimeout = 2 * time.Second, 120 * time.Second

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
// stack run. Then a run continues the merge request that a layer holds. Else
// --merge requests the merge of the selected layer, or of the top layer for
// the parent. Else the push step and the pull request steps run.
func (l *lander) stack(ctx context.Context, o Options, p task.Record) error {
	if o.BranchSet || o.ModeSet {
		return cli.Invalid("--branch and --mode do not apply to a stack task: the stack records store the branches and the delivery mode")
	}
	selected := l.r.ID
	if l.r.ID != p.ID && (p.LandStep == nil || *p.LandStep != stepReview) {
		return refuse("land_stack_not_open", "stack %s has no open pull requests. Land the stack with fledge task land --id %s", p.ID, p.ID)
	}
	l.r, l.mode, l.commit = p, p.DeliveryMode, commit(&p)
	if err := task.Require(&p, "land", task.Verified); err != nil {
		return err
	}
	if p.DeliveryMode != task.ModePR {
		return refuse("land_stack_mode", "stack %s has delivery mode %s. A stack lands only in pr mode", p.ID, p.DeliveryMode)
	}
	records, err := landing.StackLayers(l.s, p, "land")
	if err != nil {
		return err
	}
	gh := landing.GH{Dir: l.root}
	for j, r := range records {
		if r.MergeRequest != nil {
			return l.poll(ctx, gh, records, j, *r.MergeRequest)
		}
	}
	if o.Merge {
		k := slices.IndexFunc(records, func(r task.Record) bool { return r.ID == selected })
		if selected == p.ID {
			k = len(records) - 1
		}
		return l.requestMerge(ctx, gh, records, k)
	}
	layers, err := l.pushLayers(ctx, records)
	if err != nil {
		return err
	}
	return l.openLayers(ctx, layers)
}

// requestMerge requests the merge of the unlanded layers up to layer k: from
// the lowest layer that is not landed. Each of these layers needs the review
// step, a pull request, and green checks at its current head. The PUT sends
// the current head of layer k. Layer k stores the uuid of a pending answer
// before the poll starts.
func (l *lander) requestMerge(ctx context.Context, gh landing.GH, records []task.Record, k int) error {
	low := slices.IndexFunc(records, func(r task.Record) bool { return r.Status != task.Landed })
	if low < 0 || low > k {
		return refuse("land_already_merged", "stack %s has no unlanded layer up to the selected layer; nothing to merge", l.r.ID)
	}
	var layer *lander
	var pr landing.PR
	for _, r := range records[low : k+1] {
		if r.LandStep == nil || *r.LandStep != stepReview || r.PRURL == nil {
			return refuse("land_stack_not_open", "layer %s of stack %s has no pull request at the review step. Open the pull requests with fledge task land --id %s", r.ID, l.r.ID, l.r.ID)
		}
		var err error
		if layer, err = l.layer(ctx, r); err != nil {
			return err
		}
		if pr, err = layer.pullHead(ctx, gh); err != nil {
			return err
		}
		c, err := gh.Checks(ctx, pr.Number)
		if err != nil {
			return err
		}
		if err := layer.mergeable(pr, c); err != nil {
			return err
		}
	}
	l.step = stepMerge
	a, err := gh.MergeAsync(ctx, pr.Number, pr.HeadRefOid)
	switch {
	case err != nil:
		return err
	case a.Status == "failed":
		return mergeFailed(pr.Number, a)
	case a.Status == "merged":
		return l.recordMerge(pr.Number)
	}
	if err := layer.update(func(r *task.Record) { r.MergeRequest = &a.UUID }); err != nil {
		return err
	}
	records[k] = layer.r
	return l.poll(ctx, gh, records, k, a.UUID)
}

// poll reads the status of merge request uuid, which layer j holds, every
// pollInterval for at most pollTimeout. It skips the guards of a new request.
// A failed request clears the uuid. A pending request at the end keeps it.
// An HTTP 404 answer clears the uuid. Then a merged pull request goes to the
// record step, and any other one gets a new request. That rule is an
// assumption: the probes did not observe the answer to an expired uuid.
func (l *lander) poll(ctx context.Context, gh landing.GH, records []task.Record, j int, uuid string) error {
	l.step = stepMerge
	layer := &lander{s: l.s, root: l.root, caller: l.caller, r: records[j], commit: commit(&records[j])}
	n, err := strconv.Atoi(path.Base(cli.Display(layer.r.PRURL)))
	if err != nil {
		return refuse("land_gh_output_invalid", "layer %s holds merge request %s but has no pull request number", layer.r.ID, uuid)
	}
	clear := func() error { return layer.update(func(r *task.Record) { r.MergeRequest = nil }) }
	deadline := time.Now().Add(pollTimeout)
	for {
		a, err := gh.MergeStatus(ctx, n, uuid)
		switch {
		case errors.Is(err, landing.ErrMergeRequestGone):
			if err := clear(); err != nil {
				return err
			}
			p, err := gh.Pull(ctx, n)
			if err != nil {
				return err
			}
			if p.Merged {
				return l.recordMerge(n)
			}
			l.step, records[j] = stepCheck, layer.r
			return l.requestMerge(ctx, gh, records, j)
		case err != nil:
			return err
		case a.Status == "failed":
			if err := clear(); err != nil {
				return err
			}
			return mergeFailed(n, a)
		case a.Status == "merged":
			return l.recordMerge(n)
		case !time.Now().Before(deadline):
			return refuse("land_merge_pending", "GitHub did not finish merge request %s of pull request %d in %s. Continue it with fledge task land --id %s", uuid, n, pollTimeout, l.r.ID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// recordMerge is the record step after GitHub merged pull request n. Fledge
// does not record a stack merge yet, so it refuses land_unconfirmed.
func (l *lander) recordMerge(n int) error {
	return refuse("land_unconfirmed", "GitHub merged pull request %d of stack %s, but this Fledge cannot record a stack merge. Record it later with fledge task land --id %s", n, l.r.ID, l.r.ID)
}

// mergeFailed refuses the failed merge request of pull request n with the
// message of GitHub.
func mergeFailed(n int, a landing.MergeAnswer) error {
	return refuse("land_merge_failed", "GitHub could not merge pull request %d: %s", n, a.Message)
}

// pushLayers checks each layer and pushes the layer branches to origin,
// bottom first. Each layer must be verified with a commit. The local guards
// run for all layers before the first push. The target check of a layer runs
// after the push of the layer below, because that branch is its target.
// The current head of a layer with a pull request is the pull request head.
// openLayers checks that head, so the local guards skip the head check. A
// push without force cannot replace a remote head that GitHub rebased, so
// pushLayers does not push over it.
func (l *lander) pushLayers(ctx context.Context, records []task.Record) ([]*lander, error) {
	var layers []*lander
	for _, r := range records {
		layer, err := l.layer(ctx, r)
		if err != nil {
			return nil, err
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

// layer returns the lander of layer r with its branch. r must be verified
// with a commit.
func (l *lander) layer(ctx context.Context, r task.Record) (*lander, error) {
	if r.Status != task.Verified || r.VerifiedCommit == nil {
		return nil, refuse("land_stack_layer_unverified", "layer %s of stack %s is %s and has no verified commit. Verify it with fledge task verify --id %s --commit <rev>", r.ID, l.r.ID, r.Status, r.ID)
	}
	layer := &lander{s: l.s, root: l.root, caller: l.caller, r: r, mode: task.ModePR, commit: *r.VerifiedCommit, step: stepCheck}
	var err error
	if layer.branch, err = landing.TaskBranch(ctx, l.s, r, "land"); err != nil {
		return nil, err
	}
	if err := landing.ValidBranch(ctx, "branch", layer.branch); err != nil {
		return nil, refuse("land_branch_invalid", "%v", err)
	}
	return layer, nil
}

// openLayers finds or opens the pull request of each layer into its layer
// base, bottom first, and stores its pr_url. A layer with a stored pr_url
// uses that pull request and never opens another. Then it checks each pull
// request and forms their GitHub stack. Then every layer and the parent store
// the review step.
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
	if err := l.formStack(ctx, gh, prs); err != nil {
		return err
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

// formStack reads every layer pull request. One pull request shows only its
// own stack, so the check needs all of them. When no pull request is in a
// stack, formStack posts one stack of all of them, bottom first. When all are
// in one stack of the layer count, each at its layer number, it posts
// nothing. Any other state is land_stack_mismatch.
func (l *lander) formStack(ctx context.Context, gh landing.GH, prs []landing.PR) error {
	numbers := make([]int, len(prs))
	var states []string
	var first *landing.Stack
	none, same := true, true
	for i, pr := range prs {
		p, err := gh.Pull(ctx, pr.Number)
		if err != nil {
			return err
		}
		numbers[i] = pr.Number
		if p.Stack == nil {
			same = false
			states = append(states, fmt.Sprintf("#%d in no stack", pr.Number))
			continue
		}
		if first == nil {
			first = p.Stack
		}
		none = false
		same = same && p.Stack.Number == first.Number && p.Stack.Size == len(prs) && p.Stack.Position == i+1
		states = append(states, fmt.Sprintf("#%d in stack %d at position %d of %d", pr.Number, p.Stack.Number, p.Stack.Position, p.Stack.Size))
	}
	switch {
	case none:
		n, err := gh.CreateStack(ctx, numbers)
		l.stackNumber = n
		return err
	case same:
		l.stackNumber = first.Number
		return nil
	}
	return refuse("land_stack_mismatch", "the layer pull requests of stack %s are not one GitHub stack in layer order: %s", l.r.ID, strings.Join(states, ", "))
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
