// Package verify wires task verification.
package verify

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/task/verify"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options verify.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "verify", Short: "Accept a completed task's result as another agent, or verify it again", Args: cobra.NoArgs,
		Long: "Accept a completed task's result and mark it verified.\n\nThe caller must be a registered agent other than the owner, and every direct\nsubtask must be verified or cancelled. --force overrides these checks and lists\nany open subtasks; the verifier (null when unregistered) and forced: true are\nrecorded. This is a workflow guard, not a security boundary.\n\nA created or assigned task that groups subtasks can be verified without being\ncompleted once every direct subtask is verified or cancelled and at least one\nis verified; cancel a parent whose subtasks were all cancelled. --force does not\nwiden this: it never verifies a created or assigned task that fails it.\n\nA verified task can be verified again, for example after repairs, with the\nsame checks. The new verification replaces the verifier, note (null when\nomitted), forced flag, and time; only the latest is kept. Verification is not\ntied to a Git revision, so name the checked commit in --summary."}
	f := cmd.Flags()
	f.StringVar(&options.ID, "id", "", "Task ID")
	f.StringVar(&options.Summary, "summary", "", "Verification note")
	f.StringVar(&options.File, "file", "", "UTF-8 verification note file, or - for stdin")
	f.BoolVar(&options.Force, "force", false, "Verify as the owner, as an unregistered caller, or with open subtasks")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		options.SummarySet = f.Changed("summary")
		options.FileSet = f.Changed("file")
		return libagent.Finish(verify.Run(cmd.Context(), libagent.FromEnvironment(0), options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, verify.Render)
	}
	return cmd
}
