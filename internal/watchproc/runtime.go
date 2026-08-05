// Package watchproc owns the process mechanics and concrete adapters for the
// watcher engine. Lifecycle code supplies the active project/session and the
// one callback that is allowed to inject a watcher message.
package watchproc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/statedir"
	"github.com/Harrison-Blair/fledge/internal/wake"
	"github.com/Harrison-Blair/fledge/internal/watch"
)

const (
	// LogFilename is the attached watcher's human-readable decision log.
	LogFilename = "watch.log"

	lockFilename = "watch.lock"
	pidFilename  = "watch.pid"
	tailInterval = 100 * time.Millisecond
)

// Herdr is the watcher-facing subset of the Herdr client.
type Herdr interface {
	List(context.Context) ([]herdr.Session, error)
	Snapshot(context.Context, string) (herdr.Snapshot, error)
	Protocol(context.Context) (int, error)
}

// DeliverFunc injects one batched watcher message and returns its message ID.
type DeliverFunc func(context.Context, string) (string, error)

// Options contains the active session and the lifecycle-owned dependencies.
type Options struct {
	Root    string
	Session string
	Config  watch.Config
	Herdr   Herdr
	Deliver DeliverFunc
	Output  io.Writer
	Daemon  bool
}

// Run owns the per-session singleton until the engine exits. Attached mode
// runs the engine in the foreground when it wins the lock, or follows the
// existing owner's log when it does not. Daemon mode is quiet and treats an
// existing owner as a successful no-op.
func Run(ctx context.Context, options Options) (result error) {
	if !options.Config.Enabled {
		return nil
	}
	if strings.TrimSpace(options.Root) == "" {
		return errors.New("watch project root is missing")
	}
	if !statedir.ValidSessionDirName(options.Session) {
		return fmt.Errorf("Herdr session name %q is not a valid watch directory name", options.Session)
	}
	if options.Herdr == nil {
		return errors.New("watch Herdr client is missing")
	}
	if options.Deliver == nil {
		return errors.New("watch delivery callback is missing")
	}
	if options.Output == nil {
		options.Output = io.Discard
	}

	if err := ensureStateDirectories(options.Root, options.Session); err != nil {
		return err
	}
	lockPath := filepath.Join(statedir.WatchSession(options.Root, options.Session), lockFilename)
	owner, err := acquire(lockPath)
	if errors.Is(err, errAlreadyRunning) {
		if options.Daemon {
			return nil
		}
		return followLog(ctx, options.Root, options.Session, options.Output, tailInterval)
	}
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, owner.release()) }()

	pidPath := filepath.Join(statedir.WatchSession(options.Root, options.Session), pidFilename)
	if err := writePID(pidPath); err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(pidPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, fmt.Errorf("remove watcher PID file %q: %w", pidPath, err))
		}
	}()
	if err := ensureLogDirectory(options.Root, options.Session); err != nil {
		return err
	}
	logPath := filepath.Join(statedir.Session(options.Root, options.Session), LogFilename)
	logFile, err := openOwned(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open watch log %q: %w", logPath, err)
	}
	defer func() { result = errors.Join(result, logFile.Close()) }()

	var destination io.Writer = logFile
	if !options.Daemon {
		destination = io.MultiWriter(logFile, options.Output)
	}
	logger := &lineLogger{destination: destination, now: time.Now}
	config, subscriber := configureEventStream(ctx, options.Config, options.Herdr, options.Session, logger.Log)
	if err := ctx.Err(); err != nil {
		return err
	}

	ledger := wake.New(options.Root, options.Session)
	engine := watch.Engine{
		Root:        options.Root,
		Session:     options.Session,
		Config:      config,
		Herdr:       options.Herdr,
		Ledger:      wakeLedger{ledger: ledger},
		Waker:       deliveryWaker(options.Deliver),
		Completions: completionAudit{store: messaging.New(options.Root, options.Session)},
		Subscriber:  subscriber,
		Now:         time.Now,
		Sleep:       sleepContext,
		Log:         logger.Log,
		Refresh:     contextRefresher(options.Root, options.Session, logger.Log),
	}
	result = engine.Run(ctx)
	return errors.Join(result, logger.Err())
}

func configureEventStream(ctx context.Context, config watch.Config, client Herdr, session string, log func(string)) (watch.Config, watch.Subscriber) {
	disable := func(format string, args ...any) (watch.Config, watch.Subscriber) {
		config.EventStream = false
		log("event stream disabled; polling continues: " + fmt.Sprintf(format, args...))
		return config, nil
	}
	if !config.EventStream {
		return config, nil
	}
	if runtime.GOOS == "windows" {
		return disable("Unix sockets are unavailable on Windows")
	}
	protocol, err := client.Protocol(ctx)
	if err != nil {
		return disable("resolve Herdr protocol: %v", err)
	}
	if protocol < config.MinProtocol {
		return disable("Herdr protocol %d is below required protocol %d", protocol, config.MinProtocol)
	}
	sessions, err := client.List(ctx)
	if err != nil {
		return disable("resolve Herdr socket: %v", err)
	}
	socketPath := ""
	for _, candidate := range sessions {
		if candidate.Name == session && candidate.Running {
			socketPath = candidate.SocketPath
			break
		}
	}
	if strings.TrimSpace(socketPath) == "" {
		return disable("session %s has no event socket", session)
	}
	info, err := os.Stat(socketPath)
	if err != nil {
		return disable("Herdr socket %q is unavailable: %v", socketPath, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return disable("Herdr socket %q is not a Unix socket", socketPath)
	}

	dial := func(dialCtx context.Context) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(dialCtx, "unix", socketPath)
	}
	return config, func(streamCtx context.Context, paneIDs []string, onReady func(), onEvent func(watch.Event)) error {
		return watch.Subscribe(streamCtx, dial, paneIDs, onReady, onEvent)
	}
}

type lineLogger struct {
	mu          sync.Mutex
	destination io.Writer
	now         func() time.Time
	err         error
}

func (l *lineLogger) Log(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return
	}
	_, l.err = fmt.Fprintf(l.destination, "[%s] %s\n", l.now().Format("15:04:05"), message)
}

func (l *lineLogger) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

func sleepContext(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
