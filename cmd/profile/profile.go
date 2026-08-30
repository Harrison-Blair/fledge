// Package profile adapts managed agent profile inspection to Cobra.
package profile

import (
	listcmd "fledge/cmd/profile/list"
	showcmd "fledge/cmd/profile/show"

	"github.com/spf13/cobra"
)

// New constructs the profile command and its subcommands.
func New() *cobra.Command {
	command := &cobra.Command{
		Use:   "profile",
		Short: "Inspect Fledge-managed agent profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	command.AddCommand(listcmd.New())
	command.AddCommand(showcmd.New())

	return command
}
