package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	preferencesFilename = "preferences.json"
	preferencesVersion  = 1
)

// preferences remembers the last interactively selected harness and model.
// An empty Model means the harness default.
type preferences struct {
	Version int    `json:"version"`
	Harness string `json:"harness"`
	Model   string `json:"model"`
}

func writePreferences(root string, value preferences) error {
	path := preferencesPath(root)
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Fledge preferences: %w", err)
	}
	contents = append(contents, '\n')

	if err := os.WriteFile(path, contents, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

func readPreferences(root string) (preferences, bool, error) {
	path := preferencesPath(root)
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return preferences{}, false, nil
	}
	if err != nil {
		return preferences{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	var value preferences
	if err := json.Unmarshal(contents, &value); err != nil {
		return preferences{}, false, fmt.Errorf("decode %s: %w", path, err)
	}
	if value.Version != preferencesVersion {
		return preferences{}, false, fmt.Errorf("decode %s: unsupported preferences version %d", path, value.Version)
	}
	if value.Harness == "" {
		return preferences{}, false, fmt.Errorf("decode %s: missing harness", path)
	}

	return value, true, nil
}

func preferencesPath(root string) string {
	return filepath.Join(root, stateDirectory, preferencesFilename)
}
