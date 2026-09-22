// Package wait wires blocking until live agents reach a lifecycle state.
package wait

import (
	"os"
	"os/signal"

	"github.com/Harrison-Blair/fledge/internal/agent/wait"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options wait.Options
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "wait",
		Short: "Wait until live agents reach a lifecycle state",
		Long: `Wait until live agents reach a lifecycle state.

Without --until, a wait ends on idle, done, or blocked. A settled state means
the agent's turn ended, not that its assigned work succeeded. Without
--timeout, the wait is indefinite; interrupt it with Ctrl-C.

--name and --pane are repeatable and may be mixed. Several targets need --all
(every target must match; the first failure cancels the rest) or --any (the first match wins and the remaining
waits are cancelled; targets that fail are recorded while others remain).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			return libagent.Finish(wait.Run(ctx, libagent.WaitFromEnvironment(options.Timeout), options), cmd.OutOrStdout(), asJSON, wait.Render)
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&options.Names, "name", nil, "Live agent name (repeatable)")
	f.StringArrayVar(&options.Panes, "pane", nil, "Hosting pane ID (repeatable)")
	f.StringVar(&options.ID, "id", "", "Fledge agent record ID; a single target (fails if the pane now hosts another terminal)")
	f.StringArrayVar(&options.Until, "until", nil, "State to match: idle, working, blocked, done, or unknown (repeatable)")
	f.DurationVar(&options.Timeout, "timeout", 0, "Give up after this duration (default: wait indefinitely)")
	f.BoolVar(&options.All, "all", false, "With several targets, wait for every target")
	f.BoolVar(&options.Any, "any", false, "With several targets, stop at the first match")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
