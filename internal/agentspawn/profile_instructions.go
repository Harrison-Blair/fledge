package agentspawn

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
	"github.com/Harrison-Blair/fledge/internal/project"
)

const profileInstructionsDir = "profile-instructions"

// MaterializeProfileInstructions stores exact managed-instruction content in a
// deterministic private file beneath the project's disposable runtime tree.
func MaterializeProfileInstructions(projectRoot, instructions string) (string, error) {
	if instructions == "" {
		return "", nil
	}
	tempDir := project.TempDir(projectRoot)
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return "", fmt.Errorf("create project temp directory for profile instructions: %w", err)
	}
	if err := os.Chmod(tempDir, 0o700); err != nil {
		return "", fmt.Errorf("secure project temp directory for profile instructions: %w", err)
	}
	dir := filepath.Join(tempDir, profileInstructionsDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create profile instructions directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("secure profile instructions directory: %w", err)
	}

	sum := sha256.Sum256([]byte(instructions))
	path := filepath.Join(dir, hex.EncodeToString(sum[:])+".txt")
	if err := fsutil.WriteFileAtomic(path, []byte(instructions), 0o600); err != nil {
		return "", fmt.Errorf("materialize profile instructions: %w", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve profile instructions path: %w", err)
	}
	return filepath.Clean(absolute), nil
}
