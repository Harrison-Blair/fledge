package lifecycle

import (
	"testing"
	"time"
)

func TestSessionRecordLockSerializesStarters(t *testing.T) {
	root := t.TempDir()
	writeTestRecord(t, root)

	unlockFirst, err := lockSessionRecord(root)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan func() error, 1)
	errs := make(chan error, 1)
	go func() {
		unlock, err := lockSessionRecord(root)
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
