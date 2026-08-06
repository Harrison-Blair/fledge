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

	"github.com/Harrison-Blair/fledge/internal/dispatcher"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/statedir"
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
	// JSON emits the stored records verbatim instead of the human trace, and
	// Color permits ANSI in that human trace. The log on disk is JSON either way.
	JSON  bool
	Color bool
}

func Run(ctx context.Context, options Options) (result error) {
	if strings.TrimSpace(options.Root) == "" {
		return errors.New("watch project root is missing")
	}
	if !statedir.ValidSessionDirName(options.Session) {
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
	statePath := statedir.TempSession(options.Root, options.Session)
	lockPath := filepath.Join(statePath, lockFilename)
	owner, err := acquire(lockPath)
	if errors.Is(err, errAlreadyRunning) {
		if options.Daemon {
			return nil
		}
		return followLog(ctx, options.Root, options.Session, options.Output, lineRenderer(options.JSON, options.Color))
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
	logPath := filepath.Join(statedir.Session(options.Root, options.Session), LogFilename)
	logFile, err := openOwned(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, logFile.Close()) }()
	emit, flush := recordSink(logFile, options.Output, options.Daemon, options.JSON, options.Color)
	defer flush()
	// The marker file is what WaitReady watches for; readiness reaches a reader
	// as the dispatcher's own record, like every other event in the trace.
	ready := func() {
		_ = os.WriteFile(readyPath, []byte("ready\n"), 0o600)
	}
	return dispatcher.Run(ctx, dispatcher.Options{Root: options.Root, Session: options.Session,
		Herdr: options.Herdr, Ready: ready, Emit: emit})
}
