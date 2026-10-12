package supervise

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/version"
)

// stopWait bounds how long Stop waits for the daemon to release its lock.
const stopWait = 10 * time.Second

// Starter launches one detached daemon for the session; see Launch.
type Starter func(root, session string) error

// ErrTestBinary is Launch's refusal inside a test binary.
var ErrTestBinary = errors.New("supervise: refusing to launch the daemon from a test binary")

// EnsureFor is Ensure for the caller's Herdr session and this binary's
// version in the repository containing cwd. Outside Herdr it does nothing,
// and a starter refusing a test binary counts as not started.
func EnsureFor(ctx context.Context, cwd string, start Starter) (bool, error) {
	if os.Getenv("HERDR_ENV") != "1" || os.Getenv("HERDR_SOCKET_PATH") == "" {
		return false, nil
	}
	root, err := fledgedir.Root(ctx, cwd)
	if err != nil {
		return false, err
	}
	started, err := Ensure(ctx, root, os.Getenv("HERDR_SESSION"), version.Version(), start)
	if errors.Is(err, ErrTestBinary) {
		return false, nil
	}
	return started, err
}

// Note records an EnsureFor outcome on out: a started effect, or a warning
// when the daemon could not be started. Neither fails the command.
func Note(out *cli.Outcome, started bool, err error) {
	switch {
	case err != nil:
		out.Effects = append(out.Effects, cli.Effect{Action: "warning", Kind: "supervisor", ID: err.Error()})
	case started:
		out.Effects = append(out.Effects, cli.Effect{Action: "started", Kind: "supervisor", ID: os.Getenv("HERDR_SESSION")})
	}
}

// Ensure starts the daemon of session in the repository whose primary
// checkout is root when none is live, or when the live one reports another
// version than version, which is stopped first. It reports whether it
// started one. The started daemon may still lose the lock to a concurrent
// start; it then exits without a word.
func Ensure(ctx context.Context, root, session, version string, start Starter) (bool, error) {
	return ensure(ctx, Dir(root, session), version, func() error { return start(root, session) }, nil)
}

func ensure(ctx context.Context, dir, version string, start func() error, stop func(context.Context, string) error) (bool, error) {
	if stop == nil {
		stop = Stop
	}
	st, live, err := Read(dir)
	if err != nil {
		return false, err
	}
	if live && st.Version == version {
		return false, nil
	}
	if live {
		if err := stop(ctx, dir); err != nil {
			return false, err
		}
	}
	return true, start()
}

// Stop sends SIGTERM to the daemon holding the lock in dir and waits up to
// 10 seconds for it to release the lock. No live daemon is success. The pid
// is signalled only while the lock is still held, so a pid reused after the
// daemon exited is never signalled.
func Stop(ctx context.Context, dir string) error {
	st, live, err := Read(dir)
	if err != nil || !live {
		return err
	}
	if st.PID <= 0 {
		return fmt.Errorf("supervise: the live daemon in %s has not recorded its pid yet", dir)
	}
	if _, live, err := Read(dir); err != nil || !live {
		return err
	}
	if err := syscall.Kill(st.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("supervise: signal daemon %d: %w", st.PID, err)
	}
	deadline := time.Now().Add(stopWait)
	for {
		if _, live, err := Read(dir); err != nil || !live {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("supervise: daemon %d did not exit within %s", st.PID, stopWait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Launch starts this executable as a detached fledge supervise run for
// session: a new session, stdio to the log file in the daemon directory, its
// working directory root, and an environment that keeps only PATH, HOME, and
// the Herdr socket and session, so it never acts as the starter's pane. It
// refuses inside a test binary, which would rerun the tests.
func Launch(root, session string) error {
	if testing.Testing() {
		return ErrTestBinary
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("supervise: locate executable: %w", err)
	}
	dir := Dir(root, session)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("supervise: create %s: %w", dir, err)
	}
	log, err := os.OpenFile(filepath.Join(dir, "log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("supervise: open log: %w", err)
	}
	defer log.Close()
	cmd := exec.Command(exe, "supervise", "run")
	cmd.Dir, cmd.Env = root, childEnv(os.Environ())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("supervise: start daemon: %w", err)
	}
	return cmd.Process.Release()
}

// childEnv keeps from env only what the daemon needs, in order.
func childEnv(env []string) []string {
	var kept []string
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		switch key {
		case "PATH", "HOME", "HERDR_SOCKET_PATH", "HERDR_SESSION", "HERDR_ENV":
			kept = append(kept, kv)
		}
	}
	return kept
}
