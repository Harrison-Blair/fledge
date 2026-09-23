// Package doctor wires the environment diagnosis command.
package doctor

import (
	"os"
	"time"

	"github.com/Harrison-Blair/fledge/internal/doctor"
	"github.com/Harrison-Blair/fledge/internal/doctor/checks"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/models"
	"github.com/spf13/cobra"
)

// New returns a fresh doctor command.
func New() *cobra.Command {
	var asJSON, verbose bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the local Fledge and Herdr environment",
		Long:  "Diagnose the local Fledge and Herdr environment with read-only checks against\nthe Herdr socket, harness installations, local model discovery, and the Herdr\nenvironment, printing a per-check report.\nExits nonzero if any check fails; warnings never fail.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return doctor.Run(cmd.Context(), doctor.Options{
				Herdr:     herdr.Client{Socket: os.Getenv("HERDR_SOCKET_PATH"), Timeout: 15 * time.Second},
				Discovery: models.LocalDiscovery(),
				Env:       checks.LocalEnvironment(),
				Out:       cmd.OutOrStdout(),
				JSON:      asJSON,
				Verbose:   verbose,
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured report")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Reveal long per-check detail (capabilities and available harnesses)")
	return cmd
}
