// Package rename wires renaming a live agent.
package rename

import (
	"github.com/Harrison-Blair/fledge/internal/agent/rename"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options rename.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "rename", Short: "Rename a live agent and label its pane, and its tab when alone there, to match", Args: cobra.NoArgs,
		Long: "Rename a live agent and label its pane, and its tab when alone there, to match.\n\nWithout --name, --pane, or --id, rename targets the caller's own agent\n(HERDR_PANE_ID). Herdr renames the agent (agent_name_taken and\nagent_launch_pending are reported as is); a registered agent keeps its record\nand ID, which stores the new name. The pane label becomes the new name, and so\ndoes the tab label when the agent's pane is the only one in its tab; a shared\ntab keeps its label. An agent already named --to is only relabeled. Messages\nthat named the old name no longer reach the agent.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return libagent.Finish(rename.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, rename.Render)
		}}
	f := cmd.Flags()
	f.StringVar(&options.To, "to", "", "New agent name")
	f.StringVar(&options.Name, "name", "", "Live agent name (default: the caller's agent)")
	f.StringVar(&options.Pane, "pane", "", "Hosting pane ID (default: the caller's pane)")
	f.StringVar(&options.ID, "id", "", "Fledge agent record ID (follows its terminal to a new pane; fails if the terminal is gone)")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
