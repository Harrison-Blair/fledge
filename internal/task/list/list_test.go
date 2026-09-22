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
	want := "ID        STATUS     OWNER     PARENT  PROGRESS  TITLE\n" +
		first + "  created    -         -       -         first\n" +
		second + "  assigned   worker    -       -         second\n" +
		third + "  completed  0badc0de  -       -         third\n"
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
	for _, o := range []Options{{Status: "done"}, {Owner: "worker"}, {Parent: "goal"}} {
		if out := Run(context.Background(), tasktest.Client(t, repo, ""), o); out.Error == nil || out.Error.Code != "invalid_input" {
			t.Fatalf("%+v", out.Error)
		}
	}
}

func TestListParentShowsDirectChildrenAndProgress(t *testing.T) {
	repo := identitytest.Repository(t)
	goal := tasktest.Seed(t, repo, task.Record{Title: "goal", Status: task.Assigned, CreatedAt: "2026-01-01T00:00:00Z"})
	a := tasktest.Seed(t, repo, task.Record{Title: "a", Status: task.Verified, Parent: &goal, CreatedAt: "2026-01-02T00:00:00Z"})
	b := tasktest.Seed(t, repo, task.Record{Title: "b", Status: task.Assigned, Parent: &goal, CreatedAt: "2026-01-03T00:00:00Z"})
	c := tasktest.Seed(t, repo, task.Record{Title: "c", Status: task.Cancelled, Parent: &goal, CreatedAt: "2026-01-04T00:00:00Z"})
	tasktest.Seed(t, repo, task.Record{Title: "b1", Status: task.Created, Parent: &b, CreatedAt: "2026-01-05T00:00:00Z"})
	ctx := context.Background()

	out := Run(ctx, tasktest.Client(t, repo, ""), Options{Parent: goal})
	r := out.Result.(Result)
	if got := ids(r); out.Error != nil || len(got) != 3 || got[0] != a || got[1] != b || got[2] != c {
		t.Fatalf("%v %+v", got, out.Error)
	}
	if p := r.Tasks[1].Progress; p == nil || *p != (task.Progress{Total: 1}) || r.Tasks[0].Progress != nil {
		t.Fatalf("%+v", r.Tasks)
	}
	if got := ids(Run(ctx, tasktest.Client(t, repo, ""), Options{Parent: a}).Result.(Result)); len(got) != 0 {
		t.Fatalf("%v", got)
	}

	var buf bytes.Buffer
	if err := Run(ctx, tasktest.Client(t, repo, ""), Options{Status: task.Assigned}).Write(&buf, false, Render); err != nil {
		t.Fatal(err)
	}
	want := "ID        STATUS    OWNER  PARENT    PROGRESS                   TITLE\n" +
		goal + "  assigned  -      -         1/2 verified, 1 cancelled  goal\n" +
		b + "  assigned  -      " + goal + "  0/1 verified               b\n"
	if buf.String() != want {
		t.Fatalf("%q\nwant %q", buf.String(), want)
	}
}
