package complete

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

var (
	boss   = tasktest.Agent("w1:p1", "term_boss", "boss")
	worker = tasktest.Agent("w1:p3", "term_worker", "worker")
)

// setup registers boss and worker and seeds a task in status owned by worker.
func setup(t *testing.T, status string) (repo, id string) {
	repo = identitytest.Repository(t)
	tasktest.Register(t, repo, boss)
	owner := tasktest.Register(t, repo, worker)
	id = tasktest.Seed(t, repo, task.Record{Title: "t", Status: status, Owner: &owner.ID})
	return repo, id
}

func TestOwnerCompletes(t *testing.T) {
	repo, id := setup(t, task.Assigned)
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p3", tasktest.Get("w1:p3", worker)), Options{ID: id, File: "-", FileSet: true}, strings.NewReader("all done"))
	if out.Error != nil || out.Operation != "task.complete" {
		t.Fatalf("%+v", out.Error)
	}
	r := tasktest.Load(t, repo, id)
	if !reflect.DeepEqual(out.Result, r) || r.Status != task.Completed || *r.Result != "all done" || r.CompletedAt == nil {
		t.Fatalf("%+v", r)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || b.String() != "Completed task "+id+".\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestCompleteRefusesNonOwner(t *testing.T) {
	notFound := herdrscript.Call{Method: "agent.get", Err: &herdr.Error{Code: "agent_not_found", Message: "none"}}
	for label, caller := range map[string]herdrscript.Call{"other agent": tasktest.Get("w1:p1", boss), "unregistered": notFound} {
		repo, id := setup(t, task.Assigned)
		before := tasktest.Load(t, repo, id)
		out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", caller), Options{ID: id, Summary: "x", SummarySet: true}, strings.NewReader(""))
		if out.Error == nil || out.Error.Code != "task_not_owner" || !strings.Contains(out.Error.Message, "--force") {
			t.Fatalf("%s: %+v", label, out.Error)
		}
		if r := tasktest.Load(t, repo, id); !reflect.DeepEqual(r, before) {
			t.Fatalf("%s: %+v", label, r)
		}
	}
}

func TestForceCompletesForNonOwner(t *testing.T) {
	repo, id := setup(t, task.Assigned)
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{ID: id, Summary: "x", SummarySet: true, Force: true}, strings.NewReader(""))
	if out.Error != nil || tasktest.Load(t, repo, id).Status != task.Completed {
		t.Fatalf("%+v", out.Error)
	}
}

func TestCompleteRequiresAssigned(t *testing.T) {
	for _, status := range []string{task.Created, task.Completed, task.Verified, task.Cancelled} {
		repo, id := setup(t, status)
		out := Run(context.Background(), tasktest.Client(t, repo, "w1:p3", tasktest.Get("w1:p3", worker)), Options{ID: id, Summary: "x", SummarySet: true}, strings.NewReader(""))
		if out.Error == nil || out.Error.Code != "task_invalid_state" {
			t.Fatalf("%s: %+v", status, out.Error)
		}
	}
}

func TestCompleteRejectsInvalidInput(t *testing.T) {
	repo := identitytest.Repository(t)
	for label, o := range map[string]Options{"no summary": {ID: "0123abcd"}, "bad id": {ID: "x", Summary: "s", SummarySet: true}} {
		out := Run(context.Background(), tasktest.Client(t, repo, ""), o, strings.NewReader(""))
		if out.Error == nil || out.Error.Code != "invalid_input" {
			t.Fatalf("%s: %+v", label, out.Error)
		}
	}
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: "0123abcd", Summary: "s", SummarySet: true}, strings.NewReader(""))
	if out.Error == nil || out.Error.Code != "task_not_found" {
		t.Fatalf("%+v", out.Error)
	}
}
