//go:build !windows

package messaging

import (
	"errors"
	"fmt"
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

func (s *Store) acquireLock() (func() error, error) {
	path := s.lockPath()
	if err := rejectSymlink(path); err != nil {
		return nil, err
	}
	file, err := openRegular(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open messaging lock %q: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure messaging lock %q: %w", path, err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock messaging log %q: %w", path, err)
	}
	return func() error {
		unlockErr := unix.Flock(int(file.Fd()), unix.LOCK_UN)
		closeErr := file.Close()
		if unlockErr != nil {
			unlockErr = fmt.Errorf("unlock messaging log: %w", unlockErr)
		}
		if closeErr != nil {
			closeErr = fmt.Errorf("close messaging lock: %w", closeErr)
		}
		return errors.Join(unlockErr, closeErr)
	}, nil
}
