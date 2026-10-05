// Package spawn wires agent startup flags.
package spawn

import (
	"github.com/Harrison-Blair/fledge/internal/agent/spawn"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"time"
)

func New() *cobra.Command {
	var options spawn.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "spawn [flags] [-- native-args...]", Short: "Launch an agent in a Herdr pane",
		Long: "Launch an agent in a Herdr pane.\n\nA first prompt from --prompt or --file is delivered once the agent is ready, prefixed\nwith the same sender header as agent message, including the reply command\n(fledge agent message --name <sender>) when the sender is a named agent.\nIf the agent stops on a startup dialog, spawn exits partial without submitting the prompt;\nresolve the dialog and resend it with agent message.\n\nReady means Herdr admits input: interactive readiness reported, launch no longer pending,\nand idle or done. Spawn waits for it with or without a first prompt; --no-wait skips it.\n--timeout bounds launch, this wait, and the first prompt: no prompt is sent after it.\nRegistration's local state lock and record write cannot be interrupted, so spawn can\noverrun --timeout while another fledge process holds the state lock. Herdr always\nreserves at least 30s for the launch, so a spawn that returns partial on a shorter\n--timeout can still finish launching under its name.\n\n--profile NAME sends a role brief from a built-in or .fledge/profiles/NAME.md in the\ninvoking checkout (list them with fledge agent profiles): the role instructions, the shared\nFledge protocol, and the project memory index of the repository the agent is placed in,\nbefore the task in the same first prompt, under one header. A profile chooses no harness,\nmodel, or native arguments: pass --harness, and --model or native arguments as needed.\n--profile cannot be combined with --no-wait.\n\nCodex defaults to --yolo (no approvals or sandbox); Claude defaults to\n--permission-mode bypassPermissions. Explicit native permission options override\nthese defaults. --no-permission-bypass leaves permissions to native arguments and\nharness configuration; it does not remove explicitly supplied bypass arguments.\nThese defaults apply with or without a profile. Other harnesses get no defaults.\n\nPlacement: --pane reuses an existing shell pane. Otherwise spawn creates a new tab labeled\n--tab (default: the agent name) in the caller's workspace or the one --workspace or\n--workspace-id selects; tab labels need not be unique. A --workspace name that does not\nexist creates that workspace. --worktree opens a checkout in its own workspace (--workspace\nthen names the source repository). A newly created or opened workspace uses its initial\ntab; an existing one, including an already-open checkout, gets a new tab.\n\nThe agent is registered in the repository Fledge is invoked from, not the one named by\n--cwd, which only places the shell (and selects the --worktree new source). Outside Git\nthe spawn succeeds unregistered. --name and --pane select live Herdr agents from\nanywhere; --id and task records are read from the invoking repository, so a worker\nlaunched into another repository runs those commands from this one\n(cd /abs/path && fledge task ...)."}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Unique live agent name (required)")
	f.StringVar(&options.Profile, "profile", "", "Role profile whose brief precedes the first prompt")
	f.StringVar(&options.Harness, "harness", "", "Herdr harness kind (required)")
	f.StringVar(&options.Model, "model", "", "Native model selection, where verified")
	f.StringVar(&options.Workspace, "workspace", "", "Destination workspace name; source in worktree mode")
	f.StringVar(&options.WorkspaceID, "workspace-id", "", "Existing workspace ID; source in worktree mode")
	f.StringVar(&options.Tab, "tab", "", "Label for the new tab (default: the agent name)")
	f.StringVar(&options.Pane, "pane", "", "Existing shell pane ID")
	f.StringVar(&options.Worktree, "worktree", "", "Create with new, or open a checkout path")
	f.StringVar(&options.Branch, "branch", "", "New worktree branch (defaults to agent name)")
	f.StringVar(&options.Base, "base", "", "New worktree base ref")
	f.StringVar(&options.Cwd, "cwd", "", "Shell directory; repository source in worktree mode")
	f.StringArrayVar(&options.Env, "env", nil, "Environment KEY=VALUE (repeatable; ordinary shells only)")
	f.BoolVar(&options.Focus, "focus", false, "Focus the destination before launching")
	f.StringVar(&options.Label, "label", "", "Destination pane label (default: the agent name)")
	f.DurationVar(&options.Timeout, "timeout", 30*time.Second, "Spawn's startup and readiness budget (3001ms through 300000ms); Herdr reserves at least 30s")
	f.BoolVar(&options.NoWait, "no-wait", false, "Return once launch begins, without waiting for readiness")
	f.BoolVar(&options.NoPermissionBypass, "no-permission-bypass", false, "Do not add Claude or Codex permission bypass defaults")
	f.StringVar(&options.Prompt, "prompt", "", "First prompt text, sent once the agent is ready")
	f.StringVar(&options.File, "file", "", "UTF-8 prompt file, or - for stdin")
	f.StringArrayVar(&options.Args, "args", nil, "Exact native argument token (repeatable)")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		options.PromptSet = f.Changed("prompt")
		options.FileSet = f.Changed("file")
		f.Visit(func(flag *pflag.Flag) { options.Provided = append(options.Provided, flag.Name) })
		options.Args = append(options.Args, args...)
		if len(args) > 0 && cmd.ArgsLenAtDash() != 0 {
			return libagent.Finish(libagent.InvalidOutcome("agent.spawn", spawn.PositionalError()), cmd.OutOrStdout(), asJSON, spawn.Render)
		}
		return libagent.Finish(spawn.Run(cmd.Context(), libagent.FromEnvironment(options.Timeout), options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, spawn.Render)
	}
	return cmd
}
