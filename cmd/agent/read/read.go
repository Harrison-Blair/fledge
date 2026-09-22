// Package read wires terminal snapshots of a live agent's pane.
package read

import (
	"github.com/Harrison-Blair/fledge/internal/agent/read"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options read.Options
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read a terminal snapshot of a live agent's pane",
		Long: `Read a plain-text terminal snapshot of a live agent's pane.

The snapshot is what the terminal currently holds, not a conversation
transcript. Reading does not change focus or seen state. Herdr returns at
most 1000 rows; --lines 0 returns an empty, truncated snapshot.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options.LinesSet = cmd.Flags().Changed("lines")
			return libagent.Finish(read.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, read.Render)
		},
	}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Live agent name")
	f.StringVar(&options.Pane, "pane", "", "Hosting pane ID")
	f.StringVar(&options.Source, "source", "recent-unwrapped", "Snapshot source: visible, recent, recent-unwrapped, or detection")
	f.Int64Var(&options.Lines, "lines", 0, "Bottom rows to request, up to Herdr's 1000-row cap (default: Herdr's extent)")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
