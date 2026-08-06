package lifecycle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
	"github.com/Harrison-Blair/fledge/internal/statedir"
)

const (
	openCodeConfigEnvironment = "OPENCODE_CONFIG_CONTENT"
	openCodeInstructionsFile  = "opencode-orchestrator-instructions.md"
	openCodeEnvironmentFile   = "opencode-config-content"
)

type openCodeRuntime struct {
	serverEnvironment map[string]string
	paneEnvironment   map[string]string
}

func prepareOpenCodeRuntime(root, session, instructionsPath, originalConfig string) (openCodeRuntime, error) {
	mergedConfig, err := mergeOpenCodeConfig(originalConfig, instructionsPath)
	if err != nil {
		return openCodeRuntime{}, err
	}
	environmentPath := filepath.Join(statedir.TempSession(root, session), openCodeEnvironmentFile)
	if err := writeProtectedFile(environmentPath, []byte(originalConfig)); err != nil {
		return openCodeRuntime{}, errors.Join(err, removeOpenCodeRuntime(root, session))
	}
	return openCodeRuntime{
		serverEnvironment: map[string]string{openCodeConfigEnvironment: mergedConfig},
		paneEnvironment:   map[string]string{openCodeConfigEnvironment: originalConfig},
	}, nil
}

func mergeOpenCodeConfig(original, instructionsPath string) (string, error) {
	config := make(map[string]json.RawMessage)
	if original != "" {
		decoder := json.NewDecoder(bytes.NewBufferString(original))
		if err := decoder.Decode(&config); err != nil {
			return "", fmt.Errorf("decode %s: %w", openCodeConfigEnvironment, err)
		}
		if config == nil {
			return "", fmt.Errorf("decode %s: expected a JSON object", openCodeConfigEnvironment)
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			if err == nil {
				err = errors.New("multiple JSON values")
			}
			return "", fmt.Errorf("decode %s: %w", openCodeConfigEnvironment, err)
		}
	}

	var instructionPaths []string
	if raw, exists := config["instructions"]; exists {
		if err := json.Unmarshal(raw, &instructionPaths); err != nil {
			return "", fmt.Errorf("decode %s instructions: %w", openCodeConfigEnvironment, err)
		}
	}
	for _, existing := range instructionPaths {
		if existing == instructionsPath {
			encoded, err := json.Marshal(config)
			return string(encoded), err
		}
	}
	instructionPaths = append(instructionPaths, instructionsPath)
	encodedInstructions, err := json.Marshal(instructionPaths)
	if err != nil {
		return "", err
	}
	config["instructions"] = encodedInstructions
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", openCodeConfigEnvironment, err)
	}
	return string(encoded), nil
}

func openCodePaneEnvironment(root, session string) (map[string]string, error) {
	paths := []string{
		filepath.Join(statedir.TempSession(root, session), openCodeEnvironmentFile),
		filepath.Join(statedir.Session(root, session), openCodeEnvironmentFile),
	}
	var contents []byte
	var err error
	for _, path := range paths {
		contents, err = os.ReadFile(path)
		if err == nil {
			return map[string]string{openCodeConfigEnvironment: string(contents)}, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read OpenCode environment snapshot: %w", err)
		}
	}
	return nil, nil
}

func removeOpenCodeRuntime(root, session string) error {
	var result error
	paths := []string{
		filepath.Join(statedir.TempSession(root, session), openCodeEnvironmentFile),
		filepath.Join(statedir.Session(root, session), openCodeInstructionsFile),
		filepath.Join(statedir.Session(root, session), openCodeEnvironmentFile),
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return result
}

func removeSessionTemporaryState(root, session string) error {
	path := statedir.TempSession(root, session)
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove session temporary directory %q: %w", path, err)
	}
	return nil
}

func writeProtectedFile(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create OpenCode runtime directory: %w", err)
	}
	file, err := fsutil.OpenRegular(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("protect %s: %w", path, err)
	}
	// OpenRegular must not carry os.O_TRUNC (the Windows symlink-safety
	// contract), so truncate through the validated handle instead.
	if err := file.Truncate(0); err != nil {
		_ = file.Close()
		return fmt.Errorf("truncate %s: %w", path, err)
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}
