// Package initcmd adapts project initialization to Cobra.
package initcmd

import (
	"fmt"

	"fledge/internal/project"

	"github.com/spf13/cobra"
)

// New constructs the init command.
func New() *cobra.Command {
	command := &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize a Fledge project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			root, err := project.Init(path)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Initialized Fledge project in %s\n", root)
			return err
		},
	}

	return command
}
