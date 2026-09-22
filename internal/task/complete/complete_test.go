package complete

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

var (
	boss   = tasktest.Agent("w1:p1", "term_boss", "boss")
	worker = tasktest.Agent("w1:p3", "term_worker", "worker")
)

func completionMessage(id string) string {
	return "ᛉ fledge message from worker (w1:p3) · id m-0a1b2c · reply: fledge agent message --name worker\n" +
		"task completed: " + id + " · title: Fix it · verify with: fledge task verify --id " + id + " --summary \"...\"\n" +
		"result:\nall done"
}

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

// The owner keeps its task after Herdr moves its terminal to a new pane.
func TestOwnerCompletesAfterItsPaneMoved(t *testing.T) {
	repo, id := setup(t, task.Assigned)
	moved := tasktest.Agent("w2:p1", "term_worker", "worker")
	moved.Agent.WorkspaceID = "w2"
	out := Run(context.Background(), tasktest.Client(t, repo, "w2:p1", tasktest.Get("w2:p1", moved)), Options{ID: id, Summary: "done", SummarySet: true}, strings.NewReader(""))
	if r := tasktest.Load(t, repo, id); out.Error != nil || r.Status != task.Completed {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}

func TestOwnerCompletionNotifiesCreator(t *testing.T) {
	repo := identitytest.Repository(t)
	creator := tasktest.Register(t, repo, boss)
	owner := tasktest.Register(t, repo, worker)
	id := tasktest.Seed(t, repo, task.Record{Title: "Fix it", Status: task.Assigned, Owner: &owner.ID, CreatedBy: &creator.ID})
	c := tasktest.Client(t, repo, "w1:p3",
		tasktest.Get("w1:p3", worker),
		tasktest.Get("w1:p1", boss),
		tasktest.Get("w1:p3", worker),
		herdrscript.Call{Method: "agent.prompt", Params: map[string]any{"target": "w1:p1", "text": completionMessage(id)}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: boss.Agent}},
	)
	out := run(context.Background(), c, Options{ID: id, Summary: "all done", SummarySet: true}, strings.NewReader(""), "m-0a1b2c")
	if out.Error != nil || out.Status != "success" {
		t.Fatalf("%s %+v", out.Status, out.Error)
	}
	if len(out.Effects) < 2 || out.Effects[1].Action != "submitted" || out.Effects[1].Kind != "message" || out.Effects[1].ID != "w1:p1" {
		t.Fatalf("completion did not notify creator: %+v", out.Effects)
	}
	r := tasktest.Load(t, repo, id)
	n := r.CompletionNotification
	if n == nil || n.Recipient != creator.ID || n.MessageID != "m-0a1b2c" || n.Pane == nil || *n.Pane != "w1:p1" || n.DeliveredAt == nil || n.Error != nil || n.Uncertain {
		t.Fatalf("%+v", n)
	}
	var b bytes.Buffer
	out.Write(&b, false, Render)
	want := "Completed task " + id + "; notified creator " + creator.ID + " in w1:p1 as message m-0a1b2c.\n"
	if b.String() != want {
		t.Fatalf("%q want %q", b.String(), want)
	}
	b.Reset()
	if err := out.Write(&b, true, Render); err != nil || !strings.Contains(b.String(), `"completion_notification":{"recipient":"`+creator.ID+`","message_id":"m-0a1b2c","pane":"w1:p1","delivered_at":"`) {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestCompletionSkipsNotificationToCompletingCreator(t *testing.T) {
	repo := identitytest.Repository(t)
	owner := tasktest.Register(t, repo, worker)
	id := tasktest.Seed(t, repo, task.Record{Title: "t", Status: task.Assigned, Owner: &owner.ID, CreatedBy: &owner.ID})
	out := run(context.Background(), tasktest.Client(t, repo, "w1:p3", tasktest.Get("w1:p3", worker)),
		Options{ID: id, Summary: "done", SummarySet: true}, strings.NewReader(""), "m-0a1b2c")
	r := tasktest.Load(t, repo, id)
	if out.Error != nil || r.Status != task.Completed || r.CompletionNotification != nil {
		t.Fatalf("out=%+v record=%+v", out, r)
	}
}

func TestStaleCreatorLeavesTaskCompletedWithPartialOutcome(t *testing.T) {
	repo := identitytest.Repository(t)
	creator := tasktest.Register(t, repo, boss)
	owner := tasktest.Register(t, repo, worker)
	id := tasktest.Seed(t, repo, task.Record{Title: "t", Status: task.Assigned, Owner: &owner.ID, CreatedBy: &creator.ID})
	c := tasktest.Client(t, repo, "w1:p3",
		tasktest.Get("w1:p3", worker),
		herdrscript.Call{Method: "agent.get", Params: map[string]any{"target": "w1:p1"}, Err: &herdr.Error{Code: "agent_not_found", Message: "gone"}},
		herdrscript.Call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []any{}}}, herdrscript.Call{Method: "pane.list", Result: map[string]any{"type": "pane_list", "panes": []any{}}},
	)
	out := run(context.Background(), c, Options{ID: id, Summary: "done", SummarySet: true}, strings.NewReader(""), "m-0a1b2c")
	r := tasktest.Load(t, repo, id)
	if out.Status != "partial" || out.Error == nil || out.Error.Code != "agent_identity_stale" || out.Error.Phase != "identity" || r.Status != task.Completed || r.CompletionNotification == nil || r.CompletionNotification.Error == nil || r.CompletionNotification.Pane != nil {
		t.Fatalf("out=%+v record=%+v", out, r)
	}
	var b bytes.Buffer
	out.Write(&b, false, Render)
	if !strings.Contains(b.String(), "Task "+id+" is completed; its creator was not notified and the notification will not be retried.") {
		t.Fatalf("%q", b.String())
	}
}

