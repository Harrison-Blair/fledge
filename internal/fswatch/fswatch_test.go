package fswatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitSignal bounds a test's wait on an inherently asynchronous notification.
// Production code never waits on a deadline; a test has to, or a broken watcher
// hangs the suite instead of failing it.
func waitSignal(t *testing.T, watcher Watcher) bool {
	t.Helper()
	select {
	case <-watcher.Events():
		return true
	case err := <-watcher.Errors():
		t.Fatalf("watcher reported %v", err)
		return false
	case <-time.After(5 * time.Second):
		return false
	}
}

func TestFileSignalsCreationAndModification(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.jsonl")
	watcher, err := File(path)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()

	if err := os.WriteFile(path, []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !waitSignal(t, watcher) {
		t.Fatal("no signal for file creation")
	}

	// Drain any extra signal the create produced so the append is observed on
	// its own merits rather than on a leftover.
	select {
	case <-watcher.Events():
	case <-time.After(200 * time.Millisecond):
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("two\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if !waitSignal(t, watcher) {
		t.Fatal("no signal for file append")
	}
}

func TestFileSignalsRemoval(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "dispatcher.pid")
	if err := os.WriteFile(path, []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	watcher, err := File(path)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if !waitSignal(t, watcher) {
		t.Fatal("no signal for file removal")
	}
}

func TestDirectorySignalsNeighbourChange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	watcher, err := Directory(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()

	// A singleton lock release is only visible through the neighbouring files
	// the exiting owner removes, which is why Directory exists at all.
	if err := os.WriteFile(filepath.Join(dir, "dispatcher.pid"), []byte("7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !waitSignal(t, watcher) {
		t.Fatal("no signal for neighbour creation")
	}
}

func TestCloseIsIdempotentAndStopsSignals(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	watcher, err := Directory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := watcher.Close(); err != nil {
		t.Fatalf("first Close() = %v", err)
	}
	if err := watcher.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "late"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watcher.Events():
		t.Fatal("closed watcher still signalled")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestMissingDirectoryIsReported(t *testing.T) {
	t.Parallel()

	if _, err := Directory(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("Directory() on a missing path returned no error")
	}
	if _, err := File(""); err == nil {
		t.Fatal("File(\"\") returned no error")
	}
}
