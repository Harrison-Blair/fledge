// Package get wires memory inspection.
package get

import (
	"os"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/memory/get"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options get.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "get", Short: "Show one memory in full", Args: cobra.NoArgs}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Memory name")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		cwd, _ := os.Getwd()
		return cli.Finish(get.Run(cmd.Context(), cwd, options), cmd.OutOrStdout(), asJSON, get.Render)
	}
	return cmd
}
