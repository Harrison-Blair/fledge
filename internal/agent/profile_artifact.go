package agent

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const agentProfilesDirectory = "agent-profiles"

// createProfileArtifact writes one immutable instruction snapshot beneath the
// owning session record. The returned cleanup removes only this unique
// artifact directory and is safe to call after a failed launch.
func createProfileArtifact(recordPath, agentName, instructions string) (path string, cleanup func() error, err error) {
	if recordPath == "" {
		return "", nil, fmt.Errorf("session record path is empty")
	}
	info, err := os.Lstat(recordPath)
	if err != nil {
		return "", nil, fmt.Errorf("inspect session record %q: %w", recordPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("session record %q is a symlink", recordPath)
	}
	if !info.IsDir() {
		return "", nil, fmt.Errorf("session record %q is not a directory", recordPath)
	}

	recordRoot, err := os.OpenRoot(recordPath)
	if err != nil {
		return "", nil, fmt.Errorf("open session record %q: %w", recordPath, err)
	}
	defer recordRoot.Close()

	if err := ensureArtifactDirectory(recordRoot); err != nil {
		return "", nil, err
	}
	artifactsRoot, err := recordRoot.OpenRoot(agentProfilesDirectory)
	if err != nil {
		return "", nil, fmt.Errorf("open profile artifact directory: %w", err)
	}
	defer artifactsRoot.Close()

	artifactName, err := makeArtifactDirectory(artifactsRoot, agentName)
	if err != nil {
		return "", nil, err
	}
	relative := filepath.Join(agentProfilesDirectory, artifactName)
	cleanup = func() error {
		root, openErr := os.OpenRoot(recordPath)
		if openErr != nil {
			return fmt.Errorf("open session record for profile cleanup: %w", openErr)
		}
		defer root.Close()
		if removeErr := root.RemoveAll(relative); removeErr != nil {
			return fmt.Errorf("remove profile artifact %q: %w", filepath.Join(recordPath, relative), removeErr)
		}
		return nil
	}

	artifactRoot, err := artifactsRoot.OpenRoot(artifactName)
	if err != nil {
		return "", nil, errors.Join(fmt.Errorf("open profile artifact directory: %w", err), cleanup())
	}
	defer artifactRoot.Close()

	file, err := artifactRoot.OpenFile("instructions.md", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", nil, errors.Join(fmt.Errorf("create profile instructions: %w", err), cleanup())
	}
	_, writeErr := io.Copy(file, strings.NewReader(instructions))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return "", nil, errors.Join(fmt.Errorf("write profile instructions: %w", errors.Join(writeErr, closeErr)), cleanup())
	}

	return filepath.Join(recordPath, relative, "instructions.md"), cleanup, nil
}

func ensureArtifactDirectory(root *os.Root) error {
	info, err := root.Lstat(agentProfilesDirectory)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("profile artifact path %q is a symlink", agentProfilesDirectory)
		}
		if !info.IsDir() {
			return fmt.Errorf("profile artifact path %q is not a directory", agentProfilesDirectory)
		}
		return nil
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("inspect profile artifact path %q: %w", agentProfilesDirectory, err)
	}

	if err := root.Mkdir(agentProfilesDirectory, 0o700); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create profile artifact directory %q: %w", agentProfilesDirectory, err)
		}
		return ensureArtifactDirectory(root)
	}
	return nil
}

func makeArtifactDirectory(root *os.Root, agentName string) (string, error) {
	for range 10 {
		var entropy [8]byte
		if _, err := rand.Read(entropy[:]); err != nil {
			return "", fmt.Errorf("generate profile artifact name: %w", err)
		}
		name := agentName + "-" + hex.EncodeToString(entropy[:])
		if err := root.Mkdir(name, 0o700); err == nil {
			return name, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("create profile artifact directory %q: %w", name, err)
		}
	}
	return "", fmt.Errorf("create a unique profile artifact directory for agent %q", agentName)
}
