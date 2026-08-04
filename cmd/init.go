package cmd

import "github.com/spf13/cobra"

func newInitCommand(manager sessionManager, getwd func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize a Fledge project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			} else {
				var err error
				path, err = currentDirectory(getwd)
				if err != nil {
					return err
				}
			}
			_, err := manager.Init(path)
			return err
		},
	}
}
