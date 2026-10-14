package land

import (
	"context"
	"os/exec"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/landing"
	"github.com/Harrison-Blair/fledge/internal/lib/worktree"
)

// local merges the verified commit into the target branch in the checkout
// that has the target checked out: the primary checkout or a managed
// worktree. That checkout must have no staged or unstaged changes. It never
// stashes, resets, or checks out. A failed merge is aborted, and the
// checkout's HEAD and full status must then match the snapshot taken before
// the merge byte for byte.
func (l *lander) local(ctx context.Context) error {
	dir, err := l.targetCheckout(ctx)
	if err != nil {
		return err
	}
	changes, err := landing.Changes(ctx, dir)
	if err != nil {
		return err
	}
	if len(changes) > 0 {
		return refuse("land_target_checkout_dirty", "checkout %s of target %s has staged or unstaged changes: %s", dir, l.target, strings.Join(changes, ", "))
	}
	head := landing.RefCommit(ctx, dir, "HEAD")
	status, err := fullStatus(ctx, dir)
	if err != nil {
		return err
	}
	l.step = stepMerge
	output, err := l.gitIn(ctx, dir, "merge", "--no-ff", "-m", subject(l.branch, l.r.ID), l.commit).CombinedOutput()
	if err != nil {
		var conflicts []string
		if landing.RefCommit(ctx, dir, "MERGE_HEAD") != "" {
			b, _ := l.gitIn(ctx, dir, "diff", "--name-only", "--diff-filter=U", "-z").Output()
			conflicts = strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
			l.gitIn(ctx, dir, "merge", "--abort").Run()
		}
		after, statusErr := fullStatus(ctx, dir)
		if statusErr != nil || landing.RefCommit(ctx, dir, "HEAD") != head || after != status {
			return refuse("land_restore_failed", "the failed merge could not be undone in %s; inspect it by hand, HEAD should be %s and the changed paths are: %s", dir, head, strings.Join(paths(after), ", "))
		}
		if len(conflicts) > 0 && conflicts[0] != "" {
			return refuse("land_merge_conflict", "merging %s into %s conflicts in %s; the merge was aborted", l.branch, l.target, strings.Join(conflicts, ", "))
		}
		return refuse("land_merge_refused", "git refused to merge %s into %s: %s", l.branch, l.target, strings.TrimSpace(string(output)))
	}
	return l.record(landing.RefCommit(ctx, dir, "HEAD"), nil)
}

// targetCheckout returns the checkout that has the target checked out, when
// it is the primary checkout or a managed worktree.
func (l *lander) targetCheckout(ctx context.Context) (string, error) {
	paths, err := landing.Checkouts(ctx, l.root, l.target)
	if err != nil {
		return "", err
	}
	root := worktree.Canonical(l.root)
	for _, path := range paths {
		if path = worktree.Canonical(path); path == root || worktree.Managed(root, path) {
			return path, nil
		}
	}
	return "", refuse("land_target_checkout_missing", "no primary checkout or managed worktree has %s checked out; check out %s in one of them first", l.target, l.target)
}

func (l *lander) git(ctx context.Context, args ...string) *exec.Cmd {
	return l.gitIn(ctx, l.root, args...)
}

func (l *lander) gitIn(ctx context.Context, dir string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
}

// subject is the merge commit message of branch landing task id.
func subject(branch, id string) string {
	return "Merge branch '" + branch + "' (verified task " + id + ")"
}

// fullStatus is the checkout's status including every untracked file.
func fullStatus(ctx context.Context, dir string) (string, error) {
	b, err := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain=v1", "-z", "--untracked-files=all").Output()
	return string(b), err
}

// paths lists the entries of a -z porcelain status.
func paths(status string) []string {
	var ps []string
	for _, entry := range strings.Split(status, "\x00") {
		if len(entry) > 3 {
			ps = append(ps, entry[3:])
		}
	}
	return ps
}
