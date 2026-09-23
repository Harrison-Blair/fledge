// Package stop wires live agent teardown.
package stop

import (
	"github.com/Harrison-Blair/fledge/internal/agent/stop"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options stop.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "stop", Short: "Stop a live agent by closing its pane", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		options.GraceSet = cmd.Flags().Changed("grace")
		return libagent.Finish(stop.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, stop.Render)
	}}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Live agent name")
	f.StringVar(&options.Pane, "pane", "", "Hosting pane ID")
	f.StringVar(&options.ID, "id", "", "Fledge agent record ID (follows its terminal to a new pane; fails if the terminal is gone)")
	f.BoolVar(&options.Force, "force", false, "Stop even when the agent is working, blocked, or unknown, without waiting")
	f.DurationVar(&options.Grace, "grace", stop.DefaultGrace, "How long a working agent is given to finish its turn before stop refuses (0s through 60s; 0 refuses at once)")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
