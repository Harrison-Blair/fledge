// Package adopt wires registering an already-running agent.
package adopt

import (
	"github.com/Harrison-Blair/fledge/internal/agent/adopt"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options adopt.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "adopt", Short: "Register an already-running agent with a durable Fledge ID", Args: cobra.NoArgs,
		Long: "Register an already-running agent with a durable Fledge ID.\n\nWithout --pane, adopt targets the caller's own pane (HERDR_PANE_ID). An unnamed\nagent needs --name, which adopt sets through Herdr; a named agent keeps its name,\nand a different --name is refused. A terminal that already has a live record is\nrefused with its existing ID. The record lives in .fledge/state under the\nrepository's primary checkout; the parent is the caller when the caller is a\nregistered agent.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return libagent.Finish(adopt.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, adopt.Render)
		}}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Name to give an unnamed agent, or its current name")
	f.StringVar(&options.Pane, "pane", "", "Hosting pane ID (default: the caller's pane)")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
