// Package orchestratorcontext synchronizes the reserved orchestrator profile's
// instructions into repository context files that survive harness context
// resets.
package orchestratorcontext

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
)

const (
	managedToken = "fledge-managed-orchestrator"
	beginMarker  = "<!-- <" + managedToken + "> -->"
	endMarker    = "<!-- </" + managedToken + "> -->"
	lockName     = ".orchestrator-context.lock"
)

type fileAction struct {
	path   string
	data   []byte
	mode   fs.FileMode
	remove bool
	change bool
}

type managedSpan struct {
	start int
	end   int
	found bool
}

// Synchronize makes the root AGENTS.md managed block and Claude bridge match
// instructions. The two files are fully validated before either is changed,
// and concurrent Fledge writers are serialized by a project-local lock.
func Synchronize(projectRoot, instructions string) error {
	if strings.Contains(instructions, managedToken) {
		return errors.New("orchestrator profile instructions contain reserved Fledge managed-block markers")
	}
	root, err := canonicalRoot(projectRoot)
	if err != nil {
		return err
	}
	return withLock(root, func() error {
		agentsPath := filepath.Join(root, "AGENTS.md")
		agentsAction, err := planManagedFile(agentsPath, agentsBlock(instructions), instructions != "")
		if err != nil {
			return fmt.Errorf("synchronize AGENTS.md: %w", err)
		}
		claudeAction, err := planClaudeFile(filepath.Join(root, "CLAUDE.md"), agentsPath, instructions != "")
		if err != nil {
			return fmt.Errorf("synchronize CLAUDE.md: %w", err)
		}
		// Preflight both files before applying either action. This makes expected
		// failures (malformed markers, unsafe links, read-only files) all-or-none.
		if err := apply(agentsAction); err != nil {
			return fmt.Errorf("synchronize AGENTS.md: %w", err)
		}
		if err := apply(claudeAction); err != nil {
			return fmt.Errorf("synchronize CLAUDE.md: %w", err)
		}
		return nil
	})
}

func canonicalRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("project root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	info, err := os.Stat(real)
	if err != nil {
		return "", fmt.Errorf("inspect project root: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("project root is not a directory")
	}
	return filepath.Clean(real), nil
}

func withLock(root string, fn func() error) error {
	dir := filepath.Join(root, ".fledge", "tmp")
	if err := ensureLockDirectory(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, lockName)
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("open Fledge orchestrator-context lock: %w", err)
	}
	defer lock.Close()
	info, err := lock.Stat()
	if err != nil {
		return fmt.Errorf("inspect Fledge orchestrator-context lock: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("Fledge orchestrator-context lock is not a private regular file")
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquire Fledge orchestrator-context lock: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return fn()
}

func ensureLockDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create Fledge orchestrator-context lock directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect Fledge orchestrator-context lock directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Fledge orchestrator-context lock directory is not a real directory")
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("secure Fledge orchestrator-context lock directory: %w", err)
		}
	}
	return nil
}

func agentsBlock(instructions string) string {
	if instructions == "" {
		return ""
	}
	return beginMarker + "\n## Fledge Orchestrator (managed)\n\n" +
		strings.Trim(instructions, "\r\n") + "\n" + endMarker
}

func claudeBlock() string {
	return beginMarker + "\n@AGENTS.md\n" + endMarker
}

func planClaudeFile(path, agentsPath string, enabled bool) (fileAction, error) {
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		linksToAgents, linkErr := symlinkPointsTo(path, agentsPath)
		if linkErr != nil {
			return fileAction{}, linkErr
		}
		if !linksToAgents {
			return fileAction{}, errors.New("context path is an unsafe symlink")
		}
		return fileAction{}, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fileAction{}, fmt.Errorf("inspect context file: %w", err)
	}

	data, mode, exists, err := readRegular(path, info, err)
	if err != nil {
		return fileAction{}, err
	}
	span, err := locateManagedBlock(data)
	if err != nil {
		return fileAction{}, err
	}
	outside := data
	if span.found {
		outside = append(append([]byte(nil), data[:span.start]...), data[span.end:]...)
	}
	desired := ""
	if enabled && !importsAgents(outside) {
		desired = claudeBlock()
	}
	return planFromContents(path, data, mode, exists, span, desired)
}

func planManagedFile(path, desired string, enabled bool) (fileAction, error) {
	info, statErr := os.Lstat(path)
	data, mode, exists, err := readRegular(path, info, statErr)
	if err != nil {
		return fileAction{}, err
	}
	span, err := locateManagedBlock(data)
	if err != nil {
		return fileAction{}, err
	}
	if !enabled {
		desired = ""
	}
	return planFromContents(path, data, mode, exists, span, desired)
}

