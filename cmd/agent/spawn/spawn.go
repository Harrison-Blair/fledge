// Package spawn wires agent startup flags.
package spawn

import (
	"github.com/Harrison-Blair/fledge/internal/agent"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"time"
)

func New() *cobra.Command {
	var options agent.SpawnOptions
	var asJSON bool
	var ratio float64
	cmd := &cobra.Command{Use: "spawn [flags] [-- native-args...]", Short: "Launch an agent in a Herdr pane"}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Unique live agent name (required)")
	f.StringVar(&options.Harness, "harness", "", "Herdr harness kind (required)")
	f.StringVar(&options.Model, "model", "", "Native model selection, where verified")
	f.StringVar(&options.Workspace, "workspace", "", "Destination workspace name; source in worktree mode")
	f.StringVar(&options.WorkspaceID, "workspace-id", "", "Existing workspace ID; source in worktree mode")
	f.StringVar(&options.Tab, "tab", "", "Destination tab name")
	f.StringVar(&options.TabID, "tab-id", "", "Existing destination tab ID")
	f.StringVar(&options.Pane, "pane", "", "Existing shell pane ID")
	f.StringVar(&options.Worktree, "worktree", "", "Create with new, or open a checkout path")
	f.StringVar(&options.Branch, "branch", "", "New worktree branch (defaults to agent name)")
	f.StringVar(&options.Base, "base", "", "New worktree base ref")
	f.StringVar(&options.Cwd, "cwd", "", "Shell directory; repository source in worktree mode")
	f.StringArrayVar(&options.Env, "env", nil, "Environment KEY=VALUE (repeatable; ordinary shells only)")
	f.BoolVar(&options.Focus, "focus", false, "Focus the destination before launching")
	f.StringVar(&options.Label, "label", "", "Destination pane label")
	f.StringVar(&options.Direction, "direction", "right", "Split direction: right or down")
	f.Float64Var(&ratio, "ratio", 0, "Split ratio, strictly between zero and one")
	f.DurationVar(&options.Timeout, "timeout", 30*time.Second, "Startup timeout (3001ms through 300000ms)")
	f.BoolVar(&options.NoWait, "no-wait", false, "Return once launch begins, without waiting for readiness")
	f.StringArrayVar(&options.Args, "args", nil, "Exact native argument token (repeatable)")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		options.DirectionSet = f.Changed("direction")
		if f.Changed("ratio") {
			options.Ratio = &ratio
		}
		f.Visit(func(flag *pflag.Flag) { options.Provided = append(options.Provided, flag.Name) })
		options.Args = append(options.Args, args...)
		if len(args) > 0 && cmd.ArgsLenAtDash() != 0 {
			return agent.Finish(agent.InvalidOutcome("agent.spawn", agent.PositionalError()), cmd.OutOrStdout(), asJSON)
		}
		return agent.Finish(agent.FromEnvironment(options.Timeout).Spawn(cmd.Context(), options), cmd.OutOrStdout(), asJSON)
	}
	return cmd
}
