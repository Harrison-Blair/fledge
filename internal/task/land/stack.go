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

// After a merge, the reconcile reads a lagging pull request confirmReads more
// times, confirmInterval apart. Tests change them.
var confirmReads, confirmInterval = 3, 2 * time.Second

// Test hooks: beforeRecord runs between the GitHub read and the record
// writes of the reconcile, and putRecord writes one record.
var (
	beforeRecord = func() {}
	putRecord    = func(tx *state.Tx, r task.Record) error { return tx.Put(task.Kind, r.ID, r) }
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
// and the delivery mode, so --branch and --mode are invalid input. Each run
// first makes the records follow GitHub, and names the layers that it
// recorded. A landed stack then ends the run.
// A layer lands only after the parent reached review. The parent guards run
// on each other run. Then a run continues the merge request that a layer
// holds. Else --merge requests the merge of the selected layer, or of the top
// layer for the parent. Else a run with a landed layer ends. Else the push
// step and the pull request steps run.
func (l *lander) stack(ctx context.Context, o Options, p task.Record) error {
	if o.BranchSet || o.ModeSet {
		return cli.Invalid("--branch and --mode do not apply to a stack task: the stack records store the branches and the delivery mode")
	}
	selected := l.r.ID
	l.r, l.mode, l.commit = p, p.DeliveryMode, commit(&p)
	records, err := landing.StackLayers(l.s, p, "land")
	if err != nil {
		return err
	}
	gh := landing.GH{Dir: l.root}
	records, wrote, recorded, err := l.follow(ctx, gh, records, -1, "")
	if err != nil {
		return err
	}
	if len(recorded) > 0 {
		l.summary = append(l.summary, "Recorded a GitHub merge of "+layerNames("layer", recorded)+".")
	}
	p = l.r
	low := slices.IndexFunc(records, func(r task.Record) bool { return r.Status != task.Landed })
	switch {
	case p.Status == task.Landed && low < 0:
		l.summarize(records, wrote)
		return nil
	case p.Status == task.Landed:
		return refuse("task_invalid_state", "stack %s is landed, but its layer %s is %s; inspect it with fledge task get --id %s", p.ID, records[low].ID, records[low].Status, records[low].ID)
	case selected != p.ID && (p.LandStep == nil || *p.LandStep != stepReview):
		return refuse("land_stack_not_open", "stack %s has no open pull requests. Land the stack with fledge task land --id %s", p.ID, p.ID)
	}
	if err := task.Require(&p, "land", task.Verified); err != nil {
		return err
	}
	if p.DeliveryMode != task.ModePR {
		return refuse("land_stack_mode", "stack %s has delivery mode %s. A stack lands only in pr mode", p.ID, p.DeliveryMode)
	}
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
	if slices.ContainsFunc(records, func(r task.Record) bool { return r.Status == task.Landed }) {
		l.summarize(records, false)
		return nil
	}
	layers, err := l.pushLayers(ctx, records)
	if err != nil {
		return err
	}
	return l.openLayers(ctx, layers)
}

// requestMerge requests the merge of the unlanded layers up to layer k: from
// the lowest layer that is not landed. When there is no such layer, it sends
// nothing. Each of these layers needs the review step, a pull request, and
// green checks at the head that the reconcile read. The PUT sends the current
// head of layer k. Layer k stores the uuid of a pending answer before the
// poll starts.
func (l *lander) requestMerge(ctx context.Context, gh landing.GH, records []task.Record, k int) error {
	low := slices.IndexFunc(records, func(r task.Record) bool { return r.Status != task.Landed })
	if low < 0 || low > k {
		l.summarize(records, false)
		return nil
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
		if pr, err = layer.pullHead(ctx, l.pulls[r.ID]); err != nil {
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
		return l.recordMerge(ctx, gh, k, "")
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
				return l.recordMerge(ctx, gh, j, "")
			}
			l.pulls[layer.r.ID], l.step, records[j] = p, stepCheck, layer.r
			return l.requestMerge(ctx, gh, records, j)
		case err != nil:
			return err
		case a.Status == "failed":
			if err := clear(); err != nil {
				return err
			}
			return mergeFailed(n, a)
		case a.Status == "merged":
			return l.recordMerge(ctx, gh, j, uuid)
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

// recordMerge is the record step after GitHub merged the pull request of
// layer j. uuid is the merge request that layer j holds, or "". It runs the
// reconcile again, and layer j must then be landed.
func (l *lander) recordMerge(ctx context.Context, gh landing.GH, j int, uuid string) error {
	records, err := landing.StackLayers(l.s, l.r, "land")
	if err != nil {
		return err
	}
	records, wrote, _, err := l.follow(ctx, gh, records, j, uuid)
	if err != nil {
		return err
	}
	if records[j].Status != task.Landed {
		return refuse("land_unconfirmed", "GitHub merged the pull request of layer %s of stack %s, but the pull request still reads open. Record the merge later with fledge task land --id %s", records[j].ID, l.r.ID, l.r.ID)
	}
	l.summarize(records, wrote)
	return nil
}

// follow runs the reconcile. It also returns the positions of the layers
// that the reconcile moved to landed. When there are such layers, follow runs
// git fetch origin <base> one time. A failed git fetch adds a warning.
func (l *lander) follow(ctx context.Context, gh landing.GH, records []task.Record, j int, uuid string) ([]task.Record, bool, []int, error) {
	stored, wrote, err := l.reconcile(ctx, gh, records, j, uuid)
	if err != nil {
		return nil, false, nil, err
	}
	var recorded []int
	for i, r := range stored {
		if r.Status == task.Landed && records[i].Status != task.Landed {
			recorded = append(recorded, i+1)
		}
	}
	if len(recorded) > 0 {
		base := cli.Display(stored[recorded[0]-1].TargetBranch)
		if output, err := l.git(ctx, "fetch", "origin", base).CombinedOutput(); err != nil {
			l.warnings = append(l.warnings, fmt.Sprintf("git fetch origin %s failed: %s", base, strings.TrimSpace(string(output))))
		}
	}
	return stored, wrote, recorded, nil
}

// reconcile makes the stack records follow GitHub. It reads the pull request
// of each layer with a pr_url one time. A lagging read of layer j gets
// confirmReads more reads. Each merged layer that is not landed must have
// its verified commit as head, or a head with the verified patch id. Then one
// Exclusive call reads the records again and makes the writes, bottom first.
// Each merged layer moves to landed with its carry. The call clears a stored
// uuid when GitHub merged its pull request, or when it is uuid of layer j.
// When every layer is landed, the parent moves to landed. reconcile writes
// nothing before every check passes. It returns the stored layers, and
// whether it landed the parent.
func (l *lander) reconcile(ctx context.Context, gh landing.GH, records []task.Record, j int, uuid string) ([]task.Record, bool, error) {
	l.pulls = map[string]landing.Pull{}
	clears := map[string]string{}
	for i, r := range records {
		if r.PRURL == nil {
			continue
		}
		n, err := strconv.Atoi(path.Base(*r.PRURL))
		if err != nil {
			return nil, false, refuse("land_gh_output_invalid", "pull request URL %s of layer %s has no number", *r.PRURL, r.ID)
		}
		pull, err := gh.Pull(ctx, n)
		for k := 0; err == nil && i == j && !pull.Merged && k < confirmReads; k++ {
			time.Sleep(confirmInterval)
			pull, err = gh.Pull(ctx, n)
		}
		switch {
		case err != nil:
			return nil, false, err
		case pull.State == "closed" && !pull.Merged:
			return nil, false, refuse("land_pr_closed", "pull request %s of layer %s was closed without a merge; reopen it, then run task land again", *r.PRURL, r.ID)
		case pull.Merged && r.Status != task.Landed && pull.HeadSHA != commit(&r):
			if same, note := l.samePatch(ctx, r, pull); !same {
				return nil, false, refuse("land_pr_merged_other_head", "pull request %s of layer %s was merged at head %s, not the verified commit %s, and its patch id against base %s is not the verified patch id%s", *r.PRURL, r.ID, pull.HeadSHA, commit(&r), pull.BaseSHA, note)
			}
		}
		l.pulls[r.ID] = pull
		if i == j && uuid != "" {
			clears[r.ID] = uuid
		} else if pull.Merged && r.MergeRequest != nil {
			clears[r.ID] = *r.MergeRequest
		}
	}
	if len(l.pulls) == 0 {
		return records, false, nil
	}
	base, err := landing.TaskTarget(ctx, l.root, l.r, "land")
	if err != nil {
		return nil, false, err
	}
	beforeRecord()
	var stored []task.Record
	wrote := false
	err = l.s.Exclusive(func(tx *state.Tx) error {
		var p task.Record
		if err := tx.Get(task.Kind, l.r.ID, &p); err != nil {
			return err
		}
		var writes []task.Record
		step := stepConfirm
		for _, old := range records {
			var r task.Record
			if err := tx.Get(task.Kind, old.ID, &r); err != nil {
				return err
			}
			w, changed := r, false
			if pull, ok := l.pulls[r.ID]; ok && pull.Merged && old.Status != task.Landed {
				if r.Status != task.Verified || commit(&r) != commit(&old) {
					return refuse("task_state_changed", "layer %s changed while it was being recorded; inspect it with fledge task get --id %s", r.ID, r.ID)
				}
				w.Status, w.MergeCommit, w.TargetBranch, w.LandStep, w.LandedAt, w.LandedBy = task.Landed, &pull.MergeCommit, &base, &step, task.Now(), &l.caller
				if from := commit(&old); pull.HeadSHA != from {
					w.VerifiedCommit, w.CarriedFrom = &pull.HeadSHA, &from
				}
				changed = true
			}
			if u, ok := clears[r.ID]; ok {
				if r.MergeRequest == nil || *r.MergeRequest != u {
					return refuse("task_state_changed", "layer %s no longer holds merge request %s; inspect it with fledge task get --id %s", r.ID, u, r.ID)
				}
				w.MergeRequest, changed = nil, true
			}
			if changed {
				writes = append(writes, w)
			}
			stored = append(stored, w)
		}
		if p.Status == task.Verified && !slices.ContainsFunc(stored, func(r task.Record) bool { return r.Status != task.Landed }) {
			p.Status, p.MergeCommit, p.TargetBranch, p.LandStep, p.LandedAt, p.LandedBy = task.Landed, stored[len(stored)-1].MergeCommit, &base, &step, task.Now(), &l.caller
			writes, wrote = append(writes, p), true
		}
		var written []string
		for _, w := range writes {
			if err := putRecord(tx, w); err != nil {
				return refuse("land_record_incomplete", "the reconcile wrote the records %v but could not write task %s: %v. Finish the record with fledge task land --id %s", written, w.ID, err, l.r.ID)
			}
			written = append(written, w.ID)
		}
		l.r, l.commit = p, commit(&p)
		return nil
	})
	return stored, wrote, err
}

// samePatch reports whether the head of pull request p has the verified
// patch id of r against the base of p. GitHub can make a head that the
// repository does not have. When the patch id read fails and the base or the
// head is missing, samePatch runs git fetch origin <base> <head> one time by
// SHA, which moves no ref, and reads the patch id again. note names a failed
// git fetch, else it is "".
func (l *lander) samePatch(ctx context.Context, r task.Record, p landing.Pull) (same bool, note string) {
	id, err := landing.PatchID(ctx, l.root, p.BaseSHA, p.HeadSHA)
	if err != nil && (l.git(ctx, "cat-file", "-e", p.BaseSHA+"^{commit}").Run() != nil || l.git(ctx, "cat-file", "-e", p.HeadSHA+"^{commit}").Run() != nil) {
		if output, err := l.git(ctx, "fetch", "--no-write-fetch-head", "origin", p.BaseSHA, p.HeadSHA).CombinedOutput(); err != nil {
			return false, fmt.Sprintf(". git fetch origin %s %s failed: %s", p.BaseSHA, p.HeadSHA, strings.TrimSpace(string(output)))
		}
		id, err = landing.PatchID(ctx, l.root, p.BaseSHA, p.HeadSHA)
	}
	return err == nil && id != "" && r.VerifiedPatchID != nil && id == *r.VerifiedPatchID, ""
}

// summarize sets the human lines of a stack run that ends after a record:
// one line for each layer, then the stack line. wrote is true when this run
// landed the parent.
func (l *lander) summarize(records []task.Record, wrote bool) {
	var landed []int
	for i, r := range records {
		if r.Status == task.Landed {
			landed = append(landed, i+1)
			l.summary = append(l.summary, fmt.Sprintf("Layer %s: landed at %s.", r.ID, cli.Display(r.MergeCommit)))
		} else {
			l.summary = append(l.summary, fmt.Sprintf("Layer %s: land step %s.", r.ID, cli.Display(r.LandStep)))
		}
	}
	switch {
	case l.r.Status == task.Landed && wrote:
		l.summary = append(l.summary, fmt.Sprintf("Stack landed at %s.", cli.Display(l.r.MergeCommit)))
	case l.r.Status == task.Landed:
		l.summary = append(l.summary, fmt.Sprintf("Stack %s is already landed at %s.", l.r.ID, cli.Display(l.r.MergeCommit)))
	default:
		l.summary = append(l.summary, layerNames("Layer", landed)+" landed.")
	}
}

// layerNames names the layers at positions ns, in ascending order, after
// word: "Layer 2", "Layers 1 to 3" for a prefix, "Layers 2 and 3", or
// "Layers 1, 3, and 4".
func layerNames(word string, ns []int) string {
	names := make([]string, len(ns))
	for i, n := range ns {
		names[i] = strconv.Itoa(n)
	}
	switch {
	case len(ns) == 1:
		return word + " " + names[0]
	case ns[0] == 1 && ns[len(ns)-1] == len(ns):
		return fmt.Sprintf("%ss 1 to %d", word, len(ns))
	case len(ns) == 2:
		return word + "s " + names[0] + " and " + names[1]
	}
	return word + "s " + strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
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
			pr, err = layer.pullHead(ctx, l.pulls[layer.r.ID])
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
	if err := l.formStack(ctx, gh, layers, prs); err != nil {
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

// formStack uses the reconcile read of each layer pull request, and reads
// each pull request that has no reconcile read. One pull request shows only
// its own stack, so the check needs all of them. When no pull request is in a
// stack, formStack posts one stack of all of them, bottom first. When all are
// in one stack of the layer count, each at its layer number, it posts
// nothing. Any other state is land_stack_mismatch.
func (l *lander) formStack(ctx context.Context, gh landing.GH, layers []*lander, prs []landing.PR) error {
	numbers := make([]int, len(prs))
	var states []string
	var first *landing.Stack
	none, same := true, true
	for i, pr := range prs {
		p, ok := l.pulls[layers[i].r.ID]
		if !ok {
			var err error
			if p, err = gh.Pull(ctx, pr.Number); err != nil {
				return err
			}
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

// pullHead checks p, the REST read of the stored pull request of the layer,
// and returns it. A pull request closed without a merge is land_pr_closed.
// Its head is the current head of the layer, and its base is the patch id
// base. A head with the verified patch id gets the verification. Any other
// head is land_head_mismatch.
func (l *lander) pullHead(ctx context.Context, p landing.Pull) (landing.PR, error) {
	url := *l.r.PRURL
	if p.State == "closed" && !p.Merged {
		return landing.PR{}, refuse("land_pr_closed", "%s of branch %s was closed without a merge; reopen it, then run task land again", url, l.branch)
	}
	pr := landing.PR{Number: p.Number, URL: url, HeadRefOid: p.HeadSHA}
	if p.HeadSHA == l.commit {
		return pr, nil
	}
	if same, note := l.samePatch(ctx, l.r, p); !same {
		return landing.PR{}, refuse("land_head_mismatch", "pull request %s of branch %s has head %s, not the verified commit %s, and its patch id against base %s is not the verified patch id%s; verify the new head again", url, l.branch, p.HeadSHA, l.commit, p.BaseSHA, note)
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
