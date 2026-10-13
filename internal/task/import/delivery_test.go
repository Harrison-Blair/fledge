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

// withKeys adds lines to a [[tasks]] entry after its key.
func withKeys(entry, key, lines string) string {
	return strings.Replace(entry, "key = \""+key+"\"\n", "key = \""+key+"\"\n"+lines, 1)
}

func TestImportStoresDeliveryKeysAndConfigDefault(t *testing.T) {
	repo := identitytest.Repository(t)
	os.MkdirAll(filepath.Join(repo, ".fledge"), 0o755)
	if err := os.WriteFile(filepath.Join(repo, ".fledge", "config.toml"), []byte("schema_version = 1\ndelivery_mode = \"local\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := write(t, "schema_version = 1\n"+parentTOML+tasktest.Brief()+"'''\n"+
		withKeys(taskTOML("a", "A"), "a", "delivery_mode = \"pr\"\nbase_branch = \"release/1\"\n")+taskTOML("b", "B"))
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: path, FileSet: true}, strings.NewReader(""))
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	r := out.Result.(Result)
	a, b, parent := tasktest.Load(t, repo, *r.Tasks[0].ID), tasktest.Load(t, repo, *r.Tasks[1].ID), tasktest.Load(t, repo, *r.Parent)
	if a.DeliveryMode != task.ModePR || a.BaseBranch == nil || *a.BaseBranch != "release/1" {
		t.Fatalf("a: %+v", a)
	}
	if b.DeliveryMode != task.ModeLocal || b.BaseBranch != nil || parent.DeliveryMode != task.ModeLocal {
		t.Fatalf("b: %+v parent: %+v", b, parent)
	}
}

func TestImportRejectsInvalidBaseBranch(t *testing.T) {
	repo := identitytest.Repository(t)
	for _, branch := range []string{"", "-dev", "a..b", "refs/remotes/origin/-foo", "refs/heads/-foo"} {
		path := write(t, "schema_version = 1\n"+withKeys(taskTOML("a", "A"), "a", "base_branch = \""+branch+"\"\n"))
		for _, dry := range []bool{true, false} {
			out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: path, FileSet: true, DryRun: dry}, strings.NewReader(""))
			if out.Error == nil || out.Error.Code != "invalid_input" || !strings.Contains(out.Error.Message, "base_branch") {
				t.Fatalf("%q dry=%v: %+v", branch, dry, out.Error)
			}
		}
	}
	if count(t, repo) != 0 {
		t.Fatal("created tasks")
	}
}

func TestImportStoresAutoAndRejectsUnknownMode(t *testing.T) {
	repo := identitytest.Repository(t)
	path := write(t, "schema_version = 1\n"+withKeys(taskTOML("a", "A"), "a", "delivery_mode = \"auto\"\n"))
	if out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: path, FileSet: true, DryRun: true}, strings.NewReader("")); out.Error != nil {
		t.Fatalf("dry run: %+v", out.Error)
	}
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: path, FileSet: true}, strings.NewReader(""))
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if a := tasktest.Load(t, repo, *out.Result.(Result).Tasks[0].ID); a.DeliveryMode != task.ModeAuto {
		t.Fatalf("a: %+v", a)
	}
	repo = identitytest.Repository(t)
	path = write(t, "schema_version = 1\n"+withKeys(taskTOML("a", "A"), "a", "delivery_mode = \"pr-human\"\n"))
	for _, dry := range []bool{true, false} {
		out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: path, FileSet: true, DryRun: dry}, strings.NewReader(""))
		if out.Error == nil || out.Error.Code != "invalid_input" || !strings.Contains(out.Error.Message, `task "a": delivery_mode must be one of local, pr, auto, none`) {
			t.Fatalf("dry=%v: %+v", dry, out.Error)
		}
	}
	if count(t, repo) != 0 {
		t.Fatal("created tasks")
	}
}
