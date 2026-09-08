// Package project initializes and discovers Fledge projects.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	schemaVersion = 1
	stateDir      = ".fledge"
	configFile    = "config.json"

	configContents = "{\"schema_version\":1}\n"
	ignoreContents = "/sessions/\n"
)

// ErrNotInitialized indicates that no Fledge project exists at or above a path.
var ErrNotInitialized = errors.New("not inside an initialized Fledge project")

// Init initializes path as a Fledge project and returns its canonical root.
func Init(path string) (string, error) {
	return initProject(path, os.WriteFile, os.RemoveAll)
}

func initProject(
	path string,
	writeFile func(string, []byte, os.FileMode) error,
	removeAll func(string) error,
) (root string, err error) {
	root, err = canonicalDirectory(path)
	if err != nil {
		return "", err
	}

	statePath := filepath.Join(root, stateDir)
	if _, err := os.Lstat(statePath); err == nil {
		return "", fmt.Errorf("initialize Fledge project: %q already exists", statePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect Fledge marker %q: %w", statePath, err)
	}

	if err := os.Mkdir(statePath, 0o755); err != nil {
		return "", fmt.Errorf("create Fledge marker %q: %w", statePath, err)
	}
	defer func() {
		if err == nil {
			return
		}
		if cleanupErr := removeAll(statePath); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("clean up Fledge marker %q: %w", statePath, cleanupErr))
		}
	}()

	files := []struct {
		name     string
		contents string
	}{
		{name: configFile, contents: configContents},
		{name: ".gitignore", contents: ignoreContents},
	}
	for _, file := range files {
		filePath := filepath.Join(statePath, file.name)
		if err := writeFile(filePath, []byte(file.contents), 0o644); err != nil {
			return "", fmt.Errorf("write Fledge project file %q: %w", filePath, err)
		}
	}

	return root, nil
}

// Find returns the canonical root of the nearest Fledge project at or above
// start. An invalid .fledge entry is a hard boundary and returns an error.
func Find(start string) (string, error) {
	current, err := canonicalDirectory(start)
	if err != nil {
		return "", err
	}

	for {
		statePath := filepath.Join(current, stateDir)
		info, statErr := os.Lstat(statePath)
		switch {
		case statErr == nil:
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("Fledge marker %q must not be a symlink", statePath)
			}
			if !info.IsDir() {
				return "", fmt.Errorf("Fledge marker %q is not a directory", statePath)
			}
			if err := validateConfig(filepath.Join(statePath, configFile)); err != nil {
				return "", err
			}
			return current, nil
		case errors.Is(statErr, os.ErrNotExist):
			// Continue toward the filesystem root.
		default:
			return "", fmt.Errorf("inspect Fledge marker %q: %w", statePath, statErr)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("%w (searched from %q)", ErrNotInitialized, start)
		}
		current = parent
	}
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

func validateConfig(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect Fledge config %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Fledge config %q must not be a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("Fledge config %q is not a regular file", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read Fledge config %q: %w", path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	opening, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("parse Fledge config %q: %w", path, err)
	}
	if opening != json.Delim('{') {
		return fmt.Errorf("parse Fledge config %q: expected JSON object", path)
	}

	foundVersion := false
	version := 0
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("parse Fledge config %q: %w", path, err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("parse Fledge config %q: expected object key", path)
		}
		if key != "schema_version" {
			return fmt.Errorf("parse Fledge config %q: unknown field %q", path, key)
		}
		if foundVersion {
			return fmt.Errorf("parse Fledge config %q: duplicate field %q", path, key)
		}
		if err := decoder.Decode(&version); err != nil {
			return fmt.Errorf("parse Fledge config %q: %w", path, err)
		}
		foundVersion = true
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("parse Fledge config %q: %w", path, err)
	}
	if !foundVersion {
		return fmt.Errorf("parse Fledge config %q: missing schema_version", path)
	}
	if version != schemaVersion {
		return fmt.Errorf("parse Fledge config %q: unsupported schema_version %d", path, version)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("parse Fledge config %q: unexpected trailing content", path)
		}
		return fmt.Errorf("parse Fledge config %q: %w", path, err)
	}
	return nil
}
