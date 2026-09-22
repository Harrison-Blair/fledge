// Package create wires managed checkout creation.
package create

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/worktree/create"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var o create.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "create", Short: "Create a managed checkout on a new branch and open it as a workspace", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(create.Run(cmd.Context(), libagent.FromEnvironment(0), o), cmd.OutOrStdout(), asJSON, create.Render)
	}}
	cmd.Flags().StringVar(&o.Branch, "branch", "", "New branch to create")
	cmd.Flags().StringVar(&o.Base, "base", "", "Ref to start the branch from (default: Herdr's choice)")
	cmd.Flags().StringVar(&o.Cwd, "cwd", "", "Directory inside the repository (default: current directory)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
