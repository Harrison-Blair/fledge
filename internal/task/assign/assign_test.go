package assign

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
	"github.com/Harrison-Blair/fledge/internal/task/get"
)

type call = herdrscript.Call

var (
	boss   = tasktest.Agent("w1:p1", "term_boss", "boss")
	worker = tasktest.Agent("w1:p3", "term_worker", "worker")
)

func prompted(a herdr.AgentResult) herdr.AgentResult {
	return herdr.AgentResult{Type: "agent_prompted", Agent: a.Agent}
}

// brief is the exact text assign delivers for task id from boss.
func brief(id string) string {
	return "ᛉ fledge message from boss (w1:p1) · id m-0a1b2c · reply: fledge agent message --name boss\n" +
		"task: " + id + " · title: Fix it · complete with: fledge task complete --id " + id + " --summary \"...\"\n" +
		"do the thing"
}

func seed(t *testing.T, repo string) string {
	return tasktest.Seed(t, repo, task.Record{Title: "Fix it", Brief: "do the thing", Status: task.Created, CreatedAt: "2026-01-01T00:00:00Z"})
}

func TestAssignRecordsOwnerThenDelivery(t *testing.T) {
	repo := identitytest.Repository(t)
	owner := tasktest.Register(t, repo, worker)
	id := seed(t, repo)
	c := tasktest.Client(t, repo, "w1:p1",
		tasktest.Get("worker", worker),
		tasktest.Get("w1:p1", boss),
		call{Method: "agent.prompt", Params: map[string]any{"target": "w1:p3", "text": brief(id)}, Result: prompted(worker)},
	)
	out := run(context.Background(), c, Options{ID: id, Name: "worker"}, "m-0a1b2c")
	if out.Error != nil || out.Status != "success" || out.Operation != "task.assign" {
		t.Fatalf("%+v", out.Error)
	}
	r := tasktest.Load(t, repo, id)
	if !reflect.DeepEqual(out.Result.(Result).Record, r) {
		t.Fatalf("%+v != %+v", out.Result, r)
	}
	if r.Status != task.Assigned || *r.Owner != owner.ID || r.AssignedAt == nil || r.Delivery == nil ||
		r.Delivery.MessageID != "m-0a1b2c" || r.Delivery.Pane != "w1:p3" || r.Delivery.DeliveredAt == nil || r.Delivery.Error != nil {
		t.Fatalf("%+v %+v", r, r.Delivery)
	}
	want := []libagent.Effect{{Action: "updated", Kind: "task", ID: id}, {Action: "submitted", Kind: "message", ID: "w1:p3"}, {Action: "updated", Kind: "task", ID: id}}
	if !reflect.DeepEqual(out.Effects, want) {
		t.Fatalf("%+v", out.Effects)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || b.String() != "Assigned task "+id+" to worker ("+owner.ID+"); brief delivered to w1:p3 as message m-0a1b2c.\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestReassignByAgentID(t *testing.T) {
	repo := identitytest.Repository(t)
	tasktest.Register(t, repo, worker)
	other := tasktest.Agent("w1:p4", "term_other", "other")
	next := tasktest.Register(t, repo, other)
	id := tasktest.Seed(t, repo, task.Record{Title: "Fix it", Brief: "do the thing", Status: task.Assigned, Owner: tasktest.Ptr("00000000"),
		Delivery: &task.Delivery{MessageID: "m-ffffff", Pane: "w1:p3", Error: tasktest.Ptr("old")}})
	c := tasktest.Client(t, repo, "",
		tasktest.Get("w1:p4", other),
		call{Method: "agent.prompt", Result: prompted(other)},
	)
	out := Run(context.Background(), c, Options{ID: id, AgentID: next.ID})
	r := tasktest.Load(t, repo, id)
	if out.Error != nil || *r.Owner != next.ID || r.Delivery.Pane != "w1:p4" || r.Delivery.Error != nil || r.Delivery.MessageID == "m-ffffff" {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}

// A registered terminal Herdr moved to a new pane keeps its record, which
// follows it there.
func TestAssignFollowsMovedTerminal(t *testing.T) {
	repo := identitytest.Repository(t)
	owner := tasktest.Register(t, repo, worker)
	id := seed(t, repo)
	moved := tasktest.Agent("w2:p1", "term_worker", "worker")
	moved.Agent.WorkspaceID = "w2"
	c := tasktest.Client(t, repo, "",
		tasktest.Get("w2:p1", moved),
		call{Method: "agent.prompt", Result: prompted(moved)},
	)
	out := Run(context.Background(), c, Options{ID: id, Pane: "w2:p1"})
	r := tasktest.Load(t, repo, id)
	if out.Error != nil || *r.Owner != owner.ID || r.Delivery.Pane != "w2:p1" {
		t.Fatalf("%+v %+v", out.Error, r)
	}
	s, err := identity.Existing(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if rec, err := identity.Live(s, "term_worker"); err != nil || rec.ID != owner.ID || rec.Pane != "w2:p1" || rec.WorkspaceID != "w2" {
		t.Fatalf("%+v %v", rec, err)
	}
}

func TestAssignRefusesUnregisteredAgent(t *testing.T) {
	repo := identitytest.Repository(t)
	id := seed(t, repo)
	before := tasktest.Load(t, repo, id)
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p3", worker)), Options{ID: id, Pane: "w1:p3"})
	if out.Error == nil || out.Error.Code != "agent_unregistered" || out.Status != "rejected" || !strings.Contains(out.Error.Message, "fledge agent adopt --pane w1:p3") {
		t.Fatalf("%+v", out.Error)
	}
	if r := tasktest.Load(t, repo, id); !reflect.DeepEqual(r, before) {
		t.Fatalf("%+v", r)
	}
}

func TestAssignDeliveryFailureLeavesTaskAssigned(t *testing.T) {
	repo := identitytest.Repository(t)
	owner := tasktest.Register(t, repo, worker)
	id := seed(t, repo)
	c := tasktest.Client(t, repo, "w1:p1",
		tasktest.Get("worker", worker),
		tasktest.Get("w1:p1", boss),
		call{Method: "agent.prompt", Err: &herdr.Error{Code: "agent_blocked", Message: "awaiting approval"}},
	)
	out := run(context.Background(), c, Options{ID: id, Name: "worker"}, "m-0a1b2c")
	if out.Status != "partial" || out.Error == nil || out.Error.Code != "agent_blocked" || out.Error.Phase != "agent.prompt" {
		t.Fatalf("%s %+v", out.Status, out.Error)
	}
	r := tasktest.Load(t, repo, id)
	if r.Status != task.Assigned || *r.Owner != owner.ID || r.Delivery == nil || r.Delivery.DeliveredAt != nil ||
		r.Delivery.Error == nil || !strings.Contains(*r.Delivery.Error, "awaiting approval") || r.Delivery.MessageID != "m-0a1b2c" {
		t.Fatalf("%+v %+v", r, r.Delivery)
	}
	var b bytes.Buffer
	out.Write(&b, false, Render)
	if !strings.Contains(b.String(), "Task "+id+" remains assigned to worker ("+owner.ID+"); the brief was not delivered and will not be retried.") {
		t.Fatalf("%q", b.String())
	}
}

func TestAssignRefusesFinishedTasks(t *testing.T) {
	for _, status := range []string{task.Completed, task.Verified, task.Cancelled} {
		repo := identitytest.Repository(t)
		id := tasktest.Seed(t, repo, task.Record{Title: "t", Status: status})
		out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id, Name: "worker"})
		if out.Error == nil || out.Error.Code != "task_invalid_state" {
			t.Fatalf("%s: %+v", status, out.Error)
		}
	}
}

func TestAssignRejectsInvalidInput(t *testing.T) {
	repo := identitytest.Repository(t)
	for label, o := range map[string]Options{
		"bad task id":  {ID: "xyz", Name: "worker"},
		"no agent":     {ID: "0123abcd"},
		"two agents":   {ID: "0123abcd", Name: "worker", Pane: "w1:p3"},
		"bad agent id": {ID: "0123abcd", AgentID: "nope"},
	} {
		out := Run(context.Background(), tasktest.Client(t, repo, ""), o)
		if out.Error == nil || out.Error.Code != "invalid_input" {
			t.Fatalf("%s: %+v", label, out.Error)
		}
	}
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: "0123abcd", Name: "worker"})
	if out.Error == nil || out.Error.Code != "task_not_found" {
		t.Fatalf("%+v", out.Error)
	}
}