func readRegular(path string, info fs.FileInfo, statErr error) ([]byte, fs.FileMode, bool, error) {
	if errors.Is(statErr, fs.ErrNotExist) {
		return nil, 0o644, false, nil
	}
	if statErr != nil {
		return nil, 0, false, fmt.Errorf("inspect context file: %w", statErr)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, 0, false, errors.New("context path is an unsafe symlink")
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, errors.New("context path is not a regular file")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, 0, false, fmt.Errorf("open context file: %w", err)
	}
	opened, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, 0, false, fmt.Errorf("inspect open context file: %w", err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		file.Close()
		return nil, 0, false, errors.New("context file changed while opening")
	}
	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, 0, false, fmt.Errorf("read context file: %w", errors.Join(readErr, closeErr))
	}
	return data, info.Mode().Perm(), true, nil
}

func planFromContents(
	path string,
	data []byte,
	mode fs.FileMode,
	exists bool,
	span managedSpan,
	desired string,
) (fileAction, error) {
	if desired == "" && !span.found {
		return fileAction{}, nil
	}
	if desired == "" && generatedOnly(data, span) {
		return fileAction{path: path, remove: true, change: true}, writable(mode, exists)
	}

	var next []byte
	switch {
	case desired == "":
		next = append(append([]byte(nil), data[:span.start]...), data[span.end:]...)
	case span.found:
		block := []byte(withLineEnding(desired, lineEnding(data)))
		next = append(append(append([]byte(nil), data[:span.start]...), block...), data[span.end:]...)
	default:
		eol := lineEnding(data)
		block := []byte(withLineEnding(desired, eol))
		next = append([]byte(nil), data...)
		if len(next) > 0 {
			switch {
			case bytes.HasSuffix(next, []byte(eol+eol)):
			case bytes.HasSuffix(next, []byte(eol)):
				next = append(next, eol...)
			default:
				next = append(next, eol...)
				next = append(next, eol...)
			}
		}
		next = append(next, block...)
		next = append(next, eol...)
	}
	if bytes.Equal(next, data) {
		return fileAction{}, nil
	}
	if err := writable(mode, exists); err != nil {
		return fileAction{}, err
	}
	return fileAction{path: path, data: next, mode: mode, change: true}, nil
}

func writable(mode fs.FileMode, exists bool) error {
	if exists && mode&0o200 == 0 {
		return errors.New("context file is not owner-writable")
	}
	return nil
}

func generatedOnly(data []byte, span managedSpan) bool {
	if !span.found || span.start != 0 {
		return false
	}
	suffix := data[span.end:]
	return len(suffix) == 0 || bytes.Equal(suffix, []byte("\n")) || bytes.Equal(suffix, []byte("\r\n"))
}

func locateManagedBlock(data []byte) (managedSpan, error) {
	beginCount := bytes.Count(data, []byte(beginMarker))
	endCount := bytes.Count(data, []byte(endMarker))
	tokenCount := bytes.Count(data, []byte(managedToken))
	if beginCount == 0 && endCount == 0 && tokenCount == 0 {
		return managedSpan{}, nil
	}
	if beginCount != 1 || endCount != 1 || tokenCount != 2 {
		return managedSpan{}, errors.New("missing, duplicated, or partially edited Fledge managed-block markers")
	}
	start := bytes.Index(data, []byte(beginMarker))
	endStart := bytes.Index(data, []byte(endMarker))
	end := endStart + len(endMarker)
	if start >= endStart || !markerIsWholeLine(data, start, len(beginMarker)) ||
		!markerIsWholeLine(data, endStart, len(endMarker)) {
		return managedSpan{}, errors.New("reordered or partially edited Fledge managed-block markers")
	}
	return managedSpan{start: start, end: end, found: true}, nil
}

func markerIsWholeLine(data []byte, start, size int) bool {
	beforeOK := start == 0 || data[start-1] == '\n'
	after := start + size
	afterOK := after == len(data) || data[after] == '\n' ||
		(data[after] == '\r' && after+1 < len(data) && data[after+1] == '\n')
	return beforeOK && afterOK
}

func lineEnding(data []byte) string {
	if index := bytes.IndexByte(data, '\n'); index > 0 && data[index-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

func withLineEnding(value, eol string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	if eol == "\r\n" {
		value = strings.ReplaceAll(value, "\n", "\r\n")
	}
	return value
}

func importsAgents(data []byte) bool {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "@AGENTS.md" {
			return true
		}
	}
	return false
}

func symlinkPointsTo(path, agentsPath string) (bool, error) {
	target, err := os.Readlink(path)
	if err != nil {
		return false, fmt.Errorf("read context symlink: %w", err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return filepath.Clean(target) == filepath.Clean(agentsPath), nil
}

func apply(action fileAction) error {
	if !action.change {
		return nil
	}
	if action.remove {
		if err := os.Remove(action.path); err != nil {
			return fmt.Errorf("remove generated context file: %w", err)
		}
		syncDirectory(filepath.Dir(action.path))
		return nil
	}
	if err := fsutil.WriteFileAtomic(action.path, action.data, action.mode); err != nil {
		return fmt.Errorf("persist context file: %w", err)
	}
	return nil
}

func syncDirectory(path string) {
	dir, err := os.Open(path)
	if err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
}
