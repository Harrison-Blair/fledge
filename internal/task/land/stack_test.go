package land

import (
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// setupLayer is the fixture with a stack parent of two layers: lower, on
// branch low (main and one file), and the task, verified on branch up (low
// and one file). lower is stored as mod leaves it.
func setupLayer(t *testing.T, mod func(lower *task.Record)) (f fixture, parent, lower string) {
	t.Helper()
	f = setup(t, task.ModeLocal)
	git(t, f.repo, "checkout", "-qb", "low", "main")
	commitFile(t, f.repo, "lower", "lower\n")
	git(t, f.repo, "checkout", "-qb", "up")
	f.head = commitFile(t, f.repo, "upper", "upper\n")
	git(t, f.repo, "checkout", "-q", "main")
	parent = tasktest.Seed(t, f.repo, task.Record{Title: "p", Brief: "b", Status: task.Created, Shape: task.ShapeStack, CreatedAt: *task.Now()})
	r := task.Record{Title: "lower", Brief: "b", Status: task.Verified, Parent: &parent, Branch: tasktest.Ptr("low"), CreatedAt: *task.Now()}
	if mod != nil {
		mod(&r)
	}
	lower = tasktest.Seed(t, f.repo, r)
	f.id = tasktest.Seed(t, f.repo, task.Record{Title: "upper", Brief: "b", Status: task.Verified, Owner: &f.worker.ID, Verifier: &f.boss.ID,
		VerifiedAt: task.Now(), VerifiedCommit: &f.head, DeliveryMode: task.ModeLocal, Parent: &parent, After: []string{lower},
		Branch: tasktest.Ptr("up"), CreatedAt: *task.Now()})
	return f, parent, lower
}

func TestLandLayerSizeUsesLayerBase(t *testing.T) {
	f, _, _ := setupLayer(t, nil)
	out := f.land(t, Options{})
	res, _ := out.Result.(Result)
	if out.Error != nil || res.Size == nil || res.base != "refs/heads/low" || res.Size.Files.Value != 1 {
		t.Fatalf("%+v %+v", out.Error, res)
	}
}

func TestLandRefusesLayerResolverRefusals(t *testing.T) {
	f, _, lower := setupLayer(t, func(r *task.Record) { r.Branch = nil })
	f.refused(t, f.land(t, Options{}), "land_branch_unknown")
	if out := f.land(t, Options{}); !strings.Contains(out.Error.Message, lower) {
		t.Fatalf("%+v", out.Error)
	}
	f, parent, lower := setupLayer(t, nil)
	tasktest.Seed(t, f.repo, task.Record{Title: "x", Brief: "b", Status: task.Created, Parent: &parent, After: []string{lower}, CreatedAt: *task.Now()})
	f.refused(t, f.land(t, Options{}), "land_stack_fork")
}

// TestLandLayerWithoutSizeStillLands: a missing base ref or a failed diff
// leaves the size unset and does not refuse.
func TestLandLayerWithoutSizeStillLands(t *testing.T) {
	missing, _, _ := setupLayer(t, func(r *task.Record) { r.Branch = tasktest.Ptr("gone") })
	failed, _, _ := setupLayer(t, func(r *task.Record) { r.Branch = tasktest.Ptr("orphan") })
	git(t, failed.repo, "checkout", "-q", "--orphan", "orphan")
	commitFile(t, failed.repo, "other", "other\n")
	git(t, failed.repo, "checkout", "-q", "main")
	for name, f := range map[string]fixture{"missing": missing, "failed": failed} {
		out := f.land(t, Options{})
		if res, _ := out.Result.(Result); out.Error != nil || res.Size != nil || f.load(t).Status != task.Landed {
			t.Fatalf("%s: %+v %+v", name, out.Error, res.Size)
		}
	}
}

// TestLandLayerWithoutStackBaseStillLands: when the parent has no stack base,
// the size stays unset and the landing does not refuse.
func TestLandLayerWithoutStackBaseStillLands(t *testing.T) {
	f, _, _ := setupLayer(t, func(r *task.Record) { r.Status = task.Landed })
	git(t, f.repo, "branch", "-m", "main", "trunk")
	if _, err := task.Update(tasktest.Store(t, f.repo), f.id, func(r *task.Record) error {
		r.BaseBranch = tasktest.Ptr("trunk")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out := f.land(t, Options{})
	if res, _ := out.Result.(Result); out.Error != nil || res.Size != nil || f.load(t).Status != task.Landed {
		t.Fatalf("%+v %+v", out.Error, res.Size)
	}
}
