package cmd

import (
	"github.com/spf13/cobra"

	agentcmd "github.com/Harrison-Blair/fledge/cmd/agent"
	doctorcmd "github.com/Harrison-Blair/fledge/cmd/doctor"
	taskcmd "github.com/Harrison-Blair/fledge/cmd/task"
	updatecmd "github.com/Harrison-Blair/fledge/cmd/update"
	worktreecmd "github.com/Harrison-Blair/fledge/cmd/worktree"

	versioncmd "github.com/Harrison-Blair/fledge/cmd/version"
)

// NewRootCmd builds the root command and registers its children.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "fledge",
		Short:         "Fledge CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	versioncmd.Configure(root)
	root.AddCommand(agentcmd.New())
	root.AddCommand(updatecmd.New())
	root.AddCommand(doctorcmd.New())
	root.AddCommand(worktreecmd.New())
	root.AddCommand(taskcmd.New())
	return root
}
