//go:build !windows

package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func lockSessionRecord(root string) (func() error, error) {
	path := recordPath(root)
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open Fledge session lock %q: %w", path, err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock Fledge session record %q: %w", path, err)
	}
	return func() error {
		unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		closeErr := file.Close()
		return errorsJoinLock(unlockErr, closeErr)
	}, nil
}

func errorsJoinLock(unlockErr, closeErr error) error {
	if unlockErr != nil {
		unlockErr = fmt.Errorf("unlock Fledge session record: %w", unlockErr)
	}
	if closeErr != nil {
		closeErr = fmt.Errorf("close Fledge session lock: %w", closeErr)
	}
	return errors.Join(unlockErr, closeErr)
}
