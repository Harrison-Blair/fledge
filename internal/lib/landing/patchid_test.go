package landing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
)

func commitFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gittest.Git(t, dir, "add", name)
	gittest.Git(t, dir, "commit", "-qm", name)
	return strings.TrimSpace(gittest.Git(t, dir, "rev-parse", "HEAD"))
}

// A rebase with no conflict keeps the patch id. A changed line, or a change
// only in whitespace inside a Go string, gives another patch id.
func TestPatchID(t *testing.T) {
	ctx := context.Background()
	repo := gittest.Repository(t)
	gittest.Git(t, repo, "symbolic-ref", "HEAD", "refs/heads/main")
	commitFile(t, repo, "README.md", "r\n")
	gittest.Git(t, repo, "checkout", "-qb", "feat")
	head := commitFile(t, repo, "a.go", "package main\n\nvar a = \"x y\"\n")
	gittest.Git(t, repo, "checkout", "-q", "main")
	id, err := PatchID(ctx, repo, "refs/heads/main", head)
	if err != nil || !FullHash(id) {
		t.Fatalf("%q %v", id, err)
	}
	if empty, err := PatchID(ctx, repo, "refs/heads/main", "refs/heads/main"); err != nil || empty != "" {
		t.Fatalf("%q %v", empty, err)
	}
	commitFile(t, repo, "main.go", "package main\n")
	gittest.Git(t, repo, "checkout", "-q", "feat")
	gittest.Git(t, repo, "rebase", "-q", "main")
	rebased := strings.TrimSpace(gittest.Git(t, repo, "rev-parse", "HEAD"))
	if got, err := PatchID(ctx, repo, "refs/heads/main", rebased); rebased == head || err != nil || got != id {
		t.Fatalf("rebased: %q %v, want %q", got, err, id)
	}
	for _, content := range []string{"package main\n\nvar a = \"x z\"\n", "package main\n\nvar a = \"x  y\"\n"} {
		gittest.Git(t, repo, "checkout", "-q", "--detach", rebased)
		if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		gittest.Git(t, repo, "commit", "-q", "--amend", "-a", "--no-edit")
		changed := strings.TrimSpace(gittest.Git(t, repo, "rev-parse", "HEAD"))
		if got, err := PatchID(ctx, repo, "refs/heads/main", changed); err != nil || !FullHash(got) || got == id {
			t.Fatalf("%q: %q %v", content, got, err)
		}
	}
	if _, err := PatchID(ctx, repo, "refs/heads/nosuch", head); err == nil {
		t.Fatal("no error for a missing base")
	}
}
