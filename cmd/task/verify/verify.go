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
	cmd := &cobra.Command{Use: "verify", Short: "Accept a completed task's result as another agent", Args: cobra.NoArgs,
		Long: "Accept a completed task's result and mark it verified.\n\nThe caller must be a registered agent other than the owner. --force overrides\nboth checks; the verifier (null when unregistered) and forced: true are recorded.\nThis is a workflow guard, not a security boundary."}
	f := cmd.Flags()
	f.StringVar(&options.ID, "id", "", "Task ID")
	f.StringVar(&options.Summary, "summary", "", "Verification note")
	f.StringVar(&options.File, "file", "", "UTF-8 verification note file, or - for stdin")
	f.BoolVar(&options.Force, "force", false, "Verify as the owner or as an unregistered caller")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		options.SummarySet = f.Changed("summary")
		options.FileSet = f.Changed("file")
		return libagent.Finish(verify.Run(cmd.Context(), libagent.FromEnvironment(0), options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, verify.Render)
	}
	return cmd
}
