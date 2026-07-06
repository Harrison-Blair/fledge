package lock

import (
	"errors"
	"sync"
	"testing"
)

func rec(task, owner string) Record {
	return Record{Task: task, Owner: owner, PID: 1, Created: "2026-07-06T12:00:00Z", Branch: "main"}
}

func TestAcquireReleaseGet(t *testing.T) {
	dir := t.TempDir() + "/locks" // Acquire must create the dir
	if err := Acquire(dir, rec("TASK-001", "adelie")); err != nil {
		t.Fatal(err)
	}
	got, err := Get(dir, "TASK-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Owner != "adelie" || got.Task != "TASK-001" {
		t.Errorf("got %+v", got)
	}
	if err := Release(dir, "TASK-001"); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(dir, "TASK-001"); err == nil {
		t.Error("Get after Release should fail")
	}
	if err := Release(dir, "TASK-001"); err == nil {
		t.Error("double Release should fail")
	}
}

func TestAcquireHeld(t *testing.T) {
	dir := t.TempDir()
	if err := Acquire(dir, rec("TASK-001", "adelie")); err != nil {
		t.Fatal(err)
	}
	err := Acquire(dir, rec("TASK-001", "gentoo"))
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
			if Acquire(dir, rec("TASK-001", "racer")) == nil {
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
	if got, err := List(dir + "/missing"); err != nil || len(got) != 0 {
		t.Errorf("missing dir should list empty, got %v, %v", got, err)
	}
	Acquire(dir, rec("TASK-002", "b"))
	Acquire(dir, rec("TASK-001", "a"))
	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Task != "TASK-001" || got[1].Task != "TASK-002" {
		t.Errorf("List = %+v", got)
	}
}
