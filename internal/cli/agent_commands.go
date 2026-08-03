package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentprofile"
	"github.com/Harrison-Blair/fledge/internal/agentspawn"
	"github.com/Harrison-Blair/fledge/internal/fledge"
	"github.com/Harrison-Blair/fledge/internal/orchestratorcontext"
	"github.com/Harrison-Blair/fledge/internal/picker"
	"github.com/Harrison-Blair/fledge/internal/state"
	"github.com/Harrison-Blair/fledge/internal/ui"
	"github.com/spf13/cobra"
)

const lastSpawnSelectionItemID = "__fledge_last_spawn_selection__"

func newAgent(env *environment) *cobra.Command {
	cmd := &cobra.Command{
		Use: "agent", Short: "Manage logical agents", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newAgentSpawn(env),
		newAgentProfile(env),
		newAgentStop(env),
		newAgentList(env),
		newAgentStatus(env),
		newAgentRead(env),
		newAgentAttach(env),
		newAgentMessage(env),
	)
	return cmd
}

func newAgentSpawn(env *environment) *cobra.Command {
	var name, harnessName, model, cwd, launchID string
	var newTab, noPickers bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "spawn [profile] [-- <native-args...>]",
		Short: "Choose and launch an agent harness",
		Args:  validateSpawnArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName, nativeArgs := splitSpawnArgs(cmd, args)
			return runAgentSpawn(cmd, env, agentSpawnFlags{
				name: name, harness: harnessName, model: model, cwd: cwd, profile: profileName,
				newTab: newTab, noPickers: noPickers, launchID: launchID, timeout: timeout, nativeArgs: nativeArgs,
				harnessSet: cmd.Flags().Changed("harness"), modelSet: cmd.Flags().Changed("model"),
				cwdSet: cmd.Flags().Changed("cwd"),
			})
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "logical agent name")
	cmd.Flags().StringVarP(&harnessName, "harness", "k", "", "installed harness")
	cmd.Flags().StringVarP(&model, "model", "m", "", "model launch ID (custom IDs are accepted)")
	cmd.Flags().BoolVarP(&newTab, "new-tab", "N", false, "launch in a dedicated tab")
	cmd.Flags().StringVarP(&cwd, "cwd", "C", "", "working directory (defaults to project root)")
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 30*time.Second, "agent startup timeout")
	// Plumbing for the dedicated-tab bootstrap: the injected in-pane child
	// runs on a tty but must never block on an interactive picker, so unset
	// selections fall through to the harness defaults instead of prompting.
	cmd.Flags().BoolVar(&noPickers, "no-pickers", false, "never open interactive pickers; unset selections use defaults")
	_ = cmd.Flags().MarkHidden("no-pickers")
	cmd.Flags().StringVar(&launchID, "launch-id", "", "claim a reserved dedicated-pane launch")
	_ = cmd.Flags().MarkHidden("launch-id")
	return cmd
}

type agentSpawnFlags struct {
	name, harness, model, cwd string
	profile                   string
	newTab                    bool
	noPickers                 bool
	launchID                  string
	timeout                   time.Duration
	nativeArgs                []string
	harnessSet, modelSet      bool
	cwdSet                    bool
}

func validateSpawnArgs(cmd *cobra.Command, args []string) error {
	beforeDash := len(args)
	if cmd.ArgsLenAtDash() >= 0 {
		beforeDash = cmd.ArgsLenAtDash()
	}
	if beforeDash > 1 {
		if cmd.ArgsLenAtDash() < 0 {
			return usage("fledge agent spawn accepts at most one profile name; native harness arguments must follow --")
		}
		return usage("fledge agent spawn accepts at most one profile name before --")
	}
	return nil
}

func splitSpawnArgs(cmd *cobra.Command, args []string) (string, []string) {
	dash := cmd.ArgsLenAtDash()
	if dash < 0 {
		if len(args) == 1 {
			return args[0], nil
		}
		return "", nil
	}
	profileName := ""
	if dash == 1 {
		profileName = args[0]
	}
	return profileName, append([]string(nil), args[dash:]...)
}

