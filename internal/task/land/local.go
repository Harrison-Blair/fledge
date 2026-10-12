package land

import (
	"context"
	"os/exec"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/gitstatus"
	"github.com/Harrison-Blair/fledge/internal/lib/landing"
)

// local merges the verified commit into the target branch in the primary
// checkout, which must have the target checked out with no staged or
// unstaged changes. It never stashes, resets, or checks out. A failed merge
// is aborted, and the checkout's HEAD and full status must then match the
// snapshot taken before the merge byte for byte.
func (l *lander) local(ctx context.Context) error {
	if b := gitstatus.Branch(ctx, l.root); b == nil || *b != l.target {
		on := "a detached HEAD"
		if b != nil {
			on = "branch " + *b
		}
		return refuse("land_primary_not_ready", "the primary checkout %s is on %s, not %s; check out %s there first", l.root, on, l.target, l.target)
	}
	changes, err := landing.Changes(ctx, l.root)
	if err != nil {
		return err
	}
	if len(changes) > 0 {
		return refuse("land_primary_not_ready", "the primary checkout %s has staged or unstaged changes: %s", l.root, strings.Join(changes, ", "))
	}
	head := landing.RefCommit(ctx, l.root, "HEAD")
	status, err := fullStatus(ctx, l.root)
	if err != nil {
		return err
	}
	l.step = stepMerge
	output, err := l.git(ctx, "merge", "--no-ff", "-m", subject(l.branch, l.r.ID), l.commit).CombinedOutput()
	if err != nil {
		var conflicts []string
		if landing.RefCommit(ctx, l.root, "MERGE_HEAD") != "" {
			b, _ := l.git(ctx, "diff", "--name-only", "--diff-filter=U", "-z").Output()
			conflicts = strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
			l.git(ctx, "merge", "--abort").Run()
		}
		after, statusErr := fullStatus(ctx, l.root)
		if statusErr != nil || landing.RefCommit(ctx, l.root, "HEAD") != head || after != status {
			return refuse("land_restore_failed", "the failed merge could not be undone in %s; inspect it by hand, HEAD should be %s and the changed paths are: %s", l.root, head, strings.Join(paths(after), ", "))
		}
		if len(conflicts) > 0 && conflicts[0] != "" {
			return refuse("land_merge_conflict", "merging %s into %s conflicts in %s; the merge was aborted", l.branch, l.target, strings.Join(conflicts, ", "))
		}
		return refuse("land_merge_refused", "git refused to merge %s into %s: %s", l.branch, l.target, strings.TrimSpace(string(output)))
	}
	return l.record(landing.RefCommit(ctx, l.root, "HEAD"), nil)
}

func (l *lander) git(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "git", append([]string{"-C", l.root}, args...)...)
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
