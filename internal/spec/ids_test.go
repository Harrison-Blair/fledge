package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNextID(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{"empty dir", nil, "FTHR-001"},
		{"sequential", []string{"FTHR-001-a.md", "FTHR-002-b.md"}, "FTHR-003"},
		{"gaps use max not count", []string{"FTHR-001-a.md", "FTHR-007-b.md"}, "FTHR-008"},
		{"wide ids keep width", []string{"FTHR-1042-a.md"}, "FTHR-1043"},
		{"ignores non-matching files", []string{"README.md", "FTHR-002-b.md", "notes.txt"}, "FTHR-003"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := NextID(dir, "FTHR")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("NextID = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNextIDMissingDir(t *testing.T) {
	got, err := NextID(filepath.Join(t.TempDir(), "nope"), "PLM")
	if err != nil {
		t.Fatal(err)
	}
	if got != "PLM-001" {
		t.Errorf("NextID = %q, want PLM-001", got)
	}
}

// TestConcurrentAllocationYieldsDistinctIDs launches N goroutines that all
// call AllocateAndCreate against one shared dir, released simultaneously via
// a start barrier to force contention. Without serialization, two goroutines
// can both scan the dir before either creates a file and allocate the same
// ID. It loops several rounds so the race is deterministic.
func TestConcurrentAllocationYieldsDistinctIDs(t *testing.T) {
	const n = 20
	const rounds = 5

	for round := 0; round < rounds; round++ {
		dir := t.TempDir()

		var ready sync.WaitGroup
		ready.Add(n)
		start := make(chan struct{})
		var wg sync.WaitGroup
		ids := make([]string, n)
		errs := make([]error, n)

		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				ready.Done()
				<-start
				id, _, err := AllocateAndCreate(dir, "FTHR", func(id string) (string, []byte) {
					return filepath.Join(dir, fmt.Sprintf("%s-worker-%d.md", id, i)), []byte("x")
				})
				ids[i] = id
				errs[i] = err
			}(i)
		}

		ready.Wait()
		close(start)
		wg.Wait()

		seen := make(map[string]int)
		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d: goroutine %d: AllocateAndCreate: %v", round, i, err)
			}
			seen[ids[i]]++
		}
		for id, count := range seen {
			if count > 1 {
				t.Fatalf("round %d: id %s allocated %d times, want distinct IDs (ids: %v)", round, id, count, ids)
			}
		}
		if len(seen) != n {
			t.Fatalf("round %d: got %d distinct ids, want %d (ids: %v)", round, len(seen), n, ids)
		}
	}
}

func TestKebab(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Deterministic CLI", "deterministic-cli"},
		{"Wire graph: waves & cycles", "wire-graph-waves-cycles"},
		{"  spaces   everywhere  ", "spaces-everywhere"},
		{"already-kebab", "already-kebab"},
		{"Ünïcode Títle", "ünïcode-títle"},
		{"123 numbers", "123-numbers"},
	}
	for _, tt := range tests {
		if got := Kebab(tt.in); got != tt.want {
			t.Errorf("Kebab(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
