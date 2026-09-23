// Package worktree wires the worktree command group.
package worktree

import (
	"github.com/Harrison-Blair/fledge/cmd/worktree/create"
	"github.com/Harrison-Blair/fledge/cmd/worktree/list"
	"github.com/Harrison-Blair/fledge/cmd/worktree/remove"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	cmd := &cobra.Command{Use: "worktree", Short: "List, remove, and create a repository's checkouts", Args: cobra.NoArgs}
	cmd.AddCommand(list.New(), remove.New(), create.New())
	return cmd
}
