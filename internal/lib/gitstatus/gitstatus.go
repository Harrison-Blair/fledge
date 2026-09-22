// Package gitstatus answers read-only questions about checkouts with git:
// whether a checkout is dirty, which branch is the repository default, and
// whether a revision is merged into it. Each answer is "yes", "no", or
// "unknown" when git cannot say.
package gitstatus

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Dirty reports whether the checkout at path has modified, staged, or
// untracked files. A path that is not itself a checkout's top level, such as
// a leftover directory inside another checkout, is "unknown".
func Dirty(ctx context.Context, path string) string {
	top, err := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--show-toplevel").Output()
	if err != nil || !samePath(strings.TrimSpace(string(top)), path) {
		return "unknown"
	}
	status, err := exec.CommandContext(ctx, "git", "-C", path, "status", "--porcelain", "--untracked-files=all").Output()
	switch {
	case err != nil:
		return "unknown"
	case len(status) > 0:
		return "yes"
	}
	return "no"
}

func samePath(a, b string) bool {
	a, errA := filepath.EvalSymlinks(a)
	b, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && filepath.Clean(a) == filepath.Clean(b)
}

// DefaultBranch returns the full ref of the repository integration branch:
// the branch named by git config fledge.baseBranch, else the target of
// refs/remotes/origin/HEAD, else refs/heads/main, or "" when none of them
// exists. A configured branch that does not exist, or unreadable config, is an
// error rather than a fallback, so merged checks become "unknown".
func DefaultBranch(ctx context.Context, repo string) (string, error) {
	name, err := exec.CommandContext(ctx, "git", "-C", repo, "config", "--get", "fledge.baseBranch").Output()
	var exit *exec.ExitError
	switch {
	case err == nil:
		ref := "refs/heads/" + strings.TrimSpace(string(name))
		if !exists(ctx, repo, ref) {
			return "", fmt.Errorf("git config fledge.baseBranch names %s, which does not exist", ref)
		}
		return ref, nil
	case !errors.As(err, &exit) || exit.ExitCode() != 1:
		return "", fmt.Errorf("read git config fledge.baseBranch: %v", err)
	}
	if b, err := exec.CommandContext(ctx, "git", "-C", repo, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD").Output(); err == nil {
		return strings.TrimSpace(string(b)), nil
	}
	if exists(ctx, repo, "refs/heads/main") {
		return "refs/heads/main", nil
	}
	return "", nil
}

func exists(ctx context.Context, repo, ref string) bool {
	return exec.CommandContext(ctx, "git", "-C", repo, "show-ref", "--verify", "--quiet", ref).Run() == nil
}

// Merged reports whether rev, resolved in dir, is an ancestor of target.
// A squash merge leaves rev unmerged.
func Merged(ctx context.Context, dir, rev, target string) string {
	if target == "" {
		return "unknown"
	}
	err := exec.CommandContext(ctx, "git", "-C", dir, "merge-base", "--is-ancestor", rev, target).Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return "yes"
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return "no"
	}
	return "unknown"
}
