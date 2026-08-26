// Package spawn adapts agent spawning to Cobra.
package spawn

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	internalagent "fledge/internal/agent"
	"fledge/internal/herdr"

	"github.com/spf13/cobra"
)

type spawnOperation func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error)

// New constructs the agent spawn command.
func New() *cobra.Command {
	return newCommand(spawn)
}

func spawn(ctx context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
	caller, client, err := internalagent.Connect(ctx, ".", os.Getenv, herdr.New(nil, nil, nil).List)
	if err != nil {
		return internalagent.SpawnResult{}, err
	}
	return internalagent.Spawn(ctx, client, caller, options)
}

func newCommand(spawn spawnOperation) *cobra.Command {
	var options internalagent.SpawnOptions
	var ratio float64

	command := &cobra.Command{
		Use:   "spawn <name> --kind KIND [-- harness arguments]",
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
			options.Args = nil
			if dash := cmd.ArgsLenAtDash(); dash != -1 {
				options.Args = args[dash:]
			}
			if cmd.Flags().Changed("ratio") {
				options.Ratio = &ratio
			}

			result, err := spawn(cmd.Context(), options)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}

	flags := command.Flags()
	flags.StringVar(&options.Kind, "kind", "", "harness kind to start")
	flags.StringVar(&options.Model, "model", "", "model passed to the harness")
	flags.StringVar(&options.Workspace, "workspace", "", `place the agent in "new" or an existing workspace ID`)
	flags.StringVar(&options.Tab, "tab", "", "split a pane of this tab ID")
	flags.StringVar(&options.Pane, "pane", "", "split this pane ID")
	flags.StringVar(&options.Split, "split", "", "direction for --tab or --pane placement: right or down (default right)")
	flags.Float64Var(&ratio, "ratio", 0, "fraction of the split pane given to the agent")
	flags.StringVar(&options.Label, "label", "", "workspace or tab label (defaults to the agent name)")
	_ = command.MarkFlagRequired("kind")

	return command
}
