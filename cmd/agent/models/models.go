// Package models wires local model discovery.
package models

import (
	"github.com/Harrison-Blair/fledge/internal/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options agent.ModelsOptions
	var asJSON bool
	cmd := &cobra.Command{Use: "models", Short: "List models discovered from installed harnesses", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return agent.Finish(agent.LocalDiscovery().Models(cmd.Context(), options), cmd.OutOrStdout(), asJSON)
	}}
	cmd.Flags().StringVar(&options.Harness, "harness", "", "Limit to one Herdr harness kind")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
