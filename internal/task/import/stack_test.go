package taskimport

import (
	"context"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// shaped is a proposal whose [parent] has shape and whose tasks are tasks.
func shaped(shape string, tasks ...string) string {
	return "schema_version = 1\n" + strings.Replace(parentTOML, "title = \"Templates\"\n", "title = \"Templates\"\nshape = \""+shape+"\"\n", 1) + tasktest.Brief() + "'''\n" + strings.Join(tasks, "")
}

func TestImportRefusesAStackFork(t *testing.T) {
	repo := identitytest.Repository(t)
	for body, want := range map[string]string{
		shaped("stack", taskTOML("a", "A"), taskTOML("b", "B", "a"), taskTOML("c", "C", "a")): "stack layers must form one after chain: layer a has more than one dependent layer: b, c",
		shaped("stack", taskTOML("a", "A"), taskTOML("b", "B")):                               "stack layers must form one after chain: more than one layer has no prerequisite layer: a, b",
	} {
		for _, dry := range []bool{true, false} {
			out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: write(t, body), FileSet: true, DryRun: dry}, strings.NewReader(""))
			if out.Error == nil || out.Error.Code != "invalid_input" || out.Error.Message != want || out.ExitCode() != 2 {
				t.Fatalf("dry=%v: %+v", dry, out.Error)
			}
		}
	}
	if count(t, repo) != 0 {
		t.Fatal("created tasks")
	}
	for _, body := range []string{
		shaped("stack", taskTOML("c", "C", "b"), taskTOML("a", "A"), taskTOML("b", "B", "a")),
		shaped("feature", taskTOML("a", "A"), taskTOML("b", "B", "a"), taskTOML("c", "C", "a")),
	} {
		out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: write(t, body), FileSet: true}, strings.NewReader(""))
		if out.Error != nil {
			t.Fatalf("%+v", out.Error)
		}
	}
}

func TestImportUnderAStackParentChecksTheExistingLayers(t *testing.T) {
	repo := identitytest.Repository(t)
	for _, shape := range []string{task.ShapeStack, task.ShapeFeature} {
		parent := tasktest.Seed(t, repo, task.Record{Title: "p", Status: task.Created, Shape: shape})
		a := tasktest.Seed(t, repo, task.Record{Title: "a", Status: task.Created, Parent: &parent})
		b := tasktest.Seed(t, repo, task.Record{Title: "b", Status: task.Created, Parent: &parent, After: []string{a}})
		c := tasktest.Seed(t, repo, task.Record{Title: "c", Status: task.Cancelled, Parent: &parent, After: []string{b}})
		for _, dry := range []bool{true, false} {
			n := count(t, repo)
			out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: write(t, "schema_version = 1\n"+taskTOML("d", "D", b)), FileSet: true, Parent: parent, DryRun: dry}, strings.NewReader(""))
			if shape == task.ShapeFeature {
				if out.Error != nil {
					t.Fatalf("%s dry=%v: %+v", shape, dry, out.Error)
				}
				continue
			}
			want := "stack layers must form one after chain: layer " + b + " has more than one dependent layer: " + c + ", d"
			if out.Error == nil || out.Error.Code != "invalid_input" || out.Error.Message != want || count(t, repo) != n {
				t.Fatalf("dry=%v: %+v", dry, out.Error)
			}
		}
		if shape == task.ShapeStack {
			out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: write(t, "schema_version = 1\n"+taskTOML("d", "D", c)+taskTOML("e", "E", "d")), FileSet: true, Parent: parent}, strings.NewReader(""))
			if out.Error != nil {
				t.Fatalf("%+v", out.Error)
			}
		}
	}
}
