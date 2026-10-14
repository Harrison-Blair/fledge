// Package land wires task landing.
package land

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/task/land"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options land.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "land", Short: "Merge a verified task's branch into its target branch, locally or through a pull request", Args: cobra.NoArgs,
		Long: "Merge a verified task's branch into its target branch and mark the task landed.\n\n" +
			"The caller must be a registered agent other than the owner. The task must be\n" +
			"verified with --commit, and the branch head must still be that commit. The\n" +
			"branch is --branch, else the branch stored by an earlier land, else the branch\n" +
			"checked out in the owner's recorded checkout, else the branch its spawn\n" +
			"created. The target is the task's base branch, else the repository\n" +
			"integration branch. --mode overrides the task's delivery mode; --branch and\n" +
			"--mode are stored on the task even when the landing is refused.\n\n" +
			"local merges the verified commit with a merge commit in the checkout that has\n" +
			"the target checked out: the primary checkout or a managed worktree. That\n" +
			"checkout must have no staged or unstaged changes; untracked files are\n" +
			"permitted. A failed merge is aborted and the checkout is checked to be exactly\n" +
			"as before.\n\n" +
			"pr lands in two steps. First, task land pushes the verified commit to the\n" +
			"branch on origin, opens or reuses one pull request with gh, and waits until it\n" +
			"is not a draft, is mergeable, has the verified head, and every check passed\n" +
			"(zero checks pass). Then it stops with the pull request open for the owner's\n" +
			"review: land_step is review and the task stays verified. Later, task land\n" +
			"--merge runs every guard and check again and merges the pull request only\n" +
			"while its head is the verified commit. --merge applies only to pr mode and\n" +
			"needs a pull request that task land opened or found. A review-stage pull\n" +
			"request merged in GitHub is recorded by the next task land, which refuses\n" +
			"one merged at another head or closed. auto does the same\n" +
			"steps and merges in one command, with no review stop. Every step is stored\n" +
			"as land_step and can be run again after a refusal.\n\n" +
			"Every attempt that passes the head check prints the advisory review-size\n" +
			"report of the verified commit against the target, as task size does, also in\n" +
			"the JSON size field and the body of a pull request it opens; the size never\n" +
			"refuses a landing. A successful landing warns about each measure over a limit.\n\n" +
			"Every refusal names a code and the step reached. One land runs at a time per\n" +
			"repository. Land never removes a checkout, stops an agent, or deletes a\n" +
			"branch; use fledge agent cleanup for that."}
	f := cmd.Flags()
	f.StringVar(&options.ID, "id", "", "Task ID")
	f.StringVar(&options.Branch, "branch", "", "Branch to land")
	f.StringVar(&options.Mode, "mode", "", "Delivery mode for this landing: local, pr, or auto")
	f.BoolVar(&options.Merge, "merge", false, "Merge a pr task's pull request after the review stop")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		options.BranchSet = f.Changed("branch")
		options.ModeSet = f.Changed("mode")
		return cli.Finish(land.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, land.Render)
	}
	return cmd
}
