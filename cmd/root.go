package cmd

import (
	"github.com/spf13/cobra"

	versioncmd "github.com/Harrison-Blair/fledge/cmd/version"
)

// NewRootCmd builds the root command and registers its children.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "fledge",
		Short:        "Fledge CLI",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	versioncmd.Configure(root)
	return root
}
