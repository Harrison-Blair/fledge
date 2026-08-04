//go:build windows

package messaging

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// Windows does not expose an os.OpenFile O_NOFOLLOW flag. openRegular validates
// the opened handle against Lstat before callers perform destructive changes.
func openFileNoFollow(path string, flags int, permission os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flags, permission)
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
	var overlapped windows.Overlapped
	handle := windows.Handle(file.Fd())
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock messaging log %q: %w", path, err)
	}
	return func() error {
		unlockErr := windows.UnlockFileEx(handle, 0, 1, 0, &overlapped)
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
