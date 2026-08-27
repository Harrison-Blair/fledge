// Package start adapts the Fledge start lifecycle to Cobra.
package start

import (
	"context"
	"crypto/rand"
	"os"
	"time"

	"fledge/internal/catalog"
	"fledge/internal/herdr"
	"fledge/internal/picker"
	"fledge/internal/session"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type startOperation func(context.Context, string, session.StartDependencies) error
type terminalDetector func(int) bool

// modelTimeout bounds each harness's model listing.
const modelTimeout = 5 * time.Second

// New constructs the start command.
func New() *cobra.Command {
	return newCommand(session.Start, term.IsTerminal)
}

func newCommand(start startOperation, isTerminal terminalDetector) *cobra.Command {
	var newSession bool
	command := &cobra.Command{
		Use:   "start [path]",
		Short: "Start or attach to this project's Herder session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			input := cmd.InOrStdin()
			output := cmd.OutOrStdout()
			client := herdr.New(input, output, cmd.ErrOrStderr())
			return start(cmd.Context(), path, session.StartDependencies{
				Herder:  client,
				Entropy: rand.Reader,
				Now:     time.Now,
				Getenv:  os.Getenv,
				Chooser: picker.AgentChooser{
					Input:            input,
					Output:           output,
					InputIsTerminal:  streamIsTerminal(input, isTerminal),
					OutputIsTerminal: streamIsTerminal(output, isTerminal),
					Models: func(ctx context.Context, harness catalog.Harness) []string {
						return catalog.Models(ctx, harness, modelTimeout)
					},
				},
				Scoped: func(sessionName string) session.Bootstrapper {
					return client.WithSession(sessionName)
				},
				Diagnostics: cmd.ErrOrStderr(),
				New:         newSession,
			})
		},
	}

	command.Flags().BoolVar(&newSession, "new", false, "Discard this project's session claim and start a fresh session (stop running sessions first)")

	return command
}

func streamIsTerminal(stream any, isTerminal terminalDetector) bool {
	file, ok := stream.(interface{ Fd() uintptr })
	return ok && isTerminal(int(file.Fd()))
}
