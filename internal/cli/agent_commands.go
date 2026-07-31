package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentspawn"
	"github.com/Harrison-Blair/fledge/internal/fledge"
	"github.com/Harrison-Blair/fledge/internal/picker"
	"github.com/spf13/cobra"
)

func newAgent(env *environment) *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Manage logical agents"}
	cmd.AddCommand(
		newAgentSpawn(env),
		newAgentStop(env),
		newAgentList(env),
		newAgentStatus(env),
		newAgentPrompt(env),
		newAgentWait(env),
		newAgentRead(env),
		newAgentAttach(env),
		newAgentMessage(env),
	)
	return cmd
}

func newAgentSpawn(env *environment) *cobra.Command {
	var name, harnessName, model, cwd string
	var newTab bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "spawn [-- <native-args...>]",
		Short: "Choose and launch an agent harness",
		Args:  nativeArgsAfterDash,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentSpawn(cmd, env, agentSpawnFlags{
				name: name, harness: harnessName, model: model, cwd: cwd,
				newTab: newTab, timeout: timeout, nativeArgs: args,
			})
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "logical agent name")
	cmd.Flags().StringVarP(&harnessName, "harness", "k", "", "installed harness")
	cmd.Flags().StringVarP(&model, "model", "m", "", "model launch ID (custom IDs are accepted)")
	cmd.Flags().BoolVarP(&newTab, "new-tab", "N", false, "launch in a dedicated tab")
	cmd.Flags().StringVarP(&cwd, "cwd", "C", "", "working directory (defaults to project root)")
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 30*time.Second, "agent startup timeout")
	return cmd
}

type agentSpawnFlags struct {
	name, harness, model, cwd string
	newTab                    bool
	timeout                   time.Duration
	nativeArgs                []string
}

func nativeArgsAfterDash(cmd *cobra.Command, args []string) error {
	if len(args) > 0 && cmd.ArgsLenAtDash() < 0 {
		return usage("native harness arguments must follow --")
	}
	return nil
}

func runAgentSpawn(cmd *cobra.Command, env *environment, flags agentSpawnFlags) error {
	if flags.timeout <= 3*time.Second || flags.timeout > 5*time.Minute {
		return usage("--timeout must be greater than 3s and no more than 5m")
	}
	if duplicateModelFlag(flags.nativeArgs) {
		return usage("native passthrough arguments must not contain --model or -m; use fledge agent spawn --model")
	}
	tty := env.stdinTTY != nil && env.stdinTTY()
	if env.json {
		flags.newTab = true
		tty = false
	}
	if !tty && (strings.TrimSpace(flags.name) == "" || strings.TrimSpace(flags.harness) == "") {
		return usage("--name and --harness are required when prompting is unavailable")
	}

	installed := agentspawn.Installed(env.lookPath)
	if len(installed) == 0 {
		return fledge.NewError("no_harnesses_installed", "none of Claude Code, Codex, Pi, or OpenCode is installed")
	}
	var harness agentspawn.Harness
	if strings.TrimSpace(flags.harness) == "" {
		selected, cancelled, err := selectHarness(env, installed)
		if cancelled || err != nil {
			return err
		}
		harness = selected
	} else {
		var found bool
		harness, found = agentspawn.Resolve(installed, flags.harness)
		if !found {
			return usage(fmt.Sprintf("harness %q is not installed", flags.harness))
		}
	}

	if strings.TrimSpace(flags.model) == "" && tty {
		model, cancelled, err := selectModel(cmd, env, harness)
		if cancelled || err != nil {
			return err
		}
		flags.model = model
	}

	if strings.TrimSpace(flags.name) == "" {
		name, cancelled, err := promptAgentName(env)
		if cancelled || err != nil {
			return err
		}
		flags.name = name
	}
	flags.name = strings.TrimSpace(flags.name)
	if flags.name == "" {
		return usage("agent name must not be empty")
	}

	service, err := env.service(cmd.Context())
	if err != nil {
		return err
	}
	getenv := env.getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	currentPaneID := strings.TrimSpace(getenv("HERDR_PANE_ID"))
	result, err := service.SpawnAgent(cmd.Context(), fledge.AgentStartOptions{
		Name: flags.name, Kind: harness.ID, Model: strings.TrimSpace(flags.model),
		CWD: flags.cwd, Timeout: flags.timeout, Args: append([]string(nil), flags.nativeArgs...),
		NewTab: flags.newTab || currentPaneID == "", CurrentPaneID: currentPaneID, Executable: harness.Path,
	})
	if err != nil {
		return fledge.Translate(err)
	}
	return env.print(result, func(w io.Writer) {
		modelDescription := ""
		if result.Agent.Model != "" {
			modelDescription = ", model " + result.Agent.Model
		}
		fmt.Fprintf(w, "Spawned %s (%s%s) in pane %s\n",
			result.Agent.Name, result.Agent.Kind, modelDescription, result.Agent.PaneID)
	})
}

