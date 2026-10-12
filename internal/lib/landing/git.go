package landing

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
)

var fullHash = regexp.MustCompile(`^[0-9a-f]{40}$`)

// FullHash reports whether s is a 40-hex commit id.
func FullHash(s string) bool { return fullHash.MatchString(s) }

// ResolveCommit resolves rev to the 40-hex commit it names in the checkout at
// dir. --end-of-options keeps a rev that starts with - from being an option.
func ResolveCommit(ctx context.Context, dir, rev string) (string, error) {
	if strings.TrimSpace(rev) == "" || strings.ContainsAny(rev, "\x00\r\n") {
		return "", cli.Invalid("--commit must be a nonblank single-line revision")
	}
	b, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}").Output()
	hash := strings.TrimSpace(string(b))
	if err != nil || !FullHash(hash) {
		return "", cli.Invalid("--commit %q does not name a commit in %s", rev, dir)
	}
	return hash, nil
}

// ValidBranch checks a branch name with the rule .fledge/config.toml
// base_branch follows: nonblank, a single line, no leading -, and accepted by
// git check-ref-format as refs/heads/<name>. flag names the value in errors.
func ValidBranch(ctx context.Context, flag, name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return cli.Invalid("%s is blank", flag)
	case strings.ContainsAny(name, "\r\n"):
		return cli.Invalid("%s %q spans lines", flag, name)
	case strings.HasPrefix(name, "-"):
		return cli.Invalid("%s %q starts with -", flag, name)
	}
	if exec.CommandContext(ctx, "git", "check-ref-format", "refs/heads/"+name).Run() != nil {
		return cli.Invalid("%s %q is not a valid branch name", flag, name)
	}
	return nil
}

// ShortBranch reduces refs/heads/<name> or refs/remotes/origin/<name> to
// <name>; any other value is returned unchanged.
func ShortBranch(ref string) string {
	for _, prefix := range []string{"refs/heads/", "refs/remotes/origin/"} {
		if name, ok := strings.CutPrefix(ref, prefix); ok {
			return name
		}
	}
	return ref
}

// RefCommit returns the commit ref points at in the repository at dir, or ""
// when ref does not exist.
func RefCommit(ctx context.Context, dir, ref string) string {
	b, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Checkouts returns the paths of the checkouts of the repository at dir that
// have branch checked out.
func Checkouts(ctx context.Context, dir, branch string) ([]string, error) {
	b, err := exec.CommandContext(ctx, "git", "-C", dir, "worktree", "list", "--porcelain", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("list checkouts: %w", err)
	}
	var paths []string
	var path string
	for _, field := range strings.Split(string(b), "\x00") {
		switch {
		case strings.HasPrefix(field, "worktree "):
			path = strings.TrimPrefix(field, "worktree ")
		case field == "branch refs/heads/"+branch:
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// Changes lists the paths with staged or unstaged changes in the checkout at
// dir; untracked files are not changes.
func Changes(ctx context.Context, dir string) ([]string, error) {
	b, err := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain=v1", "-z", "--untracked-files=no").Output()
	if err != nil {
		return nil, fmt.Errorf("read status of %s: %w", dir, err)
	}
	var paths []string
	fields := strings.Split(string(b), "\x00")
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		paths = append(paths, entry[3:])
		// A rename or copy is followed by its source path.
		if entry[0] == 'R' || entry[0] == 'C' || entry[1] == 'R' || entry[1] == 'C' {
			i++
		}
	}
	return paths, nil
}
