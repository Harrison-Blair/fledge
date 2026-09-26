package complete

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
	"github.com/Harrison-Blair/fledge/internal/lib/usage"
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

// A notification replaced while the message was in flight is left as it is;
// the outcome fails at phase task and says it could not be recorded.
func TestNotificationNotRecordedAfterChange(t *testing.T) {
	repo := identitytest.Repository(t)
	creator := tasktest.Register(t, repo, boss)
	owner := tasktest.Register(t, repo, worker)
	id := tasktest.Seed(t, repo, task.Record{Title: "Fix it", Status: task.Assigned, Owner: &owner.ID, CreatedBy: &creator.ID})
	c := tasktest.Client(t, repo, "w1:p3",
		tasktest.Get("w1:p3", worker),
		tasktest.Get("w1:p1", boss),
		tasktest.Get("w1:p3", worker),
		herdrscript.Call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: boss.Agent}, Before: func() {
			if _, err := task.Update(mustStore(t, repo), id, func(r *task.Record) error { r.CompletionNotification.MessageID = "m-ffffff"; return nil }); err != nil {
				t.Fatal(err)
			}
		}},
	)
	out := run(context.Background(), c, Options{ID: id, Summary: "all done", SummarySet: true}, strings.NewReader(""), "m-0a1b2c")
	n := tasktest.Load(t, repo, id).CompletionNotification
	if out.Status != "partial" || out.Error == nil || out.Error.Code != "task_state_changed" || out.Error.Phase != "task" || n.MessageID != "m-ffffff" || n.DeliveredAt != nil || n.Error != nil {
		t.Fatalf("%+v %+v %+v", out, out.Error, n)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || !strings.HasSuffix(b.String(), "\nTask "+id+" is completed; the notification outcome could not be recorded.\n") {
		t.Fatalf("%q %v", b.String(), err)
	}
}

// Tests never read real harness stores unless they inject a reader.
func TestMain(m *testing.M) {
	readUsage = func(context.Context, string, usage.Ref, usage.Window) usage.Summary {
		return usage.Summary{Basis: usage.Unavailable, Reason: "no reader injected"}
	}
	os.Exit(m.Run())
}

// inject replaces the usage reader for one test and returns its calls.
func inject(t *testing.T, s usage.Summary) *[]usage.Ref {
	t.Helper()
	old := readUsage
	t.Cleanup(func() { readUsage = old })
	refs := &[]usage.Ref{}
	readUsage = func(_ context.Context, kind string, ref usage.Ref, w usage.Window) usage.Summary {
		*refs = append(*refs, ref)
		return s
	}
	return refs
}

func withSession(a herdr.AgentResult, value string) herdr.AgentResult {
	a.Agent = identitytest.WithSession(a.Agent, value)
	return a
}

func TestCompletionRecordsWorkerUsageAndSessionRef(t *testing.T) {
	refs := inject(t, usage.Summary{Turns: 14, Tokens: usage.Tokens{Input: 1200, Output: 18400}, Models: []string{"claude-opus-5"}, Basis: usage.Measured})
	repo := identitytest.Repository(t)
	owner := tasktest.Register(t, repo, worker)
	assignedAt := "2026-09-23T10:00:00Z"
	id := tasktest.Seed(t, repo, task.Record{Title: "t", Status: task.Assigned, Owner: &owner.ID, CreatedAt: "2026-09-23T09:00:00Z", AssignedAt: &assignedAt})
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p3", tasktest.Get("w1:p3", withSession(worker, "sess-1"))), Options{ID: id, Summary: "done", SummarySet: true}, strings.NewReader(""))
	r := tasktest.Load(t, repo, id)
	if out.Error != nil || r.Status != task.Completed || !reflect.DeepEqual(out.Result, r) {
		t.Fatalf("%+v %+v", out.Error, r)
	}
	if len(*refs) != 1 || (*refs)[0].Value != "sess-1" || (*refs)[0].Kind != "id" {
		t.Fatalf("reader refs %+v", *refs)
	}
	w := r.Usage.Worker
	if w == nil || r.Usage.Verifier != nil || *w.AgentID != owner.ID || *w.Harness != "claude" || *w.Session != (task.UsageSession{Kind: "id", Value: "sess-1"}) ||
		w.Window != (task.UsageWindow{From: assignedAt, To: *r.CompletedAt}) || w.Turns != 14 || w.Basis != usage.Measured || w.Reason != nil || w.CollectedAt == "" {
		t.Fatalf("%+v", w)
	}
	if rec := loadAgent(t, repo, owner.ID); rec.NativeSession == nil || rec.NativeSession.Value != "sess-1" {
		t.Fatalf("session ref not captured: %+v", rec.NativeSession)
	}
	if !slices.Contains(out.Effects, libagent.Effect{Action: "updated", Kind: "native_session", ID: owner.ID}) {
		t.Fatalf("%+v", out.Effects)
	}
}

