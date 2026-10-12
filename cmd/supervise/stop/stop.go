// Package stop wires stopping the supervision daemon.
package stop

import (
	"os"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/supervise/stop"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "stop", Short: "Stop the session's supervision daemon", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, _ := os.Getwd()
			return cli.Finish(stop.Run(cmd.Context(), stop.Options{Cwd: cwd}), cmd.OutOrStdout(), asJSON, stop.Render)
		}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
