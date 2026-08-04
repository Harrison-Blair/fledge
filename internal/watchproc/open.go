package watchproc

import (
	"os"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
)

// openOwned opens a file the watcher owns: the open refuses to follow symlinks,
// the handle is verified to still be the regular file path names, and the mode
// is narrowed to owner-only.
func openOwned(path string, flags int, permission os.FileMode) (*os.File, error) {
	file, err := fsutil.OpenRegular(path, flags, permission)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}
