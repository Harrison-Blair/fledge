//go:build !windows

package messaging

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// lockAttempts bounds the retries acquireLock makes when the lock file it
// locked turns out to have been unlinked or replaced while it waited.
const lockAttempts = 5

// removeUnderLock keeps the lock held while remove deletes the session's files,
// so no other process can acquire it and write into files that are about to
// disappear. Unlinking a file this process is still flocked to is safe because
// acquireLock re-checks that a locked descriptor still names its path.
func (s *Store) removeUnderLock(path string, remove func() error) error {
	return s.withAcquiredLock(path, remove)
}

func (s *Store) acquireLock(path string) (func() error, error) {
	// RemoveLock and RemoveAll unlink lock files while still holding them, so a
	// descriptor that stopped naming path by the time flock returned guards
	// nothing. Verify the identity and retry against whatever now occupies path.
	for attempt := 0; attempt < lockAttempts; attempt++ {
		file, err := openLockFile(path)
		if err != nil {
			return nil, err
		}
		if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("lock messaging log %q: %w", path, err)
		}
		locked, err := namesLockedFile(file, path)
		if err != nil {
			return nil, errors.Join(err, releaseLock(file))
		}
		if locked {
			return func() error { return releaseLock(file) }, nil
		}
		if err := releaseLock(file); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("lock messaging log %q: lock file was replaced on every attempt", path)
}

// namesLockedFile reports whether path still refers to the locked file.
func namesLockedFile(file *os.File, path string) (bool, error) {
	current, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect messaging lock %q: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("inspect messaging lock %q: %w", path, err)
	}
	return os.SameFile(info, current), nil
}

func releaseLock(file *os.File) error {
	unlockErr := unix.Flock(int(file.Fd()), unix.LOCK_UN)
	closeErr := file.Close()
	if unlockErr != nil {
		unlockErr = fmt.Errorf("unlock messaging log: %w", unlockErr)
	}
	if closeErr != nil {
		closeErr = fmt.Errorf("close messaging lock: %w", closeErr)
	}
	return errors.Join(unlockErr, closeErr)
}
