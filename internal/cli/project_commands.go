package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Harrison-Blair/fledge/internal/fledge"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/spf13/cobra"
)

func newInit(env *environment) *cobra.Command {
	return &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize a Fledge project marker",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usage("init accepts at most one path")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			path := env.cwd
			if len(args) == 1 {
				path = args[0]
			}
			result, err := project.Init(path)
			if err != nil {
				return fledge.Wrap("project_init_failed", err.Error(), err)
			}
			return env.print(result, func(w io.Writer) {
				action := "Initialized"
				if !result.Initialized {
					action = "Already initialized"
				}
				fmt.Fprintf(w, "%s Fledge project at %s\nMarker: %s\n",
					action, result.ProjectRoot, result.MarkerPath)
			})
		},
	}
}

func newStart(env *environment) *cobra.Command {
	var timeout time.Duration
	var detach bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the project's Herdr server and open its UI",
		Long: "Start the project's deterministic Herdr server, prepare its orchestrator tab,\n" +
			"then attach the current terminal.\n" +
			"Use --detach to leave the server running without opening the Herdr UI.",
		Example: "  fledge start\n" +
			"  fledge start --detach\n" +
			"  fledge start --detach --json",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStart(cmd, env, timeout, detach)
		},
	}
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 10*time.Second, "readiness timeout")
	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "start the server without opening the Herdr UI")
	return cmd
}

func runStart(cmd *cobra.Command, env *environment, timeout time.Duration, detach bool) error {
	if env.json && !detach {
		return usage("--json requires --detach for fledge start")
	}
	if timeout <= 0 {
		return usage("--timeout must be greater than zero")
	}
	service, err := env.service(cmd.Context())
	if err != nil {
		return err
	}
	result, err := service.Start(cmd.Context(), timeout)
	if err != nil {
		return fledge.Translate(err)
	}
	attachCWD, err := env.workingDirectory()
	if err != nil {
		return err
	}
	if err := service.EnsureAttachmentWorkspace(cmd.Context(), result.Socket, attachCWD); err != nil {
		return fledge.Translate(err)
	}
	if result.Started && !detach {
		executable, executableErr := os.Executable()
		if executableErr != nil {
			fmt.Fprintf(env.errOut, "Warning: could not locate fledge executable for orchestrator picker: %v\n", executableErr)
		} else if enqueueErr := service.EnqueueOrchestratorPicker(cmd.Context(), result.Socket, executable); enqueueErr != nil {
			fmt.Fprintf(env.errOut, "Warning: could not open orchestrator picker: %v\n", enqueueErr)
		}
	}
	if err := printStartResult(env, result); err != nil {
		return err
	}
	if detach {
		return nil
	}
	return attachStartedSession(cmd, env, service, result, attachCWD)
}

func printStartResult(env *environment, result fledge.StartResult) error {
	return env.print(result, func(w io.Writer) {
		action := "Started"
		if !result.Started {
			action = "Already running"
		}
		fmt.Fprintf(w, "%s Fledge session %s\nSocket: %s\nHerdr: %s (protocol %d)\n",
			action, result.Session, result.Socket, result.Version, result.Protocol)
		fmt.Fprintf(w, "Session source: %s\n", result.SessionSource)
	})
}

func attachStartedSession(
	cmd *cobra.Command,
	env *environment,
	service *fledge.Service,
	result fledge.StartResult,
	attachCWD string,
) error {
	stopGeneration, err := service.StopGeneration()
	if err != nil {
		return fledge.Wrap("state_unavailable",
			fmt.Sprintf("cannot record coordinated-stop state before opening the Herdr UI: %v; check access to the Fledge state directory", err), err)
	}
	if err := service.Binary.AttachSession(cmd.Context(), result.Session, attachCWD); err != nil {
		stopped, stateErr := service.WaitForStopGeneration(cmd.Context(), stopGeneration, time.Second)
		if stateErr != nil {
			return fledge.Wrap("state_unavailable",
				fmt.Sprintf("Herdr attachment exited and Fledge could not inspect coordinated-stop state: %v; check access to the Fledge state directory", stateErr), stateErr)
		}
		if stopped {
			fmt.Fprintf(env.out, "Fledge session %s stopped; Herdr UI closed.\n", result.Session)
			return nil
		}
		return fledge.Wrap("attach_failed", fmt.Sprintf("Herdr attachment failed: %v", err), err)
	}
	return nil
}

