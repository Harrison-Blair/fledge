// Package list wires memory listing.
package list

import (
	"os"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/memory"
	"github.com/Harrison-Blair/fledge/internal/memory/list"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options list.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "list", Short: "List memories as index lines, by name", Args: cobra.NoArgs}
	f := cmd.Flags()
	f.StringVar(&options.Type, "type", "", "Only memories of this type: "+strings.Join(memory.Types, ", "))
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		cwd, _ := os.Getwd()
		return cli.Finish(list.Run(cmd.Context(), cwd, options), cmd.OutOrStdout(), asJSON, list.Render)
	}
	return cmd
}
