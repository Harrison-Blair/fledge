// Package get wires read-only live agent inspection.
package get

import (
	"github.com/Harrison-Blair/fledge/internal/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options agent.GetOptions
	var asJSON bool
	cmd := &cobra.Command{Use: "get", Short: "Inspect a live agent without changing focus or seen state", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return agent.Finish(agent.FromEnvironment(0).Get(cmd.Context(), options), cmd.OutOrStdout(), asJSON)
	}}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Live agent name")
	f.StringVar(&options.Pane, "pane", "", "Hosting pane ID")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
