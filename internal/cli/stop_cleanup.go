package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/Harrison-Blair/fledge/internal/fledge"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
	"github.com/spf13/cobra"
)

const stopCleanupCommand = "__stop-cleanup"

func launchDetachedStopCleanup(request fledge.StopCleanupRequest) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate Fledge executable: %w", err)
	}
	args := []string{
		stopCleanupCommand,
		"--project-root", request.ProjectRoot,
		"--session", request.Session,
		"--state-dir", request.StateDir,
		"--herdr-bin", request.HerdrBinary,
		"--generation", strconv.FormatUint(request.BaseGeneration, 10),
		"--timeout", request.Timeout.String(),
	}
	cmd := exec.Command(executable, args...)
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = devNull.Close()
		return fmt.Errorf("start %s: %w", stopCleanupCommand, err)
	}
	_ = cmd.Process.Release()
	_ = devNull.Close()
	return nil
}

func newStopCleanup(env *environment) *cobra.Command {
	var projectRoot, session, stateDir string
	var generation uint64
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:    stopCleanupCommand,
		Hidden: true,
		Args:   noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch {
			case projectRoot == "":
				return usage("--project-root is required")
			case session == "":
				return usage("--session is required")
			case stateDir == "":
				return usage("--state-dir is required")
			case timeout <= 0:
				return usage("--timeout must be greater than zero")
			}
			store, err := state.New(stateDir)
			if err != nil {
				return fledge.Wrap("state_unavailable", err.Error(), err)
			}
			service := &fledge.Service{
				Project: project.Info{Root: projectRoot, Session: session},
				Binary:  herdr.Binary{Path: env.herdrBin},
				Store:   store,
			}
			return service.FinalizeStop(cmd.Context(), generation, timeout)
		},
	}
	cmd.Flags().StringVarP(&projectRoot, "project-root", "r", "", "exact project root")
	cmd.Flags().StringVarP(&session, "session", "s", "", "exact Herdr session name")
	cmd.Flags().StringVarP(&stateDir, "state-dir", "d", "", "exact Fledge state directory")
	cmd.Flags().Uint64VarP(&generation, "generation", "g", 0, "baseline coordinated-stop generation")
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 10*time.Second, "session stop timeout")
	return cmd
}
