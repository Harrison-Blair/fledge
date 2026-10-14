package create

import (
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestCreateAddsAStackLayerOnlyOnTop(t *testing.T) {
	repo := identitytest.Repository(t)
	parent := tasktest.Seed(t, repo, task.Record{Title: "p", Status: task.Created, Shape: task.ShapeStack})
	outside := tasktest.Seed(t, repo, task.Record{Title: "x", Status: task.Created})
	a, code := created(t, repo, Options{Parent: parent, After: []string{outside}})
	if code != "" {
		t.Fatal(code)
	}
	b := tasktest.Seed(t, repo, task.Record{Title: "b", Status: task.Created, Parent: &parent, After: []string{a.ID}})
	c := tasktest.Seed(t, repo, task.Record{Title: "c", Status: task.Cancelled, Parent: &parent, After: []string{b}})
	for _, after := range [][]string{{b}, {}, {outside}, {c, b}, {c, a.ID}} {
		n := count(t, repo)
		if _, code := created(t, repo, Options{Parent: parent, After: after}); code != "task_stack_fork" || count(t, repo) != n {
			t.Fatalf("%v: %s", after, code)
		}
	}
	if _, code := created(t, repo, Options{Parent: parent, After: []string{c, outside}}); code != "" {
		t.Fatal(code)
	}
}

func TestCreateUnderAFeatureParentAcceptsAnyOrder(t *testing.T) {
	repo := identitytest.Repository(t)
	for _, shape := range []string{task.ShapeFeature, ""} {
		parent := tasktest.Seed(t, repo, task.Record{Title: "p", Status: task.Created, Shape: shape})
		a := tasktest.Seed(t, repo, task.Record{Title: "a", Status: task.Created, Parent: &parent})
		tasktest.Seed(t, repo, task.Record{Title: "b", Status: task.Created, Parent: &parent, After: []string{a}})
		for _, after := range [][]string{{a}, {}} {
			if _, code := created(t, repo, Options{Parent: parent, After: after}); code != "" {
				t.Fatalf("%q %v: %s", shape, after, code)
			}
		}
	}
}

func TestCreateRefusesUnderAForkedStack(t *testing.T) {
	repo := identitytest.Repository(t)
	parent := tasktest.Seed(t, repo, task.Record{Title: "p", Status: task.Created, Shape: task.ShapeStack})
	a := tasktest.Seed(t, repo, task.Record{Title: "a", Status: task.Created, Parent: &parent})
	b := tasktest.Seed(t, repo, task.Record{Title: "b", Status: task.Created, Parent: &parent, After: []string{a}})
	tasktest.Seed(t, repo, task.Record{Title: "c", Status: task.Created, Parent: &parent, After: []string{a}})
	if _, code := created(t, repo, Options{Parent: parent, After: []string{b}}); code != "task_stack_fork" {
		t.Fatal(code)
	}
}
