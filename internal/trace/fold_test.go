package trace

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/messaging"
)

func at(second int) time.Time {
	return time.Date(2026, 8, 5, 20, 41, second, 0, time.UTC)
}

func accepted(value bool) *bool { return &value }

// The whole point of folding is correlation: the ledger names a wake's
// recipient once, and every later line about that wake has to inherit it.
func TestApplyCorrelatesAMessageWakeRoundTrip(t *testing.T) {
	t.Parallel()

	entries := []messaging.LedgerEntry{
		{Type: "message_created", At: at(9), MessageID: "m-9f2c", Sender: "coord", Recipient: "impl-worker",
			RecipientPane: "%12", Body: "Please rerun the build"},
		{Type: "wake_requested", At: at(9), WakeID: "w-31a4", WakeKind: "message", TaskID: "m-9f2c",
			Recipient: "impl-worker", RecipientPane: "%12", Body: "[Fledge wake] ..."},
		{Type: "delivery_attempt", At: at(10), MessageID: "m-9f2c"},
		{Type: "wake_attempt", At: at(10), WakeID: "w-31a4"},
		{Type: "delivery_outcome", At: at(11), MessageID: "m-9f2c", Accepted: accepted(true)},
		{Type: "wake_outcome", At: at(11), WakeID: "w-31a4", Accepted: accepted(true)},
	}
	want := []Record{
		{At: at(9), Kind: "message", Origin: "coord", Target: "impl-worker", Actor: "coord",
			Pane: "%12", Ref: "m-9f2c", Body: "Please rerun the build"},
		{At: at(9), Kind: "wake.queued", Origin: "ledger", Target: "impl-worker", Pane: "%12",
			Ref: "w-31a4", Rel: "m-9f2c", Note: "kind=message ref=m-9f2c"},
		{At: at(10), Kind: "delivery.attempt", Origin: "dispatcher", Target: "impl-worker", Ref: "m-9f2c"},
		{At: at(10), Kind: "wake.attempt", Origin: "dispatcher", Target: "impl-worker", Pane: "%12", Ref: "w-31a4"},
		{At: at(11), Kind: "delivery.ok", Origin: "dispatcher", Target: "impl-worker", Ref: "m-9f2c", Note: "delivered"},
		{At: at(11), Kind: "wake.ok", Origin: "dispatcher", Target: "impl-worker", Pane: "%12", Ref: "w-31a4", Note: "delivered"},
	}
	state := NewState()
	for index, entry := range entries {
		record, ok := state.Apply(entry)
		if !ok {
			t.Fatalf("Apply(%s) produced no record", entry.Type)
		}
		if record != want[index] {
			t.Fatalf("Apply(%s) =\n %#v\nwant\n %#v", entry.Type, record, want[index])
		}
	}
}

func TestApplyRendersTasksAgentsAndFailures(t *testing.T) {
	t.Parallel()

	state := NewState()
	if _, ok := state.Apply(messaging.LedgerEntry{Type: "task_assigned", At: at(12), TaskID: "t-77bd",
		Assignee: "impl-worker", Assigner: "coord", Description: "Add fswatch coverage",
		TaskStatus: messaging.TaskActive, Actor: "coord"}); !ok {
		t.Fatal("task_assigned produced no record")
	}
	tests := []struct {
		name  string
		entry messaging.LedgerEntry
		want  Record
	}{
		{
			name: "progress carries its actor and no target",
			entry: messaging.LedgerEntry{Type: "task_progress", At: at(13), TaskID: "t-77bd",
				TaskStatus: messaging.TaskActive, Detail: "build green", Actor: "impl-worker"},
			want: Record{At: at(13), Kind: "task.progress", Origin: "impl-worker", Actor: "impl-worker",
				Ref: "t-77bd", Status: "active", Body: "build green"},
		},
		{
			name: "an actorless transition falls back to the remembered assignee",
			entry: messaging.LedgerEntry{Type: "task_needs_decision", At: at(15), TaskID: "t-77bd",
				TaskStatus: messaging.TaskNeedsDecision, Detail: "which flag?"},
			want: Record{At: at(15), Kind: "task.needs-decision", Origin: "impl-worker", Target: "coord",
				Ref: "t-77bd", Status: "needs-decision", Body: "which flag?"},
		},
		{
			name: "a privileged transition names the caller, not the assignee",
			entry: messaging.LedgerEntry{Type: "task_completed", At: at(14), TaskID: "t-77bd",
				TaskStatus: messaging.TaskCompleted, Detail: "done", Actor: "orchestrator"},
			want: Record{At: at(14), Kind: "task.completed", Origin: "orchestrator", Target: "impl-worker",
				Actor: "orchestrator", Ref: "t-77bd", Status: "completed", Body: "done"},
		},
		{
			name: "agent status keeps the raw Herdr status",
			entry: messaging.LedgerEntry{Type: "agent_status_changed", At: at(16), AgentName: "impl-worker",
				PaneID: "%12", Detail: "idle"},
			want: Record{At: at(16), Kind: "agent.status", Origin: "impl-worker", Pane: "%12", Status: "idle"},
		},
		{
			name:  "agent stop",
			entry: messaging.LedgerEntry{Type: "agent_stopped", At: at(20), AgentName: "impl-worker", PaneID: "%12"},
			want:  Record{At: at(20), Kind: "agent.stop", Origin: "impl-worker", Pane: "%12"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record, ok := state.Apply(test.entry)
			if !ok {
				t.Fatalf("Apply(%s) produced no record", test.entry.Type)
			}
			if record != test.want {
				t.Fatalf("Apply(%s) =\n %#v\nwant\n %#v", test.entry.Type, record, test.want)
			}
		})
	}
}

