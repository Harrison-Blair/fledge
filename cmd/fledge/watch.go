package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/client"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/herdrwire"
	"github.com/Harrison-Blair/fledge/internal/protocol"
)

var watchPollInterval = 100 * time.Millisecond
var runLogWatcher = watchDaemonLog

func runWatch(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("watch")
	}
	if err := rejectFlags("watch", args); err != nil {
		return usageErrorFor("watch", err)
	}
	if len(args) > 1 {
		return usageErrorf("watch", "watch: unexpected argument %q", args[1])
	}
	name, err := flockArg("watch", args)
	if err != nil {
		if len(args) == 1 {
			return usageErrorFor("watch", err)
		}
		return err
	}
	root, err := workspaceRoot()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return runLogWatcher(ctx, root, name, os.Stdout)
}

// watchDaemonLog emits the complete current log, then reads from the same
// offset after every poll. daemon.log is append-only for a flock's lifetime,
// so a regular file descriptor is sufficient and no tailing dependency is
// needed.
func watchDaemonLog(ctx context.Context, root, name string, out io.Writer) error {
	if !client.Running(root, name) {
		return fmt.Errorf("flock %s: %w", name, client.ErrNotRunning)
	}

	logPath := filepath.Join(flock.Dir(root, name), protocol.LogName)
	logFile, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("watch flock %s: %w", name, err)
	}
	defer logFile.Close()

	copyAppended := func() error {
		if _, err := io.Copy(out, logFile); err != nil {
			return fmt.Errorf("watch flock %s: %w", name, err)
		}
		return nil
	}
	if err := copyAppended(); err != nil {
		return err
	}

	ticker := time.NewTicker(watchPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Drain the file before probing the daemon so its final log entry is
			// not lost when shutdown and the poll happen together.
			if err := copyAppended(); err != nil {
				return err
			}
			if !client.Running(root, name) {
				fmt.Fprintln(out, "fledge watch: daemon stopped")
				return nil
			}
		}
	}
}

// installWatcherPane replaces the existing CLI shell beside the orchestrator
// with the log watcher. It creates no workspace, tab, split, or agent. A
// command-delivery failure leaves the CLI available and prints a manual
// recovery hint there.
func installWatcherPane(socket, name, executable, shellPane, orchestratorPane string) error {
	command := "exec " + shellQuote(executable) + " watch " + shellQuote(name)
	if err := herdrwire.SendInput(socket, shellPane, command, true); err != nil {
		cause := fmt.Errorf("start watcher command: %w", err)
		warnWatcherFailure(socket, shellPane, name, cause)
		_ = herdrwire.PaneFocus(socket, orchestratorPane)
		return cause
	}
	if err := herdrwire.PaneFocus(socket, orchestratorPane); err != nil {
		return fmt.Errorf("refocus orchestrator after watcher: %w", err)
	}
	return nil
}

func warnWatcherFailure(socket, shellPane, name string, cause error) {
	message := fmt.Sprintf("fledge: automatic log watcher unavailable: %v; run fledge watch %s manually", cause, name)
	command := "printf '%s\\n' " + shellQuote(message)
	// The call is best effort: the warning is non-critical and must never
	// turn a healthy orchestrator launch into a failed start.
	_ = herdrwire.SendInput(socket, shellPane, command, true)
}

// shellQuote returns one POSIX-shell word. Error strings come from Herdr and
// may contain arbitrary punctuation, so warning commands must not interpolate
// them unquoted.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
