//go:build windows

package logging

import "os"

// Windows does not expose an os.OpenFile O_NOFOLLOW flag. openLogFile rejects a
// symlinked path with Lstat before opening and validates the opened handle.
func openFileNoFollow(path string, flags int, permission os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flags, permission)
}
