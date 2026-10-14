package taskimport

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestImportStoresParentKeysAndTaskBranch(t *testing.T) {
	repo := identitytest.Repository(t)
	os.MkdirAll(filepath.Join(repo, ".fledge"), 0o755)
	if err := os.WriteFile(filepath.Join(repo, ".fledge", "config.toml"), []byte("schema_version = 1\ndelivery_mode = \"none\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	parent := strings.Replace(parentTOML, "title = \"Templates\"\n", "title = \"Templates\"\nshape = \"stack\"\nbranch = \"stack/00\"\nbase_branch = \"dev\"\ndelivery_mode = \"pr\"\n", 1)
	path := write(t, "schema_version = 1\n"+parent+tasktest.Brief()+"'''\n"+withKeys(taskTOML("a", "A"), "a", "branch = \"stack/01-a\"\n")+taskTOML("b", "B"))
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: path, FileSet: true}, strings.NewReader(""))
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	r := out.Result.(Result)
	p, a, b := tasktest.Load(t, repo, *r.Parent), tasktest.Load(t, repo, *r.Tasks[0].ID), tasktest.Load(t, repo, *r.Tasks[1].ID)
	if p.Shape != task.ShapeStack || p.DeliveryMode != task.ModePR || p.Branch == nil || *p.Branch != "stack/00" || p.BaseBranch == nil || *p.BaseBranch != "dev" {
		t.Fatalf("parent: %+v", p)
	}
	if a.Branch == nil || *a.Branch != "stack/01-a" || a.DeliveryMode != task.ModeNone || a.Shape != "" || b.Branch != nil {
		t.Fatalf("a: %+v b: %+v", a, b)
	}
}

func TestImportRejectsInvalidBranches(t *testing.T) {
	repo := identitytest.Repository(t)
	parent := func(line string) string {
		return strings.Replace(parentTOML, "title = \"Templates\"\n", "title = \"Templates\"\n"+line, 1) + tasktest.Brief() + "'''\n" + taskTOML("a", "A")
	}
	for body, want := range map[string]string{
		withKeys(taskTOML("a", "A"), "a", "branch = \"a..b\"\n"): `task "a": branch "a..b" is not a valid branch name`,
		parent("branch = \"-x\"\n"):                              `[parent]: branch "-x" starts with -`,
		parent("base_branch = \"refs/heads/-x\"\n"):              `[parent]: base_branch "-x" starts with -`,
	} {
		path := write(t, "schema_version = 1\n"+body)
		for _, dry := range []bool{true, false} {
			out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: path, FileSet: true, DryRun: dry}, strings.NewReader(""))
			if out.Error == nil || out.Error.Code != "invalid_input" || out.Error.Message != want {
				t.Fatalf("%q dry=%v: %+v", body, dry, out.Error)
			}
		}
	}
	if count(t, repo) != 0 {
		t.Fatal("created tasks")
	}
}
