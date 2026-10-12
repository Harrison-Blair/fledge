// Package run wires the supervision daemon's foreground loop.
package run

import (
	"os"
	"os/signal"
	"syscall"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/version"
	"github.com/Harrison-Blair/fledge/internal/supervise/run"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	return &cobra.Command{Use: "run", Short: "Run the supervision daemon in the foreground", Args: cobra.NoArgs,
		Long: "Run the supervision daemon of the current Herdr session in the foreground, logging\none line per event to standard output. Commands start it detached as needed; a\nsecond daemon for the same session exits at once. SIGTERM stops it.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, os.Interrupt)
			defer stop()
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return run.Run(ctx, libagent.FromEnvironment(0), run.Options{Cwd: cwd, Version: version.Version(), Log: cmd.OutOrStdout()})
		}}
}
