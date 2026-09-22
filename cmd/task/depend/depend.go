// Package depend wires task prerequisite changes.
package depend

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/task/depend"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options depend.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "depend", Short: "Add or remove the tasks a task runs after", Args: cobra.NoArgs,
		Long: "Add or remove prerequisites of a task that is not verified or cancelled.\n\nA prerequisite is satisfied once it is verified or cancelled; task assign refuses\na task with unmet prerequisites unless --force. Removals apply before additions.\nAdding a present prerequisite or removing an absent one changes nothing. A\nprerequisite must exist and must not already run after the task, directly or\nindirectly (task_dependency_cycle)."}
	f := cmd.Flags()
	f.StringVar(&options.ID, "id", "", "Task ID")
	f.StringArrayVar(&options.After, "after", nil, "Prerequisite task ID to add (repeatable)")
	f.StringArrayVar(&options.Remove, "remove", nil, "Prerequisite task ID to remove (repeatable)")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(depend.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, depend.Render)
	}
	return cmd
}
