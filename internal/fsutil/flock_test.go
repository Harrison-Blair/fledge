package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReleaseFlockDropsLockSoItCanBeReacquired(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "resource.lock")

	held, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Flock(int(held.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatalf("initial Flock() error = %v", err)
	}

	// A second open file description must not be able to take the lock while the
	// first still holds it, so the release below is what actually frees it.
	contender, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer contender.Close()
	if err := unix.Flock(int(contender.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
		t.Fatal("second Flock() succeeded while the lock was held; want contention")
	}

	if err := ReleaseFlock(held, "session startup lock"); err != nil {
		t.Fatalf("ReleaseFlock() error = %v", err)
	}

	if err := unix.Flock(int(contender.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatalf("re-acquire after ReleaseFlock() error = %v; want success", err)
	}
	if err := unix.Flock(int(contender.Fd()), unix.LOCK_UN); err != nil {
		t.Fatalf("cleanup unlock error = %v", err)
	}
}

func TestReleaseFlockWrapsFailuresWithSubject(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "resource.lock")

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	// Closing first invalidates the descriptor, so both the unlock and the close
	// inside ReleaseFlock fail and must be reported against the named subject.
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	err = ReleaseFlock(file, "messaging lock")
	if err == nil {
		t.Fatal("ReleaseFlock(closed) error = nil, want joined failure")
	}
	if !strings.Contains(err.Error(), "messaging lock") {
		t.Errorf("ReleaseFlock() error = %v, want it to name the subject", err)
	}
	// Both the unlock and close legs are wrapped, so the subject appears twice.
	if strings.Count(err.Error(), "messaging lock") < 2 {
		t.Errorf("ReleaseFlock() error = %v, want both unlock and close legs named", err)
	}
}