func runAgentSpawn(cmd *cobra.Command, env *environment, flags agentSpawnFlags) error {
	if flags.timeout <= 3*time.Second || flags.timeout > 5*time.Minute {
		return usage("--timeout must be greater than 3s and no more than 5m")
	}
	if flags.profile != "" {
		return runProfileAgentSpawn(cmd, env, flags)
	}
	if duplicateModelFlag(flags.nativeArgs) {
		return usage("native passthrough arguments must not contain --model or -m; use fledge agent spawn --model")
	}
	tty := env.stdinTTY != nil && env.stdinTTY()
	if env.json {
		flags.newTab = true
		tty = false
	}
	if flags.noPickers {
		tty = false
	}
	if !tty && (strings.TrimSpace(flags.name) == "" || strings.TrimSpace(flags.harness) == "") {
		return usage("--name and --harness are required when prompting is unavailable")
	}

	installed := agentspawn.Installed(env.lookPath)
	if len(installed) == 0 {
		return fledge.NewError("no_harnesses_installed", "none of Claude Code, Codex, Pi, or OpenCode is installed")
	}
	var lastSelection *state.SpawnSelection
	if strings.TrimSpace(flags.harness) == "" {
		var err error
		lastSelection, err = readLastSpawnSelection(env)
		if err != nil {
			return err
		}
	}
	selection, cancelled, err := resolveSpawnSelection(
		cmd, env, installed, flags.harness, flags.model, lastSelection, tty,
	)
	if cancelled || err != nil {
		return err
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
		Name: flags.name, Kind: selection.Harness.ID, Model: selection.Model,
		CWD: flags.cwd, Timeout: flags.timeout, Args: append([]string(nil), flags.nativeArgs...),
		NewTab: flags.newTab || currentPaneID == "", CurrentPaneID: currentPaneID, Executable: selection.Harness.Path,
		RememberSelection: selection.Remember, LaunchID: flags.launchID,
	})
	if err != nil {
		return fledge.Translate(err)
	}
	return printAgentSpawnResult(env, result)
}