// handlePickerResult reports whether the user cancelled a picker, printing the
// cancellation notice, and otherwise translates a picker failure.
func handlePickerResult(env *environment, err error) (bool, error) {
	if errors.Is(err, picker.ErrCancelled) {
		fmt.Fprintln(env.out, "Cancelled.")
		return true, nil
	}
	if err != nil {
		return false, fledge.Wrap("picker_failed", err.Error(), err)
	}
	return false, nil
}

func selectHarness(env *environment, installed []agentspawn.Harness) (agentspawn.Harness, bool, error) {
	items := make([]picker.Item, 0, len(installed))
	for _, candidate := range installed {
		items = append(items, picker.Item{
			ID: candidate.ID, Title: candidate.Name, Description: candidate.Description,
		})
	}
	selected, selectErr := picker.Select(picker.Options{
		Title: "Agent harness", Items: items, Input: env.in, Output: env.out,
	})
	if cancelled, err := handlePickerResult(env, selectErr); cancelled || err != nil {
		return agentspawn.Harness{}, cancelled, err
	}
	harness, _ := agentspawn.Resolve(installed, selected.ID)
	return harness, false, nil
}

func selectModel(cmd *cobra.Command, env *environment, harness agentspawn.Harness) (string, bool, error) {
	catalog := agentspawn.Discover(cmd.Context(), harness, nil)
	if catalog.Warning != "" {
		fmt.Fprintf(env.errOut, "Warning: %s\n", catalog.Warning)
	}
	items := modelPickerItems(harness.ID, catalog.Models)
	selected, selectErr := picker.Select(picker.Options{
		Title: harness.Name + " model", Items: items, Input: env.in, Output: env.out,
		CollapsibleGroups: harness.ID == "pi",
	})
	if cancelled, err := handlePickerResult(env, selectErr); cancelled || err != nil {
		return "", cancelled, err
	}
	return selected.ID, false, nil
}

func promptAgentName(env *environment) (string, bool, error) {
	name, inputErr := picker.Input(picker.Options{
		Title: "Agent name", Placeholder: "worker", Input: env.in, Output: env.out,
	})
	if cancelled, err := handlePickerResult(env, inputErr); cancelled || err != nil {
		return "", cancelled, err
	}
	return name, false, nil
}

func modelPickerItems(harnessID string, models []agentspawn.Model) []picker.Item {
	items := make([]picker.Item, 0, len(models))
	for _, candidate := range models {
		group := candidate.Maker
		subgroup := ""
		if harnessID == "pi" && candidate.Provider != "" {
			group = agentspawn.ProviderName(candidate.Provider)
			if agentspawn.ProviderUsesCreatorGroups(candidate.Provider) {
				subgroup = candidate.Maker
			}
		}
		items = append(items, picker.Item{
			ID: candidate.ID, Title: candidate.Name, Description: candidate.Description,
			Group: group, Subgroup: subgroup,
		})
	}
	return items
}

func duplicateModelFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--model" || strings.HasPrefix(arg, "--model=") ||
			(strings.HasPrefix(arg, "-m") && !strings.HasPrefix(arg, "--")) {
			return true
		}
	}
	return false
}

