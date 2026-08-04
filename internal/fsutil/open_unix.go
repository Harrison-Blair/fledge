//go:build !windows

package fsutil

import (
	"os"

	"golang.org/x/sys/unix"
)

// OpenNoFollow opens path with O_NOFOLLOW and O_CLOEXEC, so a symlink at the
// final path component fails the open instead of being followed, and the
// descriptor does not leak into spawned processes.
func OpenNoFollow(path string, flags int, permission os.FileMode) (*os.File, error) {
	descriptor, err := unix.Open(path, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC, uint32(permission.Perm()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(descriptor), path), nil
}
