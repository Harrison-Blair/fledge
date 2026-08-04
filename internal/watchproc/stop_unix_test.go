//go:build !windows

package watchproc

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/statedir"
)

const (
	publicStopHelperEnv  = "FLEDGE_WATCHPROC_PUBLIC_STOP_HELPER"
	publicStopRootEnv    = "FLEDGE_WATCHPROC_PUBLIC_STOP_ROOT"
	publicStopReadyEnv   = "FLEDGE_WATCHPROC_PUBLIC_STOP_READY"
	publicStopSignalEnv  = "FLEDGE_WATCHPROC_PUBLIC_STOP_SIGNAL"
	publicStopReleaseLag = 200 * time.Millisecond
)

func TestTerminateProcessSendsSIGTERM(t *testing.T) {
	if os.Getenv("FLEDGE_WATCHPROC_STOP_HELPER") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	command := exec.Command(os.Args[0], "-test.run=TestTerminateProcessSendsSIGTERM")
	command.Env = append(os.Environ(), "FLEDGE_WATCHPROC_STOP_HELPER=1")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	if err := terminateProcess(command.Process.Pid); err != nil {
		_ = command.Process.Kill()
		t.Fatal(err)
	}
	err := command.Wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Wait() error = %v, want signal exit", err)
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGTERM {
		t.Fatalf("helper wait status = %#v, want SIGTERM", exitErr.Sys())
	}
}

func TestStopTerminatesOwnerAndWaitsForSingletonRelease(t *testing.T) {
	if mode := os.Getenv(publicStopHelperEnv); mode != "" {
		runPublicStopHelper(t, mode)
		return
	}

	root := t.TempDir()
	if err := ensureStateDirectories(root, testSession); err != nil {
		t.Fatal(err)
	}
	readyPath := filepath.Join(root, "owner-ready")
	signalPath := filepath.Join(root, "signal-received")
	command := startPublicStopHelper(t, root, readyPath, signalPath, "delay-release")
	waitFor(t, func() bool { _, err := os.Stat(readyPath); return err == nil })
	waited := false
	defer func() {
		if !waited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()

	started := time.Now()
	if err := Stop(root, testSession); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed < publicStopReleaseLag/2 {
		t.Fatalf("Stop() returned after %s, before the owner released its lock", elapsed)
	}
	if _, err := os.Stat(signalPath); err != nil {
		t.Fatalf("termination signal marker: %v", err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("owner helper exit: %v", err)
	}
	waited = true
}

func TestStopFailsWhileTerminatedOwnerKeepsSingletonLock(t *testing.T) {
	if mode := os.Getenv(publicStopHelperEnv); mode != "" {
		runPublicStopHelper(t, mode)
		return
	}

	root := t.TempDir()
	if err := ensureStateDirectories(root, testSession); err != nil {
		t.Fatal(err)
	}
	readyPath := filepath.Join(root, "owner-ready")
	command := startPublicStopHelper(t, root, readyPath, "", "ignore-termination")
	defer func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	}()
	waitFor(t, func() bool { _, err := os.Stat(readyPath); return err == nil })

	err := Stop(root, testSession)
	if err == nil || !strings.Contains(err.Error(), "did not release singleton lock") {
		t.Fatalf("Stop() error = %v, want lock-release timeout", err)
	}
	lockPath := filepath.Join(statedir.WatchSession(root, testSession), lockFilename)
	held, lockErr := singletonHeld(lockPath)
	if lockErr != nil {
		t.Fatal(lockErr)
	}
	if !held {
		t.Fatal("singleton lock is acquirable after Stop() reported a held-lock timeout")
	}
}

func startPublicStopHelper(t *testing.T, root, readyPath, signalPath, mode string) *exec.Cmd {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
	command.Env = append(os.Environ(),
		publicStopHelperEnv+"="+mode,
		publicStopRootEnv+"="+root,
		publicStopReadyEnv+"="+readyPath,
		publicStopSignalEnv+"="+signalPath,
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	return command
}

func runPublicStopHelper(t *testing.T, mode string) {
	t.Helper()
	var signals chan os.Signal
	switch mode {
	case "delay-release":
		signals = make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		defer signal.Stop(signals)
	case "ignore-termination":
		signal.Ignore(syscall.SIGTERM)
		defer signal.Reset(syscall.SIGTERM)
	default:
		t.Fatalf("unknown public Stop helper mode %q", mode)
	}

	root := os.Getenv(publicStopRootEnv)
	lockPath := filepath.Join(statedir.WatchSession(root, testSession), lockFilename)
	lock, err := acquire(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	pidPath := filepath.Join(statedir.WatchSession(root, testSession), pidFilename)
	if err := writePID(pidPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv(publicStopReadyEnv), []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if mode == "ignore-termination" {
		for {
			time.Sleep(time.Hour)
		}
	}
	<-signals
	if err := os.WriteFile(os.Getenv(publicStopSignalEnv), []byte("received\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(publicStopReleaseLag)
}
