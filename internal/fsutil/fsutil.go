// Package fsutil holds the small file primitives shared by the packages that
// own files beneath .fledge. The .fledge tree is user-owned, so these favor
// simplicity over crash- and symlink-hardening: a durable write is done with a
// temporary file and a rename, and an in-place write is a plain open.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// OpenRegular opens path with os.OpenFile. The permission applies only when the
// flags create the file. Callers wrap the error with their own subject.
func OpenRegular(path string, flags int, permission os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flags, permission)
}

// WriteFileAtomic writes contents to path by filling a sibling temporary file
// and renaming it into place, so a concurrent reader never observes a partial
// write. The permission is applied to the temporary file before the rename.
func WriteFileAtomic(path string, contents []byte, permission os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file for %q: %w", path, err)
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(permission); err != nil {
		_ = file.Close()
		return fmt.Errorf("secure temporary file for %q: %w", path, err)
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temporary file for %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary file for %q: %w", path, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace %q: %w", path, err)
	}
	return nil
}