func TestApplyReportsAFailedWakeWithItsReason(t *testing.T) {
	t.Parallel()

	state := NewState()
	state.Apply(messaging.LedgerEntry{Type: "wake_requested", At: at(9), WakeID: "w-31a4", WakeKind: "task-assigned",
		TaskID: "t-77bd", Recipient: "impl-worker", RecipientPane: "%12", Body: "task"})
	record, ok := state.Apply(messaging.LedgerEntry{Type: "wake_outcome", At: at(10), WakeID: "w-31a4",
		Accepted: accepted(false), Detail: "pane is gone"})
	if !ok {
		t.Fatal("wake_outcome produced no record")
	}
	want := Record{At: at(10), Kind: "wake.failed", Origin: "dispatcher", Target: "impl-worker",
		Pane: "%12", Ref: "w-31a4", Note: "pane is gone"}
	if record != want {
		t.Fatalf("record =\n %#v\nwant\n %#v", record, want)
	}
}

func TestApplyEvictsTerminalCorrelationState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resource string
		seed     messaging.LedgerEntry
		terminal messaging.LedgerEntry
		want     Record
	}{
		{
			name: "successful delivery", resource: "message",
			seed:     messaging.LedgerEntry{Type: "message_created", MessageID: "m-ok", Sender: "coord", Recipient: "worker"},
			terminal: messaging.LedgerEntry{Type: "delivery_outcome", MessageID: "m-ok", Accepted: accepted(true)},
			want:     Record{Kind: "delivery.ok", Origin: "dispatcher", Target: "worker", Ref: "m-ok", Note: "delivered"},
		},
		{
			name: "failed delivery", resource: "message",
			seed:     messaging.LedgerEntry{Type: "message_created", MessageID: "m-failed", Sender: "coord", Recipient: "worker"},
			terminal: messaging.LedgerEntry{Type: "delivery_outcome", MessageID: "m-failed", Accepted: accepted(false), Detail: "pane is gone"},
			want:     Record{Kind: "delivery.failed", Origin: "dispatcher", Target: "worker", Ref: "m-failed", Note: "pane is gone"},
		},
		{
			name: "acknowledged message", resource: "message",
			seed:     messaging.LedgerEntry{Type: "message_created", MessageID: "m-ack", Sender: "coord", Recipient: "worker"},
			terminal: messaging.LedgerEntry{Type: "acknowledged", MessageID: "m-ack"},
			want:     Record{Kind: "delivery.ack", Origin: "dispatcher", Target: "worker", Ref: "m-ack"},
		},
		{
			name: "successful wake", resource: "wake",
			seed:     messaging.LedgerEntry{Type: "wake_requested", WakeID: "w-ok", Recipient: "worker", RecipientPane: "%12"},
			terminal: messaging.LedgerEntry{Type: "wake_outcome", WakeID: "w-ok", Accepted: accepted(true)},
			want:     Record{Kind: "wake.ok", Origin: "dispatcher", Target: "worker", Pane: "%12", Ref: "w-ok", Note: "delivered"},
		},
		{
			name: "failed wake", resource: "wake",
			seed:     messaging.LedgerEntry{Type: "wake_requested", WakeID: "w-failed", Recipient: "worker", RecipientPane: "%12"},
			terminal: messaging.LedgerEntry{Type: "wake_outcome", WakeID: "w-failed", Accepted: accepted(false), Detail: "pane is gone"},
			want:     Record{Kind: "wake.failed", Origin: "dispatcher", Target: "worker", Pane: "%12", Ref: "w-failed", Note: "pane is gone"},
		},
	}
	for _, terminal := range []struct {
		typeName string
		status   messaging.TaskStatus
	}{
		{"task_completed", messaging.TaskCompleted},
		{"task_failed", messaging.TaskFailed},
		{"task_canceled", messaging.TaskCanceled},
		{"task_orphaned", messaging.TaskOrphaned},
	} {
		tests = append(tests, struct {
			name     string
			resource string
			seed     messaging.LedgerEntry
			terminal messaging.LedgerEntry
			want     Record
		}{
			name: terminal.typeName, resource: "task",
			seed:     messaging.LedgerEntry{Type: "task_assigned", TaskID: "t-terminal", Assignee: "worker", Assigner: "coord"},
			terminal: messaging.LedgerEntry{Type: terminal.typeName, TaskID: "t-terminal", TaskStatus: terminal.status, Actor: "worker"},
			want: Record{Kind: kindOf(terminal.typeName), Origin: "worker", Target: "coord", Actor: "worker",
				Ref: "t-terminal", Status: string(terminal.status)},
		})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := NewState()
			if _, ok := state.Apply(test.seed); !ok || !correlationPresent(state, test.resource) {
				t.Fatalf("seed did not create %s correlation", test.resource)
			}
			record, ok := state.Apply(test.terminal)
			if !ok || record != test.want {
				t.Fatalf("terminal record =\n %#v, %v\nwant\n %#v, true", record, ok, test.want)
			}
			if correlationPresent(state, test.resource) {
				t.Fatalf("terminal %s retained correlation state", test.terminal.Type)
			}
		})
	}
}

