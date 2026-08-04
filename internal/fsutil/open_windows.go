//go:build windows

package fsutil

import "os"

// Windows does not expose an os.OpenFile O_NOFOLLOW flag. OpenRegular's Lstat
// before the open and same-file check after it are the symlink defense there.
func OpenNoFollow(path string, flags int, permission os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flags, permission)
}
