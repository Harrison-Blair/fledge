package cmd

import "github.com/spf13/cobra"

func newStopCommand(manager sessionManager, getwd func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop and delete this directory's Fledge session",
		Args:  cobra.NoArgs,
		RunE: runInDir(getwd, func(cmd *cobra.Command, _ []string, dir string) error {
			return manager.Stop(cmd.Context(), dir)
		}),
	}
}
