//go:build !windows

package messaging

import (
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// RemoveLock and RemoveAll unlink lock files while still holding them, so a
// waiter can be granted a lock on a file that has already left the lock path.
// Such a lock excludes nobody and must not be reported as acquired.
func TestAcquireLockRejectsALockFileUnlinkedWhileWaiting(t *testing.T) {
	store := initializedStore(t)
	path := store.lockPath()
	for iteration := 0; iteration < 20; iteration++ {
		held, err := store.acquireLock(path)
		if err != nil {
			t.Fatal(err)
		}
		acquired := make(chan func() error, 1)
		refused := make(chan error, 1)
		go func() {
			unlock, err := store.acquireLock(path)
			if err != nil {
				refused <- err
				return
			}
			acquired <- unlock
		}()

		// Let the waiter open and block on the current lock file before it is
		// unlinked; opening the replacement instead only weakens the iteration.
		time.Sleep(time.Millisecond)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := held(); err != nil {
			t.Fatal(err)
		}

		select {
		case err := <-refused:
			t.Fatalf("iteration %d: %v", iteration, err)
		case unlock := <-acquired:
			assertExclusivelyLocked(t, path, iteration)
			if err := unlock(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func assertExclusivelyLocked(t *testing.T, path string, iteration int) {
	t.Helper()
	file, err := openLockFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	switch err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); {
	case err == nil:
		_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
		t.Fatalf("iteration %d: %s is unlocked; its holder locked a file that no longer names it", iteration, path)
	case errors.Is(err, unix.EWOULDBLOCK):
	default:
		t.Fatal(err)
	}
}
