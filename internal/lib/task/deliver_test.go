package task_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
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

// seeded stores an assigned task whose delivery to w1:p3 is pending: no
// inbox id is recorded yet.
func seeded(t *testing.T) (string, string) {
	repo := identitytest.Repository(t)
	return repo, tasktest.Seed(t, repo, task.Record{Title: "Fix it", Status: task.Assigned, Delivery: &task.Delivery{Pane: "w1:p3"}})
}

// pending locates the seeded delivery, as a caller's attempt function does,
// and records the inbox id on it.
func pending(r *task.Record, messageID string) (*task.Attempt, error) {
	if r.Delivery == nil || r.Delivery.MessageID != "" {
		return nil, &cli.Error{Code: "task_state_changed", Message: "changed"}
	}
	r.Delivery.MessageID = messageID
	return &r.Delivery.Attempt, nil
}

// recorder keeps the text of every prompt.
type recorder struct {
	inner   libagent.API
	prompts []string
}

func (r *recorder) Call(ctx context.Context, method string, params, result any) error {
	if method == "agent.prompt" {
		r.prompts = append(r.prompts, params.(map[string]any)["text"].(string))
	}
	return r.inner.Call(ctx, method, params, result)
}

func deliver(t *testing.T, repo, id string, prompt herdrscript.Call, attempt func(*task.Record, string) (*task.Attempt, error)) (cli.Outcome, task.Record, bool, *recorder) {
	t.Helper()
	c := tasktest.Client(t, repo, "", prompt)
	rec := &recorder{inner: c.API}
	c.API = rec
	s, err := task.Existing(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	out := cli.Outcome{Operation: "task.test", Status: "success", Effects: []cli.Effect{}}
	msg := inbox.Record{Kind: inbox.Assignment, From: inbox.From{Pane: tasktest.Ptr("w1:p1")}, To: "bbbbbbbb", Task: &id, Body: "SECRET brief"}
	r, ok := task.Deliver(context.Background(), c, s, &out, msg, "w1:p3", "Fix it", attempt)
	return out, r, ok, rec
}

// The body is stored and only the doorbell is typed; the task's delivery
// names the inbox record.
func TestDeliverStoresBodyAndRingsDoorbell(t *testing.T) {
	repo, id := seeded(t)
	prompt := herdrscript.Call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: tasktest.Agent("w1:p3", "term_w", "worker").Agent}}
	out, r, ok, rec := deliver(t, repo, id, prompt, pending)
	if !ok || out.Error != nil || r.Delivery.DeliveredAt == nil || r.Delivery.Error != nil || !state.ValidID(r.Delivery.MessageID) {
		t.Fatalf("%v %+v %+v", ok, out, r.Delivery)
	}
	m, err := inbox.Get(tasktest.Store(t, repo), r.Delivery.MessageID)
	if err != nil || m.Body != "SECRET brief" || inbox.State(m) != "rung" || *m.Task != id {
		t.Fatalf("%+v %v", m, err)
	}
	if len(rec.prompts) != 1 || rec.prompts[0] != inbox.Doorbell(m, "Fix it") || strings.Contains(rec.prompts[0], "SECRET") {
		t.Fatalf("%q", rec.prompts)
	}
	want := []cli.Effect{{Action: "created", Kind: "inbox", ID: m.ID}, {Action: "submitted", Kind: "message", ID: "w1:p3"}, {Action: "updated", Kind: "task", ID: id}}
	if !reflect.DeepEqual(out.Effects, want) {
		t.Fatalf("%+v", out.Effects)
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
		"blocked":   {&herdr.Error{Code: "agent_blocked", Message: "dialog"}, false, "partial"},
		"uncertain": {&herdr.Error{Code: "timeout", Message: "lost", Uncertain: true}, true, "unknown"},
		"local":     {errors.New("socket closed"), false, "partial"},
	} {
		t.Run(name, func(t *testing.T) {
			repo, id := seeded(t)
			out, r, ok, _ := deliver(t, repo, id, herdrscript.Call{Method: "agent.prompt", Err: tc.err}, pending)
			d := r.Delivery
			if !ok || d.DeliveredAt != nil || d.Error == nil || *d.Error != tc.err.Error() || d.Uncertain != tc.uncertain || d.MessageID == "" {
				t.Fatalf("%v %+v", ok, d)
			}
			m, _ := inbox.Get(tasktest.Store(t, repo), d.MessageID)
			if inbox.State(m) != "queued" || len(m.Rings) != 1 || m.Rings[0].OK {
				t.Fatalf("%+v", m)
			}
			want := []cli.Effect{{Action: "created", Kind: "inbox", ID: d.MessageID}, {Action: "updated", Kind: "task", ID: id}}
			if out.Status != tc.status || out.Error.Phase != "agent.prompt" || !reflect.DeepEqual(out.Effects, want) {
				t.Fatalf("%+v %+v", out, out.Error)
			}
		})
	}
}

// When the task no longer holds this attempt, nothing is recorded on it and
// the lookup failure is located at phase task after the doorbell.
func TestDeliverLookupFailure(t *testing.T) {
	repo, id := seeded(t)
	prompt := herdrscript.Call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: tasktest.Agent("w1:p3", "term_w", "worker").Agent}}
	changed := func(*task.Record, string) (*task.Attempt, error) {
		return nil, &cli.Error{Code: "task_state_changed", Message: "changed"}
	}
	out, _, ok, _ := deliver(t, repo, id, prompt, changed)
	if ok || out.Status != "partial" || out.Error.Code != "task_state_changed" || out.Error.Phase != "task" || len(out.Effects) != 2 || out.Effects[1] != (cli.Effect{Action: "submitted", Kind: "message", ID: "w1:p3"}) {
		t.Fatalf("%v %+v %+v", ok, out, out.Error)
	}
	if d := tasktest.Load(t, repo, id).Delivery; d.DeliveredAt != nil || d.Error != nil || d.MessageID != "" {
		t.Fatalf("recorded %+v", d)
	}
}
