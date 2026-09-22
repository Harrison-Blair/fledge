package cancel

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestCancelFromEachOpenState(t *testing.T) {
	for _, status := range []string{task.Created, task.Assigned, task.Completed} {
		repo := identitytest.Repository(t)
		id := tasktest.Seed(t, repo, task.Record{Title: "t", Status: status})
		out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id, Reason: "superseded"})
		r := tasktest.Load(t, repo, id)
		if out.Error != nil || out.Operation != "task.cancel" || !reflect.DeepEqual(out.Result, r) || r.Status != task.Cancelled || r.CancelledAt == nil || *r.CancelReason != "superseded" {
			t.Fatalf("%s: %+v %+v", status, out.Error, r)
		}
		var b bytes.Buffer
		if err := out.Write(&b, false, Render); err != nil || b.String() != "Cancelled task "+id+".\n" {
			t.Fatalf("%q %v", b.String(), err)
		}
	}
	repo := identitytest.Repository(t)
	id := tasktest.Seed(t, repo, task.Record{Title: "t", Status: task.Created})
	if out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id}); out.Error != nil || tasktest.Load(t, repo, id).CancelReason != nil {
		t.Fatalf("%+v", out.Error)
	}
}

func TestCancelRefusesTerminalStates(t *testing.T) {
	for _, status := range []string{task.Verified, task.Cancelled} {
		repo := identitytest.Repository(t)
		id := tasktest.Seed(t, repo, task.Record{Title: "t", Status: status})
		before := tasktest.Load(t, repo, id)
		out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id})
		if out.Error == nil || out.Error.Code != "task_invalid_state" || !reflect.DeepEqual(tasktest.Load(t, repo, id), before) {
			t.Fatalf("%s: %+v", status, out.Error)
		}
	}
}

func TestCancelRejectsInvalidInput(t *testing.T) {
	repo := identitytest.Repository(t)
	if out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: "BAD"}); out.Error == nil || out.Error.Code != "invalid_input" {
		t.Fatalf("%+v", out.Error)
	}
	if out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: "0123abcd"}); out.Error == nil || out.Error.Code != "task_not_found" {
		t.Fatalf("%+v", out.Error)
	}
}

func TestCancelParentLeavesSubtasks(t *testing.T) {
	repo := identitytest.Repository(t)
	parent := tasktest.Seed(t, repo, task.Record{Title: "goal", Status: task.Assigned})
	child := tasktest.Seed(t, repo, task.Record{Title: "child", Status: task.Assigned, Parent: &parent})
	before := tasktest.Load(t, repo, child)
	if out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: parent}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if !reflect.DeepEqual(tasktest.Load(t, repo, child), before) {
		t.Fatalf("%+v", tasktest.Load(t, repo, child))
	}
}
