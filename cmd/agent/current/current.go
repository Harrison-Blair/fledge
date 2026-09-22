// Package current wires the caller's own agent record lookup.
package current

import (
	"github.com/Harrison-Blair/fledge/internal/agent/current"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "current", Short: "Show the caller's own agent record, parent, and assigned tasks", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(current.Run(cmd.Context(), libagent.FromEnvironment(0)), cmd.OutOrStdout(), asJSON, current.Render)
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
