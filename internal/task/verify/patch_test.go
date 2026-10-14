package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/landing"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
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

// branched is a repository on main with branch feat checked out one commit
// ahead, and a completed task the worker owns, with mod applied.
func branched(t *testing.T, mod func(*task.Record)) (repo, id, head string) {
	t.Helper()
	repo = identitytest.Repository(t)
	gittest.Git(t, repo, "symbolic-ref", "HEAD", "refs/heads/main")
	commitFile(t, repo, "README.md", "r\n")
	gittest.Git(t, repo, "checkout", "-qb", "feat")
	head = commitFile(t, repo, "a.go", "package main\n")
	owner := tasktest.Register(t, repo, worker)
	r := task.Record{Title: "t", Status: task.Completed, Owner: &owner.ID, Result: tasktest.Ptr("done")}
	if mod != nil {
		mod(&r)
	}
	id = tasktest.Seed(t, repo, r)
	tasktest.Register(t, repo, boss)
	return repo, id, head
}

func TestCommitStoresPatchID(t *testing.T) {
	repo, id, head := branched(t, nil)
	want, err := landing.PatchID(context.Background(), repo, "refs/heads/main", head)
	if err != nil {
		t.Fatal(err)
	}
	r, text, code := verifyCommit(t, repo, id, "HEAD")
	if code != nil || r.VerifiedPatchID == nil || !landing.FullHash(*r.VerifiedPatchID) || *r.VerifiedPatchID != want || strings.Contains(text, "Warning") {
		t.Fatalf("%v %+v %q", code, r, text)
	}
	// A verification without --commit stores a null patch id.
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{ID: id}, strings.NewReader(""))
	if r := tasktest.Load(t, repo, id); out.Error != nil || r.VerifiedPatchID != nil {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}

// For a stack layer the patch id base is the branch of the layer below.
func TestCommitPatchIDUsesLayerBase(t *testing.T) {
	repo, parent, l1 := branched(t, func(r *task.Record) { r.Shape = task.ShapeStack })
	below := tasktest.Seed(t, repo, task.Record{Title: "l1", Status: task.Verified, Parent: &parent, Branch: tasktest.Ptr("feat")})
	gittest.Git(t, repo, "checkout", "-qb", "l2")
	head := commitFile(t, repo, "b.go", "package main\n")
	id := tasktest.Seed(t, repo, task.Record{Title: "l2", Status: task.Completed, Parent: &parent, After: []string{below}, Branch: tasktest.Ptr("l2"), Result: tasktest.Ptr("done")})
	want, err := landing.PatchID(context.Background(), repo, "refs/heads/feat", head)
	if err != nil || l1 == head {
		t.Fatal(err)
	}
	r, text, code := verifyCommit(t, repo, id, "HEAD")
	if code != nil || r.VerifiedPatchID == nil || *r.VerifiedPatchID != want || strings.Contains(text, "Warning") {
		t.Fatalf("%v %+v %q", code, r, text)
	}
}

// A missing base or an empty diff stores a null patch id with a warning,
// and the verification still succeeds.
func TestCommitWarnsWithoutPatchID(t *testing.T) {
	repo, id, _ := branched(t, func(r *task.Record) { r.BaseBranch = tasktest.Ptr("nosuch") })
	r, text, code := verifyCommit(t, repo, id, "HEAD")
	if code != nil || r.Status != task.Verified || r.VerifiedCommit == nil || r.VerifiedPatchID != nil || !strings.Contains(text, "Warning: no patch id is stored") {
		t.Fatalf("%v %+v %q", code, r, text)
	}
	repo, id, _ = branched(t, nil)
	r, text, code = verifyCommit(t, repo, id, "main")
	if code != nil || r.VerifiedCommit == nil || r.VerifiedPatchID != nil || !strings.Contains(text, "Warning: no patch id is stored") || !strings.Contains(text, "empty") {
		t.Fatalf("%v %+v %q", code, r, text)
	}
}

// A new verification clears carried_from, with or without --commit.
func TestVerifyAgainClearsCarriedFrom(t *testing.T) {
	carried := func(r *task.Record) {
		r.Status, r.CarriedFrom, r.VerifiedPatchID = task.Verified, tasktest.Ptr(strings.Repeat("a", 40)), tasktest.Ptr(strings.Repeat("b", 40))
	}
	repo, id, head := branched(t, carried)
	if r, _, code := verifyCommit(t, repo, id, "HEAD"); code != nil || r.CarriedFrom != nil || *r.VerifiedCommit != head {
		t.Fatalf("%v %+v", code, r)
	}
	repo, id, _ = branched(t, carried)
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{ID: id}, strings.NewReader(""))
	if r := tasktest.Load(t, repo, id); out.Error != nil || r.CarriedFrom != nil || r.VerifiedPatchID != nil {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}
