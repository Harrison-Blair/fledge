// Package watchproc owns the event dispatcher's singleton process mechanics.
package watchproc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
	"github.com/Harrison-Blair/fledge/internal/herdr"
)

const (
	LogFilename   = "dispatcher.log"
	lockFilename  = "dispatcher.lock"
	pidFilename   = "dispatcher.pid"
	readyFilename = "dispatcher.ready"
)

type Herdr interface {
	Protocol(context.Context) (int, error)
	List(context.Context) ([]herdr.Session, error)
	Snapshot(context.Context, string) (herdr.Snapshot, error)
	PromptAgent(context.Context, string, string, string) error
}

type Options struct {
	Root, Session string
	Herdr         Herdr
	Output        io.Writer
	Daemon        bool

	// WatchFile, Subscribe, and Ready are dispatcher seams. Production leaves
	// WatchFile and Subscribe nil so the dispatcher installs its native ledger
	// watcher and Herdr event socket; tests inject fakes. Ready is set here so
	// the readiness marker is written when the dispatcher announces.
	WatchFile WatchFile
	Subscribe Subscribe
	Ready     func()

	// clock, newTimer, eventApplied, and selectPrepared are internal deterministic seams.
	// Production uses the wall clock and one reusable runtime timer, and does not
	// observe individual applications or select boundaries.
	clock          func() time.Time
	newTimer       func(time.Duration) dispatcherTimer
	eventApplied   func()
	selectPrepared func(deadlineEnabled bool)
}

func Run(ctx context.Context, options Options) (result error) {
	if strings.TrimSpace(options.Root) == "" {
		return errors.New("watch project root is missing")
	}
	if !fsutil.ValidSessionDirName(options.Session) {
		return fmt.Errorf("Herdr session name %q is not a valid dispatcher directory name", options.Session)
	}
	if options.Herdr == nil {
		return errors.New("dispatcher Herdr client is missing")
	}
	if options.Output == nil {
		options.Output = io.Discard
	}
	if options.Daemon {
		var stopSignals func()
		ctx, stopSignals = dispatcherContext(ctx)
		defer stopSignals()
	}
	if err := ensureStateDirectories(options.Root, options.Session); err != nil {
		return err
	}
	statePath := fsutil.TempSession(options.Root, options.Session)
	lockPath := filepath.Join(statePath, lockFilename)
	owner, err := acquire(lockPath)
	if errors.Is(err, errAlreadyRunning) {
		if options.Daemon {
			return nil
		}
		_, _ = fmt.Fprintln(options.Output, "dispatcher already running")
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, owner.release()) }()
	pidPath := filepath.Join(statePath, pidFilename)
	if err := writePID(pidPath); err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(pidPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}()
	readyPath := filepath.Join(statePath, readyFilename)
	defer os.Remove(readyPath)
	if err := ensureLogDirectory(options.Root, options.Session); err != nil {
		return err
	}
	logPath := filepath.Join(fsutil.Session(options.Root, options.Session), LogFilename)
	logFile, err := openOwned(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, logFile.Close()) }()
	destination := io.Writer(logFile)
	if !options.Daemon {
		destination = io.MultiWriter(logFile, options.Output)
	}
	// The marker file is what WaitReady watches for; a foreground watcher also
	// prints readiness so an attached terminal shows the dispatcher came up.
	options.Ready = func() {
		_ = os.WriteFile(readyPath, []byte("ready\n"), 0o600)
		_, _ = fmt.Fprintln(destination, "dispatcher ready")
	}
	return runDispatcher(ctx, options)
}