func newAgentStop(env *environment) *cobra.Command {
	var timeout time.Duration
	var force bool
	cmd := &cobra.Command{
		Use:   "stop <name>",
		Short: "Gracefully stop an agent while retaining its pane",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if timeout <= 0 {
				return usage("--timeout must be greater than zero")
			}
			service, err := env.service(cmd.Context())
			if err != nil {
				return err
			}
			result, err := service.StopAgent(cmd.Context(), args[0], timeout, force)
			if err != nil {
				return fledge.Translate(err)
			}
			return env.print(result, func(w io.Writer) {
				if result.Forced {
					fmt.Fprintf(w, "Force-closed pane for agent %s\n", result.Agent.Name)
				} else {
					fmt.Fprintf(w, "Stopped agent %s; pane %s retained\n", result.Agent.Name, result.Agent.PaneID)
				}
			})
		},
	}
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 10*time.Second, "graceful stop timeout")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "close the pane if graceful stop times out")
	return cmd
}

func newAgentList(env *environment) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List managed agents",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			service, err := env.service(cmd.Context())
			if err != nil {
				return err
			}
			agents, err := service.ListAgents(cmd.Context())
			if err != nil {
				return fledge.Translate(err)
			}
			return printAgents(env, agents)
		},
	}
}

func newAgentStatus(env *environment) *cobra.Command {
	return &cobra.Command{
		Use:   "status [name]",
		Short: "Show one or all managed agents",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usage("agent status accepts at most one name")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			service, err := env.service(cmd.Context())
			if err != nil {
				return err
			}
			agents, err := service.AgentStatus(cmd.Context(), name)
			if err != nil {
				return fledge.Translate(err)
			}
			return printAgents(env, agents)
		},
	}
}

func printAgents(env *environment, agents []fledge.AgentView) error {
	return env.print(map[string]any{"agents": agents}, func(w io.Writer) {
		if len(agents) == 0 {
			fmt.Fprintln(w, "No managed agents")
			return
		}
		fmt.Fprintln(w, "NAME\tHARNESS\tMODEL\tSTATE\tPENDING\tPLACEMENT\tPANE\tCWD")
		for _, agent := range agents {
			model := agent.Model
			if model == "" {
				model = "default"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
				agent.Name, agent.Kind, model, agent.State, agent.PendingMessages, agent.Placement, agent.PaneID, agent.CWD)
		}
	})
}

type promptOptions struct {
	file    string
	wait    bool
	until   []string
	timeout time.Duration
}

func newAgentPrompt(env *environment) *cobra.Command {
	var opts promptOptions
	cmd := &cobra.Command{
		Use:   "prompt <name> [text]",
		Short: "Submit a prompt, optionally waiting atomically for a lifecycle state",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) < 1 || len(args) > 2 {
				return usage("agent prompt requires a name and at most one positional prompt")
			}
			if (len(args) == 2) == (opts.file != "") {
				return usage("provide exactly one prompt source: positional text or --file path|-")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentPrompt(cmd, env, args, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.file, "file", "F", "", "read prompt from a file, or - for stdin")
	cmd.Flags().BoolVarP(&opts.wait, "wait", "w", false, "wait atomically after submitting")
	cmd.Flags().StringSliceVarP(&opts.until, "until", "u", nil,
		"settled state(s): "+strings.Join(fledge.WaitStates, ", "))
	cmd.Flags().DurationVarP(&opts.timeout, "timeout", "t", 0, "server-side wait timeout")
	return cmd
}

func runAgentPrompt(cmd *cobra.Command, env *environment, args []string, opts promptOptions) error {
	if err := validateStates(opts.until); err != nil {
		return err
	}
	if opts.timeout < 0 {
		return usage("--timeout must not be negative")
	}
	if (len(opts.until) > 0 || cmd.Flags().Changed("timeout")) && !opts.wait {
		return usage("--until and --timeout require --wait")
	}
	text, err := readPromptInput(env, args, opts.file)
	if err != nil {
		return err
	}
	service, err := env.service(cmd.Context())
	if err != nil {
		return err
	}
	agent, err := service.Prompt(cmd.Context(), fledge.PromptOptions{
		Name: args[0], Text: text, Wait: opts.wait, Until: opts.until, Timeout: opts.timeout,
	})
	if err != nil {
		return fledge.Translate(err)
	}
	return env.print(map[string]any{"agent": agent}, func(w io.Writer) {
		if opts.wait {
			fmt.Fprintf(w, "Prompt completed for %s: %s\n", agent.Name, agent.State)
		} else {
			fmt.Fprintf(w, "Prompt submitted to %s\n", agent.Name)
		}
	})
}

func readPromptInput(env *environment, args []string, file string) (string, error) {
	if len(args) == 2 {
		return args[1], nil
	}
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(env.in)
	} else {
		data, err = os.ReadFile(file)
	}
	if err != nil {
		return "", fledge.Wrap("prompt_read_failed", fmt.Sprintf("read prompt: %v", err), err)
	}
	return string(data), nil
}

