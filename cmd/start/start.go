// Package start adapts the Fledge start lifecycle to Cobra.
package start

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"time"

	"fledge/internal/catalog"
	"fledge/internal/herdr"
	"fledge/internal/picker"
	"fledge/internal/profile"
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
	var harness string
	var model string
	var profileName string
	var noProfile bool
	command := &cobra.Command{
		Use:   "start [path] [-- harness arguments]",
		Short: "Start or attach to this project's Herder session",
		Args: func(cmd *cobra.Command, args []string) error {
			named := len(args)
			if dash := cmd.ArgsLenAtDash(); dash != -1 {
				named = dash
			}
			return cobra.MaximumNArgs(1)(cmd, args[:named])
		},
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("profile") && cmd.Flags().Changed("no-profile") {
				return fmt.Errorf("--profile and --no-profile cannot be used together")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			named := len(args)
			var harnessArgs []string
			if dash := cmd.ArgsLenAtDash(); dash != -1 {
				named = dash
				harnessArgs = append([]string(nil), args[dash:]...)
			}
			if named == 1 {
				path = args[0]
			}

			input := cmd.InOrStdin()
			output := cmd.OutOrStdout()
			interactive := streamIsTerminal(input, isTerminal) && streamIsTerminal(output, isTerminal)
			client := herdr.New(input, output, cmd.ErrOrStderr())
			return start(cmd.Context(), path, session.StartDependencies{
				Herder:  client,
				Entropy: rand.Reader,
				Now:     time.Now,
				Getenv:  os.Getenv,
				Chooser: picker.SessionChooser{
					Resolver: picker.Resolver{
						Input:  input,
						Output: output,
						Models: func(ctx context.Context, harness catalog.Harness) []string {
							return catalog.Models(ctx, harness, modelTimeout)
						},
					},
					Request: picker.LaunchRequest{
						Harness:        harness,
						Model:          model,
						Profile:        profileName,
						NoProfile:      noProfile,
						DefaultProfile: profile.OrchestratorName,
						Args:           harnessArgs,
						AllowShellOnly: true,
						Interactive:    interactive,
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

	flags := command.Flags()
	flags.BoolVar(&newSession, "new", false, "Discard this project's session claim and start a fresh session (stop running sessions first)")
	flags.StringVar(&harness, "harness", "", "agent harness to start")
	flags.StringVar(&model, "model", "", "model passed to the harness")
	flags.StringVar(&profileName, "profile", "", "managed agent profile to apply")
	flags.BoolVar(&noProfile, "no-profile", false, "start without an agent profile")

	return command
}

func streamIsTerminal(stream any, isTerminal terminalDetector) bool {
	file, ok := stream.(interface{ Fd() uintptr })
	return ok && isTerminal(int(file.Fd()))
}
