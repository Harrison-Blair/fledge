// Package assign wires task assignment.
package assign

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/task/assign"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options assign.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "assign", Short: "Make a registered agent a task's owner and deliver it the brief", Args: cobra.NoArgs,
		Long: "Make a registered agent a task's owner and deliver it the brief.\n\nThe agent must have a Fledge record (see fledge agent adopt). A created or\nassigned task is recorded as assigned first; the brief is then submitted as a\nmessage with the sender header and a line naming the task and how to complete it.\nA failed delivery leaves the task assigned with the error recorded and is never\nretried; the outcome is partial.\n\nA task whose prerequisites are not all verified or cancelled is refused with\ntask_dependencies_unmet. --force assigns it anyway and records the unmet\nprerequisites as unmet_at_assign."}
	f := cmd.Flags()
	f.StringVar(&options.ID, "id", "", "Task ID")
	f.StringVar(&options.Agent.Name, "name", "", "Live agent name")
	f.StringVar(&options.Agent.Pane, "pane", "", "Hosting pane ID")
	f.StringVar(&options.Agent.ID, "agent-id", "", "Fledge agent record ID (follows its terminal to a new pane; fails if the terminal is gone)")
	f.BoolVar(&options.Force, "force", false, "Assign before every prerequisite is satisfied")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(assign.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, assign.Render)
	}
	return cmd
}
