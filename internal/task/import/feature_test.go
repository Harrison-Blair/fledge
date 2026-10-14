package taskimport

import (
	"context"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// featured is a proposal whose [parent] has shape feature, delivery_mode pr,
// and the lines, and whose tasks are tasks.
func featured(lines string, tasks ...string) string {
	return "schema_version = 1\n" + strings.Replace(parentTOML, "title = \"Templates\"\n", "title = \"Templates\"\nshape = \"feature\"\ndelivery_mode = \"pr\"\n"+lines, 1) + tasktest.Brief() + "'''\n" + strings.Join(tasks, "")
}

func TestImportDefaultsFeatureMembers(t *testing.T) {
	repo := identitytest.Repository(t)
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: write(t, featured("branch = \"feat/x\"\n", taskTOML("a", "A"))), FileSet: true}, strings.NewReader(""))
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	r := out.Result.(Result)
	p, a := tasktest.Load(t, repo, *r.Parent), tasktest.Load(t, repo, *r.Tasks[0].ID)
	if p.DeliveryMode != task.ModePR || p.BaseBranch != nil {
		t.Fatalf("parent: %+v", p)
	}
	if a.DeliveryMode != task.ModeLocal || a.BaseBranch == nil || *a.BaseBranch != "feat/x" {
		t.Fatalf("a: %+v", a)
	}
}

func TestImportRefusesAFeatureParentWithoutBranchOrAConflictingMember(t *testing.T) {
	repo := identitytest.Repository(t)
	for body, want := range map[string]string{
		featured("", taskTOML("a", "A")): "[parent]: shape feature needs a branch",
		featured("branch = \"feat/x\"\n", withKeys(taskTOML("a", "A"), "a", "delivery_mode = \"pr\"\n")): `task "a": a member of feature branch "feat/x" needs delivery_mode local, not "pr"`,
		featured("branch = \"feat/x\"\n", withKeys(taskTOML("a", "A"), "a", "base_branch = \"dev\"\n")):  `task "a": a member of feature branch "feat/x" needs base_branch "feat/x", not "dev"`,
	} {
		for _, dry := range []bool{true, false} {
			out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: write(t, body), FileSet: true, DryRun: dry}, strings.NewReader(""))
			if out.Error == nil || out.Error.Code != "invalid_input" || out.Error.Message != want || out.ExitCode() != 2 {
				t.Fatalf("dry=%v: want %q, got %+v", dry, want, out.Error)
			}
		}
	}
	if count(t, repo) != 0 {
		t.Fatal("created tasks")
	}
}

func TestImportUnderAFeatureParentDefaultsItsMembers(t *testing.T) {
	repo := identitytest.Repository(t)
	branch := "feat/x"
	parent := tasktest.Seed(t, repo, task.Record{Title: "p", Status: task.Created, Shape: task.ShapeFeature, Branch: &branch, DeliveryMode: task.ModePR})
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: write(t, "schema_version = 1\n"+taskTOML("a", "A")), FileSet: true, Parent: parent}, strings.NewReader(""))
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	a := tasktest.Load(t, repo, *out.Result.(Result).Tasks[0].ID)
	if a.DeliveryMode != task.ModeLocal || a.BaseBranch == nil || *a.BaseBranch != branch {
		t.Fatalf("a: %+v", a)
	}
	if p := tasktest.Load(t, repo, parent); p.DeliveryMode != task.ModePR {
		t.Fatalf("parent: %+v", p)
	}
	n := count(t, repo)
	for _, dry := range []bool{true, false} {
		body := "schema_version = 1\n" + withKeys(taskTOML("b", "B"), "b", "base_branch = \"dev\"\n")
		out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: write(t, body), FileSet: true, Parent: parent, DryRun: dry}, strings.NewReader(""))
		want := `task "b": a member of feature branch "feat/x" needs base_branch "feat/x", not "dev"`
		if out.Error == nil || out.Error.Code != "invalid_input" || out.Error.Message != want || count(t, repo) != n {
			t.Fatalf("dry=%v: %+v", dry, out.Error)
		}
	}
}
