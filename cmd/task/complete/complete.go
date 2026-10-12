// Package complete wires task completion.
package complete

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/task/complete"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options complete.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "complete", Short: "Record an assigned task's result as its owner", Args: cobra.NoArgs,
		Long: "Record an assigned task's result and mark it completed.\n\nThe caller must be the task's owner, identified by the caller pane's agent record,\nunless --force is given. A short notice naming the task and the fledge task get\nand fledge task verify commands is then stored in the inbox of a distinct\nregistered task creator, and its doorbell is rung; the result stays on the task\nrecord. Notification failures never roll completion back; the notice is never\ntyped, and the supervision daemon may ring its doorbell again."}
	f := cmd.Flags()
	f.StringVar(&options.ID, "id", "", "Task ID")
	f.StringVar(&options.Summary, "summary", "", "Result text")
	f.StringVar(&options.File, "file", "", "UTF-8 result file, or - for stdin")
	f.BoolVar(&options.Force, "force", false, "Complete a task the caller does not own")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		options.SummarySet = f.Changed("summary")
		options.FileSet = f.Changed("file")
		return cli.Finish(complete.Run(cmd.Context(), libagent.FromEnvironment(0), options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, complete.Render)
	}
	return cmd
}
