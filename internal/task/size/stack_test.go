package size

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

// stack is a repository on dev with a stack parent and layers l1, l2, and
// l3. Each layer branch adds one 10-line Go file on the branch below.
type stack struct {
	fixture
	parent string
	layers []string
}

func tenLines(name string) string {
	return "package main\n\n" + strings.Repeat("// "+name+"\n", 7) + "var " + name + " = 1\n"
}

// setupStack stores the layers with mod applied to each record.
func setupStack(t *testing.T, mod func(i int, r *task.Record)) stack {
	t.Helper()
	repo := identitytest.Repository(t)
	gittest.Git(t, repo, "symbolic-ref", "HEAD", "refs/heads/dev")
	commit(t, repo, "README.md", "r\n")
	f := stack{fixture: fixture{repo: repo}}
	f.parent = f.seed(t, task.Record{Shape: task.ShapeStack, DeliveryMode: task.ModeNone})
	for i := 1; i <= 3; i++ {
		branch := fmt.Sprintf("l%d", i)
		gittest.Git(t, repo, "checkout", "-qb", branch)
		commit(t, repo, branch+".go", tenLines(branch))
		r := task.Record{Title: branch, Brief: "b", Status: task.Completed, CreatedAt: "2026-01-01T00:00:00Z", Parent: &f.parent, Branch: &branch, DeliveryMode: task.ModeNone}
		if i > 1 {
			r.After = []string{f.layers[i-2]}
		}
		if mod != nil {
			mod(i, &r)
		}
		f.layers = append(f.layers, f.seed(t, r))
	}
	gittest.Git(t, repo, "checkout", "-q", "dev")
	return f
}

func (f stack) sized(t *testing.T, id, target string, lines int) {
	t.Helper()
	out := f.run(t, id)
	r, _ := out.Result.(Result)
	if out.Error != nil || r.Target != target || r.Base != "refs/heads/"+target || r.Size.SourceLines.Value != lines {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}

func TestSizeLayerCountsOnlyItsOwnLines(t *testing.T) {
	f := setupStack(t, nil)
	f.sized(t, f.layers[0], "dev", 10)
	f.sized(t, f.layers[1], "l1", 10)
	f.sized(t, f.layers[2], "l2", 10)
}

func TestSizeStackParentCountsEveryLayer(t *testing.T) {
	f := setupStack(t, nil)
	f.sized(t, f.parent, "dev", 30)
	if r := f.run(t, f.parent).Result.(Result); r.Branch != "l3" {
		t.Fatalf("%+v", r)
	}
}

func TestSizeLayerAboveLandedLayerUsesStackBase(t *testing.T) {
	f := setupStack(t, func(i int, r *task.Record) {
		if i == 1 {
			r.Status = task.Landed
		}
	})
	f.sized(t, f.layers[1], "dev", 20)
}

func TestSizeRefusesUnknownLowerBranch(t *testing.T) {
	f := setupStack(t, func(i int, r *task.Record) {
		if i == 1 {
			r.Branch = nil
		}
	})
	out := f.run(t, f.layers[1])
	if out.Error == nil || out.Error.Code != "size_branch_unknown" || !strings.Contains(out.Error.Message, f.layers[0]) {
		t.Fatalf("%+v", out.Error)
	}
}

func TestSizeRefusesStackFork(t *testing.T) {
	f := setupStack(t, nil)
	f.seed(t, task.Record{Parent: &f.parent, After: []string{f.layers[0]}})
	for _, id := range []string{f.layers[2], f.parent} {
		if out := f.run(t, id); out.Error == nil || out.Error.Code != "size_stack_fork" {
			t.Fatalf("%s: %+v", id, out.Error)
		}
	}
}
