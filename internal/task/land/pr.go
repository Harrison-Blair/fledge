package land

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/landing"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// pr pushes the verified commit to the branch on origin, finds or opens one
// pull request into the target, checks it, merges it only while its head is
// the verified commit, and records the landing once GitHub reports it
// merged. In pr mode without --merge it stops at review after the checks.
// Each step is idempotent, so a refused landing can be run again; the step
// reached is stored as land_step before the step runs.
func (l *lander) pr(ctx context.Context) error {
	gh := landing.GH{Dir: l.root}
	if err := l.reach(stepPush, nil); err != nil {
		return err
	}
	if landing.RefCommit(ctx, l.root, "refs/remotes/origin/"+l.branch) != l.commit {
		if output, err := l.git(ctx, "push", "origin", l.commit+":refs/heads/"+l.branch).CombinedOutput(); err != nil {
			return refuse("land_push_rejected", "pushing %s to origin %s failed: %s", l.commit, l.branch, strings.TrimSpace(string(output)))
		}
	}
	if err := l.reach(stepPR, nil); err != nil {
		return err
	}
	pr, err := l.pullRequest(ctx, gh)
	if err != nil {
		return err
	}
	if err := l.reach(stepChecks, &pr.URL); err != nil {
		return err
	}
	c, err := gh.Checks(ctx, pr.Number)
	if err != nil {
		return err
	}
	if err := l.mergeable(pr, c); err != nil {
		return err
	}
	if l.mode == task.ModePR && !l.merge {
		l.review = true
		return l.reach(stepReview, nil)
	}
	if err := l.reach(stepMerge, nil); err != nil {
		return err
	}
	if err := gh.Merge(ctx, pr.Number, l.commit, subject(l.branch, l.r.ID)); err != nil {
		return refuse("land_merge_failed", "gh could not merge %s: %v", pr.URL, message(err))
	}
	if err := l.reach(stepConfirm, nil); err != nil {
		return err
	}
	m, err := gh.Merged(ctx, pr.Number)
	if err != nil {
		return err
	}
	if m.State != "MERGED" || !landing.FullHash(m.MergeCommit) {
		return refuse("land_unconfirmed", "%s is %s after the merge request, not merged with a merge commit; check it, for example for a merge queue, and run task land again", pr.URL, m.State)
	}
	if err := l.record(m.MergeCommit, &pr.URL); err != nil {
		return err
	}
	// Update the local view of the target so merged checks see the landing.
	if output, err := l.git(ctx, "fetch", "origin", l.target).CombinedOutput(); err != nil {
		l.warnings = append(l.warnings, fmt.Sprintf("git fetch origin %s failed: %s", l.target, strings.TrimSpace(string(output))))
	}
	return nil
}

// reach stores step as the land step reached, and prURL when set.
func (l *lander) reach(step string, prURL *string) error {
	l.step = step
	return l.update(func(r *task.Record) {
		r.Branch, r.LandStep = &l.branch, &l.step
		if prURL != nil {
			r.PRURL = prURL
		}
	})
}

// pullRequest returns the one open pull request from the branch into the
// target, opening it when there is none. The URL of an opened pull request is
// stored before it is listed again, so a failed lookup keeps it.
func (l *lander) pullRequest(ctx context.Context, gh landing.GH) (landing.PR, error) {
	prs, err := gh.List(ctx, l.branch, l.target)
	if err == nil && len(prs) == 0 {
		var url string
		if url, err = l.open(ctx, gh); err == nil && url != "" {
			err = l.reach(stepPR, &url)
		}
		if err == nil {
			prs, err = gh.List(ctx, l.branch, l.target)
		}
	}
	switch {
	case err != nil:
		return landing.PR{}, err
	case len(prs) == 0:
		return landing.PR{}, refuse("land_gh_output_invalid", "gh pr create succeeded but gh pr list shows no open pull request from %s into %s", l.branch, l.target)
	case len(prs) > 1:
		var urls []string
		for _, p := range prs {
			urls = append(urls, p.URL)
		}
		return landing.PR{}, refuse("land_pr_ambiguous", "more than one open pull request from %s into %s: %s; close the extras", l.branch, l.target, strings.Join(urls, ", "))
	}
	return prs[0], nil
}

// open creates the pull request with a body naming the task, the verified
// commit, its verifier and note, and its owner, and returns its URL.
func (l *lander) open(ctx context.Context, gh landing.GH) (string, error) {
	f, err := os.CreateTemp("", "fledge-land-*.md")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	body := fmt.Sprintf("Fledge task %s, verified at commit %s.\n\n- Verifier: %s\n- Owner: %s\n", l.r.ID, l.commit, cli.Display(l.r.Verifier), cli.Display(l.r.Owner))
	if l.r.VerificationNote != nil {
		body += "\nVerification note:\n\n" + *l.r.VerificationNote + "\n"
	}
	_, err = f.WriteString(body)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	return gh.Create(ctx, l.branch, l.target, l.r.Title, f.Name())
}

// mergeable refuses a pull request that is a draft, not cleanly mergeable,
// at another head, or whose checks are pending or failed. Zero checks pass.
func (l *lander) mergeable(pr landing.PR, c landing.Checks) error {
	switch {
	case c.IsDraft:
		return refuse("land_pr_draft", "%s is a draft", pr.URL)
	case c.Mergeable == "UNKNOWN":
		return refuse("land_mergeable_unknown", "GitHub has not yet computed whether %s can merge; run task land again shortly", pr.URL)
	case c.Mergeable != "MERGEABLE":
		return refuse("land_pr_not_mergeable", "%s is %s", pr.URL, c.Mergeable)
	case c.HeadRefOid != l.commit:
		return refuse("land_head_mismatch", "%s has head %s, not the verified commit %s", pr.URL, c.HeadRefOid, l.commit)
	}
	for _, check := range c.Rollup {
		if check.Typename == "CheckRun" && check.Status != "COMPLETED" || check.Typename == "StatusContext" && (check.State == "PENDING" || check.State == "EXPECTED") {
			return refuse("land_checks_pending", "checks on %s are still running; run task land again when they finish", pr.URL)
		}
	}
	for _, check := range c.Rollup {
		if check.Typename == "CheckRun" && !slices.Contains([]string{"SUCCESS", "NEUTRAL", "SKIPPED"}, check.Conclusion) || check.Typename == "StatusContext" && check.State != "SUCCESS" {
			return refuse("land_checks_failed", "checks on %s failed", pr.URL)
		}
	}
	return nil
}

// message is err's message without a code prefix.
func message(err error) string {
	var coded *cli.Error
	if errors.As(err, &coded) {
		return coded.Message
	}
	return err.Error()
}
