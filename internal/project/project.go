package project

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	SchemaVersion = 1
	markerDir     = ".fledge"
	markerFile    = "config.json"
)

var ErrNotInitialized = errors.New("project is not initialized")

// Info identifies the project whose Fledge session is being managed.
type Info struct {
	Root          string `json:"root"`
	Session       string `json:"session"`
	SessionSource string `json:"session_source"`
}

type Config struct {
	SchemaVersion int `json:"schema_version"`
}

type InitResult struct {
	ProjectRoot string `json:"project_root"`
	MarkerPath  string `json:"marker_path"`
	Initialized bool   `json:"initialized"`
}

// Init creates a project marker at path. A valid existing marker is accepted
// without rewriting it.
func Init(path string) (InitResult, error) {
	root, err := Canonical(path)
	if err != nil {
		return InitResult{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return InitResult{}, fmt.Errorf("inspect initialization path: %w", err)
	}
	if !info.IsDir() {
		return InitResult{}, fmt.Errorf("initialization path %q is not a directory", root)
	}
	home, _ := canonicalHome()
	if home != "" && root == home {
		return InitResult{}, fmt.Errorf("the home directory cannot be a Fledge project root")
	}

	marker := filepath.Join(root, markerDir, markerFile)
	if _, err := readConfig(marker); err == nil {
		if err := EnsureLogsIgnored(root); err != nil {
			return InitResult{}, err
		}
		return InitResult{ProjectRoot: root, MarkerPath: marker, Initialized: false}, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return InitResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return InitResult{}, fmt.Errorf("create marker directory: %w", err)
	}
	if err := writeConfig(marker); err != nil {
		return InitResult{}, err
	}
	if err := EnsureLogsIgnored(root); err != nil {
		return InitResult{}, err
	}
	return InitResult{ProjectRoot: root, MarkerPath: marker, Initialized: true}, nil
}

// EnsureLogsIgnored idempotently excludes private, project-local audit logs
// without disturbing any unrelated ignore entries.
func EnsureLogsIgnored(root string) error {
	dir := filepath.Join(root, markerDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create Fledge marker directory: %w", err)
	}
	path := filepath.Join(dir, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "/logs/" {
			return nil
		}
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	data = append(data, []byte("/logs/\n")...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("update %s: %w", path, err)
	}
	return nil
}

// Discover canonicalizes dir and walks upward to the closest valid project
// marker. When invoked below the user's home directory, the home directory
// itself is deliberately excluded from the search.
func Discover(dir string) (Info, error) {
	current, err := Canonical(dir)
	if err != nil {
		return Info{}, err
	}
	info, err := os.Stat(current)
	if err != nil {
		return Info{}, fmt.Errorf("inspect invocation directory: %w", err)
	}
	if !info.IsDir() {
		return Info{}, fmt.Errorf("invocation path %q is not a directory", current)
	}
	home, _ := canonicalHome()
	withinHome := home != "" && (current == home || isBelow(current, home))

	for {
		if withinHome && current == home {
			break
		}
		marker := filepath.Join(current, markerDir, markerFile)
		if _, err := readConfig(marker); err == nil {
			return Info{Root: current}, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return Info{}, err
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return Info{}, fmt.Errorf("%w: no %s/%s found; run `fledge init` in the project root",
		ErrNotInitialized, markerDir, markerFile)
}

func readConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("decode Fledge marker %s: %w", path, err)
	}
	if config.SchemaVersion != SchemaVersion {
		return Config{}, fmt.Errorf("unsupported Fledge marker schema %d in %s", config.SchemaVersion, path)
	}
	return config, nil
}

func writeConfig(path string) error {
	data, err := json.MarshalIndent(Config{SchemaVersion: SchemaVersion}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Fledge marker: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary marker: %w", err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o644); err != nil {
		return fmt.Errorf("set marker permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write marker: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync marker: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close marker: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace marker: %w", err)
	}
	ok = true
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func canonicalHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return Canonical(home)
}

func isBelow(path, parent string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// Canonical returns an absolute, symlink-resolved path. Missing final path
// components are preserved so callers can still compare stable identities.
func Canonical(path string) (string, error) {
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get current directory: %w", err)
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return filepath.Clean(abs), nil
}

// ValidateSession validates a Herdr session name supplied by a user or state.
func ValidateSession(session string) error {
	if strings.TrimSpace(session) == "" || strings.ContainsAny(session, "/\x00\n\r") {
		return fmt.Errorf("invalid session name %q", session)
	}
	return nil
}

// SessionName creates a stable, readable name from a canonical project root.
func SessionName(root string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	name := slug(filepath.Base(root))
	if name == "" {
		name = "project"
	}
	return fmt.Sprintf("fledge-%s-%x", name, sum[:4])
}

// WorkspaceLabel returns the user-visible Herdr workspace name for a project.
// Unlike session names, workspace labels preserve the project directory's
// capitalization and spaces.
func WorkspaceLabel(root string) string {
	name := filepath.Base(filepath.Clean(root))
	if strings.TrimSpace(name) == "" || name == "." || name == string(filepath.Separator) {
		return "project"
	}
	return name
}

func slug(value string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
