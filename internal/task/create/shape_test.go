package create

import (
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func TestCreateStoresShapeAndBranch(t *testing.T) {
	repo := identitytest.Repository(t)
	if r, code := created(t, repo, Options{}); code != "" || r.Shape != "" || r.Branch != nil {
		t.Fatalf("%s %+v", code, r)
	}
	r, code := created(t, repo, Options{Shape: task.ShapeStack, ShapeSet: true, Branch: "stack/01-a", BranchSet: true})
	if code != "" || r.Shape != task.ShapeStack || r.Branch == nil || *r.Branch != "stack/01-a" {
		t.Fatalf("%s %+v", code, r)
	}
	if r, code := created(t, repo, Options{Shape: task.ShapeFeature, ShapeSet: true, Branch: "feat/x", BranchSet: true}); code != "" || r.Shape != task.ShapeFeature {
		t.Fatalf("%s %+v", code, r)
	}
}

func TestCreateRejectsInvalidShapeAndBranch(t *testing.T) {
	repo := identitytest.Repository(t)
	for _, o := range []Options{
		{Shape: "tower", ShapeSet: true},
		{Shape: "", ShapeSet: true},
		{Branch: "a..b", BranchSet: true},
		{Branch: "", BranchSet: true},
		{Branch: "-dev", BranchSet: true},
	} {
		if _, code := created(t, repo, o); code != "invalid_input" {
			t.Fatalf("%+v: %s", o, code)
		}
	}
}