func newStatus(env *environment) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show project, server, protocol, and agent status",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			service, err := env.service(cmd.Context())
			if err != nil {
				return err
			}
			result, err := service.Status(cmd.Context())
			if err != nil {
				return fledge.Translate(err)
			}
			return env.print(result, func(w io.Writer) {
				fmt.Fprintf(w, "Project: %s\nSession: %s\nSession source: %s\nServer: %s\n",
					result.ProjectRoot, result.Session, result.SessionSource, result.ServerState)
				if result.Socket != "" {
					fmt.Fprintf(w, "Socket: %s\n", result.Socket)
				}
				fmt.Fprintf(w, "Herdr: %s (protocol %d)\n", result.HerdrVersion, result.HerdrProtocol)
				if result.ServerState == "running" {
					fmt.Fprintf(w, "Server: %s (protocol %d, compatible: %t)\n", result.ServerVersion, result.ServerProtocol, result.ProtocolCompatible)
				}
				fmt.Fprintf(w, "Agents: idle=%d working=%d blocked=%d done=%d unknown=%d stopped=%d\n",
					result.AgentStates["idle"], result.AgentStates["working"], result.AgentStates["blocked"],
					result.AgentStates["done"], result.AgentStates["unknown"], result.AgentStates["stopped"])
				fmt.Fprintf(w, "User pending messages: %d\n", result.UserPendingMessages)
			})
		},
	}
}

func newStop(env *environment) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop and delete the project's Herdr session",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStop(cmd, env, force)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false,
		"skip confirmation and complete shutdown after graceful agent stop attempts")
	return cmd
}

func runStop(cmd *cobra.Command, env *environment, force bool) error {
	service, err := env.service(cmd.Context())
	if err != nil {
		return err
	}
	stopAuthorized := force
	if !force && !env.json && env.stdinTTY != nil && env.stdinTTY() {
		inspection, inspectErr := service.InspectStop(cmd.Context())
		if inspectErr != nil {
			return fledge.Translate(inspectErr)
		}
		if len(inspection.LiveAgents) > 0 {
			printStopAgents(env.out, inspection.LiveAgents)
		}
		confirmed, confirmErr := confirmStop(env, inspection)
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			fmt.Fprintln(env.out, "Cancelled; Fledge session unchanged.")
			return nil
		}
		stopAuthorized = len(inspection.LiveAgents) > 0
	}
	result, err := service.Stop(cmd.Context(), stopAuthorized)
	if err != nil {
		return fledge.Translate(err)
	}
	return env.print(result, func(w io.Writer) { printStopResult(w, result) })
}

func printStopResult(w io.Writer, result fledge.StopResult) {
	if len(result.ForcedAgents) > 0 {
		fmt.Fprintf(w, "Agents requiring session shutdown: %s\n", strings.Join(result.ForcedAgents, ", "))
	}
	if result.Stopped {
		fmt.Fprintf(w, "Stopped Fledge session %s\n", result.Session)
	}
	if result.Deleted {
		fmt.Fprintf(w, "Deleted Fledge session %s\n", result.Session)
	} else if !result.Stopped {
		fmt.Fprintf(w, "Fledge session %s does not exist\n", result.Session)
	}
}

func printStopAgents(w io.Writer, agents []fledge.StopAgentInspection) {
	fmt.Fprintln(w, "Running agents:")
	table := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(table, "NAME\tHARNESS\tSTATE\tWORKSPACE\tPANE")
	for _, agent := range agents {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n",
			agent.Name, agent.Harness, agent.State, agent.WorkspaceID, agent.PaneID)
	}
	_ = table.Flush()
}

func confirmStop(env *environment, inspection fledge.StopInspection) (bool, error) {
	prompt := fmt.Sprintf("Shut down and delete Fledge session %s? [y/N] ", inspection.Session)
	if len(inspection.LiveAgents) > 0 {
		prompt = fmt.Sprintf(
			"Running agents will be shut down. Are you sure you want to shut down Fledge session %s? [y/N] ",
			inspection.Session)
	}
	return confirm(env, prompt)
}
