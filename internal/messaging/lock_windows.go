//go:build windows

package messaging

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// Windows refuses to delete a file that is still open, so the lock has to be
// released before remove can unlink it. Unix closes the window this leaves
// between unlocking and deleting by holding the lock throughout instead.
func (s *Store) removeUnderLock(path string, remove func() error) error {
	unlock, err := s.acquireLock(path)
	if err != nil {
		return err
	}
	if err := unlock(); err != nil {
		return err
	}
	return remove()
}

func (s *Store) acquireLock(path string) (func() error, error) {
	file, err := openLockFile(path)
	if err != nil {
		return nil, err
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
