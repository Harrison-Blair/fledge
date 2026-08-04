package cmd

import "github.com/spf13/cobra"

func newStopCommand(manager sessionManager, getwd func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop and delete this directory's Fledge session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := currentDirectory(getwd)
			if err != nil {
				return err
			}
			return manager.Stop(cmd.Context(), dir)
		},
	}
}
