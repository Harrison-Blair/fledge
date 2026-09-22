// Package install replaces the running fledge executable in place.
package install

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveExecutable returns the real file behind path, following symlinks so
// the link itself is never replaced. An empty path means the running binary.
func ResolveExecutable(path string) (string, error) {
	if path == "" {
		var err error
		path, err = os.Executable()
		if err != nil {
			return "", err
		}
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("update: resolve executable: %w", err)
	}
	return resolved, nil
}

// Replace atomically swaps executable for binary through a temporary file in
// the same directory, keeping the original permissions. The original is left
// untouched on any failure.
func Replace(executable string, binary []byte) error {
	info, err := os.Stat(executable)
	if err != nil {
		return fmt.Errorf("update: stat executable: %w", err)
	}
	dir := filepath.Dir(executable)
	f, err := os.CreateTemp(dir, ".fledge-update-*")
	if err != nil {
		return fmt.Errorf("update: cannot write to %s; use an account with write permission: %w", dir, err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(binary); err != nil {
		return fmt.Errorf("update: write executable: %w", err)
	}
	if err := f.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), executable); err != nil {
		return fmt.Errorf("update: replace executable: %w", err)
	}
	return nil
}
