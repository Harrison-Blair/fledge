// Package remove wires memory removal.
package remove

import (
	"os"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/memory/remove"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options remove.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "remove", Short: "Delete one memory and regenerate the index", Args: cobra.NoArgs}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Memory name")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		cwd, _ := os.Getwd()
		return libagent.Finish(remove.Run(cmd.Context(), cwd, options), cmd.OutOrStdout(), asJSON, remove.Render)
	}
	return cmd
}
