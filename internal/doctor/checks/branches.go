package checks

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/Harrison-Blair/fledge/internal/doctor/report"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/repoconfig"
)

// Branches reports whether the dev branch that [branches] names exists as a
// local branch or on origin. When the dev branch is missing, the detail gives
// the git commands that make it. The check does not make the branch.
func Branches(ctx context.Context, e Environment) report.Check {
	check := report.Check{Name: "branches", Status: report.Warn}
	cwd, err := e.Getwd()
	if err != nil {
		check.Detail = err.Error()
		return check
	}
	root, err := fledgedir.Root(ctx, cwd)
	if errors.Is(err, fledgedir.ErrNotRepository) {
		check.Status, check.Detail = report.OK, "not in a repository"
		return check
	}
	if err != nil {
		check.Detail = err.Error()
		return check
	}
	c, err := repoconfig.Read(ctx, root)
	if err != nil {
		check.Detail = err.Error()
		return check
	}
	data := report.BranchesData{Main: c.Branches.Main, Dev: c.Branches.Dev}
	for _, ref := range []string{"refs/heads/" + data.Dev, "refs/remotes/origin/" + data.Dev} {
		if exec.CommandContext(ctx, "git", "-C", root, "show-ref", "--verify", "--quiet", ref).Run() == nil {
			data.Ref = ref
			break
		}
	}
	check.Data = data
	if data.Ref != "" {
		check.Status, check.Detail = report.OK, fmt.Sprintf("dev branch %s exists as %s", data.Dev, data.Ref)
		return check
	}
	check.Detail = fmt.Sprintf("dev branch %s does not exist locally or on origin. Make it with:\ngit branch %s %s\ngit push -u origin %s", data.Dev, data.Dev, data.Main, data.Dev)
	return check
}
