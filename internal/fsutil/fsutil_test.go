package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWriteFileAtomicPersistsContentAndPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := WriteFileAtomic(path, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second\n" {
		t.Fatalf("content = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".*.tmp"))
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func TestWriteFileAtomicLeavesNoTemporaryFileWhenRenameFails(t *testing.T) {
	dir := t.TempDir()
	// A directory cannot be replaced by a file, so the rename fails.
	path := filepath.Join(dir, "occupied")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("data"), 0o600); err == nil {
		t.Fatal("expected an error")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".*.tmp"))
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func TestWithFlockSerializesConcurrentCallers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.lock")
	const callers = 16
	var (
		mu     sync.Mutex
		inside bool
		shared int
		wg     sync.WaitGroup
	)
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- WithFlock(path, func() error {
				mu.Lock()
				busy := inside
				inside = true
				value := shared
				mu.Unlock()
				if busy {
					return errors.New("lock did not exclude a concurrent caller")
				}
				// Widen the window in which an unserialized caller observes
				// the stale counter value.
				time.Sleep(time.Millisecond)
				mu.Lock()
				shared = value + 1
				inside = false
				mu.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if shared != callers {
		t.Fatalf("shared = %d, want %d", shared, callers)
	}
}

func TestWithFlockReturnsCallbackErrorUnchanged(t *testing.T) {
	sentinel := errors.New("callback failed")
	err := WithFlock(filepath.Join(t.TempDir(), "run.lock"), func() error { return sentinel })
	if err != sentinel {
		t.Fatalf("error = %v", err)
	}
}

func TestWithFlockReportsLockFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "run.lock")
	called := false
	err := WithFlock(path, func() error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrLock) {
		t.Fatalf("error = %v, want ErrLock", err)
	}
	if called {
		t.Fatal("callback ran without the lock")
	}
}
