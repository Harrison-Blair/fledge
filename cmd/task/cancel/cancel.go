// Package cancel wires task cancellation.
package cancel

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/task/cancel"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options cancel.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "cancel", Short: "Cancel a task that is not verified or already cancelled", Args: cobra.NoArgs,
		Long: "Cancel a created, assigned, or completed task. Verified and cancelled tasks are\nfinal. Cancelling does not message or stop the owner and leaves subtasks as they\nare. A cancelled prerequisite counts as satisfied but stays listed on its\ndependents; the output names created tasks it left ready."}
	f := cmd.Flags()
	f.StringVar(&options.ID, "id", "", "Task ID")
	f.StringVar(&options.Reason, "reason", "", "Why the task was cancelled")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(cancel.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, cancel.Render)
	}
	return cmd
}
