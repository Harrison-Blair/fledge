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

	"github.com/Harrison-Blair/fledge/internal/fswatch"
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

// followLog streams an already-running dispatcher's log until that dispatcher
// exits. Two watches are needed: the log itself, and the singleton directory,
// because a dispatcher that exits without logging is only observable through
// the PID and readiness files it removes on the way out.
func followLog(ctx context.Context, root, session string, output io.Writer, render func([]byte) []byte) error {
	path := filepath.Join(statedir.Session(root, session), LogFilename)
	singletonPath := statedir.TempSession(root, session)
	lockPath := filepath.Join(singletonPath, lockFilename)
	offset, err := writeBacklog(path, output, 50, render)
	if err != nil {
		return err
	}
	logChanges, err := fswatch.File(path)
	if err != nil {
		return err
	}
	defer logChanges.Close()
	singletonChanges, err := fswatch.Directory(singletonPath)
	if err != nil {
		return err
	}
	defer singletonChanges.Close()
	flush := func() error {
		contents, next, err := readComplete(path, offset)
		if err != nil {
			return err
		}
		offset = next
		if len(contents) == 0 {
			return nil
		}
		_, err = output.Write(renderLines(contents, render))
		return err
	}
	// The dispatcher can exit between reading the backlog and arming the watches
	// above, which would leave nothing left to notify this follower. Re-check
	// once the watches are live so that race resolves as a clean exit.
	held, err := singletonHeld(lockPath)
	if err != nil {
		return err
	}
	if !held {
		return flush()
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-logChanges.Errors():
			return fmt.Errorf("follow watch log: %w", err)
		case err := <-singletonChanges.Errors():
			return fmt.Errorf("follow watcher shutdown: %w", err)
		case <-logChanges.Events():
			if err := flush(); err != nil {
				return err
			}
		case <-singletonChanges.Events():
			if err := flush(); err != nil {
				return err
			}
			held, err := singletonHeld(lockPath)
			if err != nil {
				return err
			}
			if !held {
				return flush()
			}
		}
	}
}

func writeBacklog(path string, output io.Writer, lines int, render func([]byte) []byte) (int64, error) {
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
	if _, err := output.Write(renderLines(backlog, render)); err != nil {
		return 0, err
	}
	return int64(end), nil
}

// renderLines applies render to each whole line of a newline-terminated block.
func renderLines(contents []byte, render func([]byte) []byte) []byte {
	var rendered []byte
	for line := range bytes.SplitSeq(bytes.TrimSuffix(contents, []byte{'\n'}), []byte{'\n'}) {
		rendered = append(rendered, render(line)...)
		rendered = append(rendered, '\n')
	}
	return rendered
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
