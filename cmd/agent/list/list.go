// Package list wires agent enumeration.
package list

import (
	"github.com/Harrison-Blair/fledge/internal/agent/list"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "list", Short: "List all live Herdr agents", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(list.Run(cmd.Context(), libagent.FromEnvironment(0)), cmd.OutOrStdout(), asJSON, list.Render)
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
