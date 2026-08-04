package watchproc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/statedir"
)

func TestStopDoesNothingWhenLockIsAcquirable(t *testing.T) {
	root := t.TempDir()
	if err := ensureStateDirectories(root, testSession); err != nil {
		t.Fatal(err)
	}
	pidPath := filepath.Join(statedir.WatchSession(root, testSession), pidFilename)
	if err := os.WriteFile(pidPath, []byte("not-a-pid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	err := stopWith(root, testSession, func(int) error { called = true; return nil })
	if err != nil || called {
		t.Fatalf("stopWith() = %v, terminate called %v", err, called)
	}
}

func TestStopTerminatesValidatedPIDOnlyWhileLockIsHeld(t *testing.T) {
	root := t.TempDir()
	if err := ensureStateDirectories(root, testSession); err != nil {
		t.Fatal(err)
	}
	lock, err := acquire(filepath.Join(statedir.WatchSession(root, testSession), lockFilename))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	pidPath := filepath.Join(statedir.WatchSession(root, testSession), pidFilename)
	if err := os.WriteFile(pidPath, []byte("4321\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gotPID := 0
	err = stopWith(root, testSession, func(pid int) error { gotPID = pid; return nil })
	if err != nil || gotPID != 4321 {
		t.Fatalf("stopWith() = %v, PID %d", err, gotPID)
	}
}

func TestStopDoesNothingForHeldLockWithoutPID(t *testing.T) {
	root := t.TempDir()
	if err := ensureStateDirectories(root, testSession); err != nil {
		t.Fatal(err)
	}
	lock, err := acquire(filepath.Join(statedir.WatchSession(root, testSession), lockFilename))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	called := false
	err = stopWith(root, testSession, func(int) error { called = true; return nil })
	if err != nil || called {
		t.Fatalf("stopWith() = %v, terminate called %v", err, called)
	}
}

func TestStopRefusesCleanupForHeldLockWithoutPID(t *testing.T) {
	root := t.TempDir()
	if err := ensureStateDirectories(root, testSession); err != nil {
		t.Fatal(err)
	}
	lock, err := acquire(filepath.Join(statedir.WatchSession(root, testSession), lockFilename))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	if err := Stop(root, testSession); err == nil {
		t.Fatal("Stop() error = nil, want held-without-PID state preserved")
	}
}

func TestStopRejectsInvalidPIDWhileLockIsHeld(t *testing.T) {
	for _, contents := range []string{"", "0\n", "-1\n", "12x\n", "12\n13\n"} {
		t.Run(contents, func(t *testing.T) {
			root := t.TempDir()
			if err := ensureStateDirectories(root, testSession); err != nil {
				t.Fatal(err)
			}
			lock, err := acquire(filepath.Join(statedir.WatchSession(root, testSession), lockFilename))
			if err != nil {
				t.Fatal(err)
			}
			defer lock.release()
			pidPath := filepath.Join(statedir.WatchSession(root, testSession), pidFilename)
			if err := os.WriteFile(pidPath, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			called := false
			err = stopWith(root, testSession, func(int) error { called = true; return nil })
			if err == nil || called {
				t.Fatalf("stopWith() = %v, terminate called %v", err, called)
			}
		})
	}
}

func TestStopReturnsTerminationError(t *testing.T) {
	root := t.TempDir()
	if err := ensureStateDirectories(root, testSession); err != nil {
		t.Fatal(err)
	}
	lock, err := acquire(filepath.Join(statedir.WatchSession(root, testSession), lockFilename))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	pidPath := filepath.Join(statedir.WatchSession(root, testSession), pidFilename)
	if err := os.WriteFile(pidPath, []byte("4321\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := errors.New("terminate failed")
	if err := stopWith(root, testSession, func(int) error { return want }); !errors.Is(err, want) {
		t.Fatalf("stopWith() error = %v, want %v", err, want)
	}
}

func TestWaitForReleaseWaitsForTheOwnerAndTimesOutSafely(t *testing.T) {
	root := t.TempDir()
	if err := ensureStateDirectories(root, testSession); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(statedir.WatchSession(root, testSession), lockFilename)
	lock, err := acquire(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = lock.release()
		close(released)
	}()
	if err := waitForRelease(lockPath, time.Second, 5*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	<-released

	lock, err = acquire(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	if err := waitForRelease(lockPath, 20*time.Millisecond, 5*time.Millisecond); err == nil {
		t.Fatal("waitForRelease() error = nil, want a timeout while the owner still holds the lock")
	}
}