func TestCreatorLookupTransportFailureRendersNotNotified(t *testing.T) {
	// A transport failure while resolving the creator relocates the error phase
	// to the failing Herdr call; the stored completion must still render.
	down := &herdr.Error{Code: "transport_error", Message: "connection reset"}
	notFound := herdrscript.Call{Method: "agent.get", Params: map[string]any{"target": "w1:p1"}, Err: &herdr.Error{Code: "agent_not_found", Message: "gone"}}
	noAgents := herdrscript.Call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []any{}}}
	for _, tc := range []struct {
		phase  string
		lookup []herdrscript.Call
	}{
		{"agent.get", []herdrscript.Call{{Method: "agent.get", Params: map[string]any{"target": "w1:p1"}, Err: down}}},
		{"agent.list", []herdrscript.Call{notFound, {Method: "agent.list", Err: down}}},
		{"pane.list", []herdrscript.Call{notFound, noAgents, {Method: "pane.list", Err: down}}},
	} {
		t.Run(tc.phase, func(t *testing.T) {
			repo := identitytest.Repository(t)
			creator := tasktest.Register(t, repo, boss)
			owner := tasktest.Register(t, repo, worker)
			id := tasktest.Seed(t, repo, task.Record{Title: "t", Status: task.Assigned, Owner: &owner.ID, CreatedBy: &creator.ID})
			c := tasktest.Client(t, repo, "w1:p3", append([]herdrscript.Call{tasktest.Get("w1:p3", worker)}, tc.lookup...)...)
			out := run(context.Background(), c, Options{ID: id, Summary: "done", SummarySet: true}, strings.NewReader(""), "m-0a1b2c")
			r := tasktest.Load(t, repo, id)
			if out.Status != "partial" || out.Error == nil || out.Error.Code != "transport_error" || out.Error.Phase != tc.phase || r.Status != task.Completed || r.CompletionNotification == nil || r.CompletionNotification.Error == nil {
				t.Fatalf("out=%+v record=%+v", out, r)
			}
			var b bytes.Buffer
			out.Write(&b, false, Render)
			if !strings.Contains(b.String(), "Task "+id+" is completed; its creator was not notified and the notification will not be retried.") {
				t.Fatalf("%q", b.String())
			}
		})
	}
}

