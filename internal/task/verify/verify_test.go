package verify

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
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
		herdrscript.Call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: worker.Agent}}), assign.Options{ID: id, Agent: identity.Target{Pane: "w1:p3"}})
	if assigned.Error != nil {
		t.Fatalf("assign: %+v", assigned.Error)
	}
	r = tasktest.Load(t, repo, id)
	if r.Delivery == nil || r.AssignedAt == nil || r.Delivery.DeliveredAt == nil {
		t.Fatalf("after assign %+v", r)
	}
	want.Status, want.Owner, want.AssignedAt = task.Assigned, &workerRec.ID, r.AssignedAt
	want.Delivery = &task.Delivery{MessageID: r.Delivery.MessageID, Pane: "w1:p3", Attempt: task.Attempt{DeliveredAt: r.Delivery.DeliveredAt}}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("after assign %+v", r)
	}

	completed := complete.Run(ctx, tasktest.Client(t, repo, "w1:p3",
		tasktest.Get("w1:p3", worker),
		tasktest.Get("w1:p1", boss),
		tasktest.Get("w1:p3", worker),
		herdrscript.Call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: boss.Agent}},
	), complete.Options{ID: id, Summary: "fixed", SummarySet: true}, strings.NewReader(""))
	if completed.Error != nil {
		t.Fatalf("complete: %+v", completed.Error)
	}
	r = tasktest.Load(t, repo, id)
	want.Status, want.Result, want.CompletedAt = task.Completed, tasktest.Ptr("fixed"), r.CompletedAt
	if r.CompletionNotification == nil || r.CompletionNotification.DeliveredAt == nil {
		t.Fatalf("after complete %+v", r)
	}
	want.CompletionNotification = &task.CompletionNotification{Recipient: bossRec.ID, MessageID: r.CompletionNotification.MessageID, Pane: tasktest.Ptr("w1:p1"), Attempt: task.Attempt{DeliveredAt: r.CompletionNotification.DeliveredAt}}
	if !reflect.DeepEqual(r, want) || r.CompletedAt == nil {
		t.Fatalf("after complete %+v", r)
	}

	verified := Run(ctx, tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{ID: id, Summary: "looks right", SummarySet: true}, strings.NewReader(""))
	if verified.Error != nil || verified.Operation != "task.verify" {
		t.Fatalf("verify: %+v", verified.Error)
	}
	r = tasktest.Load(t, repo, id)
	want.Status, want.Verifier, want.VerificationNote, want.VerifiedAt = task.Verified, &bossRec.ID, tasktest.Ptr("looks right"), r.VerifiedAt
	if !reflect.DeepEqual(r, want) || r.VerifiedAt == nil || !reflect.DeepEqual(verified.Result, Result{Record: r, OpenSubtasks: []string{}}) {
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

func TestParentWithOpenSubtasksNeedsForce(t *testing.T) {
	repo, id := setup(t, task.Completed)
	for _, status := range []string{task.Verified, task.Cancelled} {
		done := tasktest.Seed(t, repo, task.Record{Title: status, Status: status, Parent: &id})
		tasktest.Seed(t, repo, task.Record{Title: "grandchild", Status: task.Created, Parent: &done})
	}
	open := tasktest.Seed(t, repo, task.Record{Title: "open", Status: task.Completed, Parent: &id})
	before := tasktest.Load(t, repo, id)
	c := func() libagent.Client { return tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)) }
	tasktest.Register(t, repo, boss)
	out := Run(context.Background(), c(), Options{ID: id}, strings.NewReader(""))
	if out.Error == nil || out.Error.Code != "task_open_subtasks" || !strings.Contains(out.Error.Message, open) || !strings.Contains(out.Error.Message, "--force") || !reflect.DeepEqual(tasktest.Load(t, repo, id), before) {
		t.Fatalf("%+v", out.Error)
	}
	out = Run(context.Background(), c(), Options{ID: id, Force: true}, strings.NewReader(""))
	r := tasktest.Load(t, repo, id)
	if out.Error != nil || r.Status != task.Verified || !r.Forced || !reflect.DeepEqual(out.Result, Result{Record: r, OpenSubtasks: []string{open}}) {
		t.Fatalf("%+v %+v", out.Error, out.Result)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || !strings.HasSuffix(b.String(), " (forced).\nOpen subtasks: "+open+".\n") {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestParentWithFinishedSubtasksVerifies(t *testing.T) {
	repo, id := setup(t, task.Completed)
	tasktest.Register(t, repo, boss)
	for _, status := range []string{task.Verified, task.Cancelled} {
		tasktest.Seed(t, repo, task.Record{Title: status, Status: status, Parent: &id})
	}
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{ID: id}, strings.NewReader(""))
	if r := tasktest.Load(t, repo, id); out.Error != nil || r.Status != task.Verified || r.Forced {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}

// A subtask created while its parent is being verified must either land
// before verify's subtask scan, blocking it, or be refused as the parent is
// already verified; both succeeding would leave an open subtask under a
// parent verified without --force.
func TestConcurrentSubtaskCreateAndParentVerify(t *testing.T) {
	repo := identitytest.Repository(t)
	owner := tasktest.Register(t, repo, worker)
	tasktest.Register(t, repo, boss)
	for range 50 {
		parent := tasktest.Seed(t, repo, task.Record{Title: "goal", Status: task.Completed, Owner: &owner.ID})
		var created, verified libagent.Outcome
		var wg sync.WaitGroup
		wg.Go(func() {
			created = create.Run(context.Background(), tasktest.Client(t, repo, ""), create.Options{Title: "late", Body: "b", BodySet: true, Parent: parent}, strings.NewReader(""))
		})
		wg.Go(func() {
			verified = Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{ID: parent}, strings.NewReader(""))
		})
		wg.Wait()
		switch {
		case created.Error == nil && verified.Error != nil && verified.Error.Code == "task_open_subtasks":
		case verified.Error == nil && created.Error != nil && created.Error.Code == "task_invalid_state":
		default:
			t.Fatalf("create %+v verify %+v", created.Error, verified.Error)
		}
	}
}