func runProfileAgentSpawn(cmd *cobra.Command, env *environment, flags agentSpawnFlags) error {
	if flags.cwdSet {
		return usage(fmt.Sprintf("--cwd is locked by agent profile %q; profile agents always launch at the project root", flags.profile))
	}
	if len(flags.nativeArgs) > 0 {
		return usage(fmt.Sprintf("agent profile %q does not accept extra -- arguments; configure native_args in the profile", flags.profile))
	}

	var profileRoot string
	var profile agentprofile.Profile
	err := withProfileStore(env, func(store *agentprofile.Store) error {
		var err error
		profile, err = store.Load(flags.profile)
		if err != nil {
			return translateProfileError(err)
		}
		profileRoot = store.ProjectRoot()
		return nil
	})
	if err != nil {
		return err
	}
	if flags.harnessSet && profile.Harness != "" {
		return usage(fmt.Sprintf("--harness is locked by agent profile %q; configure native_args in the profile", flags.profile))
	}
	if flags.modelSet && profile.Model != "" {
		return usage(fmt.Sprintf("--model is locked by agent profile %q", flags.profile))
	}
	launchInstructions := profileLaunchInstructions(profile)
	if profile.Harness != "" {
		if err := agentspawn.ValidateProfileLaunch(agentspawn.ProfileLaunchOptions{
			Harness: profile.Harness, Effort: profile.Effort,
			Instructions: launchInstructions, NativeArgs: profile.NativeArgs,
		}); err != nil {
			return fledge.Wrap("profile_launch_invalid",
				fmt.Sprintf("agent profile %q cannot be launched: %v", flags.profile, err), err)
		}
	}

	requestedHarness, requestedModel := profile.Harness, profile.Model
	if flags.harnessSet {
		requestedHarness = flags.harness
	}
	if flags.modelSet {
		requestedModel = flags.model
	}
	tty := env.stdinTTY != nil && env.stdinTTY()
	if env.json {
		flags.newTab = true
		tty = false
	}
	if flags.noPickers {
		tty = false
	}
	if profile.Harness == "" && flags.harnessSet && strings.TrimSpace(flags.harness) == "" {
		return usage("--harness must not be empty")
	}
	if !tty && strings.TrimSpace(requestedHarness) == "" {
		return usage(fmt.Sprintf("--harness is required when agent profile %q omits harness and prompting is unavailable", flags.profile))
	}

	installed := agentspawn.Installed(env.lookPath)
	if profile.Harness != "" {
		if _, found := agentspawn.Resolve(installed, profile.Harness); !found {
			return fledge.NewError("profile_harness_not_installed",
				fmt.Sprintf("agent profile %q requires harness %q, but it is not installed", flags.profile, profile.Harness))
		}
	}
	compatible := compatibleProfileHarnesses(installed, profile)
	if requestedHarness != "" {
		if _, found := agentspawn.Resolve(installed, requestedHarness); !found {
			return usage(fmt.Sprintf("harness %q is not installed", requestedHarness))
		}
		if _, found := agentspawn.Resolve(compatible, requestedHarness); !found {
			return usage(fmt.Sprintf("harness %q is not compatible with agent profile %q", requestedHarness, flags.profile))
		}
	}
	if len(compatible) == 0 {
		return fledge.NewError("profile_launch_invalid",
			fmt.Sprintf("agent profile %q has no compatible installed harness", flags.profile))
	}
	var lastSelection *state.SpawnSelection
	if requestedHarness == "" {
		lastSelection, err = readLastSpawnSelection(env)
		if err != nil {
			return err
		}
	}
	selection, cancelled, err := resolveSpawnSelection(
		cmd, env, compatible, requestedHarness, requestedModel, lastSelection, tty,
	)
	if cancelled || err != nil {
		return err
	}
	instructionsFile := ""
	if selection.Harness.ID == "pi" && launchInstructions != "" {
		instructionsFile, err = agentspawn.MaterializeProfileInstructions(profileRoot, launchInstructions)
		if err != nil {
			return fledge.Wrap("profile_launch_invalid",
				fmt.Sprintf("agent profile %q could not prepare Pi instructions: %v", flags.profile, err), err)
		}
	}
	profileArgs, err := agentspawn.BuildProfileArgs(agentspawn.ProfileLaunchOptions{
		Harness: selection.Harness.ID, Effort: profile.Effort,
		Instructions: launchInstructions, InstructionsFile: instructionsFile,
		NativeArgs: profile.NativeArgs,
	})
	if err != nil {
		return fledge.Wrap("profile_launch_invalid",
			fmt.Sprintf("agent profile %q cannot be launched: %v", flags.profile, err), err)
	}
	if selection.Harness.ID == "" {
		return fledge.NewError("profile_harness_not_installed",
			fmt.Sprintf("agent profile %q did not resolve an installed harness", flags.profile))
	}
	if profile.Name == orchestratorProfileName {
		if err := orchestratorcontext.Synchronize(profileRoot, profile.Instructions); err != nil {
			return fledge.Wrap("profile_launch_invalid",
				fmt.Sprintf("agent profile %q could not synchronize managed repository context: %v", flags.profile, err), err)
		}
	}
	name := strings.TrimSpace(flags.name)
	if name == "" {
		name = flags.profile
	}
	service, err := env.service(cmd.Context())
	if err != nil {
		return err
	}
	currentPaneID := strings.TrimSpace(env.getenvValue("HERDR_PANE_ID"))
	result, err := service.SpawnAgent(cmd.Context(), fledge.AgentStartOptions{
		Name: name, Kind: selection.Harness.ID, Model: selection.Model, Profile: flags.profile,
		ProfileLocksHarness: profile.Harness != "", ProfileLocksModel: profile.Model != "",
		CWD: profileRoot, Timeout: flags.timeout, Args: append([]string(nil), profileArgs...),
		NewTab: flags.newTab || currentPaneID == "", CurrentPaneID: currentPaneID, Executable: selection.Harness.Path,
		RememberSelection: selection.Remember, LaunchID: flags.launchID,
	})
	if err != nil {
		return fledge.Translate(err)
	}
	return printAgentSpawnResult(env, result)
}

func compatibleProfileHarnesses(installed []agentspawn.Harness, profile agentprofile.Profile) []agentspawn.Harness {
	launchInstructions := profileLaunchInstructions(profile)
	compatible := make([]agentspawn.Harness, 0, len(installed))
	for _, harness := range installed {
		if profile.Harness != "" && harness.ID != profile.Harness {
			continue
		}
		err := agentspawn.ValidateProfileLaunch(agentspawn.ProfileLaunchOptions{
			Harness: harness.ID, Effort: profile.Effort,
			Instructions: launchInstructions, NativeArgs: profile.NativeArgs,
		})
		if err == nil {
			compatible = append(compatible, harness)
		}
	}
	return compatible
}

func profileLaunchInstructions(profile agentprofile.Profile) string {
	if profile.Name == orchestratorProfileName {
		return ""
	}
	return profile.Instructions
}

