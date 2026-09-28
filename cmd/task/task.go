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
	"github.com/Harrison-Blair/fledge/cmd/task/list"
	"github.com/Harrison-Blair/fledge/cmd/task/template"
	"github.com/Harrison-Blair/fledge/cmd/task/verify"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	cmd := &cobra.Command{Use: "task", Short: "Create, import, order, assign, complete, verify, cancel, list, and inspect durable tasks, and print brief templates", Args: cobra.NoArgs,
		Long: "Create, import, order, assign, complete, verify, cancel, list, and inspect durable tasks.\n\nA task moves created → assigned → completed → verified, and can be cancelled\nbefore it is verified. A task may be a subtask of a parent task and may run after\nprerequisite tasks. Records live in .fledge/state/tasks under the repository's\nprimary checkout and outlive their agents' panes. Owners and verifiers are Fledge\nagent record ids. Task status changes only through these commands; completion is\nnever derived from Herdr idle or done.\n\nBriefs follow a six-heading template; task template prints it."}
	cmd.AddCommand(create.New(), importcmd.New(), depend.New(), assign.New(), complete.New(), verify.New(), cancel.New(), list.New(), get.New(), template.New())
	return cmd
}
