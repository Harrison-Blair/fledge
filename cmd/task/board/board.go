// Package board wires the interactive task board.
package board

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/task/board"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	return &cobra.Command{Use: "board", Short: "Browse a read-only task outline and worker activity", Args: cobra.NoArgs,
		Long: "Browse a read-only task outline with scrollable details. Active tasks and their\nancestors appear initially; h toggles history. Arrows select, expand, collapse,\nand scroll. Enter opens details, Tab switches wide panels, Escape returns to the\noutline, r refreshes, g visits the current worker, and q or Ctrl-C exits.\n\nRefreshes every two seconds. At 100 columns or more, details appear beside the\noutline. Requires terminal input and output. Task inspection works outside Herdr;\nworker observation and navigation require Herdr. Visiting a worker marks its\noutput seen and leaves the board running in its own tab.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cli.Finish(board.Run(cmd.Context(), libagent.FromEnvironment(0), cmd.InOrStdin(), cmd.OutOrStdout()), cmd.OutOrStdout(), false, nil)
		},
	}
}
