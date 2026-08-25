// Package start adapts the Fledge start lifecycle to Cobra.
package start

import (
	"context"
	"crypto/rand"
	"os"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/session"

	"github.com/spf13/cobra"
)

type startOperation func(context.Context, string, session.StartDependencies) error

// New constructs the start command.
func New() *cobra.Command {
	return newCommand(session.Start)
}

func newCommand(start startOperation) *cobra.Command {
	command := &cobra.Command{
		Use:   "start [path]",
		Short: "Start or attach to this project's Herder session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			client := herdr.New(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			return start(cmd.Context(), path, session.StartDependencies{
				Herder:  client,
				Entropy: rand.Reader,
				Now:     time.Now,
				Getenv:  os.Getenv,
			})
		},
	}

	return command
}
