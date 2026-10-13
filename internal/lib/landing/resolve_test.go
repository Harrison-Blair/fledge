package landing

import (
	"context"
	"path/filepath"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// resolveRepo is a repository on main with branch feat checked out in a
// linked checkout at feat.
func resolveRepo(t *testing.T) (repo, feat string) {
	t.Helper()
	repo = gittest.Repository(t)
	gittest.Git(t, repo, "symbolic-ref", "HEAD", "refs/heads/main")
	gittest.Commit(t, repo)
	feat = filepath.Join(t.TempDir(), "feat")
	gittest.Git(t, repo, "worktree", "add", "-q", "-b", "feat", feat)
	return repo, feat
}

func code(err error) string {
	c, _ := cli.Coded(err)
	return c
}

func TestTaskBranch(t *testing.T) {
	ctx := context.Background()
	repo, feat := resolveRepo(t)
	tasktest.Seed(t, repo, task.Record{Title: "t", Status: task.Created})
	s := tasktest.Store(t, repo)
	register := func(pane string, c *identity.Checkout) *string {
		r, err := identity.Register(ctx, s, libagent.Client{}, tasktest.Agent(pane, "term_"+pane, "a"+pane).Agent, "spawn", c, nil)
		if err != nil {
			t.Fatal(err)
		}
		return &r.ID
	}
	stored, spawned := "stored", "spawned"
	withPath := register("w1:p1", &identity.Checkout{Path: feat})
	gone := register("w1:p2", &identity.Checkout{Path: filepath.Join(t.TempDir(), "missing"), Branch: &spawned, Created: true})
	bare := register("w1:p3", nil)
	for _, c := range []struct {
		r    task.Record
		want string
	}{
		{task.Record{Branch: &stored, Owner: withPath}, "stored"},
		{task.Record{Owner: withPath}, "feat"},
		{task.Record{Owner: gone}, "spawned"},
	} {
		if got, err := TaskBranch(ctx, s, c.r, "size"); err != nil || got != c.want {
			t.Fatalf("%+v: %q %v", c.r, got, err)
		}
	}
	for _, prefix := range []string{"land", "size"} {
		for _, r := range []task.Record{{}, {Owner: bare}, {Owner: tasktest.Ptr("0123abcd")}} {
			if _, err := TaskBranch(ctx, s, r, prefix); code(err) != prefix+"_branch_unknown" {
				t.Fatalf("%s %+v: %v", prefix, r, err)
			}
		}
	}
}

func TestTaskTarget(t *testing.T) {
	ctx := context.Background()
	repo, _ := resolveRepo(t)
	for base, want := range map[string]string{"dev": "dev", "refs/heads/dev": "dev", "refs/remotes/origin/a/b": "a/b"} {
		if got, err := TaskTarget(ctx, repo, task.Record{BaseBranch: &base}, "size"); err != nil || got != want {
			t.Fatalf("%s: %q %v", base, got, err)
		}
	}
	// Without a base branch, the integration branch: here main.
	if got, err := TaskTarget(ctx, repo, task.Record{}, "size"); err != nil || got != "main" {
		t.Fatalf("%q %v", got, err)
	}
	bad := "-dev"
	empty := gittest.Repository(t)
	for _, prefix := range []string{"land", "size"} {
		if _, err := TaskTarget(ctx, repo, task.Record{BaseBranch: &bad}, prefix); code(err) != prefix+"_target_invalid" {
			t.Fatalf("%s: %v", prefix, err)
		}
		if _, err := TaskTarget(ctx, empty, task.Record{}, prefix); code(err) != prefix+"_target_missing" {
			t.Fatalf("%s: %v", prefix, err)
		}
	}
}

func TestBaseRef(t *testing.T) {
	ctx := context.Background()
	repo, _ := resolveRepo(t)
	gittest.Git(t, repo, "update-ref", "refs/remotes/origin/main", "main")
	gittest.Git(t, repo, "update-ref", "refs/remotes/origin/only", "main")
	gittest.Git(t, repo, "branch", "local", "main")
	for _, c := range []struct{ mode, target, ref, missing string }{
		{task.ModeLocal, "main", "refs/heads/main", ""},
		{task.ModeNone, "main", "refs/heads/main", ""},
		{task.ModePR, "main", "refs/remotes/origin/main", ""},
		{task.ModeAuto, "main", "refs/remotes/origin/main", ""},
		{task.ModeLocal, "only", "refs/remotes/origin/only", "refs/heads/only"},
		{task.ModePR, "local", "refs/heads/local", "refs/remotes/origin/local"},
	} {
		ref, missing, err := BaseRef(ctx, repo, c.mode, c.target, "size")
		if err != nil || ref != c.ref || missing != c.missing {
			t.Fatalf("%+v: %q %q %v", c, ref, missing, err)
		}
	}
	for _, prefix := range []string{"land", "size"} {
		if _, _, err := BaseRef(ctx, repo, task.ModePR, "nope", prefix); code(err) != prefix+"_target_missing" {
			t.Fatalf("%s: %v", prefix, err)
		}
	}
}
