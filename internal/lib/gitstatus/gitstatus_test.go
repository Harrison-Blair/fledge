package gitstatus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
)

// repository returns a repo on main with one commit.
func repository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gittest.Git(t, root, "init", "-q", "-b", "main")
	gittest.Commit(t, root)
	return root
}

func commit(t *testing.T, dir, file string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	gittest.Git(t, dir, "add", file)
	gittest.Git(t, dir, "commit", "-qm", file)
}

// configure writes content as root's .fledge/config.toml.
func configure(t *testing.T, root, content string) {
	t.Helper()
	os.MkdirAll(filepath.Join(root, ".fledge"), 0o755)
	if err := os.WriteFile(filepath.Join(root, ".fledge", "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
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
	gittest.Git(t, root, "worktree", "add", "-q", "-b", "topic", linked)
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
	originHEAD := func(t *testing.T, root string) {
		gittest.Git(t, root, "update-ref", "refs/remotes/origin/trunk", "HEAD")
		gittest.Git(t, root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
	}
	check := func(t *testing.T, root, want string) {
		t.Helper()
		if got, err := DefaultBranch(ctx, root); got != want || err != nil {
			t.Fatalf("got %q, %v want %q", got, err, want)
		}
	}
	t.Run("configured before origin HEAD", func(t *testing.T) {
		root := repository(t)
		gittest.Git(t, root, "branch", "integration")
		originHEAD(t, root)
		configure(t, root, "schema_version = 1\n[branches]\ndev = \"integration\"\n")
		check(t, root, "refs/heads/integration")
	})
	// Git configuration is never read, in any scope.
	t.Run("git config ignored", func(t *testing.T) {
		root := repository(t)
		gittest.Git(t, root, "branch", "wrong")
		originHEAD(t, root)
		gittest.Git(t, root, "config", "fledge.baseBranch", "wrong")
		check(t, root, "refs/remotes/origin/trunk")
	})
	// A default dev that does not exist falls back to origin/HEAD.
	t.Run("default dev missing", func(t *testing.T) {
		root := repository(t)
		originHEAD(t, root)
		configure(t, root, "schema_version = 1\n")
		check(t, root, "refs/remotes/origin/trunk")
	})
	// A fresh clone has origin/<name> but no local branch yet.
	t.Run("configured remote tracking only", func(t *testing.T) {
		root := repository(t)
		gittest.Git(t, root, "update-ref", "refs/remotes/origin/dev", "HEAD")
		originHEAD(t, root)
		configure(t, root, "schema_version = 1\n[branches]\ndev = \"dev\"\n")
		check(t, root, "refs/remotes/origin/dev")
	})
	t.Run("configured local before remote tracking", func(t *testing.T) {
		root := repository(t)
		gittest.Git(t, root, "branch", "dev")
		gittest.Git(t, root, "update-ref", "refs/remotes/origin/dev", "HEAD")
		configure(t, root, "schema_version = 1\n[branches]\ndev = \"dev\"\n")
		check(t, root, "refs/heads/dev")
	})
	// Without a file, dev is the default dev branch.
	t.Run("default dev before origin HEAD", func(t *testing.T) {
		root := repository(t)
		gittest.Git(t, root, "branch", "dev")
		originHEAD(t, root)
		check(t, root, "refs/heads/dev")
	})
	t.Run("default dev remote tracking only", func(t *testing.T) {
		root := repository(t)
		gittest.Git(t, root, "update-ref", "refs/remotes/origin/dev", "HEAD")
		originHEAD(t, root)
		check(t, root, "refs/remotes/origin/dev")
	})
	t.Run("origin HEAD before main", func(t *testing.T) {
		root := repository(t)
		originHEAD(t, root)
		check(t, root, "refs/remotes/origin/trunk")
	})
	// A default dev that does not exist, without origin/HEAD, falls back to main.
	t.Run("main", func(t *testing.T) {
		root := repository(t)
		gittest.Git(t, root, "branch", "other")
		check(t, root, "refs/heads/main")
	})
	t.Run("none", func(t *testing.T) {
		root := t.TempDir()
		gittest.Git(t, root, "init", "-q", "-b", "trunk")
		gittest.Commit(t, root)
		check(t, root, "")
	})
	// A configured branch that does not exist locally or on origin is an
	// error, not a fallback.
	t.Run("configured missing", func(t *testing.T) {
		root := repository(t)
		originHEAD(t, root)
		configure(t, root, "schema_version = 1\n[branches]\ndev = \"missing\"\n")
		got, err := DefaultBranch(ctx, root)
		if got != "" || err == nil || !strings.Contains(err.Error(), ".fledge/config.toml [branches] dev names missing") {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	// A dev that the file names is set even when it is the default name.
	t.Run("configured default name missing", func(t *testing.T) {
		root := repository(t)
		originHEAD(t, root)
		configure(t, root, "schema_version = 1\n[branches]\ndev = \"dev\"\n")
		got, err := DefaultBranch(ctx, root)
		if got != "" || err == nil || !strings.Contains(err.Error(), ".fledge/config.toml [branches] dev names dev") {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	// An invalid file is an error carrying the reader's explanation.
	t.Run("config invalid", func(t *testing.T) {
		root := repository(t)
		originHEAD(t, root)
		configure(t, root, "schema_version = 1\n[branches]\ndev = \"-x\"\n")
		got, err := DefaultBranch(ctx, root)
		if got != "" || err == nil || !strings.Contains(err.Error(), ".fledge/config.toml") || !strings.Contains(err.Error(), "starts with -") {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	// An unreadable file is an error, not a fallback.
	t.Run("config unreadable", func(t *testing.T) {
		root := repository(t)
		originHEAD(t, root)
		os.MkdirAll(filepath.Join(root, ".fledge", "config.toml"), 0o755)
		got, err := DefaultBranch(ctx, root)
		if got != "" || err == nil || !strings.Contains(err.Error(), ".fledge/config.toml") || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("got %q, %v", got, err)
		}
	})
}

func TestMerged(t *testing.T) {
	ctx := context.Background()
	root := repository(t)
	for _, b := range []string{"merged", "unmerged", "squashed"} {
		gittest.Git(t, root, "branch", b)
	}
	gittest.Git(t, root, "switch", "-q", "merged")
	commit(t, root, "merged")
	gittest.Git(t, root, "switch", "-q", "unmerged")
	commit(t, root, "unmerged")
	gittest.Git(t, root, "switch", "-q", "squashed")
	commit(t, root, "squashed")
	gittest.Git(t, root, "switch", "-q", "main")
	gittest.Git(t, root, "merge", "-q", "--no-edit", "merged")
	gittest.Git(t, root, "merge", "-q", "--squash", "squashed")
	gittest.Git(t, root, "commit", "-qm", "squash")
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
