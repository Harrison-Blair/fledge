// Package spawn wires agent startup flags.
package spawn

import (
	"context"
	"os"
	"os/signal"

	"github.com/Harrison-Blair/fledge/internal/agent/spawn"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	libmodels "github.com/Harrison-Blair/fledge/internal/lib/models"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
	"time"
)

func New() *cobra.Command {
	var options spawn.Options
	var asJSON bool
	var ratio float64
	cmd := &cobra.Command{Use: "spawn [flags] [-- native-args...]", Short: "Launch an agent in a Herdr pane",
		Long: "Launch an agent in a Herdr pane.\n\nA first prompt from --prompt or --file is delivered once the agent is ready, prefixed\nwith the same sender header as agent message, including the reply command\n(fledge agent message --name <sender>) when the sender is a named agent.\nIf the agent stops on a startup dialog, spawn exits partial without submitting the prompt;\nresolve the dialog and resend it with agent message.\n\nReady means Herdr admits input: interactive readiness reported, launch no longer pending,\nand idle or done. Spawn waits for it with or without a first prompt; --no-wait skips it.\n--timeout bounds launch, this wait, registration, and the first prompt: a prompt is never\nsent after it. Herdr always reserves at least 30s for the launch, so a spawn that\nreturns partial on a shorter --timeout can still finish launching under its name.\n\n--profile NAME supplies the harness, model, native arguments, and role from a built-in or\n.fledge/profiles/NAME.toml in the invoking checkout (list them with fledge agent profiles).\nThe role is sent before the task in the same first prompt, under one header. --harness\nthat differs from the profile's drops its model and arguments; --model, --args, and\ntokens after -- replace the profile's.\n\nRun with no flags or native arguments on an interactive terminal to choose the harness,\nmodel, name, and placement from prompts; the equivalent flags are printed before launch.\n\nThe agent is registered in the repository Fledge is invoked from, not the one named by\n--cwd, which only places the shell (and selects the --worktree new source). Outside Git\nthe spawn succeeds unregistered. --name and --pane select live Herdr agents from\nanywhere; --id and task records are read from the invoking repository, so a worker\nlaunched into another repository runs those commands from this one\n(cd /abs/path && fledge task ...)."}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Unique live agent name (required)")
	f.StringVar(&options.Profile, "profile", "", "Launch profile supplying harness, model, args, and role")
	f.StringVar(&options.Harness, "harness", "", "Herdr harness kind (required unless the profile sets one)")
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
	f.DurationVar(&options.Timeout, "timeout", 30*time.Second, "Spawn's startup and readiness budget (3001ms through 300000ms); Herdr reserves at least 30s")
	f.BoolVar(&options.NoWait, "no-wait", false, "Return once launch begins, without waiting for readiness")
	f.StringVar(&options.Prompt, "prompt", "", "First prompt text, sent once the agent is ready")
	f.StringVar(&options.File, "file", "", "UTF-8 prompt file, or - for stdin")
	f.StringArrayVar(&options.Args, "args", nil, "Exact native argument token (repeatable)")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		options.DirectionSet = f.Changed("direction")
		options.PromptSet = f.Changed("prompt")
		options.FileSet = f.Changed("file")
		if f.Changed("ratio") {
			options.Ratio = &ratio
		}
		f.Visit(func(flag *pflag.Flag) { options.Provided = append(options.Provided, flag.Name) })
		options.Args = append(options.Args, args...)
		if len(args) > 0 && cmd.ArgsLenAtDash() != 0 {
			return libagent.Finish(libagent.InvalidOutcome("agent.spawn", spawn.PositionalError()), cmd.OutOrStdout(), asJSON, spawn.Render)
		}
		if len(options.Provided) == 0 && len(args) == 0 && isTerminal(cmd.InOrStdin()) && isTerminal(cmd.OutOrStdout()) {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			client := libagent.FromEnvironment(options.Timeout)
			picker := spawn.Picker{In: cmd.InOrStdin(), Out: cmd.OutOrStdout(),
				Models: func(ctx context.Context, harness string) []string {
					rows, _ := libmodels.LocalDiscovery().Discover(ctx, harness)
					ids := []string{}
					for _, r := range rows {
						ids = append(ids, r.Model)
					}
					return ids
				},
				CallerTab: func(ctx context.Context) (string, error) { return spawn.CallerTab(ctx, client) }}
			picked, err := picker.Pick(ctx, options)
			stop()
			if err != nil {
				out := libagent.Outcome{Operation: "agent.spawn", Effects: []libagent.Effect{}}
				out.Fail(err, "picker", false)
				return libagent.Finish(out, cmd.OutOrStdout(), false, nil)
			}
			options = picked
		}
		return libagent.Finish(spawn.Run(cmd.Context(), libagent.FromEnvironment(options.Timeout), options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, spawn.Render)
	}
	return cmd
}
func isTerminal(stream any) bool {
	f, ok := stream.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
