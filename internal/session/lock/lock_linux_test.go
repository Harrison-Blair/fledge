//go:build linux

package lock

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"fledge/internal/session/sessiontest"
)

const (
	projectStartLockTestTimeout      = time.Second
	projectStartLockHelperTimeout    = 5 * time.Second
	projectStartLockContenderTimeout = 25 * time.Millisecond
)

func acquireProjectStartLock(t *testing.T, path string) (func() error, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), projectStartLockTestTimeout)
	t.Cleanup(cancel)
	return Acquire(ctx, path)
}

func projectStartLockDirectoryFDs(t *testing.T, path string) int {
	t.Helper()
	want, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat lock directory: %v", err)
	}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("read open descriptors: %v", err)
	}
	count := 0
	for _, entry := range entries {
		got, err := os.Stat(filepath.Join("/proc/self/fd", entry.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("stat descriptor %q: %v", entry.Name(), err)
		}
		if os.SameFile(want, got) {
			count++
		}
	}
	return count
}

func TestProjectStartLockExcludesAndHonorsCancellation(t *testing.T) {
	root := sessiontest.NewProject(t)
	path := filepath.Join(root, ".fledge")
	release, err := acquireProjectStartLock(t, path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), projectStartLockContenderTimeout)
	defer cancel()
	_, err = Acquire(ctx, filepath.Join(root, ".fledge"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire() error = %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatalf("second release: %v", err)
	}
	second, err := acquireProjectStartLock(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := second(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectStartLockRejectsSymlinkAndNonDirectory(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(root, "symlink")
	if err := os.Symlink(directory, symlink); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{symlink, filepath.Join(root, "file")} {
		if path == filepath.Join(root, "file") {
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := Acquire(context.Background(), path); err == nil {
			t.Fatalf("Acquire(%q) error = nil, want rejection", path)
		}
	}
}

func TestProjectStartLockCancellationClosesContendedDescriptor(t *testing.T) {
	root := sessiontest.NewProject(t)
	path := filepath.Join(root, ".fledge")
	release, err := acquireProjectStartLock(t, path)
	if err != nil {
		t.Fatal(err)
	}
	before := projectStartLockDirectoryFDs(t, path)
	if before != 1 {
		t.Fatalf("open descriptors for held lock directory = %d, want 1", before)
	}
	ctx, cancel := context.WithTimeout(context.Background(), projectStartLockContenderTimeout)
	defer cancel()
	if _, err := Acquire(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended acquire error = %v, want deadline exceeded", err)
	}
	after := projectStartLockDirectoryFDs(t, path)
	if after != before {
		t.Fatalf("open descriptors for held lock directory after canceled contender = %d, want %d", after, before)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	release, err = acquireProjectStartLock(t, path)
	if err != nil {
		t.Fatalf("acquire after cancelled contender error = %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectStartLockConcurrentReleaseCachesResult(t *testing.T) {
	root := sessiontest.NewProject(t)
	path := filepath.Join(root, ".fledge")
	release, err := acquireProjectStartLock(t, path)
	if err != nil {
		t.Fatal(err)
	}

	const callers = 16
	start := make(chan struct{})
	errs := make(chan error, callers)
	var callersDone sync.WaitGroup
	callersDone.Add(callers)
	for range callers {
		go func() {
			defer callersDone.Done()
			<-start
			errs <- release()
		}()
	}
	close(start)

	done := make(chan struct{})
	go func() {
		callersDone.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(projectStartLockTestTimeout):
		t.Fatal("concurrent release calls did not finish")
	}
	close(errs)
	results := 0
	for err := range errs {
		results++
		if err != nil {
			t.Fatalf("release() error = %v, want cached nil result", err)
		}
	}
	if results != callers {
		t.Fatalf("release() results = %d, want %d", results, callers)
	}

	second, err := acquireProjectStartLock(t, path)
	if err != nil {
		t.Fatalf("acquire after concurrent release error = %v", err)
	}
	if err := second(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectStartLockReleasesOnProcessExit(t *testing.T) {
	root := sessiontest.NewProject(t)
	path := filepath.Join(root, ".fledge")
	ctx, cancel := context.WithTimeout(context.Background(), projectStartLockHelperTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProjectStartLockReleasesOnProcessExitHelper$")
	command.Env = append(os.Environ(), "FLEDGE_LOCK_HELPER_PATH="+path)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("lock helper failed: %v (context error: %v)\n%s", err, ctx.Err(), output)
	}
	release, err := acquireProjectStartLock(t, path)
	if err != nil {
		t.Fatalf("acquire after helper exit error = %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectStartLockReleasesOnProcessExitHelper(t *testing.T) {
	path := os.Getenv("FLEDGE_LOCK_HELPER_PATH")
	if path == "" {
		return
	}
	release, err := acquireProjectStartLock(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if release == nil {
		t.Fatal("Acquire() returned nil release")
	}
}
