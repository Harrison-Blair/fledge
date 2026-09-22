// Package pause wires foreground agent interruption.
package pause

import (
	"time"

	"github.com/Harrison-Blair/fledge/internal/agent/pause"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options pause.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "pause", Short: "Interrupt a live agent's foreground turn", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(pause.Run(cmd.Context(), libagent.FromEnvironment(options.Timeout), options), cmd.OutOrStdout(), asJSON, pause.Render)
	}}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Live agent name")
	f.StringVar(&options.Pane, "pane", "", "Hosting pane ID")
	f.DurationVar(&options.Timeout, "timeout", 10*time.Second, "Positive timeout for interruption and settlement")
	f.BoolVar(&options.NoWait, "no-wait", false, "Return after key delivery acknowledgement without waiting for settlement")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
