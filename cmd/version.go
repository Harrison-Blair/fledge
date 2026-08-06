package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newVersionCommand prints the same injected version the root command carries,
// so the subcommand adds a surface rather than a second source of truth.
func newVersionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Fledge version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version)
			return nil
		},
	}
}
