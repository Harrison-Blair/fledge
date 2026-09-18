// Package stop wires live agent teardown.
package stop

import (
	"github.com/Harrison-Blair/fledge/internal/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options agent.StopOptions
	var asJSON bool
	cmd := &cobra.Command{Use: "stop", Short: "Stop a live agent by closing its pane", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return agent.Finish(agent.FromEnvironment(0).Stop(cmd.Context(), options), cmd.OutOrStdout(), asJSON)
	}}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Live agent name")
	f.StringVar(&options.Pane, "pane", "", "Hosting pane ID")
	f.BoolVar(&options.Force, "force", false, "Stop even when the agent is working, blocked, or unknown")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
