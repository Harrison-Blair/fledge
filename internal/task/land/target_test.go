package land

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// onFeature returns f with branch feature/x made at main and a verified local
// task that lands feat into feature/x. The primary checkout stays on main.
func onFeature(t *testing.T) fixture {
	t.Helper()
	f := setup(t, task.ModeLocal)
	git(t, f.repo, "branch", "feature/x", "main")
	f.id = tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &f.worker.ID, Verifier: &f.boss.ID, VerifiedAt: task.Now(),
		VerifiedCommit: &f.head, DeliveryMode: task.ModeLocal, BaseBranch: tasktest.Ptr("feature/x"), CreatedAt: *task.Now()})
	return f
}

// managed checks out feature/x in the managed worktree of f and returns its path.
func (f fixture) managed(t *testing.T) string {
	t.Helper()
	path := filepath.Join(f.repo, ".fledge", "worktrees", "feature", "x")
	git(t, f.repo, "worktree", "add", "-q", path, "feature/x")
	return path
}

func TestLandLocalMergesInManagedWorktree(t *testing.T) {
	f := onFeature(t)
	path := f.managed(t)
	primary := snapshot(t, f.repo)
	before := git(t, path, "rev-parse", "HEAD")
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	merge := git(t, path, "rev-parse", "HEAD")
	if parents := git(t, path, "rev-list", "--parents", "-n1", merge); parents != merge+" "+before+" "+f.head {
		t.Fatalf("%s", parents)
	}
	if msg := git(t, path, "log", "-1", "--format=%s"); msg != "Merge branch 'feat' (verified task "+f.id+")" {
		t.Fatalf("%q", msg)
	}
	if s := git(t, path, "status", "--porcelain"); s != "" {
		t.Fatalf("%q", s)
	}
	if after := snapshot(t, f.repo); after != primary {
		t.Fatalf("primary changed: %q != %q", after, primary)
	}
	if r := f.load(t); r.Status != task.Landed || *r.MergeCommit != merge || *r.TargetBranch != "feature/x" {
		t.Fatalf("%+v", r)
	}
}

func TestLandRefusesTargetCheckoutMissing(t *testing.T) {
	f := onFeature(t)
	f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_target_checkout_missing")
	// A checkout outside .fledge/worktrees is not managed.
	path := filepath.Join(t.TempDir(), "x")
	git(t, f.repo, "worktree", "add", "-q", path, "feature/x")
	before := git(t, path, "rev-parse", "HEAD")
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	f.refused(t, out, "land_target_checkout_missing")
	if git(t, path, "rev-parse", "HEAD") != before {
		t.Fatal("unmanaged checkout changed")
	}
}

func TestLandRefusesDirtyTargetWorktree(t *testing.T) {
	f := onFeature(t)
	path := f.managed(t)
	writeFile(t, path, "base", "edited\n")
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	f.refused(t, out, "land_target_checkout_dirty")
	if !strings.Contains(out.Error.Message, path) || !strings.Contains(out.Error.Message, "base") {
		t.Fatalf("%s", out.Error.Message)
	}
}

func TestLandAbortsConflictInWorktree(t *testing.T) {
	f := onFeature(t)
	path := f.managed(t)
	commitFile(t, path, "feature", "conflicting\n")
	writeFile(t, path, "notes", "untracked\n")
	before := snapshot(t, path)
	f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_merge_conflict")
	if after := snapshot(t, path); after != before {
		t.Fatalf("%q != %q", after, before)
	}
	if _, err := os.Stat(git(t, path, "rev-parse", "--git-path", "MERGE_HEAD")); !os.IsNotExist(err) {
		t.Fatal("merge left in progress")
	}
	if s := git(t, path, "status", "--porcelain", "--untracked-files=no"); s != "" {
		t.Fatalf("%q", s)
	}
}
