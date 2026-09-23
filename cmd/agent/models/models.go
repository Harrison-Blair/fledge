// Package models wires local model discovery.
package models

import (
	"github.com/Harrison-Blair/fledge/internal/agent/models"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	libmodels "github.com/Harrison-Blair/fledge/internal/lib/models"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options models.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "models", Short: "List models discovered from installed harnesses", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(models.Run(cmd.Context(), libmodels.LocalDiscovery(), options), cmd.OutOrStdout(), asJSON, models.Render)
	}}
	cmd.Flags().StringVar(&options.Harness, "harness", "", "Limit to one Herdr harness kind")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
