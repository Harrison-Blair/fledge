package lifecycle

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestSessionRecordLockSerializesStarters(t *testing.T) {
	root := t.TempDir()
	// Initialize a project without writing session.json so the first acquisition
	// proves lockSessionRecord creates its own stable lock file rather than
	// depending on the record already existing.
	initTestProject(t, root)

	unlockFirst, err := lockSessionRecord(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(sessionLockPath(root))
	if err != nil {
		t.Fatalf("stat session lock: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("session lock permissions = %o, want 600", perm)
	}
	if _, err := os.Stat(recordPath(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session record error = %v, want the record absent after a first lock", err)
	}
	acquired := make(chan func() error, 1)
	errs := make(chan error, 1)
	go func() {
		unlock, err := lockSessionRecord(context.Background(), root)
		if err != nil {
			errs <- err
			return
		}
		acquired <- unlock
	}()

	select {
	case err := <-errs:
		t.Fatal(err)
	case unlock := <-acquired:
		_ = unlock()
		t.Fatal("second starter acquired the session lock before the first released it")
	case <-time.After(25 * time.Millisecond):
	}

	if err := unlockFirst(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errs:
		t.Fatal(err)
	case unlock := <-acquired:
		if err := unlock(); err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("second starter did not acquire the released session lock")
	}
}

func TestSessionRecordLockGivesUpWhenTheContextEnds(t *testing.T) {
	root := t.TempDir()
	writeTestRecord(t, root)

	unlockFirst, err := lockSessionRecord(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unlockFirst() }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	blocked := make(chan error, 1)
	go func() {
		unlock, err := lockSessionRecord(ctx, root)
		if err == nil {
			_ = unlock()
		}
		blocked <- err
	}()

	select {
	case err := <-blocked:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("blocked lock error = %v, want the context deadline", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lockSessionRecord wedged behind another holder instead of honoring the context")
	}
}
