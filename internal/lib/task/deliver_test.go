package task_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestAttemptState(t *testing.T) {
	for _, tc := range []struct {
		attempt task.Attempt
		want    string
	}{
		{task.Attempt{}, "outcome unknown"},
		{task.Attempt{DeliveredAt: tasktest.Ptr("2026-01-01T00:00:00Z")}, "delivered 2026-01-01T00:00:00Z"},
		{task.Attempt{Error: tasktest.Ptr("timeout: lost"), Uncertain: true}, "outcome unknown: timeout: lost"},
		{task.Attempt{Error: tasktest.Ptr("agent_not_found: gone")}, "failed: agent_not_found: gone"},
	} {
		if got := tc.attempt.State(); got != tc.want {
			t.Errorf("%+v: got %q want %q", tc.attempt, got, tc.want)
		}
	}
}

// The embedded attempt keeps each record's stored JSON byte for byte.
func TestAttemptKeepsStoredJSON(t *testing.T) {
	d, _ := json.Marshal(task.Delivery{MessageID: "m-0a1b2c", Pane: "w1:p3", Attempt: task.Attempt{Error: tasktest.Ptr("e"), Uncertain: true}})
	n, _ := json.Marshal(task.CompletionNotification{Recipient: "bbbbbbbb", MessageID: "m-abcdef", Pane: tasktest.Ptr("w1:p1"), Attempt: task.Attempt{DeliveredAt: tasktest.Ptr("t")}})
	if string(d) != `{"message_id":"m-0a1b2c","pane":"w1:p3","delivered_at":null,"error":"e","uncertain":true}` ||
		string(n) != `{"recipient":"bbbbbbbb","message_id":"m-abcdef","pane":"w1:p1","delivered_at":"t","error":null,"uncertain":false}` {
		t.Fatalf("%s\n%s", d, n)
	}
}

// seeded stores an assigned task whose delivery to w1:p3 is pending.
func seeded(t *testing.T) (string, string) {
	repo := identitytest.Repository(t)
	return repo, tasktest.Seed(t, repo, task.Record{Title: "t", Status: task.Assigned, Delivery: &task.Delivery{MessageID: "m-0a1b2c", Pane: "w1:p3"}})
}

// pending locates the seeded delivery, as a caller's attempt function does.
func pending(r *task.Record) (*task.Attempt, error) {
	if r.Delivery == nil || r.Delivery.MessageID != "m-0a1b2c" {
		return nil, &herdr.Error{Code: "task_state_changed", Message: "changed"}
	}
	return &r.Delivery.Attempt, nil
}

func deliver(t *testing.T, repo, id string, prompt herdrscript.Call, attempt func(*task.Record) (*task.Attempt, error)) (libagent.Outcome, task.Record, bool) {
	t.Helper()
	c := tasktest.Client(t, repo, "", prompt)
	s, err := task.Existing(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	out := libagent.Outcome{Operation: "task.test", Status: "success", Effects: []libagent.Effect{}}
	r, ok := task.Deliver(context.Background(), c, s, &out, id, "w1:p3", "m-0a1b2c", "body", attempt)
	return out, r, ok
}

func TestDeliverRecordsDelivery(t *testing.T) {
	repo, id := seeded(t)
	prompt := herdrscript.Call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: tasktest.Agent("w1:p3", "term_w", "worker").Agent}}
	out, r, ok := deliver(t, repo, id, prompt, pending)
	want := []libagent.Effect{{Action: "submitted", Kind: "message", ID: "w1:p3"}, {Action: "updated", Kind: "task", ID: id}}
	if !ok || out.Error != nil || !reflect.DeepEqual(out.Effects, want) || r.Delivery.DeliveredAt == nil || r.Delivery.Error != nil {
		t.Fatalf("%v %+v %+v", ok, out, r.Delivery)
	}
	if stored := tasktest.Load(t, repo, id); !reflect.DeepEqual(stored, r) {
		t.Fatalf("stored %+v, returned %+v", stored, r)
	}
}

func TestDeliverRecordsFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		err       error
		uncertain bool
		status    string
	}{
		"failed":    {&herdr.Error{Code: "agent_not_found", Message: "gone"}, false, "partial"},
		"uncertain": {&herdr.Error{Code: "timeout", Message: "lost", Uncertain: true}, true, "unknown"},
		"local":     {errors.New("socket closed"), false, "partial"},
	} {
		t.Run(name, func(t *testing.T) {
			repo, id := seeded(t)
			out, r, ok := deliver(t, repo, id, herdrscript.Call{Method: "agent.prompt", Err: tc.err}, pending)
			d := r.Delivery
			if !ok || d.DeliveredAt != nil || d.Error == nil || *d.Error != tc.err.Error() || d.Uncertain != tc.uncertain {
				t.Fatalf("%v %+v", ok, d)
			}
			want := []libagent.Effect{{Action: "updated", Kind: "task", ID: id}}
			if out.Status != tc.status || out.Error.Phase != "agent.prompt" || !reflect.DeepEqual(out.Effects, want) {
				t.Fatalf("%+v %+v", out, out.Error)
			}
		})
	}
}

// When the task no longer holds this attempt, nothing is recorded and the
// lookup failure is located at phase task after the submitted message.
func TestDeliverLookupFailure(t *testing.T) {
	repo, id := seeded(t)
	prompt := herdrscript.Call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: tasktest.Agent("w1:p3", "term_w", "worker").Agent}}
	changed := func(*task.Record) (*task.Attempt, error) {
		return nil, &herdr.Error{Code: "task_state_changed", Message: "changed"}
	}
	out, _, ok := deliver(t, repo, id, prompt, changed)
	want := []libagent.Effect{{Action: "submitted", Kind: "message", ID: "w1:p3"}}
	if ok || out.Status != "partial" || out.Error.Code != "task_state_changed" || out.Error.Phase != "task" || !reflect.DeepEqual(out.Effects, want) {
		t.Fatalf("%v %+v %+v", ok, out, out.Error)
	}
	if d := tasktest.Load(t, repo, id).Delivery; d.DeliveredAt != nil || d.Error != nil {
		t.Fatalf("recorded %+v", d)
	}
}
