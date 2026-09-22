package list

import (
	"bytes"
	"context"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func ids(out Result) []string {
	s := []string{}
	for _, r := range out.Tasks {
		s = append(s, r.ID)
	}
	return s
}

func TestListFiltersAndNamesLiveOwners(t *testing.T) {
	repo := identitytest.Repository(t)
	worker := tasktest.Register(t, repo, tasktest.Agent("w1:p3", "term_worker", "worker"))
	gone := "0badc0de"
	second := tasktest.Seed(t, repo, task.Record{Title: "second", Status: task.Assigned, Owner: &worker.ID, CreatedAt: "2026-01-02T00:00:00Z"})
	first := tasktest.Seed(t, repo, task.Record{Title: "first", Status: task.Created, CreatedAt: "2026-01-01T00:00:00Z"})
	third := tasktest.Seed(t, repo, task.Record{Title: "third", Status: task.Completed, Owner: &gone, CreatedAt: "2026-01-03T00:00:00Z"})
	ctx := context.Background()

	out := Run(ctx, tasktest.Client(t, repo, ""), Options{})
	if out.Error != nil || out.Operation != "task.list" {
		t.Fatalf("%+v", out.Error)
	}
	r := out.Result.(Result)
	if got := ids(r); len(got) != 3 || got[0] != first || got[1] != second || got[2] != third {
		t.Fatalf("%v", got)
	}
	if r.Tasks[1].OwnerName == nil || *r.Tasks[1].OwnerName != "worker" || r.Tasks[2].OwnerName != nil {
		t.Fatalf("%+v", r.Tasks)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	want := "ID        STATUS     OWNER     TITLE\n" +
		first + "  created    -         first\n" +
		second + "  assigned   worker    second\n" +
		third + "  completed  0badc0de  third\n"
	if b.String() != want {
		t.Fatalf("%q\nwant %q", b.String(), want)
	}

	if got := ids(Run(ctx, tasktest.Client(t, repo, ""), Options{Status: task.Assigned}).Result.(Result)); len(got) != 1 || got[0] != second {
		t.Fatalf("%v", got)
	}
	if got := ids(Run(ctx, tasktest.Client(t, repo, ""), Options{Owner: gone}).Result.(Result)); len(got) != 1 || got[0] != third {
		t.Fatalf("%v", got)
	}
}

func TestListEmptyAndInvalid(t *testing.T) {
	repo := identitytest.Repository(t)
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{})
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); out.Error != nil || err != nil || b.String() != "No tasks.\n" || out.Result.(Result).Tasks == nil {
		t.Fatalf("%+v %q", out.Error, b.String())
	}
	for _, o := range []Options{{Status: "done"}, {Owner: "worker"}} {
		if out := Run(context.Background(), tasktest.Client(t, repo, ""), o); out.Error == nil || out.Error.Code != "invalid_input" {
			t.Fatalf("%+v", out.Error)
		}
	}
}
