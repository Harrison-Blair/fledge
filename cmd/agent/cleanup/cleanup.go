// Package cleanup wires retiring the caller's finished workers.
package cleanup

import (
	"github.com/Harrison-Blair/fledge/internal/agent/cleanup"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options cleanup.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "cleanup", Short: "Stop finished workers the caller spawned and remove the clean, merged checkouts their spawns created", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(cleanup.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, cleanup.Render)
	}}
	f := cmd.Flags()
	f.BoolVar(&options.DryRun, "dry-run", false, "Report what would be stopped, removed, or held, and why, without changing anything")
	f.BoolVar(&options.ResultsCollected, "results-collected", false, "Assert you have read the results of workers that own no task, so they may be stopped")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
