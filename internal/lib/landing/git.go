package landing

import (
	"context"
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
