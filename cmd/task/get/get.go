// Package get wires task inspection.
package get

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/task/get"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options get.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "get", Short: "Show one task's full record", Args: cobra.NoArgs}
	f := cmd.Flags()
	f.StringVar(&options.ID, "id", "", "Task ID")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(get.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, get.Render)
	}
	return cmd
}
