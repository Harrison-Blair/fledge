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
// cancellable while another fledge process holds the session startup lock.
const lockRetryInterval = 100 * time.Millisecond

// lockSessionRecord serializes Start/Stop on a dedicated, never-renamed
// session.lock file. Locking session.json directly would orphan the flock the
// moment rewriteRecord renames a fresh inode over it. The lock file is created
// lazily on first acquisition (no O_EXCL, so concurrent first starters converge
// on one inode) and is never removed by Fledge.
func lockSessionRecord(ctx context.Context, root string) (func() error, error) {
	path := sessionLockPath(root)
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Fledge session startup lock %q: %w", path, err)
	}
	if err := waitForRecordLock(ctx, file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock Fledge session startup lock %q: %w", path, err)
	}
	return func() error {
		unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		closeErr := file.Close()
		return errorsJoinLock(unlockErr, closeErr)
	}, nil
}

// waitForRecordLock retries a non-blocking exclusive flock so a caller whose
// context ends gives up instead of wedging behind another fledge process
// holding the session startup lock.
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
		unlockErr = fmt.Errorf("unlock Fledge session startup lock: %w", unlockErr)
	}
	if closeErr != nil {
		closeErr = fmt.Errorf("close Fledge session startup lock: %w", closeErr)
	}
	return errors.Join(unlockErr, closeErr)
}
