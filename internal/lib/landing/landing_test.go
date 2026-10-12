package landing

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
)

func TestShortBranch(t *testing.T) {
	for in, want := range map[string]string{"refs/heads/dev": "dev", "refs/remotes/origin/a/b": "a/b", "dev": "dev", "refs/remotes/upstream/dev": "refs/remotes/upstream/dev"} {
		if got := ShortBranch(in); got != want {
			t.Fatalf("%s: %s", in, got)
		}
	}
}

func TestValidBranchAndMode(t *testing.T) {
	ctx := context.Background()
	for _, ok := range []string{"dev", "feat/x", "release-1.2"} {
		if err := ValidBranch(ctx, "--branch", ok); err != nil {
			t.Fatalf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", " ", "-dev", "a\nb", "a..b", "a b", "@{-1}"} {
		var input *cli.InputError
		if err := ValidBranch(ctx, "--branch", bad); err == nil || !asInput(err, &input) {
			t.Fatalf("%q: %v", bad, err)
		}
	}
	// A target is checked after it is reduced to a short name.
	for _, ok := range []string{"dev", "refs/heads/dev", "refs/remotes/origin/a/b"} {
		if err := ValidTarget(ctx, "--base-branch", ok); err != nil {
			t.Fatalf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"-dev", "refs/heads/-dev", "refs/remotes/origin/-dev", "refs/remotes/origin/a..b"} {
		var input *cli.InputError
		if err := ValidTarget(ctx, "--base-branch", bad); err == nil || !asInput(err, &input) {
			t.Fatalf("%q: %v", bad, err)
		}
	}
	if ValidMode("--mode", "pr") != nil || ValidMode("--mode", "") == nil || ValidMode("--mode", "merge") == nil {
		t.Fatal("mode")
	}
}

func asInput(err error, target **cli.InputError) bool {
	e, ok := err.(*cli.InputError)
	*target = e
	return ok
}

func TestResolveCommitAndChanges(t *testing.T) {
	ctx := context.Background()
	repo := gittest.Repository(t)
	gittest.Commit(t, repo)
	head := strings.TrimSpace(gittest.Git(t, repo, "rev-parse", "HEAD"))
	if got, err := ResolveCommit(ctx, repo, "HEAD"); got != head || err != nil || !FullHash(got) {
		t.Fatalf("%s %v", got, err)
	}
	for _, bad := range []string{"", "--all", "nosuch"} {
		if _, err := ResolveCommit(ctx, repo, bad); err == nil {
			t.Fatalf("%q resolved", bad)
		}
	}
	if RefCommit(ctx, repo, "refs/heads/nosuch") != "" {
		t.Fatal("missing ref resolved")
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a", "a")
	write("b", "b")
	gittest.Git(t, repo, "add", "a", "b")
	gittest.Git(t, repo, "commit", "-qm", "ab")
	write("untracked", "x")
	if got, err := Changes(ctx, repo); len(got) != 0 || err != nil {
		t.Fatalf("%v %v", got, err)
	}
	gittest.Git(t, repo, "mv", "a", "renamed")
	write("b", "changed")
	if got, err := Changes(ctx, repo); !slices.Equal(got, []string{"b", "renamed"}) || err != nil {
		t.Fatalf("%q %v", got, err)
	}
	paths, err := Checkouts(ctx, repo, strings.TrimSpace(gittest.Git(t, repo, "branch", "--show-current")))
	if err != nil || len(paths) != 1 || paths[0] != repo {
		t.Fatalf("%v %v", paths, err)
	}
}

func TestLockSerializes(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".fledge", "state"), 0o755)
	unlock, err := Lock(root)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan struct{})
	go func() {
		second, err := Lock(root)
		if err == nil {
			second()
		}
		close(acquired)
	}()
	select {
	case <-acquired:
		t.Fatal("second lock acquired while the first was held")
	case <-time.After(200 * time.Millisecond):
	}
	unlock()
	<-acquired
}
