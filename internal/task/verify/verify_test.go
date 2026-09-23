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

func TestVerifyRequiresCompletedOrVerified(t *testing.T) {
	for _, status := range []string{task.Created, task.Assigned, task.Cancelled} {
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

// verified seeds a task already verified by boss, forced, with a note, so a
// repeat verification must replace every verification field.
func verified(t *testing.T) (repo, id string, before task.Record) {
	repo = identitytest.Repository(t)
	owner := tasktest.Register(t, repo, worker)
	first := tasktest.Register(t, repo, boss)
	id = tasktest.Seed(t, repo, task.Record{Title: "t", Status: task.Verified, Owner: &owner.ID, Result: tasktest.Ptr("done"),
		CompletedAt: tasktest.Ptr("2026-01-01T00:00:00Z"), CompletionNotification: &task.CompletionNotification{Recipient: first.ID, MessageID: "m-1"},
		Verifier: &first.ID, VerificationNote: tasktest.Ptr("old note"), Forced: true, VerifiedAt: tasktest.Ptr("2026-01-02T00:00:00Z")})
	return repo, id, tasktest.Load(t, repo, id)
}

func TestRepeatVerificationReplacesLatestVerification(t *testing.T) {
	repo, id, before := verified(t)
	other := tasktest.Agent("w1:p5", "term_other", "other")
	otherRec := tasktest.Register(t, repo, other)
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p5", tasktest.Get("w1:p5", other)), Options{ID: id}, strings.NewReader(""))
	r := tasktest.Load(t, repo, id)
	if out.Error != nil || r.VerifiedAt == nil || *r.VerifiedAt == *before.VerifiedAt {
		t.Fatalf("%+v %+v", out.Error, r)
	}
	want := before
	want.Verifier, want.VerificationNote, want.Forced, want.VerifiedAt = &otherRec.ID, nil, false, r.VerifiedAt
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("got %+v\nwant %+v", r, want)
	}
}

func TestRepeatVerificationRejectsOwner(t *testing.T) {
	repo, id, before := verified(t)
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p3", tasktest.Get("w1:p3", worker)), Options{ID: id}, strings.NewReader(""))
	if out.Error == nil || out.Error.Code != "task_self_verification" || !reflect.DeepEqual(tasktest.Load(t, repo, id), before) {
		t.Fatalf("%+v", out.Error)
	}
}

func TestRepeatUnregisteredForceClearsVerifierAndNote(t *testing.T) {
	repo, id, before := verified(t)
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id, Force: true}, strings.NewReader(""))
	r := tasktest.Load(t, repo, id)
	want := before
	want.Verifier, want.VerificationNote, want.VerifiedAt = nil, nil, r.VerifiedAt
	if out.Error != nil || !reflect.DeepEqual(r, want) || *r.VerifiedAt == *before.VerifiedAt {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}

func TestRepeatAfterForcedAcceptanceStillChecksOpenSubtasks(t *testing.T) {
	repo, id := setup(t, task.Completed)
	tasktest.Register(t, repo, boss)
	open := tasktest.Seed(t, repo, task.Record{Title: "open", Status: task.Created, Parent: &id})
	c := func() libagent.Client { return tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)) }
	if out := Run(context.Background(), c(), Options{ID: id, Force: true}, strings.NewReader("")); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	before := tasktest.Load(t, repo, id)
	out := Run(context.Background(), c(), Options{ID: id}, strings.NewReader(""))
	if out.Error == nil || out.Error.Code != "task_open_subtasks" || !strings.Contains(out.Error.Message, open) || !reflect.DeepEqual(tasktest.Load(t, repo, id), before) {
		t.Fatalf("%+v", out.Error)
	}
}

// parent seeds a grouping parent in status, owned by worker when assigned,
// with one direct subtask per status in children, and registers boss.
func parent(t *testing.T, status string, children ...string) (repo, id string, subtasks []string) {
	repo = identitytest.Repository(t)
	owner := tasktest.Register(t, repo, worker)
	tasktest.Register(t, repo, boss)
	r := task.Record{Title: "goal", Status: status}
	if status == task.Assigned {
		r.Owner = &owner.ID
	}
	id = tasktest.Seed(t, repo, r)
	for _, c := range children {
		subtasks = append(subtasks, tasktest.Seed(t, repo, task.Record{Title: c, Status: c, Parent: &id}))
	}
	return repo, id, subtasks
}

