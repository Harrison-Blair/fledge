// Package stop adapts the Fledge stop lifecycle to Cobra.
package stop

import (
	"context"
	"crypto/rand"
	"os"

	"fledge/internal/herdr"
	"fledge/internal/session"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type stopOperation func(context.Context, string, session.StopDependencies) error
type terminalDetector func(int) bool

// New constructs the stop command.
func New() *cobra.Command {
	return newCommand(session.Stop, term.IsTerminal)
}

func newCommand(stop stopOperation, isTerminal terminalDetector) *cobra.Command {
	command := &cobra.Command{
		Use:   "stop [path]",
		Short: "Stop this project's running Herder sessions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			input := cmd.InOrStdin()
			output := cmd.OutOrStdout()
			client := herdr.New(input, output, cmd.ErrOrStderr())
			return stop(cmd.Context(), path, session.StopDependencies{
				Herder: client,
				Confirmer: session.TerminalConfirmer{
					Input:            input,
					Output:           output,
					InputIsTerminal:  streamIsTerminal(input, isTerminal),
					OutputIsTerminal: streamIsTerminal(output, isTerminal),
				},
				Output: output,
				Getenv: os.Getenv,
				Scoped: func(sessionName string) session.PaneResolver {
					return client.WithSession(sessionName)
				},
				Entropy: rand.Reader,
			})
		},
	}

	return command
}

func streamIsTerminal(stream any, isTerminal terminalDetector) bool {
	file, ok := stream.(interface{ Fd() uintptr })
	return ok && isTerminal(int(file.Fd()))
}
