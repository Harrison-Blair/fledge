// Package status wires the supervision daemon report.
package status

import (
	"os"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/supervise/status"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "status", Short: "Show whether the session's supervision daemon runs, and what it watches", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, _ := os.Getwd()
			return cli.Finish(status.Run(cmd.Context(), status.Options{Cwd: cwd, Now: time.Now()}), cmd.OutOrStdout(), asJSON, status.Render)
		}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
