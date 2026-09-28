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

--name, --pane, and --id are repeatable and may be mixed. Instead of explicit
targets, filter flags select the live agents to wait on; they AND together,
repeated values of one flag OR together, and the caller is never selected.
Filter matches are reported by record ID, or by pane when unregistered, and an
empty match fails with no_agents_matched. Several targets need --all
(every target must match; the first failure cancels the rest) or --any (the
first match wins and the remaining waits are cancelled). With --any, a target
that fails, such as one that is stopped, is reported on stderr at once while
the wait continues on the remaining targets; --json reports only the final
outcome.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			if !asJSON {
				options.Progress = cmd.ErrOrStderr()
			}
			return libagent.Finish(wait.Run(ctx, libagent.WaitFromEnvironment(options.Timeout), options), cmd.OutOrStdout(), asJSON, wait.Render)
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&options.Names, "name", nil, "Live agent name (repeatable)")
	f.StringArrayVar(&options.Panes, "pane", nil, "Hosting pane ID (repeatable)")
	f.StringArrayVar(&options.IDs, "id", nil, "Fledge agent record ID (repeatable; follows its terminal to a new pane; fails if the terminal is gone)")
	f.StringArrayVar(&options.Filter.States, "state", nil, "Filter: live state idle, working, blocked, done, or unknown (repeatable)")
	f.StringArrayVar(&options.Filter.Harnesses, "harness", nil, "Filter: live harness kind (repeatable)")
	f.StringArrayVar(&options.Filter.Profiles, "profile", nil, "Filter: recorded spawn profile (repeatable)")
	f.StringArrayVar(&options.Filter.Tasks, "task", nil, "Filter: owner of this task ID (repeatable)")
	f.StringArrayVar(&options.Filter.Worktrees, "worktree", nil, "Filter: recorded worktree path (repeatable)")
	f.BoolVar(&options.Filter.Registered, "registered", false, "Filter: only agents with a live record in this repository")
	f.BoolVar(&options.Filter.Mine, "mine", false, "Filter: only agents whose parent is the caller's own record")
	f.StringVar(&options.Filter.Parent, "parent", "", "Filter: only agents whose parent is this Fledge agent record ID")
	f.StringArrayVar(&options.Until, "until", nil, "State to match: idle, working, blocked, done, or unknown (repeatable)")
	f.DurationVar(&options.Timeout, "timeout", 0, "Give up after this duration (default: wait indefinitely)")
	f.BoolVar(&options.All, "all", false, "With several targets, wait for every target")
	f.BoolVar(&options.Any, "any", false, "With several targets, stop at the first match")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
