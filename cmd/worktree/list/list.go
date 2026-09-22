// Package list wires worktree enumeration.
package list

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/worktree/list"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var o list.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "list", Short: "List a repository's checkouts with their workspace and git state", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(list.Run(cmd.Context(), libagent.FromEnvironment(0), o), cmd.OutOrStdout(), asJSON, list.Render)
	}}
	cmd.Flags().StringVar(&o.Cwd, "cwd", "", "Directory inside the repository (default: current directory)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
