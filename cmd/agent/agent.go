// Package agent adapts the Herder agent commands to Cobra.
package agent

import (
	listcmd "fledge/cmd/agent/list"
	messagecmd "fledge/cmd/agent/message"
	spawncmd "fledge/cmd/agent/spawn"
	stopcmd "fledge/cmd/agent/stop"

	"github.com/spf13/cobra"
)

// New constructs the agent command and its subcommands.
func New() *cobra.Command {
	command := &cobra.Command{
		Use:   "agent",
		Short: "Spawn and drive Herder agents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	command.AddCommand(spawncmd.New())
	command.AddCommand(messagecmd.New())
	command.AddCommand(listcmd.New())
	command.AddCommand(stopcmd.New())

	return command
}
