package worktree

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if b, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, b)
	}
}

// A marker identifies one checkout incarnation: it reads back from the marked
// checkout, is never replaced, and a checkout removed and added again at the same path, even on
// the same branch, has none.
func TestMarkerIdentifiesCheckoutIncarnation(t *testing.T) {
	ctx := context.Background()
	root := repository(t)
	path := filepath.Join(t.TempDir(), "topic")
	gitIn(t, root, "worktree", "add", "-q", "-b", "topic", path)
	if got := Marker(ctx, path); got != "" {
		t.Fatalf("unmarked checkout: %q", got)
	}
	id, err := Mark(ctx, path)
	if err != nil || len(id) != 32 {
		t.Fatalf("Mark = %q, %v", id, err)
	}
	if got := Marker(ctx, path); got != id {
		t.Fatalf("Marker = %q; want %q", got, id)
	}
	if _, err := Mark(ctx, path); err == nil || Marker(ctx, path) != id {
		t.Fatal("marked checkout marked again")
	}
	gitIn(t, root, "worktree", "remove", "--force", path)
	gitIn(t, root, "worktree", "add", "-q", path, "topic")
	if got := Marker(ctx, path); got != "" {
		t.Fatalf("recreated checkout inherited marker: %q", got)
	}
	if got := Marker(ctx, filepath.Join(t.TempDir(), "missing")); got != "" {
		t.Fatalf("missing checkout: %q", got)
	}
}

// The primary checkout is never marked: its admin directory is the whole
// repository's.
func TestMarkRefusesPrimaryCheckout(t *testing.T) {
	root := repository(t)
	if _, err := Mark(context.Background(), root); err == nil {
		t.Fatal("marked the primary checkout")
	}
}
