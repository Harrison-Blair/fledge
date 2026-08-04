//go:build !windows

package logging

import (
	"os"

	"golang.org/x/sys/unix"
)

func openFileNoFollow(path string, flags int, permission os.FileMode) (*os.File, error) {
	descriptor, err := unix.Open(path, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC, uint32(permission.Perm()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(descriptor), path), nil
}
