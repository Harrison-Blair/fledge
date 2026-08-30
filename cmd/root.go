package cmd

import (
	"os"

	agentcmd "fledge/cmd/agent"
	initcmd "fledge/cmd/init"
	profilecmd "fledge/cmd/profile"
	startcmd "fledge/cmd/start"
	stopcmd "fledge/cmd/stop"
	versioncmd "fledge/cmd/version"

	"github.com/spf13/cobra"
)

// New constructs the root command and its CLI adapters.
func New() *cobra.Command {
	command := &cobra.Command{
		Use:   "fledge",
		Short: "Manage project-local Herder sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}

	versioncmd.Configure(command)
	command.AddCommand(agentcmd.New())
	command.AddCommand(initcmd.New())
	command.AddCommand(profilecmd.New())
	command.AddCommand(startcmd.New())
	command.AddCommand(stopcmd.New())

	return command
}

// Execute constructs and runs the root command.
func Execute() {
	err := New().Execute()
	if err != nil {
		os.Exit(1)
	}
}
