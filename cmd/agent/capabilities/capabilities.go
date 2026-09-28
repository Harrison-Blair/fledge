// Package capabilities wires the harness capability report.
package capabilities

import (
	"github.com/Harrison-Blair/fledge/internal/agent/capabilities"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options capabilities.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "capabilities", Short: "Report which operations each harness supports", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(capabilities.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, capabilities.Render)
	}}
	f := cmd.Flags()
	f.StringVar(&options.Harness, "harness", "", "Limit to one Herdr harness kind")
	f.BoolVar(&options.Live, "live", false, "Add binary availability and hook state from Herdr's integration.list")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
