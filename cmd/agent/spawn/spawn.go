// Package spawn adapts agent spawning to Cobra.
package spawn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	internalagent "fledge/internal/agent"
	"fledge/internal/catalog"
	"fledge/internal/herdr"
	"fledge/internal/picker"
	"fledge/internal/session"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type spawnOperation func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error)
type terminalDetector func(int) bool
type resolverFactory func(io.Reader, io.Writer) picker.Resolver

const modelTimeout = 5 * time.Second

// New constructs the agent spawn command.
func New() *cobra.Command {
	return newCommand(spawn, term.IsTerminal, func(input io.Reader, output io.Writer) picker.Resolver {
		return picker.Resolver{
			Input:  input,
			Output: output,
			Models: func(ctx context.Context, harness catalog.Harness) []string {
				return catalog.Models(ctx, harness, modelTimeout)
			},
		}
	})
}

func spawn(ctx context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
	base := herdr.New(nil, nil, nil)
	caller, client, err := internalagent.Connect(ctx, ".", os.Getenv, base.List, func(name string) session.PaneResolver { return base.WithSession(name) })
	if err != nil {
		return internalagent.SpawnResult{}, err
	}
	return internalagent.Spawn(ctx, client, caller, options)
}

func newCommand(spawn spawnOperation, isTerminal terminalDetector, resolver resolverFactory) *cobra.Command {
	var options internalagent.SpawnOptions
	var request picker.LaunchRequest
	var ratio float64

	command := &cobra.Command{
		Use:   "spawn <name> [--harness HARNESS] [-- harness arguments]",
		Short: "Start an agent in a new Herder pane",
		Args: func(cmd *cobra.Command, args []string) error {
			named := len(args)
			if dash := cmd.ArgsLenAtDash(); dash != -1 {
				named = dash
			}
			if named != 1 {
				return fmt.Errorf("accepts 1 name argument, received %d", named)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Name = args[0]
			request.Args = nil
			if dash := cmd.ArgsLenAtDash(); dash != -1 {
				request.Args = args[dash:]
			}
			if cmd.Flags().Changed("ratio") {
				options.Ratio = &ratio
			}

			input := cmd.InOrStdin()
			output := cmd.OutOrStdout()
			request.PromptProfile = true
			request.Interactive = streamIsTerminal(input, isTerminal) && streamIsTerminal(output, isTerminal)
			choice, err := resolver(input, output).Resolve(cmd.Context(), request)
			if err != nil {
				return err
			}
			options.Harness = string(choice.Harness)
			options.Model = choice.Model
			options.Profile = choice.Profile
			options.Args = choice.Args

			result, err := spawn(cmd.Context(), options)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}

	flags := command.Flags()
	flags.StringVar(&request.Harness, "harness", "", "agent harness to start")
	flags.StringVar(&request.Model, "model", "", "model passed to the harness")
	flags.StringVar(&request.Profile, "profile", "", "Fledge-managed agent profile to load")
	flags.BoolVar(&request.NoProfile, "no-profile", false, "start without an agent profile")
	flags.StringVar(&options.Workspace, "workspace", "", `place the agent in "new" or an existing workspace ID`)
	flags.StringVar(&options.Tab, "tab", "", "split a pane of this tab ID")
	flags.StringVar(&options.Pane, "pane", "", "split this pane ID")
	flags.StringVar(&options.Split, "split", "", "direction for --tab or --pane placement: right or down (default right)")
	flags.Float64Var(&ratio, "ratio", 0, "fraction of the split pane given to the agent")
	flags.StringVar(&options.Label, "label", "", "workspace or tab label (defaults to the agent name)")
	command.MarkFlagsMutuallyExclusive("profile", "no-profile")

	return command
}

func streamIsTerminal(stream any, isTerminal terminalDetector) bool {
	file, ok := stream.(interface{ Fd() uintptr })
	return ok && isTerminal(int(file.Fd()))
}
