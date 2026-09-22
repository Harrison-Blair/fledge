// Package list wires task listing.
package list

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/task/list"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options list.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "list", Short: "List tasks, oldest first", Args: cobra.NoArgs,
		Long: "List tasks, oldest first. The owner column shows the owner's name while its\nagent record is live, otherwise its record ID. PROGRESS counts a task's direct\nsubtasks, for example \"2/3 verified, 1 cancelled\"; cancelled subtasks are shown\nbut not counted toward the total."}
	f := cmd.Flags()
	f.StringVar(&options.Status, "status", "", "Only tasks in this state: created, assigned, completed, verified, or cancelled")
	f.StringVar(&options.Owner, "owner", "", "Only tasks owned by this agent record ID")
	f.StringVar(&options.Parent, "parent", "", "Only direct subtasks of this task ID")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(list.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, list.Render)
	}
	return cmd
}