func TestCompletionNotificationFailuresAreRecorded(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		remote       *herdr.Error
		uncertain    bool
	}{
		{name: "known", status: "partial", remote: &herdr.Error{Code: "agent_blocked", Message: "approval"}},
		{name: "uncertain", status: "unknown", remote: &herdr.Error{Code: "transport_error", Message: "connection reset", Uncertain: true}, uncertain: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := identitytest.Repository(t)
			creator := tasktest.Register(t, repo, boss)
			owner := tasktest.Register(t, repo, worker)
			id := tasktest.Seed(t, repo, task.Record{Title: "Fix it", Status: task.Assigned, Owner: &owner.ID, CreatedBy: &creator.ID})
			c := tasktest.Client(t, repo, "w1:p3",
				tasktest.Get("w1:p3", worker),
				tasktest.Get("w1:p1", boss),
				tasktest.Get("w1:p3", worker),
				herdrscript.Call{Method: "agent.prompt", Err: tc.remote},
			)
			out := run(context.Background(), c, Options{ID: id, Summary: "all done", SummarySet: true}, strings.NewReader(""), "m-0a1b2c")
			r := tasktest.Load(t, repo, id)
			n := r.CompletionNotification
			if out.Status != tc.status || out.Error == nil || out.Error.Code != tc.remote.Code || out.Error.Phase != "agent.prompt" || r.Status != task.Completed || n == nil || n.Error == nil || n.Uncertain != tc.uncertain || n.DeliveredAt != nil {
				t.Fatalf("out=%+v record=%+v", out, r)
			}
			var b bytes.Buffer
			out.Write(&b, false, Render)
			if tc.uncertain && !strings.Contains(b.String(), "notification outcome is unknown") {
				t.Fatalf("%q", b.String())
			}
			if !tc.uncertain && !strings.Contains(b.String(), "creator was not notified") {
				t.Fatalf("%q", b.String())
			}
		})
	}
}

func TestNotificationOutcomeSurvivesLaterTaskState(t *testing.T) {
	for _, status := range []string{task.Verified, task.Cancelled} {
		t.Run(status, func(t *testing.T) {
			repo := identitytest.Repository(t)
			creator := tasktest.Register(t, repo, boss)
			owner := tasktest.Register(t, repo, worker)
			id := tasktest.Seed(t, repo, task.Record{Title: "Fix it", Status: task.Assigned, Owner: &owner.ID, CreatedBy: &creator.ID})
			c := tasktest.Client(t, repo, "w1:p3",
				tasktest.Get("w1:p3", worker),
				tasktest.Get("w1:p1", boss),
				tasktest.Get("w1:p3", worker),
				herdrscript.Call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: boss.Agent}, Before: func() {
					_, err := task.Update(mustStore(t, repo), id, func(r *task.Record) error {
						r.Status = status
						if status == task.Verified {
							r.VerifiedAt = task.Now()
						} else {
							r.CancelledAt = task.Now()
						}
						return nil
					})
					if err != nil {
						t.Fatal(err)
					}
				}},
			)
			out := run(context.Background(), c, Options{ID: id, Summary: "all done", SummarySet: true}, strings.NewReader(""), "m-0a1b2c")
			r := tasktest.Load(t, repo, id)
			if out.Error != nil || r.Status != status || r.CompletionNotification == nil || r.CompletionNotification.DeliveredAt == nil {
				t.Fatalf("out=%+v record=%+v", out, r)
			}
		})
	}
}

func mustStore(t *testing.T, repo string) *state.Store {
	t.Helper()
	s, err := task.Existing(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return s
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
