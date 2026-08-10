// Package logging opens the per-session structured debug log (fledge.log).
//
// Every fledge process that touches a session appends to the same file. That is
// safe without a lock file: the log is opened with O_APPEND and slog emits one
// Write per record, so the kernel keeps individual records intact and no reader
// ever observes an interleaved line. Records from different processes may be
// interleaved with each other, but never within a line.
//
// The debug log is deliberately separate from the messaging audit log: nothing
// here writes message bodies.
package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
)

// FileName is the debug log file name inside a session log directory.
const FileName = "fledge.log"

// LevelEnvVar names the environment variable that selects the log level. It
// accepts debug, info, warn and error; anything else means info.
const LevelEnvVar = "FLEDGE_LOG_LEVEL"

// ParseLevel maps an environment value to a slog level. Unknown, blank and
// malformed values fall back to slog.LevelInfo.
func ParseLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Open creates dir if needed and opens dir/fledge.log for appending, returning
// a JSON logger and the underlying file so callers can close it.
func Open(dir string, level slog.Leveler) (*slog.Logger, io.Closer, error) {
	if err := ensureDirectory(dir); err != nil {
		return nil, nil, err
	}
	file, err := openLogFile(dir)
	if err != nil {
		return nil, nil, err
	}
	logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{Level: level}))
	return logger, file, nil
}

// Discard returns a logger that drops every record. Callers use it when the log
// file cannot be opened: logging must never break a lifecycle operation.
func Discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func ensureDirectory(dir string) error {
	levels, err := ownedLevels(dir)
	if err != nil {
		return err
	}
	for _, path := range levels {
		if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create log directory %q: %w", path, err)
		}
		info, err := os.Lstat(path)
		switch {
		case err != nil:
			return fmt.Errorf("inspect log directory %q: %w", path, err)
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("log directory %q must not be a symlink", path)
		case !info.IsDir():
			return fmt.Errorf("log path %q is not a directory", path)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("secure log directory %q: %w", path, err)
		}
	}
	return nil
}

// ownedLevels returns dir plus every ancestor that does not exist yet, ordered
// outermost first. The deepest existing ancestor is validated as a real
// directory but is not returned: fledge only re-permissions what it owns.
func ownedLevels(dir string) ([]string, error) {
	clean := filepath.Clean(dir)
	var levels []string
	for path := clean; ; {
		info, err := os.Lstat(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			parent := filepath.Dir(path)
			if parent == path {
				return nil, fmt.Errorf("log directory %q has no existing ancestor", dir)
			}
			levels = append(levels, path)
			path = parent
		case err != nil:
			return nil, fmt.Errorf("inspect log directory %q: %w", path, err)
		case info.Mode()&os.ModeSymlink != 0:
			return nil, fmt.Errorf("log directory %q must not be a symlink", path)
		case !info.IsDir():
			return nil, fmt.Errorf("log path %q is not a directory", path)
		default:
			if len(levels) == 0 {
				levels = append(levels, clean)
			}
			slices.Reverse(levels)
			return levels, nil
		}
	}
}

func openLogFile(dir string) (*os.File, error) {
	path := filepath.Join(dir, FileName)
	switch info, err := os.Lstat(path); {
	case errors.Is(err, os.ErrNotExist):
		// The log file does not exist yet; it will be created below.
	case err != nil:
		return nil, fmt.Errorf("inspect log file %q: %w", path, err)
	case info.Mode()&os.ModeSymlink != 0:
		return nil, fmt.Errorf("log file %q must not be a symlink", path)
	}
	file, err := fsutil.OpenRegular(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure log file %q: %w", path, err)
	}
	return file, nil
}
