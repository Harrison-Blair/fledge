package roster

import (
	"fmt"
	"sync"
	"testing"
)

// TestSpeciesList pins the canonical 18-species list and its exact order
// (AC-2).
func TestSpeciesList(t *testing.T) {
	want := []string{
		"adelie", "emperor", "gentoo", "king", "chinstrap", "little",
		"african", "humboldt", "magellanic", "galapagos", "yelloweyed",
		"fiordland", "snares", "erectcrested", "rockhopper", "royal",
		"macaroni", "northernrockhopper",
	}
	if len(Species) != 18 {
		t.Fatalf("len(Species) = %d, want 18", len(Species))
	}
	for i, s := range want {
		if Species[i] != s {
			t.Errorf("Species[%d] = %q, want %q", i, Species[i], s)
		}
	}
}

// TestAssignSequentialAndOverflow checks that Assign hands out species in
// canonical order and, once all 18 bases are in use, overflows to the first
// numeric-suffixed variant (adelie-2).
func TestAssignSequentialAndOverflow(t *testing.T) {
	dir := t.TempDir()

	got, err := Assign(dir, "FTHR-001", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "adelie" {
		t.Fatalf("first Assign = %v, want [adelie]", got)
	}

	got, err = Assign(dir, "FTHR-002", false)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "emperor" {
		t.Fatalf("second Assign = %v, want [emperor]", got)
	}

	// Use the remaining 16 bases so all 18 species are in use.
	for i := 2; i < 18; i++ {
		if _, err := Assign(dir, "F", false); err != nil {
			t.Fatal(err)
		}
	}

	got, err = Assign(dir, "F", false)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "adelie-2" {
		t.Fatalf("overflow Assign = %v, want [adelie-2]", got)
	}
}

// TestPairReleaseFreesOnlyWhenBothReleased verifies per-member release
// tracking: a pair holds one species across two members, releasing one does
// not free it, releasing both does.
func TestPairReleaseFreesOnlyWhenBothReleased(t *testing.T) {
	dir := t.TempDir()

	got, err := Assign(dir, "F", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "adelie" || got[1] != "adelie" {
		t.Fatalf("pair Assign = %v, want [adelie adelie]", got)
	}

	if err := Release(dir, "adelie"); err != nil {
		t.Fatal(err)
	}
	// One member still unreleased -> adelie must not be reused.
	n, err := Assign(dir, "F", false)
	if err != nil {
		t.Fatal(err)
	}
	if n[0] != "emperor" {
		t.Fatalf("after releasing one pair member, Assign = %v, want [emperor]", n)
	}

	// Release the second member; adelie is now fully free again.
	if err := Release(dir, "adelie"); err != nil {
		t.Fatal(err)
	}
	r, err := Assign(dir, "F", false)
	if err != nil {
		t.Fatal(err)
	}
	if r[0] != "adelie" {
		t.Fatalf("after full release, Assign = %v, want [adelie] reused", r)
	}
}

// TestListOmitsFullyReleased checks List returns only live entries.
func TestListOmitsFullyReleased(t *testing.T) {
	dir := t.TempDir()
	if _, err := Assign(dir, "F", false); err != nil { // adelie
		t.Fatal(err)
	}
	if _, err := Assign(dir, "F", false); err != nil { // emperor
		t.Fatal(err)
	}
	if err := Release(dir, "adelie"); err != nil {
		t.Fatal(err)
	}

	live, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 {
		t.Fatalf("List len = %d, want 1 (%v)", len(live), live)
	}
	if live[0].Species != "emperor" {
		t.Fatalf("live[0].Species = %q, want emperor", live[0].Species)
	}
}

// TestConcurrentAssignYieldsDistinctSpecies launches N goroutines that all
// call Assign against one shared state dir, released simultaneously via a
// start barrier to force contention. Without the flock, two goroutines can
// both load the state before either saves and allocate the same species. It
// loops several rounds so the race is deterministic. Mirrors internal/spec's
// TestConcurrentAllocationYieldsDistinctIDs.
func TestConcurrentAssignYieldsDistinctSpecies(t *testing.T) {
	const n = 18
	const rounds = 5

	for round := 0; round < rounds; round++ {
		dir := t.TempDir()

		var ready sync.WaitGroup
		ready.Add(n)
		start := make(chan struct{})
		var wg sync.WaitGroup
		names := make([]string, n)
		errs := make([]error, n)

		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				ready.Done()
				<-start
				got, err := Assign(dir, fmt.Sprintf("F-%d", i), false)
				errs[i] = err
				if len(got) > 0 {
					names[i] = got[0]
				}
			}(i)
		}

		ready.Wait()
		close(start)
		wg.Wait()

		seen := make(map[string]int)
		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d: goroutine %d: Assign: %v", round, i, err)
			}
			seen[names[i]]++
		}
		for s, c := range seen {
			if c > 1 {
				t.Fatalf("round %d: species %s assigned %d times (names: %v)", round, s, c, names)
			}
		}
		if len(seen) != n {
			t.Fatalf("round %d: got %d distinct species, want %d (names: %v)", round, len(seen), n, names)
		}
	}
}
