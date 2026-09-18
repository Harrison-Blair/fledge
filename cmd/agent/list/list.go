// Package list wires agent enumeration.
package list

import (
	"github.com/Harrison-Blair/fledge/internal/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "list", Short: "List all live Herdr agents", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return agent.Finish(agent.FromEnvironment(0).List(cmd.Context()), cmd.OutOrStdout(), asJSON)
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
