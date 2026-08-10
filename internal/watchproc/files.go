package watchproc

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/Harrison-Blair/fledge/internal/statedir"
)

func ensureStateDirectories(root, session string) error {
	if err := ensureStateRoot(root); err != nil {
		return err
	}
	return ensureDirectories(
		statedir.Temp(root), statedir.TempSession(root, session),
	)
}

func ensureLogDirectory(root, session string) error {
	if err := ensureStateRoot(root); err != nil {
		return err
	}
	return ensureDirectories(statedir.Logs(root), statedir.Session(root, session))
}

// ensureStateRoot creates .fledge if the watcher got there before the project
// did. The directory is user-facing — project.Init creates it 0755 and people
// browse it — so the watcher creates it that way and never re-modes an
// existing one. Only the state below it is the watcher's to keep owner-only.
func ensureStateRoot(root string) error {
	path := statedir.Root(root)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create watch directory %q: %w", path, err)
	}
	return inspectDirectory(path)
}

func ensureDirectories(paths ...string) error {
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create watch directory %q: %w", path, err)
		}
		if err := inspectDirectory(path); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("secure watch directory %q: %w", path, err)
		}
	}
	return nil
}

func inspectDirectory(path string) error {
	info, err := os.Lstat(path)
	switch {
	case err != nil:
		return fmt.Errorf("inspect watch directory %q: %w", path, err)
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("watch directory %q must not be a symlink", path)
	case !info.IsDir():
		return fmt.Errorf("watch path %q is not a directory", path)
	}
	return nil
}

func writePID(path string) error {
	file, err := openOwned(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open watcher PID file %q: %w", path, err)
	}
	if err := file.Truncate(0); err != nil {
		_ = file.Close()
		return fmt.Errorf("truncate watcher PID file %q: %w", path, err)
	}
	_, writeErr := io.WriteString(file, strconv.Itoa(os.Getpid())+"\n")
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

func singletonHeld(path string) (bool, error) {
	lock, err := acquire(path)
	if errors.Is(err, errAlreadyRunning) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, lock.release()
}
