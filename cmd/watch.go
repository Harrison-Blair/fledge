package cmd

import (
	"github.com/Harrison-Blair/fledge/internal/lifecycle"
	"github.com/spf13/cobra"
)

func newWatchCommand(manager sessionManager, getwd func() (string, error)) *cobra.Command {
	var options lifecycle.WatchOptions
	command := &cobra.Command{
		Use:   "watch",
		Short: "Monitor this directory's Fledge session",
		Args:  cobra.NoArgs,
		RunE: runInDir(getwd, func(cmd *cobra.Command, _ []string, dir string) error {
			return manager.Watch(cmd.Context(), dir, options)
		}),
	}
	command.Flags().BoolVar(&options.Daemon, "daemon", false, "run the watcher in the background")
	_ = command.Flags().MarkHidden("daemon")
	return command
}
