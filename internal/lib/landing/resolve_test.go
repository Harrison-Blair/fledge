package landing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// seedStack stores a stack parent with base and one layer for each of
// layers, each after the layer before it, and returns the parent id and the
// layer ids bottom first.
func seedStack(t *testing.T, repo string, base *string, layers ...task.Record) (string, []string) {
	t.Helper()
	parent := tasktest.Seed(t, repo, task.Record{Title: "p", Status: task.Created, Shape: task.ShapeStack, BaseBranch: base})
	var ids []string
	for i, l := range layers {
		l.Title, l.Parent = "l", &parent
		if l.Status == "" {
			l.Status = task.Created
		}
		if i > 0 {
			l.After = []string{ids[i-1]}
		}
		ids = append(ids, tasktest.Seed(t, repo, l))
	}
	return parent, ids
}

func TestLayerBase(t *testing.T) {
	ctx := context.Background()
	repo, _ := resolveRepo(t)
	l1, l2 := tasktest.Ptr("l1"), tasktest.Ptr("l2")
	_, plain := seedStack(t, repo, nil, task.Record{Branch: l1}, task.Record{Branch: l2}, task.Record{})
	_, based := seedStack(t, repo, tasktest.Ptr("dev"), task.Record{Branch: l1}, task.Record{Branch: l2})
	_, landed := seedStack(t, repo, nil, task.Record{Branch: l1, Status: task.Landed}, task.Record{Branch: l2})
	loose := tasktest.Seed(t, repo, task.Record{Title: "t", Status: task.Created, BaseBranch: tasktest.Ptr("other")})
	feature := tasktest.Seed(t, repo, task.Record{Title: "f", Status: task.Created, Shape: task.ShapeFeature})
	child := tasktest.Seed(t, repo, task.Record{Title: "c", Status: task.Created, Parent: &feature})
	s := tasktest.Store(t, repo)
	for _, c := range []struct {
		name, id, want string
	}{
		{"bottom, no parent base: the integration branch", plain[0], "main"},
		{"middle", plain[1], "l1"},
		{"top", plain[2], "l2"},
		{"bottom, parent base", based[0], "dev"},
		{"above a landed layer: the stack base", landed[1], "main"},
		{"no parent: TaskTarget", loose, "other"},
		{"feature parent: TaskTarget", child, "main"},
	} {
		if got, err := LayerBase(ctx, s, repo, tasktest.Load(t, repo, c.id), "size"); err != nil || got != c.want {
			t.Fatalf("%s: %q %v", c.name, got, err)
		}
	}
	// The [branches] dev branch is the stack base when the parent has none.
	gittest.Git(t, repo, "branch", "trunk", "main")
	if err := os.WriteFile(filepath.Join(repo, ".fledge", "config.toml"), []byte("schema_version = 1\n\n[branches]\ndev = \"trunk\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := LayerBase(ctx, s, repo, tasktest.Load(t, repo, plain[0]), "size"); err != nil || got != "trunk" {
		t.Fatalf("dev branch: %q %v", got, err)
	}
	_, unknown := seedStack(t, repo, nil, task.Record{}, task.Record{Branch: l2})
	parent, fork := seedStack(t, repo, nil, task.Record{Branch: l1}, task.Record{Branch: l2})
	tasktest.Seed(t, repo, task.Record{Title: "x", Status: task.Created, Parent: &parent, After: []string{fork[0]}})
	for _, prefix := range []string{"land", "size"} {
		_, err := LayerBase(ctx, s, repo, tasktest.Load(t, repo, unknown[1]), prefix)
		if code(err) != prefix+"_branch_unknown" || !strings.Contains(err.Error(), unknown[0]) {
			t.Fatalf("%s: %v", prefix, err)
		}
		if _, err := LayerBase(ctx, s, repo, tasktest.Load(t, repo, fork[1]), prefix); code(err) != prefix+"_stack_fork" {
			t.Fatalf("%s: %v", prefix, err)
		}
	}
}

func TestTaskTargetMissingNamesDevBranch(t *testing.T) {
	_, err := TaskTarget(context.Background(), gittest.Repository(t), task.Record{}, "size")
	if err == nil || !strings.Contains(err.Error(), "set [branches] dev in .fledge/config.toml") {
		t.Fatalf("%v", err)
	}
}
