// Package task wires the task command group.
package task

import (
	"github.com/Harrison-Blair/fledge/cmd/task/assign"
	"github.com/Harrison-Blair/fledge/cmd/task/cancel"
	"github.com/Harrison-Blair/fledge/cmd/task/complete"
	"github.com/Harrison-Blair/fledge/cmd/task/create"
	"github.com/Harrison-Blair/fledge/cmd/task/depend"
	"github.com/Harrison-Blair/fledge/cmd/task/get"
	importcmd "github.com/Harrison-Blair/fledge/cmd/task/import"
	"github.com/Harrison-Blair/fledge/cmd/task/land"
	"github.com/Harrison-Blair/fledge/cmd/task/list"
	"github.com/Harrison-Blair/fledge/cmd/task/size"
	"github.com/Harrison-Blair/fledge/cmd/task/template"
	"github.com/Harrison-Blair/fledge/cmd/task/verify"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	cmd := &cobra.Command{Use: "task", Short: "Create, import, order, assign, complete, verify, land, cancel, list, size, and inspect durable tasks, and print brief templates", Args: cobra.NoArgs,
		Long: "Create, import, order, assign, complete, verify, land, cancel, list, size, and inspect durable tasks.\n\nA task moves created → assigned → completed → verified → landed, and can be\ncancelled before it is verified. A task may be a subtask of a parent task and\nmay run after prerequisite tasks. Records live in .fledge/state/tasks under the\nrepository's primary checkout and outlive their agents' panes. Owners and\nverifiers are Fledge agent record ids. Task status changes only through these commands; completion is\nnever derived from Herdr idle or done.\n\nA brief is any nonblank UTF-8 text without NUL; task template prints an optional\nsix-heading skeleton to start from.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(create.New(), importcmd.New(), depend.New(), assign.New(), complete.New(), verify.New(), land.New(), cancel.New(), list.New(), get.New(), size.New(), template.New())
	return cmd
}