func TestFinishedParentVerifiesFromCreatedOrAssigned(t *testing.T) {
	for _, tc := range []struct {
		status   string
		children []string
	}{
		{task.Created, []string{task.Verified, task.Verified}},
		{task.Assigned, []string{task.Verified, task.Verified}},
		{task.Created, []string{task.Verified, task.Cancelled}},
		{task.Assigned, []string{task.Cancelled, task.Verified}},
	} {
		t.Run(tc.status+"/"+strings.Join(tc.children, ","), func(t *testing.T) {
			repo, id, _ := parent(t, tc.status, tc.children...)
			before := tasktest.Load(t, repo, id)
			out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{ID: id, Summary: "all done", SummarySet: true}, strings.NewReader(""))
			r := tasktest.Load(t, repo, id)
			if out.Error != nil || r.Verifier == nil || r.VerifiedAt == nil {
				t.Fatalf("%+v %+v", out.Error, r)
			}
			want := before
			want.Status, want.Verifier, want.VerificationNote, want.VerifiedAt = task.Verified, r.Verifier, tasktest.Ptr("all done"), r.VerifiedAt
			if !reflect.DeepEqual(r, want) || !reflect.DeepEqual(out.Result, Result{Record: r, OpenSubtasks: []string{}}) {
				t.Fatalf("got %+v\nwant %+v", r, want)
			}
		})
	}
}

func TestParentWithOnlyCancelledSubtasksIsRefused(t *testing.T) {
	for _, status := range []string{task.Created, task.Assigned} {
		repo, id, _ := parent(t, status, task.Cancelled, task.Cancelled)
		before := tasktest.Load(t, repo, id)
		for _, force := range []bool{false, true} {
			out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{ID: id, Force: force}, strings.NewReader(""))
			if out.Error == nil || out.Error.Code != "task_invalid_state" || !strings.Contains(out.Error.Message, "fledge task cancel") || !reflect.DeepEqual(tasktest.Load(t, repo, id), before) {
				t.Fatalf("%s force=%v: %+v", status, force, out.Error)
			}
		}
	}
}

func TestUnfinishedParentIsRefusedEvenWithForce(t *testing.T) {
	for _, status := range []string{task.Created, task.Assigned} {
		repo, id, subtasks := parent(t, status, task.Verified, task.Completed)
		before := tasktest.Load(t, repo, id)
		for _, force := range []bool{false, true} {
			out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)), Options{ID: id, Force: force}, strings.NewReader(""))
			if out.Error == nil || out.Error.Code != "task_open_subtasks" || !strings.Contains(out.Error.Message, subtasks[1]) || strings.Contains(out.Error.Message, subtasks[0]) || strings.Contains(out.Error.Message, "--force") || !reflect.DeepEqual(tasktest.Load(t, repo, id), before) {
				t.Fatalf("%s force=%v: %+v", status, force, out.Error)
			}
		}
	}
}

func TestAssignedParentOwnerNeedsForce(t *testing.T) {
	repo, id, _ := parent(t, task.Assigned, task.Verified)
	before := tasktest.Load(t, repo, id)
	c := func() libagent.Client { return tasktest.Client(t, repo, "w1:p3", tasktest.Get("w1:p3", worker)) }
	out := Run(context.Background(), c(), Options{ID: id}, strings.NewReader(""))
	if out.Error == nil || out.Error.Code != "task_self_verification" || !reflect.DeepEqual(tasktest.Load(t, repo, id), before) {
		t.Fatalf("%+v", out.Error)
	}
	out = Run(context.Background(), c(), Options{ID: id, Force: true}, strings.NewReader(""))
	r := tasktest.Load(t, repo, id)
	if out.Error != nil || r.Status != task.Verified || !r.Forced || *r.Verifier != *before.Owner {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}
