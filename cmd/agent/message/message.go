// Package message adapts agent prompting to Cobra.
package message

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	internalagent "fledge/internal/agent"
	"fledge/internal/herdr"
	"fledge/internal/session"

	"github.com/spf13/cobra"
)

type messageOperation func(context.Context, internalagent.MessageOptions) (json.RawMessage, error)

// New constructs the agent message command.
func New() *cobra.Command {
	return newCommand(message)
}

func message(ctx context.Context, options internalagent.MessageOptions) (json.RawMessage, error) {
	base := herdr.New(nil, nil, nil)
	_, client, err := internalagent.Connect(ctx, ".", os.Getenv, base.List, func(name string) session.PaneResolver { return base.WithSession(name) })
	if err != nil {
		return nil, err
	}
	return internalagent.Message(ctx, client, options)
}

func newCommand(message messageOperation) *cobra.Command {
	var options internalagent.MessageOptions

	command := &cobra.Command{
		Use:   "message <target> <text>",
		Short: "Send prompt text to an agent",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Target = args[0]
			options.Text = args[1]

			result, err := message(cmd.Context(), options)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", result)
			return err
		},
	}

	flags := command.Flags()
	flags.BoolVar(&options.Wait, "wait", false, "wait until the agent settles")
	flags.StringArrayVar(&options.Until, "until", nil, "agent state to wait for; repeatable")
	flags.IntVar(&options.TimeoutMS, "timeout", 0, "wait timeout in milliseconds")

	return command
}
