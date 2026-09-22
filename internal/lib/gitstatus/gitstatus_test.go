package gitstatus

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=t@example.com"}, args...)...)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, b)
	}
}

// repository returns a repo on main with one commit.
func repository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "commit", "-qm", "initial", "--allow-empty")
	return root
}

func commit(t *testing.T, dir, file string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", file)
	git(t, dir, "commit", "-qm", file)
}

func TestDirty(t *testing.T) {
	ctx := context.Background()
	root := repository(t)
	if got := Dirty(ctx, root); got != "no" {
		t.Fatalf("clean: %s", got)
	}
	os.WriteFile(filepath.Join(root, "untracked"), []byte("x"), 0o644)
	if got := Dirty(ctx, root); got != "yes" {
		t.Fatalf("untracked: %s", got)
	}
	if got := Dirty(ctx, filepath.Join(root, "missing")); got != "unknown" {
		t.Fatalf("missing: %s", got)
	}
	// A leftover directory inside another checkout is not itself a checkout.
	os.Remove(filepath.Join(root, "untracked"))
	leftover := filepath.Join(root, "leftover")
	os.Mkdir(leftover, 0o755)
	if got := Dirty(ctx, leftover); got != "unknown" {
		t.Fatalf("leftover: %s", got)
	}
}

func TestDirtyLinkedCheckout(t *testing.T) {
	ctx := context.Background()
	root := repository(t)
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, root, "worktree", "add", "-q", "-b", "topic", linked)
	if got := Dirty(ctx, linked); got != "no" {
		t.Fatalf("clean linked: %s", got)
	}
	os.WriteFile(filepath.Join(linked, "new"), []byte("x"), 0o644)
	if got := Dirty(ctx, linked); got != "yes" {
		t.Fatalf("dirty linked: %s", got)
	}
	if got := Dirty(ctx, root); got != "no" {
		t.Fatalf("primary reported linked changes: %s", got)
	}
}

func TestDefaultBranch(t *testing.T) {
	ctx := context.Background()
	t.Run("origin HEAD", func(t *testing.T) {
		root := repository(t)
		git(t, root, "branch", "dev")
		git(t, root, "update-ref", "refs/remotes/origin/trunk", "HEAD")
		git(t, root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
		if got := DefaultBranch(ctx, root); got != "refs/remotes/origin/trunk" {
			t.Fatal(got)
		}
	})
	t.Run("dev before main", func(t *testing.T) {
		root := repository(t)
		git(t, root, "branch", "dev")
		if got := DefaultBranch(ctx, root); got != "refs/heads/dev" {
			t.Fatal(got)
		}
	})
	t.Run("main", func(t *testing.T) {
		root := repository(t)
		if got := DefaultBranch(ctx, root); got != "refs/heads/main" {
			t.Fatal(got)
		}
	})
	t.Run("none", func(t *testing.T) {
		root := t.TempDir()
		git(t, root, "init", "-q", "-b", "trunk")
		git(t, root, "commit", "-qm", "initial", "--allow-empty")
		if got := DefaultBranch(ctx, root); got != "" {
			t.Fatal(got)
		}
	})
}

func TestMerged(t *testing.T) {
	ctx := context.Background()
	root := repository(t)
	for _, b := range []string{"merged", "unmerged", "squashed"} {
		git(t, root, "branch", b)
	}
	git(t, root, "switch", "-q", "merged")
	commit(t, root, "merged")
	git(t, root, "switch", "-q", "unmerged")
	commit(t, root, "unmerged")
	git(t, root, "switch", "-q", "squashed")
	commit(t, root, "squashed")
	git(t, root, "switch", "-q", "main")
	git(t, root, "merge", "-q", "--no-edit", "merged")
	git(t, root, "merge", "-q", "--squash", "squashed")
	git(t, root, "commit", "-qm", "squash")
	target := "refs/heads/main"
	for _, tc := range []struct{ rev, want string }{
		{"refs/heads/merged", "yes"},
		{"refs/heads/unmerged", "no"},
		{"refs/heads/squashed", "no"},
		{"refs/heads/missing", "unknown"},
	} {
		if got := Merged(ctx, root, tc.rev, target); got != tc.want {
			t.Errorf("%s: got %s want %s", tc.rev, got, tc.want)
		}
	}
	if got := Merged(ctx, root, "refs/heads/merged", ""); got != "unknown" {
		t.Errorf("no target: %s", got)
	}
	if got := Merged(ctx, root, "refs/heads/merged", "refs/heads/missing"); got != "unknown" {
		t.Errorf("missing target: %s", got)
	}
}
