// Package list wires agent enumeration.
package list

import (
	"github.com/Harrison-Blair/fledge/internal/agent/list"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options list.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "list", Short: "List all live Herdr agents", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(list.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, list.Render)
	}}
	f := cmd.Flags()
	f.BoolVar(&options.Mine, "mine", false, "Only agents whose parent is the caller's own record")
	f.StringVar(&options.Parent, "parent", "", "Only agents whose parent is this Fledge agent record ID")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
