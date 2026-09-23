package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

// A missing path keeps its missing tail but resolves symlinks in the longest
// existing ancestor, as for a pruned checkout reached through a symlink.
func TestCanonicalResolvesExistingAncestorOfMissingPath(t *testing.T) {
	real, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		link:                                   real,
		filepath.Join(link, "gone"):            filepath.Join(real, "gone"),
		filepath.Join(link, "gone", "deeper/"): filepath.Join(real, "gone", "deeper"),
	} {
		if got := Canonical(in); got != want {
			t.Errorf("Canonical(%s) = %s, want %s", in, got, want)
		}
	}
}
