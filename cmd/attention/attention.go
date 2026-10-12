// Package attention wires the caller's read-only attention summary.
package attention

import (
	"os"
	"time"

	"github.com/Harrison-Blair/fledge/internal/attention"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func New() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "attention", Short: "Show what needs you now: unread inbox, your workers by state, tasks to verify", Args: cobra.NoArgs,
		Long: "Show, for the caller's registered agent: unread inbox records; the live workers it\nspawned, grouped as blocked, quiet (reported by the supervision daemon while it\nruns), working, idle, done, and unknown; the tasks it created that are completed\nbut not verified; and whether the session's supervision daemon runs. Read-only.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			o := attention.Options{Now: time.Now()}
			if f, ok := cmd.OutOrStdout().(*os.File); ok {
				o.Width, _, _ = term.GetSize(int(f.Fd()))
			}
			return cli.Finish(attention.Run(cmd.Context(), libagent.FromEnvironment(0), o), cmd.OutOrStdout(), asJSON, attention.Render)
		}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