// A failing reader, a failing session write, or an unassigned window never
// stops completion; each is recorded on the snapshot or as a warning.
func TestCompletionSucceedsWhenUsageCollectionFails(t *testing.T) {
	inject(t, usage.Summary{Basis: usage.Unavailable, Reason: "no claude session file for id sess-1"})
	old := observeSession
	t.Cleanup(func() { observeSession = old })
	observeSession = func(*state.Store, string, herdr.AgentSession, time.Time) (identity.Record, bool, error) {
		return identity.Record{}, false, errors.New("read-only store")
	}
	repo := identitytest.Repository(t)
	owner := tasktest.Register(t, repo, worker)
	id := tasktest.Seed(t, repo, task.Record{Title: "t", Status: task.Assigned, Owner: &owner.ID, CreatedAt: "2026-09-23T09:00:00Z"})
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p3", tasktest.Get("w1:p3", withSession(worker, "sess-1"))), Options{ID: id, Summary: "done", SummarySet: true}, strings.NewReader(""))
	r := tasktest.Load(t, repo, id)
	if out.Error != nil || out.Status != "success" || r.Status != task.Completed {
		t.Fatalf("%+v %+v", out.Error, r)
	}
	if !slices.Contains(out.Effects, libagent.Effect{Action: "warning", Kind: "native_session", ID: owner.ID}) {
		t.Fatalf("%+v", out.Effects)
	}
	w := r.Usage.Worker
	want := "task was never assigned; window starts at created_at; no claude session file for id sess-1"
	if w == nil || w.Basis != usage.Unavailable || w.Reason == nil || *w.Reason != want || w.Window.From != "2026-09-23T09:00:00Z" || w.Session.Value != "sess-1" {
		t.Fatalf("%+v %v", w, w.Reason)
	}
}

// A forced completion on the owner's behalf snapshots the owner from its
// persisted ref, and says so.
func TestForcedCompletionSnapshotsTheOwner(t *testing.T) {
	refs := inject(t, usage.Summary{Basis: usage.Measured})
	repo, id := setup(t, task.Assigned)
	owner := *tasktest.Load(t, repo, id).Owner
	if _, _, err := identity.ObserveSession(mustStore(t, repo), owner, *identitytest.WithSession(herdr.AgentDetails{}, "persisted").AgentSession, time.Now()); err != nil {
		t.Fatal(err)
	}
	for label, c := range map[string]libagent.Client{
		"other agent": tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss)),
		"outside":     tasktest.Client(t, repo, ""),
	} {
		t.Run(label, func(t *testing.T) {
			if _, err := task.Update(mustStore(t, repo), id, func(r *task.Record) error { r.Status, r.Usage = task.Assigned, nil; return nil }); err != nil {
				t.Fatal(err)
			}
			*refs = nil
			out := Run(context.Background(), c, Options{ID: id, Summary: "x", SummarySet: true, Force: true}, strings.NewReader(""))
			w := tasktest.Load(t, repo, id).Usage.Worker
			if out.Error != nil || w == nil || *w.AgentID != owner || w.Session.Value != "persisted" || len(*refs) != 1 || w.Reason == nil || !strings.Contains(*w.Reason, "completed with --force by ") || !strings.Contains(*w.Reason, "on the owner's behalf") {
				t.Fatalf("%+v %+v %+v", out.Error, w, *refs)
			}
		})
	}
}

// An owner without any session ref, completing from outside Herdr, is unavailable.
func TestCompletionOutsideHerdrWithoutRefIsUnavailable(t *testing.T) {
	refs := inject(t, usage.Summary{Basis: usage.Measured})
	repo, id := setup(t, task.Assigned)
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id, Summary: "x", SummarySet: true, Force: true}, strings.NewReader(""))
	r := tasktest.Load(t, repo, id)
	if out.Error != nil || r.Status != task.Completed || len(*refs) != 0 || r.Usage.Worker.Basis != usage.Unavailable || !strings.HasSuffix(*r.Usage.Worker.Reason, "no native session ref observed") {
		t.Fatalf("%+v %+v", out.Error, r.Usage.Worker)
	}
}

// A worker snapshot already on the record is never replaced.
func TestWorkerSnapshotIsWrittenOnce(t *testing.T) {
	inject(t, usage.Summary{Basis: usage.Measured, Turns: 2})
	repo := identitytest.Repository(t)
	owner := tasktest.Register(t, repo, worker)
	earlier := &task.UsageSnapshot{Basis: usage.Measured, Turns: 1}
	id := tasktest.Seed(t, repo, task.Record{Title: "t", Status: task.Assigned, Owner: &owner.ID, Usage: &task.Usage{Worker: earlier}})
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p3", tasktest.Get("w1:p3", worker)), Options{ID: id, Summary: "done", SummarySet: true}, strings.NewReader(""))
	r := tasktest.Load(t, repo, id)
	if out.Error != nil || r.Status != task.Completed || !reflect.DeepEqual(r.Usage.Worker, earlier) {
		t.Fatalf("%+v %+v", out.Error, r.Usage.Worker)
	}
}

func loadAgent(t *testing.T, repo, id string) identity.Record {
	t.Helper()
	var rec identity.Record
	if err := mustStore(t, repo).Get(identity.Kind, id, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}
