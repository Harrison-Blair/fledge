package watchproc

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
	"github.com/Harrison-Blair/fledge/internal/fswatch"
)

const maxPIDFileBytes = 32

const (
	stopReleaseTimeout = 10 * time.Second
)

// Stop terminates the watcher that currently owns the session singleton. A
// missing watcher, an acquirable lock, or a held lock without a PID file is a
// successful no-op. Lifecycle calls this before removing temporary state.
func Stop(root, session string) error {
	terminated, err := stopAttempt(root, session, terminateProcess)
	if err != nil {
		return err
	}
	watchPath := fsutil.TempSession(root, session)
	lockPath := filepath.Join(watchPath, lockFilename)
	if !terminated {
		if _, err := os.Lstat(lockPath); errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return fmt.Errorf("inspect watch lock %q: %w", lockPath, err)
		}
		held, err := singletonHeld(lockPath)
		if err != nil {
			return err
		}
		if held {
			return fmt.Errorf("watcher owns singleton lock %q but has no recorded PID", lockPath)
		}
		return nil
	}
	return waitForRelease(lockPath, stopReleaseTimeout)
}

// waitForRelease blocks until the terminating watcher has dropped the
// singleton lock. Releasing an advisory lock changes no file, so the whole
// directory is watched: what is actually observable is the exiting watcher
// removing its PID and readiness files beside the lock. The timeout bounds a
// process that dies without unwinding; it is not a poll interval.
func waitForRelease(lockPath string, timeout time.Duration) error {
	changes, err := fswatch.Directory(filepath.Dir(lockPath))
	if err != nil {
		return err
	}
	defer changes.Close()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		held, err := singletonHeld(lockPath)
		if err != nil {
			return err
		}
		if !held {
			return nil
		}
		select {
		case <-changes.Events():
			continue
		case err := <-changes.Errors():
			return fmt.Errorf("await watcher shutdown: %w", err)
		case <-timer.C:
			return fmt.Errorf("watcher did not release singleton lock %q within %s", lockPath, timeout)
		}
	}
}

func stopWith(root, session string, terminate func(int) error) error {
	_, err := stopAttempt(root, session, terminate)
	return err
}

func stopAttempt(root, session string, terminate func(int) error) (bool, error) {
	if strings.TrimSpace(root) == "" {
		return false, errors.New("watch project root is missing")
	}
	if !fsutil.ValidSessionDirName(session) {
		return false, fmt.Errorf("Herdr session name %q is not a valid watch directory name", session)
	}
	watchPath := fsutil.TempSession(root, session)
	if _, err := os.Lstat(watchPath); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect watch state directory %q: %w", watchPath, err)
	}
	lockPath := filepath.Join(watchPath, lockFilename)
	if _, err := os.Lstat(lockPath); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect watch lock %q: %w", lockPath, err)
	}
	lock, err := acquire(lockPath)
	switch {
	case err == nil:
		return false, lock.release()
	case !errors.Is(err, errAlreadyRunning):
		return false, err
	}

	pidPath := filepath.Join(watchPath, pidFilename)
	pid, err := readPID(pidPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if pid == os.Getpid() {
		return false, fmt.Errorf("refuse to terminate current process from watcher PID file %q", pidPath)
	}
	if err := terminate(pid); err != nil {
		return false, fmt.Errorf("terminate watcher process %d: %w", pid, err)
	}
	return true, nil
}

func readPID(path string) (int, error) {
	file, err := openOwned(path, os.O_RDONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, maxPIDFileBytes+1))
	if err != nil {
		return 0, fmt.Errorf("read watcher PID file %q: %w", path, err)
	}
	if len(contents) > maxPIDFileBytes {
		return 0, fmt.Errorf("watcher PID file %q is too large", path)
	}
	text := string(contents)
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return 0, fmt.Errorf("watcher PID file %q is empty", path)
	}
	for _, character := range text {
		if character < '0' || character > '9' {
			return 0, fmt.Errorf("watcher PID file %q is invalid", path)
		}
	}
	pid64, err := strconv.ParseInt(text, 10, 32)
	if err != nil || pid64 <= 0 {
		return 0, fmt.Errorf("watcher PID file %q is invalid", path)
	}
	return int(pid64), nil
}
