// Package list adapts agent listing to Cobra.
package list

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	internalagent "fledge/internal/agent"
	"fledge/internal/herdr"
	"fledge/internal/session"

	"github.com/spf13/cobra"
)

type listOperation func(context.Context) ([]herdr.Agent, error)
type rawListOperation func(context.Context) (json.RawMessage, error)

// New constructs the agent list command.
func New() *cobra.Command {
	return newCommand(list, rawList)
}

func list(ctx context.Context) ([]herdr.Agent, error) {
	base := herdr.New(nil, nil, nil)
	_, client, err := internalagent.Connect(ctx, ".", os.Getenv, base.List, func(name string) session.PaneResolver { return base.WithSession(name) })
	if err != nil {
		return nil, err
	}
	return internalagent.List(ctx, client)
}

func rawList(ctx context.Context) (json.RawMessage, error) {
	base := herdr.New(nil, nil, nil)
	_, client, err := internalagent.Connect(ctx, ".", os.Getenv, base.List, func(name string) session.PaneResolver { return base.WithSession(name) })
	if err != nil {
		return nil, err
	}
	return client.Invoke(ctx, "agent", "list")
}

func newCommand(list listOperation, rawList rawListOperation) *cobra.Command {
	var raw bool

	command := &cobra.Command{
		Use:   "list",
		Short: "List the agents of this project's Herder session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if raw {
				result, err := rawList(cmd.Context())
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", result)
				return err
			}

			agents, err := list(cmd.Context())
			if err != nil {
				return err
			}
			return writeTable(cmd.OutOrStdout(), agents)
		},
	}

	command.Flags().BoolVar(&raw, "json", false, "print Herder's raw agent list")

	return command
}

func writeTable(output io.Writer, agents []herdr.Agent) error {
	writer := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tHARNESS\tSTATUS\tWORKSPACE\tTAB\tPANE"); err != nil {
		return err
	}
	for _, agent := range agents {
		name := agent.Name
		if name == "" {
			name = "-"
		}
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", name, agent.Kind, agent.Status, agent.WorkspaceID, agent.TabID, agent.PaneID); err != nil {
			return err
		}
	}
	return writer.Flush()
}
