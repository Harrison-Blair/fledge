// Package remove wires checkout removal.
package remove

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/worktree/remove"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var o remove.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "remove", Short: "Remove a linked checkout, keeping its branch", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(remove.Run(cmd.Context(), libagent.FromEnvironment(0), o), cmd.OutOrStdout(), asJSON, remove.Render)
	}}
	cmd.Flags().StringVar(&o.Path, "path", "", "Checkout path to remove")
	cmd.Flags().StringVar(&o.Branch, "branch", "", "Branch whose checkout to remove")
	cmd.Flags().BoolVar(&o.Force, "force", false, "Remove even if dirty or unmerged (never while a live agent is in its workspace)")
	cmd.Flags().StringVar(&o.Cwd, "cwd", "", "Directory inside the repository (default: current directory)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
