//go:build !windows

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// lockRetryInterval paces the non-blocking retries that keep a waiter
// cancellable while another fledge process holds the session record.
const lockRetryInterval = 100 * time.Millisecond

func lockSessionRecord(ctx context.Context, root string) (func() error, error) {
	path := recordPath(root)
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open Fledge session lock %q: %w", path, err)
	}
	if err := waitForRecordLock(ctx, file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock Fledge session record %q: %w", path, err)
	}
	return func() error {
		unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		closeErr := file.Close()
		return errorsJoinLock(unlockErr, closeErr)
	}, nil
}

// waitForRecordLock retries a non-blocking exclusive flock so a caller whose
// context ends gives up instead of wedging behind another fledge process.
func waitForRecordLock(ctx context.Context, file *os.File) error {
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return err
		}
		timer := time.NewTimer(lockRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
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
