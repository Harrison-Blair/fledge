package land

import (
	"context"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/landing"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// rebased stores the patch id of the verified head, then moves main one
// commit ahead and rebases feat onto it. edit, when set, then changes the
// rebased commit. It returns the new head of feat.
func (f fixture) rebased(t *testing.T, edit string) string {
	t.Helper()
	ctx := context.Background()
	id, err := landing.PatchID(ctx, f.repo, "refs/heads/main", f.head)
	if err != nil {
		t.Fatal(err)
	}
	s, err := task.Existing(ctx, f.repo)
	if err == nil {
		_, err = task.Update(s, f.id, func(r *task.Record) error { r.VerifiedPatchID = &id; return nil })
	}
	if err != nil {
		t.Fatal(err)
	}
	commitFile(t, f.repo, "unrelated", "u\n")
	git(t, f.repo, "checkout", "-q", "feat")
	git(t, f.repo, "rebase", "-q", "main")
	if edit != "" {
		writeFile(t, f.repo, "feature", edit)
		git(t, f.repo, "commit", "-q", "--amend", "-a", "--no-edit")
	}
	head := git(t, f.repo, "rev-parse", "HEAD")
	git(t, f.repo, "checkout", "-q", "main")
	return head
}

func TestLandCarriesHeadWithEqualPatchID(t *testing.T) {
	f := setup(t, task.ModeLocal)
	head := f.rebased(t, "")
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	r := f.load(t)
	if out.Error != nil || r.Status != task.Landed || *r.VerifiedCommit != head || r.CarriedFrom == nil || *r.CarriedFrom != f.head {
		t.Fatalf("%+v %+v", out.Error, r)
	}
	if parents := git(t, f.repo, "rev-list", "--parents", "-n1", *r.MergeCommit); parents[len(parents)-40:] != head {
		t.Fatalf("%s", parents)
	}
}

func TestLandRefusesHeadWithChangedLine(t *testing.T) {
	f := setup(t, task.ModeLocal)
	f.rebased(t, "changed\n")
	// Store the branch first, so the refusal is the only land run.
	f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_head_mismatch")
	before := f.load(t)
	out := f.land(t, Options{})
	f.refused(t, out, "land_head_mismatch")
	if !reflect.DeepEqual(f.load(t), before) {
		t.Fatalf("record changed: %+v", f.load(t))
	}
}
