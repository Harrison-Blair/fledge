package cancel

import (
	"context"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// A landed task is final: cancel refuses it and leaves the record unchanged.
func TestRefusesLandedTask(t *testing.T) {
	repo := identitytest.Repository(t)
	id := tasktest.Seed(t, repo, task.Record{Title: "t", Status: task.Landed, Result: tasktest.Ptr("done")})
	before := tasktest.Load(t, repo, id)
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id, Reason: "late"})
	if out.Error == nil || out.Error.Code != "task_invalid_state" || !reflect.DeepEqual(tasktest.Load(t, repo, id), before) {
		t.Fatalf("%+v", out.Error)
	}
}
