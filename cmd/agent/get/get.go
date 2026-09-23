// Package get wires read-only live agent inspection.
package get

import (
	"github.com/Harrison-Blair/fledge/internal/agent/get"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options get.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "get", Short: "Inspect a live agent without changing focus or seen state", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(get.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, get.Render)
	}}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Live agent name")
	f.StringVar(&options.Pane, "pane", "", "Hosting pane ID")
	f.StringVar(&options.ID, "id", "", "Fledge agent record ID (follows its terminal to a new pane; fails if the terminal is gone)")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
