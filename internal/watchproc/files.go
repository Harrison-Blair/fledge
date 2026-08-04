package watchproc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Harrison-Blair/fledge/internal/statedir"
)

func ensureStateDirectories(root, session string) error {
	return ensureDirectories(
		statedir.Root(root), statedir.Temp(root),
		statedir.TempSession(root, session), statedir.WatchSession(root, session),
	)
}

func ensureLogDirectory(root, session string) error {
	return ensureDirectories(statedir.Root(root), statedir.Logs(root), statedir.Session(root, session))
}

func ensureDirectories(paths ...string) error {
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create watch directory %q: %w", path, err)
		}
		info, err := os.Lstat(path)
		switch {
		case err != nil:
			return fmt.Errorf("inspect watch directory %q: %w", path, err)
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("watch directory %q must not be a symlink", path)
		case !info.IsDir():
			return fmt.Errorf("watch path %q is not a directory", path)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("secure watch directory %q: %w", path, err)
		}
	}
	return nil
}

func writePID(path string) error {
	file, err := openOwned(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open watcher PID file %q: %w", path, err)
	}
	_, writeErr := io.WriteString(file, strconv.Itoa(os.Getpid())+"\n")
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

func createBeacon(path string) error {
	file, err := openOwned(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create watcher beacon %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return err
	}
	return touchBeacon(path, time.Now())
}

func touchBeacon(path string, at time.Time) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("watcher beacon %q is not a regular file", path)
	}
	return os.Chtimes(path, at, at)
}

func followLog(ctx context.Context, root, session string, output io.Writer, interval time.Duration) error {
	path := filepath.Join(statedir.Session(root, session), LogFilename)
	lockPath := filepath.Join(statedir.WatchSession(root, session), lockFilename)
	offset, err := writeBacklog(path, output, 50)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var contents []byte
			contents, offset, err = readComplete(path, offset)
			if err != nil {
				return err
			}
			if len(contents) > 0 {
				if _, err := output.Write(contents); err != nil {
					return err
				}
			}
			held, err := singletonHeld(lockPath)
			if err != nil {
				return err
			}
			if !held {
				contents, _, err := readComplete(path, offset)
				if err != nil {
					return err
				}
				if len(contents) > 0 {
					_, err = output.Write(contents)
				}
				return err
			}
		}
	}
}

func writeBacklog(path string, output io.Writer, lines int) (int64, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read watch log %q: %w", path, err)
	}
	end := bytes.LastIndexByte(contents, '\n') + 1
	if end == 0 {
		return 0, nil
	}
	complete := contents[:end]
	starts := bytes.Split(bytes.TrimSuffix(complete, []byte{'\n'}), []byte{'\n'})
	if len(starts) > lines {
		starts = starts[len(starts)-lines:]
	}
	backlog := append(bytes.Join(starts, []byte{'\n'}), '\n')
	if _, err := output.Write(backlog); err != nil {
		return 0, err
	}
	return int64(end), nil
}

func readComplete(path string, offset int64) ([]byte, int64, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, offset, nil
	}
	if err != nil {
		return nil, offset, fmt.Errorf("open watch log %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, offset, err
	}
	if info.Size() < offset {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return nil, offset, err
	}
	end := bytes.LastIndexByte(contents, '\n') + 1
	if end == 0 {
		return nil, offset, nil
	}
	return contents[:end], offset + int64(end), nil
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
