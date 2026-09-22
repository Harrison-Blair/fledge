package verify

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
	"github.com/Harrison-Blair/fledge/internal/task/assign"
	"github.com/Harrison-Blair/fledge/internal/task/complete"
	"github.com/Harrison-Blair/fledge/internal/task/create"
)

var (
	boss   = tasktest.Agent("w1:p1", "term_boss", "boss")
	worker = tasktest.Agent("w1:p3", "term_worker", "worker")
)

func TestLifecycleCreateAssignCompleteVerify(t *testing.T) {
	ctx := context.Background()
	repo := identitytest.Repository(t)
	bossRec := tasktest.Register(t, repo, boss)
	workerRec := tasktest.Register(t, repo, worker)

	created := create.Run(ctx, tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), create.Options{Title: "Fix it", Body: "do the thing", BodySet: true}, strings.NewReader(""))
	if created.Error != nil {
		t.Fatalf("create: %+v", created.Error)
	}
	id := created.Result.(task.Record).ID
	want := task.Record{ID: id, Title: "Fix it", Brief: "do the thing", Status: task.Created, CreatedBy: &bossRec.ID}
	r := tasktest.Load(t, repo, id)
	want.CreatedAt = r.CreatedAt
	if !reflect.DeepEqual(r, want) || r.CreatedAt == "" {
		t.Fatalf("after create %+v", r)
	}

	assigned := assign.Run(ctx, tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p3", worker), tasktest.Get("w1:p1", boss),
		herdrscript.Call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: worker.Agent}}), assign.Options{ID: id, Pane: "w1:p3"})
	if assigned.Error != nil {
		t.Fatalf("assign: %+v", assigned.Error)
	}
	r = tasktest.Load(t, repo, id)
	if r.Delivery == nil || r.AssignedAt == nil || r.Delivery.DeliveredAt == nil {
		t.Fatalf("after assign %+v", r)
	}
	want.Status, want.Owner, want.AssignedAt = task.Assigned, &workerRec.ID, r.AssignedAt
	want.Delivery = &task.Delivery{MessageID: r.Delivery.MessageID, Pane: "w1:p3", DeliveredAt: r.Delivery.DeliveredAt}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("after assign %+v", r)
	}

	completed := complete.Run(ctx, tasktest.Client(t, repo, "w1:p3", tasktest.Get("w1:p3", worker)), complete.Options{ID: id, Summary: "fixed", SummarySet: true}, strings.NewReader(""))
	if completed.Error != nil {
		t.Fatalf("complete: %+v", completed.Error)
	}
	r = tasktest.Load(t, repo, id)
	want.Status, want.Result, want.CompletedAt = task.Completed, tasktest.Ptr("fixed"), r.CompletedAt
	if !reflect.DeepEqual(r, want) || r.CompletedAt == nil {
		t.Fatalf("after complete %+v", r)
	}

	verified := Run(ctx, tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{ID: id, Summary: "looks right", SummarySet: true}, strings.NewReader(""))
	if verified.Error != nil || verified.Operation != "task.verify" {
		t.Fatalf("verify: %+v", verified.Error)
	}
	r = tasktest.Load(t, repo, id)
	want.Status, want.Verifier, want.VerificationNote, want.VerifiedAt = task.Verified, &bossRec.ID, tasktest.Ptr("looks right"), r.VerifiedAt
	if !reflect.DeepEqual(r, want) || r.VerifiedAt == nil || !reflect.DeepEqual(verified.Result, r) {
		t.Fatalf("after verify %+v", r)
	}
	var b bytes.Buffer
	if err := verified.Write(&b, false, Render); err != nil || b.String() != "Verified task "+id+" as "+bossRec.ID+".\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func setup(t *testing.T, status string) (repo, id string) {
	repo = identitytest.Repository(t)
	owner := tasktest.Register(t, repo, worker)
	id = tasktest.Seed(t, repo, task.Record{Title: "t", Status: status, Owner: &owner.ID, Result: tasktest.Ptr("done")})
	return repo, id
}

func TestOwnerVerificationNeedsForceAndIsRecorded(t *testing.T) {
	repo, id := setup(t, task.Completed)
	before := tasktest.Load(t, repo, id)
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p3", tasktest.Get("w1:p3", worker)), Options{ID: id}, strings.NewReader(""))
	if out.Error == nil || out.Error.Code != "task_self_verification" || !strings.Contains(out.Error.Message, "--force") || !reflect.DeepEqual(tasktest.Load(t, repo, id), before) {
		t.Fatalf("%+v", out.Error)
	}
	out = Run(context.Background(), tasktest.Client(t, repo, "w1:p3", tasktest.Get("w1:p3", worker)), Options{ID: id, Force: true}, strings.NewReader(""))
	r := tasktest.Load(t, repo, id)
	if out.Error != nil || r.Status != task.Verified || !r.Forced || *r.Verifier != *before.Owner || r.VerificationNote != nil {
		t.Fatalf("%+v %+v", out.Error, r)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || b.String() != "Verified task "+id+" as "+*r.Verifier+" (forced).\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestUnregisteredVerifierNeedsForce(t *testing.T) {
	repo, id := setup(t, task.Completed)
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id}, strings.NewReader(""))
	if out.Error == nil || out.Error.Code != "caller_unregistered" {
		t.Fatalf("%+v", out.Error)
	}
	out = Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id, Force: true, Summary: "ok", SummarySet: true}, strings.NewReader(""))
	r := tasktest.Load(t, repo, id)
	if out.Error != nil || r.Verifier != nil || !r.Forced || *r.VerificationNote != "ok" {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}

func TestVerifyRequiresCompleted(t *testing.T) {
	for _, status := range []string{task.Created, task.Assigned, task.Verified, task.Cancelled} {
		repo, id := setup(t, status)
		out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{ID: id, Force: true}, strings.NewReader(""))
		if out.Error == nil || out.Error.Code != "task_invalid_state" {
			t.Fatalf("%s: %+v", status, out.Error)
		}
	}
}
