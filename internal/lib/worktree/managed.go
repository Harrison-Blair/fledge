package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
)

// Prepare returns the managed checkout path root/.fledge/worktrees/<branch>.
// It validates all known collisions before touching managed paths, then
// ensures the ignored .fledge directory and the checkout's parent directories.
func Prepare(ctx context.Context, root, branch string, out *libagent.Outcome) (string, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	checked, err := exec.CommandContext(ctx, "git", "check-ref-format", "--branch", branch).Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("check branch: %w", ctx.Err())
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() < 0 {
			return "", fmt.Errorf("check branch: %w", err)
		}
	}
	if err != nil || strings.TrimSpace(string(checked)) != branch || strings.Contains(branch, "@{") {
		return "", libagent.Invalid("invalid exact branch name %q", branch)
	}
	refs, err := exec.CommandContext(ctx, "git", "-C", root, "for-each-ref", "--format=%(refname)", "refs/heads/").Output()
	if err != nil {
		return "", fmt.Errorf("list branches: %w", err)
	}
	requested := "refs/heads/" + branch
	for _, ref := range strings.Fields(string(refs)) {
		if ref == requested {
			return "", libagent.Invalid("branch %q already exists", branch)
		}
		if strings.HasPrefix(requested, ref+"/") || strings.HasPrefix(ref, requested+"/") {
			return "", libagent.Invalid("branch %q conflicts with existing branch %q", branch, strings.TrimPrefix(ref, "refs/heads/"))
		}
	}

	managed := filepath.Join(root, ".fledge", "worktrees")
	path := filepath.Join(managed, filepath.FromSlash(branch))
	relative, err := filepath.Rel(managed, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", libagent.Invalid("branch path escapes managed worktrees")
	}
	if _, err = os.Lstat(path); err == nil {
		return "", libagent.Invalid("worktree destination %s already exists", path)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err = fledgedir.CheckParents(root, filepath.Dir(path)); err != nil {
		return "", err
	}
	if _, err = fledgedir.Ensure(root, out); err != nil {
		return "", err
	}
	if err = fledgedir.MakeParents(root, filepath.Dir(path), out); err != nil {
		return "", err
	}

	rel, _ := filepath.Rel(root, path)
	if b, err := exec.CommandContext(ctx, "git", "-C", root, "check-ignore", "--no-index", "--quiet", "--", rel).CombinedOutput(); err != nil {
		return "", fmt.Errorf("managed checkout is not ignored: %w %s", err, b)
	}
	return path, nil
}