// Two callers assign the same created task at once: the second reaches the
// store lock after the first has assigned it, and must refuse.
func TestConcurrentAssignExactlyOneSucceeds(t *testing.T) {
	repo := identitytest.Repository(t)
	first := tasktest.Register(t, repo, worker)
	other := tasktest.Agent("w1:p4", "term_other", "other")
	tasktest.Register(t, repo, other)
	id := seed(t, repo)
	var winner libagent.Outcome
	loser := tasktest.Client(t, repo, "",
		call{Method: "agent.get", Params: map[string]any{"target": "other"}, Result: other, Before: func() {
			c := tasktest.Client(t, repo, "", tasktest.Get("worker", worker), call{Method: "agent.prompt", Result: prompted(worker)})
			winner = Run(context.Background(), c, Options{ID: id, Name: "worker"})
		}},
	)
	lost := Run(context.Background(), loser, Options{ID: id, Name: "other"})
	if winner.Error != nil || lost.Error == nil || lost.Error.Code != "task_state_changed" || lost.Status != "rejected" {
		t.Fatalf("winner %+v loser %+v", winner.Error, lost.Error)
	}
	if r := tasktest.Load(t, repo, id); *r.Owner != first.ID || r.Delivery.DeliveredAt == nil {
		t.Fatalf("%+v", r)
	}
}

