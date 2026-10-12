// Package verify wires task verification.
package verify

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/task/verify"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options verify.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "verify", Short: "Accept a completed task's result as another agent, or verify it again", Args: cobra.NoArgs,
		Long: "Accept a completed task's result and mark it verified.\n\nThe caller must be a registered agent other than the owner, and every direct\nsubtask must be verified, landed, or cancelled. --force overrides these checks and lists\nany open subtasks; the verifier (null when unregistered) and forced: true are\nrecorded. This is a workflow guard, not a security boundary.\n\nA created or assigned task that groups subtasks can be verified without being\ncompleted once every direct subtask is verified, landed, or cancelled and at\nleast one is verified or landed; cancel a parent whose subtasks were all cancelled. --force does not\nwiden this: it never verifies a created or assigned task that fails it.\n\nA verified task can be verified again, for example after repairs, with the\nsame checks. The new verification replaces the verifier, note (null when\nomitted), forced flag, time, and bound commit; only the latest is kept.\n\n--commit binds the verification to a Git revision: it is resolved in the\ncaller's checkout and stored as verified_commit, which fledge task land\nrequires. When that checkout has uncommitted changes, verify warns that the\nbound commit is not exactly what was tested. A landed task is final and\ncannot be verified again."}
	f := cmd.Flags()
	f.StringVar(&options.ID, "id", "", "Task ID")
	f.StringVar(&options.Summary, "summary", "", "Verification note")
	f.StringVar(&options.File, "file", "", "UTF-8 verification note file, or - for stdin")
	f.StringVar(&options.Commit, "commit", "", "Revision to bind as the verified commit")
	f.BoolVar(&options.Force, "force", false, "Verify as the owner, as an unregistered caller, or with open subtasks")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		options.SummarySet = f.Changed("summary")
		options.FileSet = f.Changed("file")
		options.CommitSet = f.Changed("commit")
		return cli.Finish(verify.Run(cmd.Context(), libagent.FromEnvironment(0), options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, verify.Render)
	}
	return cmd
}
