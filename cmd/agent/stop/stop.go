// Package stop adapts agent shutdown to Cobra.
package stop

import (
	"context"
	"fmt"
	"os"

	internalagent "fledge/internal/agent"
	"fledge/internal/herdr"
	"fledge/internal/session"

	"github.com/spf13/cobra"
)

type stopOperation func(context.Context, string) (string, error)

// New constructs the agent stop command.
func New() *cobra.Command {
	return newCommand(stop)
}

func stop(ctx context.Context, target string) (string, error) {
	base := herdr.New(nil, nil, nil)
	_, client, err := internalagent.Connect(ctx, ".", os.Getenv, base.List, func(name string) session.PaneResolver { return base.WithSession(name) })
	if err != nil {
		return "", err
	}
	return internalagent.Stop(ctx, client, target)
}

func newCommand(stop stopOperation) *cobra.Command {
	return &cobra.Command{
		Use:   "stop <target>",
		Short: "Close the pane hosting an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pane, err := stop(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "closed %s\n", pane)
			return err
		},
	}
}