// An uncertain prompt failure may have delivered the brief, so neither the
// outcome, the rendering, nor the record may claim it was not delivered.
func TestAssignUncertainDeliveryIsReportedUnknown(t *testing.T) {
	repo := identitytest.Repository(t)
	owner := tasktest.Register(t, repo, worker)
	id := seed(t, repo)
	c := tasktest.Client(t, repo, "w1:p1",
		tasktest.Get("worker", worker),
		tasktest.Get("w1:p1", boss),
		call{Method: "agent.prompt", Err: &herdr.Error{Code: "transport_error", Message: "connection reset", Uncertain: true}},
	)
	out := run(context.Background(), c, Options{ID: id, Name: "worker"}, "m-0a1b2c")
	if out.Status != "unknown" || out.Error == nil || out.Error.Code != "transport_error" {
		t.Fatalf("%s %+v", out.Status, out.Error)
	}
	var b bytes.Buffer
	out.Write(&b, false, Render)
	if want := "Task " + id + " remains assigned to worker (" + owner.ID + "); the delivery outcome is unknown and will not be retried.\n"; !strings.HasSuffix(b.String(), want) || strings.Contains(b.String(), "not delivered") {
		t.Fatalf("%q", b.String())
	}
	r := tasktest.Load(t, repo, id)
	if r.Status != task.Assigned || r.Delivery == nil || !r.Delivery.Uncertain || r.Delivery.Error == nil || r.Delivery.DeliveredAt != nil {
		t.Fatalf("%+v %+v", r, r.Delivery)
	}
	b.Reset()
	get.Render(&b, get.Run(context.Background(), tasktest.Client(t, repo, ""), get.Options{ID: id}))
	if want := "delivery: message m-0a1b2c to w1:p3, outcome unknown: transport_error: connection reset\n"; !strings.Contains(b.String(), want) || strings.Contains(b.String(), "failed") {
		t.Fatalf("%q", b.String())
	}
}

func TestAssignRefusesAgentWhoseRecordIsAnotherHarness(t *testing.T) {
	repo := identitytest.Repository(t)
	codex := worker
	harness := "codex"
	codex.Agent.Agent = &harness
	tasktest.Register(t, repo, codex)
	id := seed(t, repo)
	out := Run(context.Background(), tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p3", worker)), Options{ID: id, Pane: "w1:p3"})
	if out.Error == nil || out.Error.Code != "agent_unregistered" {
		t.Fatalf("%+v", out.Error)
	}
}

func TestAssignWaitsForPrerequisitesUnlessForced(t *testing.T) {
	repo := identitytest.Repository(t)
	owner := tasktest.Register(t, repo, worker)
	open := tasktest.Seed(t, repo, task.Record{Title: "research", Status: task.Completed})
	done := tasktest.Seed(t, repo, task.Record{Title: "done", Status: task.Verified})
	id := tasktest.Seed(t, repo, task.Record{Title: "Fix it", Brief: "do the thing", Status: task.Created, After: []string{done, open}})
	before := tasktest.Load(t, repo, id)
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{ID: id, Name: "worker"})
	if out.Error == nil || out.Error.Code != "task_dependencies_unmet" || out.Error.Phase != "task" || !strings.Contains(out.Error.Message, open) ||
		strings.Contains(out.Error.Message, done) || !strings.Contains(out.Error.Message, "--force") || !reflect.DeepEqual(tasktest.Load(t, repo, id), before) {
		t.Fatalf("%+v", out.Error)
	}
	c := tasktest.Client(t, repo, "", tasktest.Get("worker", worker), call{Method: "agent.prompt", Result: prompted(worker)})
	out = Run(context.Background(), c, Options{ID: id, Name: "worker", Force: true})
	r := tasktest.Load(t, repo, id)
	if out.Error != nil || r.Status != task.Assigned || *r.Owner != owner.ID || !reflect.DeepEqual(r.UnmetAtAssign, []string{open}) || r.Forced {
		t.Fatalf("%+v %+v", out.Error, r)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || !strings.HasSuffix(b.String(), "Assigned with --force before prerequisites "+open+" were satisfied.\n") {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestAssignWithSatisfiedPrerequisites(t *testing.T) {
	repo := identitytest.Repository(t)
	tasktest.Register(t, repo, worker)
	done := tasktest.Seed(t, repo, task.Record{Title: "done", Status: task.Verified})
	dropped := tasktest.Seed(t, repo, task.Record{Title: "dropped", Status: task.Cancelled})
	id := tasktest.Seed(t, repo, task.Record{Title: "Fix it", Brief: "do the thing", Status: task.Created, After: []string{done, dropped}})
	c := tasktest.Client(t, repo, "", tasktest.Get("worker", worker), call{Method: "agent.prompt", Result: prompted(worker)})
	out := Run(context.Background(), c, Options{ID: id, Name: "worker", Force: true})
	if r := tasktest.Load(t, repo, id); out.Error != nil || r.Status != task.Assigned || r.UnmetAtAssign != nil {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}