func newAgentWait(env *environment) *cobra.Command {
	var until []string
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "wait <name>",
		Short: "Wait for an agent lifecycle state",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateStates(until); err != nil {
				return err
			}
			if timeout < 0 {
				return usage("--timeout must not be negative")
			}
			service, err := env.service(cmd.Context())
			if err != nil {
				return err
			}
			agent, err := service.Wait(cmd.Context(), args[0], until, timeout)
			if err != nil {
				return fledge.Translate(err)
			}
			return env.print(map[string]any{"agent": agent}, func(w io.Writer) {
				fmt.Fprintf(w, "%s reached %s\n", agent.Name, agent.State)
			})
		},
	}
	cmd.Flags().StringSliceVarP(&until, "until", "u", nil, "target state(s); defaults to idle, done, or blocked")
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 0, "server-side wait timeout")
	return cmd
}

func validateStates(states []string) error {
	valid := make(map[string]bool, len(fledge.WaitStates))
	for _, name := range fledge.WaitStates {
		valid[name] = true
	}
	for _, stateName := range states {
		if !valid[stateName] {
			return usage(fmt.Sprintf("invalid agent state %q", stateName))
		}
	}
	return nil
}

func newAgentRead(env *environment) *cobra.Command {
	var source string
	var lines int
	var ansi bool
	cmd := &cobra.Command{
		Use:   "read <name>",
		Short: "Read recent agent terminal output",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			valid := map[string]bool{"visible": true, "recent": true, "recent-unwrapped": true, "detection": true}
			if !valid[source] {
				return usage(fmt.Sprintf("invalid read source %q", source))
			}
			if lines < 0 {
				return usage("--lines must not be negative")
			}
			service, err := env.service(cmd.Context())
			if err != nil {
				return err
			}
			result, err := service.ReadAgent(cmd.Context(), args[0], source, lines, ansi)
			if err != nil {
				return fledge.Translate(err)
			}
			return env.print(result, func(w io.Writer) { fmt.Fprint(w, result.Text) })
		},
	}
	cmd.Flags().StringVarP(&source, "source", "S", "recent-unwrapped", "output source: visible, recent, recent-unwrapped, detection")
	cmd.Flags().IntVarP(&lines, "lines", "n", 120, "maximum lines to read")
	cmd.Flags().BoolVarP(&ansi, "ansi", "a", false, "preserve ANSI formatting")
	return cmd
}

func newAgentAttach(env *environment) *cobra.Command {
	var takeover bool
	cmd := &cobra.Command{
		Use:   "attach <name>",
		Short: "Attach interactively through the Herdr CLI",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if env.json {
				return usage("--json cannot be used with interactive attachment")
			}
			service, err := env.service(cmd.Context())
			if err != nil {
				return err
			}
			managed, err := service.AgentTarget(cmd.Context(), args[0])
			if err != nil {
				return fledge.Translate(err)
			}
			if err := service.Binary.Attach(cmd.Context(), service.Project.Session, managed.PaneID, takeover); err != nil {
				return fledge.Wrap("attach_failed", fmt.Sprintf("Herdr attachment failed: %v", err), err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&takeover, "takeover", "T", false, "take over an existing attachment")
	return cmd
}
