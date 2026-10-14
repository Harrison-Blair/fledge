// Package gitstatus answers read-only questions about checkouts with git:
// whether a checkout is dirty, which branch it has checked out, which branch is
// the repository integration branch, and whether a revision is merged into it.
// Each yes-or-no answer is "yes", "no", or "unknown" when git cannot say.
package gitstatus

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/repoconfig"
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
// the [branches] dev branch of .fledge/config.toml in the primary checkout
// repo, as refs/heads/<dev> or else refs/remotes/origin/<dev>; else the
// target of refs/remotes/origin/HEAD, else refs/heads/main, or "" when none
// of them exists. Git configuration is not read. A dev branch that the file
// names and that does not exist, or an unreadable or invalid file, is an
// error rather than a fallback, so merged checks become "unknown". A default
// dev branch that does not exist falls back.
func DefaultBranch(ctx context.Context, repo string) (string, error) {
	c, err := repoconfig.Read(ctx, repo)
	if err != nil {
		return "", err
	}
	dev := c.Branches.Dev
	for _, ref := range []string{"refs/heads/" + dev, "refs/remotes/origin/" + dev} {
		if exists(ctx, repo, ref) {
			return ref, nil
		}
	}
	if c.Branches.DevSet {
		return "", fmt.Errorf("%s [branches] dev names %s, which does not exist locally or on origin", repoconfig.Path, dev)
	}
	if b, err := exec.CommandContext(ctx, "git", "-C", repo, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD").Output(); err == nil {
		return strings.TrimSpace(string(b)), nil
	}
	if exists(ctx, repo, "refs/heads/main") {
		return "refs/heads/main", nil
	}
	return "", nil
}

// Branch returns the short name of the branch checked out at dir, or nil
// when HEAD is detached or git cannot say.
func Branch(ctx context.Context, dir string) *string {
	b, err := exec.CommandContext(ctx, "git", "-C", dir, "symbolic-ref", "--quiet", "--short", "HEAD").Output()
	if err != nil {
		return nil
	}
	name := strings.TrimSpace(string(b))
	return &name
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
