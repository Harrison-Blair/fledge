// Package project manages Fledge's tracked project metadata and profiles.
package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	SchemaVersion = 1

	stateDirectory  = ".fledge"
	configFilename  = "config.json"
	profilesDir     = "profiles"
	profileFilename = "orchestrator.toml"

	ignoreContents       = "session.json\npreferences.json\nlogs/\ntmp/\nprofiles/generated/\n"
	legacyIgnoreContents = "*\n!.gitignore\n"
)

// ErrNotInitialized indicates that no initialized Fledge project was found.
var ErrNotInitialized = errors.New("not inside an initialized Fledge project")

// Config is the tracked marker for an initialized Fledge project.
type Config struct {
	SchemaVersion int `json:"schema_version"`
}

// Init initializes path as a Fledge project and returns its canonical root.
// Existing valid metadata and profiles are preserved.
func Init(path string) (string, error) {
	root, err := canonicalDirectory(path)
	if err != nil {
		return "", err
	}

	configPath := filepath.Join(root, stateDirectory, configFilename)
	profilePath := filepath.Join(root, stateDirectory, profilesDir, profileFilename)
	ignorePath := filepath.Join(root, stateDirectory, ".gitignore")

	configExists, err := validateExistingConfig(configPath)
	if err != nil {
		return "", err
	}
	profileExists, err := validateExistingProfile(profilePath)
	if err != nil {
		return "", err
	}
	ignoreAction, err := inspectIgnore(ignorePath)
	if err != nil {
		return "", err
	}
	if err := ensureCodexRulesForInit(root); err != nil {
		return "", err
	}

	// .fledge and .fledge/profiles hold user-facing configuration, so they stay
	// 0755 and are never chmodded by state code; 0700 belongs only to logs/,
	// tmp/, session directories, and profiles/generated/.
	if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
		return "", fmt.Errorf("create Fledge project directories: %w", err)
	}
	if !configExists {
		if err := createFile(configPath, configContents(), 0o644, validateConfigFile); err != nil {
			return "", err
		}
	}
	if !profileExists {
		if err := createFile(profilePath, []byte(defaultProfileContents), 0o644, validateProfileFile); err != nil {
			return "", err
		}
	}
	if err := applyIgnoreAction(ignorePath, ignoreAction); err != nil {
		return "", err
	}

	return root, nil
}

// Find returns the canonical root of the nearest initialized Fledge project at
// or above start. A marker encountered with invalid metadata returns an error.
func Find(start string) (string, error) {
	current, err := canonicalDirectory(start)
	if err != nil {
		return "", err
	}

	for {
		marker := filepath.Join(current, stateDirectory, configFilename)
		_, err := os.Stat(marker)
		switch {
		case err == nil:
			if _, err := loadConfigFile(marker); err != nil {
				return "", err
			}
			return current, nil
		case errors.Is(err, os.ErrNotExist):
			// Keep looking upward.
		default:
			return "", fmt.Errorf("inspect Fledge marker %q: %w", marker, err)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("%w (searched from %q)", ErrNotInitialized, start)
		}
		current = parent
	}
}

// LoadConfig reads and strictly validates the project marker at root.
func LoadConfig(root string) (Config, error) {
	return loadConfigFile(filepath.Join(root, stateDirectory, configFilename))
}

func canonicalDirectory(path string) (string, error) {
	if path == "" {
		path = "."
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve directory %q: %w", path, err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve directory %q: %w", path, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect directory %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path %q is not a directory", resolved)
	}
	return filepath.Clean(resolved), nil
}

func configContents() []byte {
	return []byte("{\n  \"schema_version\": 1\n}\n")
}

func loadConfigFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read Fledge config %q: %w", path, err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	config, err := decodeConfig(decoder)
	if err != nil {
		return Config{}, fmt.Errorf("parse Fledge config %q: %w", path, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Config{}, fmt.Errorf("parse Fledge config %q: %w", path, err)
	}
	if config.SchemaVersion != SchemaVersion {
		return Config{}, fmt.Errorf("parse Fledge config %q: unsupported schema_version %d", path, config.SchemaVersion)
	}
	return config, nil
}

func decodeConfig(decoder *json.Decoder) (Config, error) {
	opening, err := decoder.Token()
	if err != nil {
		return Config{}, err
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return Config{}, errors.New("expected a JSON object")
	}

	var config Config
	seenVersion := false
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return Config{}, err
		}
		key, ok := token.(string)
		if !ok {
			return Config{}, errors.New("expected an object key")
		}
		if key != "schema_version" {
			return Config{}, fmt.Errorf("unknown field %q", key)
		}
		if seenVersion {
			return Config{}, fmt.Errorf("duplicate field %q", key)
		}
		seenVersion = true
		if err := decoder.Decode(&config.SchemaVersion); err != nil {
			return Config{}, fmt.Errorf("field %q: %w", key, err)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("multiple JSON values")
}

func validateExistingConfig(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect Fledge config %q: %w", path, err)
	}
	_, err := loadConfigFile(path)
	return true, err
}

func validateConfigFile(path string) error {
	_, err := loadConfigFile(path)
	return err
}

func validateExistingProfile(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect orchestrator profile %q: %w", path, err)
	}
	return true, validateProfileFile(path)
}

func validateProfileFile(path string) error {
	_, err := loadProfileFile(path)
	return err
}

type ignoreAction struct {
	contents []byte
	write    bool
}

func inspectIgnore(path string) (ignoreAction, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ignoreAction{contents: []byte(ignoreContents), write: true}, nil
	}
	if err != nil {
		return ignoreAction{}, fmt.Errorf("read Fledge ignore file %q: %w", path, err)
	}
	if string(data) == legacyIgnoreContents {
		return ignoreAction{contents: []byte(ignoreContents), write: true}, nil
	}
	updated := appendMissingIgnoreEntries(data)
	return ignoreAction{contents: updated, write: !bytes.Equal(data, updated)}, nil
}

func applyIgnoreAction(path string, action ignoreAction) error {
	if !action.write {
		return nil
	}
	if err := os.WriteFile(path, action.contents, 0o644); err != nil {
		return fmt.Errorf("update Fledge ignore file %q: %w", path, err)
	}
	return nil
}

func appendMissingIgnoreEntries(data []byte) []byte {
	present := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		present[strings.TrimSuffix(line, "\r")] = true
	}
	updated := append([]byte(nil), data...)
	for _, entry := range strings.Split(strings.TrimSuffix(ignoreContents, "\n"), "\n") {
		if present[entry] {
			continue
		}
		if len(updated) > 0 && updated[len(updated)-1] != '\n' {
			updated = append(updated, '\n')
		}
		updated = append(updated, entry...)
		updated = append(updated, '\n')
	}
	return updated
}

// EnsureRuntimeIgnore adds Fledge's untracked runtime files without changing
// existing user entries.
func EnsureRuntimeIgnore(root string) error {
	path := filepath.Join(root, stateDirectory, ".gitignore")
	action, err := inspectIgnore(path)
	if err != nil {
		return err
	}
	return applyIgnoreAction(path, action)
}

func createFile(path string, contents []byte, mode os.FileMode, validateExisting func(string) error) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if errors.Is(err, os.ErrExist) && validateExisting != nil {
		return validateExisting(path)
	}
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create %q: %w", path, err)
	}

	_, writeErr := file.Write(contents)
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("write %q: %w", path, writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close %q: %w", path, closeErr)
	}
	return nil
}