func correlationPresent(state *State, resource string) bool {
	switch resource {
	case "message":
		return len(state.messages) != 0
	case "wake":
		return len(state.wakes) != 0
	case "task":
		return len(state.tasks) != 0
	default:
		return false
	}
}

// An event type this Fledge has no rendering for is skipped, never fatal.
func TestApplySkipsUnknownEventTypes(t *testing.T) {
	t.Parallel()

	if _, ok := NewState().Apply(messaging.LedgerEntry{Type: "invented_later", At: at(1)}); ok {
		t.Fatal("Apply rendered an unknown event type")
	}
}

// Seeding is what stops a restarted dispatcher replaying the whole session as
// if it had just happened, while still learning who the earlier lines named.
func TestSeedBuildsCorrelationWithoutReplayingHistory(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "events.jsonl")
	history := `{"version":1,"type":"wake_requested","at":"2026-08-05T20:41:09Z","session_id":"s","wake_id":"w-31a4","wake_kind":"message","task_id":"m-9f2c","recipient":"impl-worker","recipient_pane":"%12","body":"x"}` + "\n"
	if err := os.WriteFile(path, []byte(history), 0o600); err != nil {
		t.Fatal(err)
	}
	state, offset, err := Seed(path)
	if err != nil {
		t.Fatal(err)
	}
	if offset != int64(len(history)) {
		t.Fatalf("offset = %d, want %d", offset, len(history))
	}
	record, ok := state.Apply(messaging.LedgerEntry{Type: "wake_outcome", At: at(10), WakeID: "w-31a4", Accepted: accepted(true)})
	if !ok || record.Target != "impl-worker" || record.Pane != "%12" {
		t.Fatalf("record = %#v, %v", record, ok)
	}
}

func TestSeedOnAnAbsentLedgerStartsEmpty(t *testing.T) {
	t.Parallel()

	state, offset, err := Seed(filepath.Join(t.TempDir(), "missing.jsonl"))
	if err != nil || state == nil || offset != 0 {
		t.Fatalf("Seed(missing) = %v, %d, %v", state, offset, err)
	}
}
