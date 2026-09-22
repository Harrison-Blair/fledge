package depend

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func run(t *testing.T, repo string, o Options) (task.Record, *string, string) {
	t.Helper()
	out := Run(context.Background(), tasktest.Client(t, repo, ""), o)
	if out.Error != nil {
		return task.Record{}, &out.Error.Code, out.Error.Message
	}
	if out.Operation != "task.depend" {
		t.Fatalf("%+v", out)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	return out.Result.(task.Record), nil, b.String()
}

func TestAddAndRemovePrerequisites(t *testing.T) {
	repo := identitytest.Repository(t)
	a := tasktest.Seed(t, repo, task.Record{Title: "a", Status: task.Created})
	b := tasktest.Seed(t, repo, task.Record{Title: "b", Status: task.Verified})
	c := tasktest.Seed(t, repo, task.Record{Title: "c", Status: task.Cancelled})
	x := tasktest.Seed(t, repo, task.Record{Title: "x", Status: task.Assigned, After: []string{a}})

	r, code, text := run(t, repo, Options{ID: x, After: []string{b, a, c, b}})
	if code != nil || !reflect.DeepEqual(r.After, []string{a, b, c}) || !reflect.DeepEqual(tasktest.Load(t, repo, x), r) {
		t.Fatalf("%v %+v", code, r)
	}
	if want := "Task " + x + " is after " + a + ", " + b + ", " + c + ".\n"; text != want {
		t.Fatalf("%q", text)
	}
	r, code, _ = run(t, repo, Options{ID: x, Remove: []string{a, "0123abcd"}})
	if code != nil || !reflect.DeepEqual(r.After, []string{b, c}) {
		t.Fatalf("%v %+v", code, r)
	}
	r, code, text = run(t, repo, Options{ID: x, Remove: []string{b, c}})
	if code != nil || r.After != nil || tasktest.Load(t, repo, x).After != nil || text != "Task "+x+" has no prerequisites.\n" {
		t.Fatalf("%v %+v %q", code, r, text)
	}
}

func TestRejectsCycles(t *testing.T) {
	repo := identitytest.Repository(t)
	c := tasktest.Seed(t, repo, task.Record{Title: "c", Status: task.Created})
	b := tasktest.Seed(t, repo, task.Record{Title: "b", Status: task.Created, After: []string{c}})
	a := tasktest.Seed(t, repo, task.Record{Title: "a", Status: task.Created, After: []string{b}})
	for _, o := range []Options{{ID: a, After: []string{a}}, {ID: c, After: []string{a}}, {ID: b, After: []string{c, a}}} {
		before := tasktest.Load(t, repo, o.ID)
		_, code, msg := run(t, repo, o)
		if code == nil || *code != "task_dependency_cycle" || !reflect.DeepEqual(tasktest.Load(t, repo, o.ID), before) {
			t.Fatalf("%+v: %v %s", o, code, msg)
		}
	}
	if _, _, msg := run(t, repo, Options{ID: a, After: []string{a}}); !strings.Contains(msg, "itself") {
		t.Fatalf("%s", msg)
	}
	if _, _, msg := run(t, repo, Options{ID: c, After: []string{a}}); !strings.Contains(msg, a+" → "+b+" → "+c) {
		t.Fatalf("%s", msg)
	}
}

func TestRejectsUnknownPrerequisiteAndFinishedTask(t *testing.T) {
	repo := identitytest.Repository(t)
	a := tasktest.Seed(t, repo, task.Record{Title: "a", Status: task.Created})
	for _, status := range []string{task.Verified, task.Cancelled} {
		x := tasktest.Seed(t, repo, task.Record{Title: "x", Status: status})
		for _, o := range []Options{{ID: x, After: []string{a}}, {ID: x, Remove: []string{a}}} {
			if _, code, msg := run(t, repo, o); code == nil || *code != "task_invalid_state" {
				t.Fatalf("%s %+v: %v %s", status, o, code, msg)
			}
		}
	}
	x := tasktest.Seed(t, repo, task.Record{Title: "x", Status: task.Completed})
	if _, code, msg := run(t, repo, Options{ID: x, After: []string{a, "0123abcd"}}); code == nil || *code != "task_not_found" || !strings.Contains(msg, "0123abcd") || tasktest.Load(t, repo, x).After != nil {
		t.Fatalf("%v %s", code, msg)
	}
	if _, code, _ := run(t, repo, Options{ID: "0123abcd", After: []string{a}}); code == nil || *code != "task_not_found" {
		t.Fatalf("%v", code)
	}
}

func TestRejectsInvalidInput(t *testing.T) {
	repo := identitytest.Repository(t)
	for _, o := range []Options{{ID: "01234567"}, {ID: "BAD", After: []string{"01234567"}}, {ID: "01234567", After: []string{"nope"}},
		{ID: "01234567", Remove: []string{"nope"}}, {ID: "01234567", After: []string{"89abcdef"}, Remove: []string{"89abcdef"}}} {
		out := Run(context.Background(), tasktest.Client(t, repo, ""), o)
		if out.Error == nil || out.Error.Code != "invalid_input" || out.ExitCode() != 2 {
			t.Fatalf("%+v: %+v", o, out.Error)
		}
	}
}

func TestConcurrentOppositeEdgesCannotBothLand(t *testing.T) {
	repo := identitytest.Repository(t)
	for range 20 {
		a := tasktest.Seed(t, repo, task.Record{Title: "a", Status: task.Created})
		b := tasktest.Seed(t, repo, task.Record{Title: "b", Status: task.Created})
		var wg sync.WaitGroup
		codes := make([]string, 2)
		for i, o := range []Options{{ID: a, After: []string{b}}, {ID: b, After: []string{a}}} {
			wg.Go(func() {
				out := Run(context.Background(), tasktest.Client(t, repo, ""), o)
				if out.Error != nil {
					codes[i] = out.Error.Code
				}
			})
		}
		wg.Wait()
		if !(codes[0] == "" && codes[1] == "task_dependency_cycle" || codes[1] == "" && codes[0] == "task_dependency_cycle") {
			t.Fatalf("%v", codes)
		}
		if len(tasktest.Load(t, repo, a).After)+len(tasktest.Load(t, repo, b).After) != 1 {
			t.Fatal("both or neither edge stored")
		}
	}
}
