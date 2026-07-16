package lock

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func rec(task, owner string) Record {
	return Record{Task: task, Owner: owner, PID: 1, Created: "2026-07-06T12:00:00Z", Branch: "main"}
}

func TestAcquireReleaseGet(t *testing.T) {
	dir := t.TempDir() + "/locks" // Acquire must create the dir
	if err := Acquire(dir, rec("FTHR-001", "adelie")); err != nil {
		t.Fatal(err)
	}
	got, err := Get(dir, "FTHR-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Owner != "adelie" || got.Task != "FTHR-001" {
		t.Errorf("got %+v", got)
	}
	if err := Release(dir, "FTHR-001"); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(dir, "FTHR-001"); err == nil {
		t.Error("Get after Release should fail")
	}
	if err := Release(dir, "FTHR-001"); err == nil {
		t.Error("double Release should fail")
	}
}

// TestWorktreeRoundTrip pins that a non-empty Worktree survives the JSON
// marshal/unmarshal through Acquire/Get/List.
func TestWorktreeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := rec("FTHR-050", "adelie")
	in.Worktree = "/tmp/wt/FTHR-050"
	if err := Acquire(dir, in); err != nil {
		t.Fatal(err)
	}
	got, err := Get(dir, "FTHR-050")
	if err != nil {
		t.Fatal(err)
	}
	if got.Worktree != "/tmp/wt/FTHR-050" {
		t.Errorf("Get Worktree = %q, want /tmp/wt/FTHR-050", got.Worktree)
	}
	recs, _, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Worktree != "/tmp/wt/FTHR-050" {
		t.Errorf("List Worktree = %+v, want /tmp/wt/FTHR-050", recs)
	}
}

// TestWorktreeBackwardCompat pins that a .brood file written before this
// change (JSON with no "worktree" key) still unmarshals cleanly with an empty
// Worktree.
func TestWorktreeBackwardCompat(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"feather":"FTHR-001","owner":"adelie","pid":1,"created":"2026-07-06T12:00:00Z","branch":"main"}`
	if err := os.WriteFile(filepath.Join(dir, "FTHR-001.brood"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Get(dir, "FTHR-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Worktree != "" {
		t.Errorf("legacy record Worktree = %q, want empty", got.Worktree)
	}
}

func TestAcquireHeld(t *testing.T) {
	dir := t.TempDir()
	if err := Acquire(dir, rec("FTHR-001", "adelie")); err != nil {
		t.Fatal(err)
	}
	err := Acquire(dir, rec("FTHR-001", "gentoo"))
	var held *HeldError
	if !errors.As(err, &held) {
		t.Fatalf("want HeldError, got %v", err)
	}
	if held.Existing.Owner != "adelie" {
		t.Errorf("holder = %+v", held.Existing)
	}
}

// Exactly one of N concurrent acquirers wins.
func TestAcquireContention(t *testing.T) {
	dir := t.TempDir()
	const n = 16
	var wg sync.WaitGroup
	wins := make(chan int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if Acquire(dir, rec("FTHR-001", "racer")) == nil {
				wins <- i
			}
		}(i)
	}
	wg.Wait()
	close(wins)
	count := 0
	for range wins {
		count++
	}
	if count != 1 {
		t.Errorf("want exactly 1 winner, got %d", count)
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	if got, skipped, err := List(dir + "/missing"); err != nil || len(got) != 0 || len(skipped) != 0 {
		t.Errorf("missing dir should list empty, got %v, %v, %v", got, skipped, err)
	}
	Acquire(dir, rec("FTHR-002", "b"))
	Acquire(dir, rec("FTHR-001", "a"))
	got, skipped, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %v, want none", skipped)
	}
	if len(got) != 2 || got[0].Task != "FTHR-001" || got[1].Task != "FTHR-002" {
		t.Errorf("List = %+v", got)
	}
}

// TestListSkipsCorruptBroodFile pins FC-2: one unparseable .brood file must
// not hide the healthy claims, and List must report which file(s) it skipped.
func TestListSkipsCorruptBroodFile(t *testing.T) {
	dir := t.TempDir()
	if err := Acquire(dir, rec("FTHR-001", "a")); err != nil {
		t.Fatal(err)
	}
	if err := Acquire(dir, rec("FTHR-002", "b")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "FTHR-998.brood"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "FTHR-999.brood"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, skipped, err := List(dir)
	if err != nil {
		t.Fatalf("List should not abort on corrupt files, got err: %v", err)
	}
	if len(got) != 2 || got[0].Task != "FTHR-001" || got[1].Task != "FTHR-002" {
		t.Errorf("healthy records = %+v, want FTHR-001 and FTHR-002", got)
	}
	wantSkipped := map[string]bool{"FTHR-998.brood": true, "FTHR-999.brood": true}
	if len(skipped) != len(wantSkipped) {
		t.Fatalf("skipped = %v, want %v", skipped, wantSkipped)
	}
	for _, s := range skipped {
		if !wantSkipped[s] {
			t.Errorf("unexpected skipped entry %q", s)
		}
	}
}

// TestAcquireWritesAtomically pins FC-3: a successful Acquire never leaves a
// zero-length/partial .brood file observable, leaves no leftover temp file,
// and still enforces exclusivity via *HeldError.
func TestAcquireWritesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "FTHR-777.brood")

	var sawPartial atomic.Bool
	done := make(chan struct{})
	watcher := make(chan struct{})
	go func() {
		defer close(watcher)
		for {
			select {
			case <-done:
				return
			default:
			}
			b, err := os.ReadFile(path)
			if err == nil {
				if len(b) == 0 || json.Unmarshal(b, new(Record)) != nil {
					sawPartial.Store(true)
				}
			}
		}
	}()

	for i := 0; i < 500; i++ {
		if err := Acquire(dir, rec("FTHR-777", "adelie")); err != nil {
			t.Fatalf("iter %d: Acquire: %v", i, err)
		}
		if err := Release(dir, "FTHR-777"); err != nil {
			t.Fatalf("iter %d: Release: %v", i, err)
		}
	}
	close(done)
	<-watcher

	if sawPartial.Load() {
		t.Error("observed a zero-length or partial .brood file mid-Acquire")
	}

	if err := Acquire(dir, rec("FTHR-777", "adelie")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "FTHR-777.brood" {
		t.Errorf("dir entries = %v, want exactly one FTHR-777.brood (no leftover temp file)", entries)
	}

	err = Acquire(dir, rec("FTHR-777", "gentoo"))
	var held *HeldError
	if !errors.As(err, &held) {
		t.Fatalf("second Acquire for held task: want HeldError, got %v", err)
	}
	if held.Existing.Owner != "adelie" {
		t.Errorf("holder = %+v, want adelie", held.Existing)
	}
}
