package create

import (
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestCreateRefusesAFeatureWithoutBranch(t *testing.T) {
	repo := identitytest.Repository(t)
	if _, code := created(t, repo, Options{Shape: task.ShapeFeature, ShapeSet: true}); code != "invalid_input" || count(t, repo) != 0 {
		t.Fatal(code)
	}
}

func TestCreateDefaultsAFeatureMember(t *testing.T) {
	repo := identitytest.Repository(t)
	configure(t, repo, "schema_version = 1\ndelivery_mode = \"pr\"\n")
	branch := "feat/x"
	parent := tasktest.Seed(t, repo, task.Record{Title: "p", Status: task.Created, Shape: task.ShapeFeature, Branch: &branch, DeliveryMode: task.ModePR})
	for _, o := range []Options{
		{Parent: parent},
		{Parent: parent, DeliveryMode: task.ModeLocal, DeliveryModeSet: true, BaseBranch: branch, BaseBranchSet: true},
	} {
		r, code := created(t, repo, o)
		if code != "" || r.DeliveryMode != task.ModeLocal || r.BaseBranch == nil || *r.BaseBranch != branch {
			t.Fatalf("%+v: %s %+v", o, code, r)
		}
	}
	if p := tasktest.Load(t, repo, parent); p.DeliveryMode != task.ModePR {
		t.Fatalf("parent: %+v", p)
	}
}

func TestCreateRefusesAnotherModeOrBaseUnderAFeatureParent(t *testing.T) {
	repo := identitytest.Repository(t)
	branch := "feat/x"
	parent := tasktest.Seed(t, repo, task.Record{Title: "p", Status: task.Created, Shape: task.ShapeFeature, Branch: &branch})
	n := count(t, repo)
	for code, o := range map[string]Options{
		"task_feature_member_mode": {Parent: parent, DeliveryMode: task.ModePR, DeliveryModeSet: true},
		"task_feature_member_base": {Parent: parent, BaseBranch: "dev", BaseBranchSet: true},
	} {
		if _, got := created(t, repo, o); got != code || count(t, repo) != n {
			t.Fatalf("%+v: want %s, got %s", o, code, got)
		}
	}
}
