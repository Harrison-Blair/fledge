package trace

import (
	"strings"

	"github.com/Harrison-Blair/fledge/internal/messaging"
)

// dispatcherOrigin marks the records the dispatcher causes rather than
// observes; ledgerOrigin marks the ones the durable log itself produced.
const (
	dispatcherOrigin = "dispatcher"
	ledgerOrigin     = "ledger"
)

type messageRef struct{ sender, recipient, pane string }
type wakeRef struct{ recipient, pane string }
type taskRef struct{ assignee, assigner string }

// State remembers the identities a later ledger event refers to but does not
// repeat, so a delivery outcome can still name the agent it woke.
type State struct {
	messages map[string]messageRef
	wakes    map[string]wakeRef
	tasks    map[string]taskRef
}

// NewState returns correlation state for a ledger that has not been read yet.
func NewState() *State {
	return &State{
		messages: make(map[string]messageRef),
		wakes:    make(map[string]wakeRef),
		tasks:    make(map[string]taskRef),
	}
}

// Apply folds one ledger entry into the trace, returning the record it
// produces. An entry this Fledge has no rendering for reports false, so a
// ledger written by a newer one still reads as far as it can.
func (s *State) Apply(e messaging.LedgerEntry) (Record, bool) {
	record := Record{At: e.At, Kind: kindOf(e.Type)}
	switch e.Type {
	case "session_start":
		record.Ref = e.SessionID
		record.Note = "session=" + e.Session
	case "message_created", "message_replied", "reply_created":
		s.messages[e.MessageID] = messageRef{sender: e.Sender, recipient: e.Recipient, pane: e.RecipientPane}
		record.Origin, record.Target, record.Actor = e.Sender, e.Recipient, e.Sender
		record.Ref, record.Rel, record.Pane, record.Body = e.MessageID, e.ReplyTo, e.RecipientPane, e.Body
	case "delivery_attempt", "delivery_outcome", "acknowledged":
		if e.Type == "delivery_outcome" {
			record.Kind = outcomeKind("delivery", e)
		}
		record.Origin, record.Target = dispatcherOrigin, s.messages[e.MessageID].recipient
		record.Ref = e.MessageID
		record.Note = outcomeNote(e)
		if e.Type != "delivery_attempt" {
			delete(s.messages, e.MessageID)
		}
	case "agent_registered":
		record.Origin, record.Pane = e.AgentName, e.PaneID
		record.Note = "harness=" + e.Harness
	case "agent_stopped":
		record.Origin, record.Pane = e.AgentName, e.PaneID
	case "agent_status_changed":
		record.Origin, record.Pane, record.Status = e.AgentName, e.PaneID, e.Detail
	case "task_assigned":
		s.tasks[e.TaskID] = taskRef{assignee: e.Assignee, assigner: e.Assigner}
		record.Origin, record.Target, record.Actor = e.Assigner, e.Assignee, e.Assigner
		record.Ref, record.Rel, record.Body = e.TaskID, e.ParentTaskID, e.Description
		record.Status = string(e.TaskStatus)
	case "task_progress":
		record.Origin, record.Actor = s.transitionActor(e), e.Actor
		record.Ref, record.Body, record.Status = e.TaskID, e.Detail, string(e.TaskStatus)
	case "task_blocked", "task_needs_decision", "task_resumed", "task_completed",
		"task_failed", "task_canceled", "task_orphaned":
		task := s.tasks[e.TaskID]
		record.Origin, record.Actor = s.transitionActor(e), e.Actor
		// The target is whoever the transition concerns other than the actor: an
		// assignee reports up to its assigner, and anyone else acting on the task
		// is acting on the assignee's work.
		record.Target = task.assignee
		if record.Origin == task.assignee {
			record.Target = task.assigner
		}
		record.Ref, record.Body, record.Status = e.TaskID, e.Detail, string(e.TaskStatus)
		switch e.TaskStatus {
		case messaging.TaskCompleted, messaging.TaskFailed, messaging.TaskCanceled, messaging.TaskOrphaned:
			delete(s.tasks, e.TaskID)
		}
	case "wake_requested":
		s.wakes[e.WakeID] = wakeRef{recipient: e.Recipient, pane: e.RecipientPane}
		record.Origin, record.Target = ledgerOrigin, e.Recipient
		record.Ref, record.Rel, record.Pane = e.WakeID, e.TaskID, e.RecipientPane
		record.Note = "kind=" + e.WakeKind
		if e.TaskID != "" {
			record.Note += " ref=" + e.TaskID
		}
	case "wake_attempt", "wake_outcome":
		if e.Type == "wake_outcome" {
			record.Kind = outcomeKind("wake", e)
		}
		wake := s.wakes[e.WakeID]
		record.Origin, record.Target, record.Pane = dispatcherOrigin, wake.recipient, wake.pane
		record.Ref = e.WakeID
		record.Note = outcomeNote(e)
		if e.Type == "wake_outcome" {
			delete(s.wakes, e.WakeID)
		}
	default:
		return Record{}, false
	}
	return record, true
}

// Seed replays an existing ledger to build correlation state and returns the
// offset at its end, so a restarted dispatcher resumes without re-emitting
// history it did not cause.
func Seed(path string) (*State, int64, error) {
	state := NewState()
	entries, offset, err := Read(path, 0)
	if err != nil {
		return nil, 0, err
	}
	for _, entry := range entries {
		state.Apply(entry)
	}
	return state, offset, nil
}

// transitionActor prefers the caller the store authorized. Older ledger lines
// carry no actor, and there the assignee is the only defensible attribution.
func (s *State) transitionActor(e messaging.LedgerEntry) string {
	if e.Actor != "" {
		return e.Actor
	}
	return s.tasks[e.TaskID].assignee
}

// outcomeKind splits a recorded outcome by what it reported, because a failed
// delivery is the line a reader is scanning for.
func outcomeKind(family string, e messaging.LedgerEntry) string {
	if e.Accepted != nil && !*e.Accepted {
		return family + ".failed"
	}
	return family + ".ok"
}

func outcomeNote(e messaging.LedgerEntry) string {
	if e.Accepted == nil {
		return e.Detail
	}
	if *e.Accepted {
		return "delivered"
	}
	return e.Detail
}

// kindOf names the record after the event that produced it: the first
// underscore separates the family from the action, and the rest read as one
// hyphenated action.
func kindOf(eventType string) string {
	switch eventType {
	case "message_created":
		return "message"
	case "message_replied", "reply_created":
		return "reply"
	case "delivery_attempt":
		return "delivery.attempt"
	case "acknowledged":
		return "delivery.ack"
	case "agent_registered":
		return "agent.start"
	case "agent_stopped":
		return "agent.stop"
	case "agent_status_changed":
		return "agent.status"
	case "wake_requested":
		return "wake.queued"
	case "wake_attempt":
		return "wake.attempt"
	}
	family, action, found := strings.Cut(eventType, "_")
	if !found {
		return eventType
	}
	return family + "." + strings.ReplaceAll(action, "_", "-")
}