func printAgentSpawnResult(env *environment, result fledge.AgentStartResult) error {
	return env.print(result, func(w io.Writer, theme *ui.Theme) {
		modelDescription := ""
		if result.Agent.Model != "" {
			modelDescription = ", model " + result.Agent.Model
		}
		profileDescription := ""
		if result.Agent.Profile != "" {
			profileDescription = ", profile " + result.Agent.Profile
		}
		fmt.Fprintf(w, "%s %s (%s%s%s) in pane %s\n",
			theme.Accent("Spawned"), terminalSafeText(result.Agent.Name), terminalSafeText(result.Agent.Kind),
			terminalSafeText(modelDescription), terminalSafeText(profileDescription), terminalSafeText(result.Agent.PaneID))
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

type resolvedSpawnSelection struct {
	Harness  agentspawn.Harness
	Model    string
	Remember bool
}

func readLastSpawnSelection(env *environment) (*state.SpawnSelection, error) {
	service, err := env.messagingService()
	if err != nil {
		return nil, err
	}
	st, found, err := service.Store.ReadExisting(service.Project.Session, service.Project.Root)
	if err != nil {
		return nil, fledge.Wrap("state_unavailable", err.Error(), err)
	}
	if !found || st.LastSpawnSelection == nil {
		return nil, nil
	}
	selection := *st.LastSpawnSelection
	return &selection, nil
}

func resolveSpawnSelection(
	cmd *cobra.Command,
	env *environment,
	installed []agentspawn.Harness,
	requestedHarness string,
	requestedModel string,
	last *state.SpawnSelection,
	tty bool,
) (resolvedSpawnSelection, bool, error) {
	selection := resolvedSpawnSelection{Model: strings.TrimSpace(requestedModel)}
	savedModelApplied := false
	if strings.TrimSpace(requestedHarness) == "" {
		harness, usedLast, cancelled, err := selectHarness(env, installed, last)
		if cancelled || err != nil {
			return resolvedSpawnSelection{}, cancelled, err
		}
		selection.Harness = harness
		selection.Remember = true
		if usedLast && selection.Model == "" {
			selection.Model = strings.TrimSpace(last.Model)
			savedModelApplied = true
		}
	} else {
		var found bool
		selection.Harness, found = agentspawn.Resolve(installed, requestedHarness)
		if !found {
			return resolvedSpawnSelection{}, false,
				usage(fmt.Sprintf("harness %q is not installed", requestedHarness))
		}
	}

	if selection.Model == "" && tty && !savedModelApplied {
		model, cancelled, err := selectModel(cmd, env, selection.Harness)
		if cancelled || err != nil {
			return resolvedSpawnSelection{}, cancelled, err
		}
		selection.Model = strings.TrimSpace(model)
		selection.Remember = true
	}
	return selection, false, nil
}

func selectHarness(
	env *environment,
	installed []agentspawn.Harness,
	last *state.SpawnSelection,
) (agentspawn.Harness, bool, bool, error) {
	items := harnessPickerItems(installed, last)
	selected, selectErr := picker.Select(picker.Options{
		Title: "Agent harness", Items: items, Input: env.in, Output: env.out, Theme: env.stdoutTheme(),
	})
	if cancelled, err := handlePickerResult(env, selectErr); cancelled || err != nil {
		return agentspawn.Harness{}, false, cancelled, err
	}
	if selected.ID == lastSpawnSelectionItemID {
		harness, found := agentspawn.Resolve(installed, last.Harness)
		if !found {
			return agentspawn.Harness{}, false, false,
				fledge.NewError("picker_failed", "last-used harness is no longer installed")
		}
		return harness, true, false, nil
	}
	harness, _ := agentspawn.Resolve(installed, selected.ID)
	return harness, false, false, nil
}

func harnessPickerItems(installed []agentspawn.Harness, last *state.SpawnSelection) []picker.Item {
	items := make([]picker.Item, 0, len(installed)+1)
	if last != nil {
		if harness, found := agentspawn.Resolve(installed, last.Harness); found {
			model := strings.TrimSpace(last.Model)
			if model == "" {
				model = "default model"
			}
			items = append(items, picker.Item{
				ID:          lastSpawnSelectionItemID,
				Title:       fmt.Sprintf("Last used — %s · %s", harness.Name, model),
				Description: "Reuse the last picker-selected harness and model",
			})
		}
	}
	for index, candidate := range installed {
		items = append(items, picker.Item{
			ID: candidate.ID, Title: candidate.Name, Description: candidate.Description,
			SeparatorBefore: index == 0 && len(items) > 0,
		})
	}
	return items
}

func selectModel(cmd *cobra.Command, env *environment, harness agentspawn.Harness) (string, bool, error) {
	catalog := agentspawn.Discover(cmd.Context(), harness, nil)
	if catalog.Warning != "" {
		fmt.Fprintf(env.errOut, "%s %s\n", env.stderrTheme().Warning("Warning:"), catalog.Warning)
	}
	items := modelPickerItems(harness.ID, catalog.Models)
	selected, selectErr := picker.Select(picker.Options{
		Title: harness.Name + " model", Items: items, Input: env.in, Output: env.out,
		CollapsibleGroups: harness.ID == "pi", Theme: env.stdoutTheme(),
	})
	if cancelled, err := handlePickerResult(env, selectErr); cancelled || err != nil {
		return "", cancelled, err
	}
	return selected.ID, false, nil
}

func promptAgentName(env *environment) (string, bool, error) {
	name, inputErr := picker.Input(picker.Options{
		Title: "Agent name", Placeholder: "worker", Input: env.in, Output: env.out, Theme: env.stdoutTheme(),
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
		Short: "Stop an agent and clean up its dedicated tab",
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
			return env.print(result, func(w io.Writer, theme *ui.Theme) {
				if result.Forced {
					if result.TabClosed {
						fmt.Fprintf(w, "%s agent %s and closed dedicated tab %s\n",
							theme.Accent("Force-stopped"), result.Agent.Name, result.Agent.TabID)
					} else {
						fmt.Fprintf(w, "%s pane for agent %s\n", theme.Accent("Force-closed"), result.Agent.Name)
					}
				} else if result.TabClosed {
					fmt.Fprintf(w, "%s agent %s; closed dedicated tab %s\n",
						theme.Accent("Stopped"), result.Agent.Name, result.Agent.TabID)
				} else {
					fmt.Fprintf(w, "%s agent %s; pane %s retained\n", theme.Accent("Stopped"), result.Agent.Name, result.Agent.PaneID)
				}
			})
		},
	}
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 10*time.Second, "graceful stop timeout")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip graceful shutdown and close the pane or safe dedicated tab")
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
	return env.print(map[string]any{"agents": agents}, func(w io.Writer, theme *ui.Theme) {
		printAgentTable(w, agents, theme, true)
	})
}

func printAgentTable(w io.Writer, agents []fledge.AgentView, theme *ui.Theme, showEmpty bool) {
	if len(agents) == 0 {
		if showEmpty {
			fmt.Fprintln(w, "No managed agents")
		}
		return
	}
	showProfile := false
	for _, agent := range agents {
		if agent.Profile != "" {
			showProfile = true
			break
		}
	}
	if showProfile {
		fmt.Fprintln(w, theme.Accent("NAME\tHARNESS\tMODEL\tPROFILE\tSTATE\tPENDING\tPLACEMENT\tPANE\tCWD"))
	} else {
		fmt.Fprintln(w, theme.Accent("NAME\tHARNESS\tMODEL\tSTATE\tPENDING\tPLACEMENT\tPANE\tCWD"))
	}
	for _, agent := range agents {
		model := agent.Model
		if model == "" {
			model = "default"
		}
		if showProfile {
			profile := agent.Profile
			if profile == "" {
				profile = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
				terminalSafeText(agent.Name), terminalSafeText(agent.Kind), terminalSafeText(model),
				terminalSafeText(profile), theme.Status(terminalSafeText(agent.State)), agent.PendingMessages,
				terminalSafeText(agent.Placement), terminalSafeText(agent.PaneID), terminalSafeText(agent.CWD))
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			terminalSafeText(agent.Name), terminalSafeText(agent.Kind), terminalSafeText(model),
			theme.Status(terminalSafeText(agent.State)), agent.PendingMessages,
			terminalSafeText(agent.Placement), terminalSafeText(agent.PaneID), terminalSafeText(agent.CWD))
	}
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
			// Agent terminal content is an opaque payload and must never receive
			// Fledge presentation styling.
			return env.print(result, func(w io.Writer, _ *ui.Theme) { fmt.Fprint(w, result.Text) })
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
