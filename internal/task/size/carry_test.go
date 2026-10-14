package size

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/landing"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// carried is a verified local task on branch carry, one commit ahead of main
// that adds a.go with a Go string. Its patch id is stored. Then main moves
// one commit ahead, and carry is rebased onto it. edit, when set, then
// replaces a.go in the rebased commit. It returns the task, the verified
// commit, and the new head.
func carried(t *testing.T, edit string) (f fixture, id, old, head string) {
	t.Helper()
	f = setup(t)
	gittest.Git(t, f.repo, "checkout", "-qb", "carry")
	old = commit(t, f.repo, "a.go", "package main\n\nvar a = \"x y\"\n")
	gittest.Git(t, f.repo, "checkout", "-q", "main")
	patch, err := landing.PatchID(context.Background(), f.repo, "refs/heads/main", old)
	if err != nil {
		t.Fatal(err)
	}
	id = f.seed(t, task.Record{Title: "t", Brief: "b", Status: task.Verified, CreatedAt: "2026-01-01T00:00:00Z", DeliveryMode: task.ModeLocal,
		Branch: tasktest.Ptr("carry"), VerifiedCommit: &old, VerifiedPatchID: &patch})
	commit(t, f.repo, "unrelated.go", "package main\n")
	gittest.Git(t, f.repo, "checkout", "-q", "carry")
	gittest.Git(t, f.repo, "rebase", "-q", "main")
	if edit != "" {
		commit(t, f.repo, "a.go", edit)
		gittest.Git(t, f.repo, "reset", "-q", "--soft", "HEAD~2")
		gittest.Git(t, f.repo, "commit", "-qm", "a.go")
		gittest.Git(t, f.repo, "rebase", "-q", "main")
	}
	head = strings.TrimSpace(gittest.Git(t, f.repo, "rev-parse", "HEAD"))
	gittest.Git(t, f.repo, "checkout", "-q", "main")
	return f, id, old, head
}

func TestSizeCarriesHeadWithEqualPatchID(t *testing.T) {
	f, id, old, head := carried(t, "")
	out := f.run(t, id)
	res, _ := out.Result.(Result)
	if out.Error != nil || res.Head != head || res.Size.Files.Value != 1 {
		t.Fatalf("%+v %+v", out.Error, res)
	}
	if r := tasktest.Load(t, f.repo, id); *r.VerifiedCommit != head || r.CarriedFrom == nil || *r.CarriedFrom != old {
		t.Fatalf("%+v", r)
	}
}

// --verbatim keeps whitespace, so a change only in whitespace inside a Go
// string is a mismatch like a changed line.
func TestSizeRefusesHeadWithOtherPatchID(t *testing.T) {
	for _, edit := range []string{"package main\n\nvar a = \"x z\"\n", "package main\n\nvar a = \"x  y\"\n"} {
		f, id, _, _ := carried(t, edit)
		before := tasktest.Load(t, f.repo, id)
		if out := f.run(t, id); out.Error == nil || out.Error.Code != "land_head_mismatch" {
			t.Fatalf("%q: %+v", edit, out.Error)
		}
		if !reflect.DeepEqual(tasktest.Load(t, f.repo, id), before) {
			t.Fatalf("%q: record changed", edit)
		}
	}
}

// Without the task branch, size measures the verified commit.
func TestSizeWithoutBranchMeasuresVerifiedCommit(t *testing.T) {
	f, id, old, _ := carried(t, "")
	gittest.Git(t, f.repo, "branch", "-qD", "carry")
	out := f.run(t, id)
	if res, _ := out.Result.(Result); out.Error != nil || res.Head != old {
		t.Fatalf("%+v %+v", out.Error, res)
	}
	if r := tasktest.Load(t, f.repo, id); *r.VerifiedCommit != old || r.CarriedFrom != nil {
		t.Fatalf("%+v", r)
	}
}
