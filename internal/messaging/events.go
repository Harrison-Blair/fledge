package messaging

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	eventSessionStart    = "session_start"
	eventMessageCreated  = "message_created"
	eventMessageReplied  = "message_replied" // Legacy reply-and-acknowledge event.
	eventReplyCreated    = "reply_created"
	eventDeliveryAttempt = "delivery_attempt"
	eventDeliveryOutcome = "delivery_outcome"
	eventAcknowledged    = "acknowledged"
	eventAgentRegistered = "agent_registered"
	eventAgentStopped    = "agent_stopped"
	eventAgentStatus     = "agent_status_changed"
	eventTaskAssigned    = "task_assigned"
	eventTaskProgress    = "task_progress"
	eventTaskBlocked     = "task_blocked"
	eventTaskDecision    = "task_needs_decision"
	eventTaskResumed     = "task_resumed"
	eventTaskCompleted   = "task_completed"
	eventTaskFailed      = "task_failed"
	eventTaskCanceled    = "task_canceled"
	eventTaskOrphaned    = "task_orphaned"
	eventWakeRequested   = "wake_requested"
	eventWakeAttempt     = "wake_attempt"
	eventWakeOutcome     = "wake_outcome"
)

type event struct {
	Version       int        `json:"version"`
	Type          string     `json:"type"`
	At            time.Time  `json:"at"`
	SessionID     string     `json:"session_id"`
	Session       string     `json:"session,omitempty"`
	MessageID     string     `json:"message_id,omitempty"`
	Sender        string     `json:"sender,omitempty"`
	Recipient     string     `json:"recipient,omitempty"`
	ReplyTo       string     `json:"reply_to,omitempty"`
	Body          string     `json:"body,omitempty"`
	RecipientPane string     `json:"recipient_pane,omitempty"`
	Accepted      *bool      `json:"accepted,omitempty"`
	Detail        string     `json:"detail,omitempty"`
	AgentName     string     `json:"agent_name,omitempty"`
	PaneID        string     `json:"pane_id,omitempty"`
	Harness       string     `json:"harness,omitempty"`
	AuthorityHash string     `json:"authority_hash,omitempty"`
	CanDelegate   bool       `json:"can_delegate,omitempty"`
	ParentTaskID  string     `json:"parent_task_id,omitempty"`
	TaskID        string     `json:"task_id,omitempty"`
	Assignee      string     `json:"assignee,omitempty"`
	Assigner      string     `json:"assigner,omitempty"`
	Description   string     `json:"description,omitempty"`
	TaskStatus    TaskStatus `json:"task_status,omitempty"`
	WakeID        string     `json:"wake_id,omitempty"`
	WakeKind      string     `json:"wake_kind,omitempty"`
	Actor         string     `json:"actor,omitempty"`
}

// LedgerEntry is the exported projection of one durable ledger event. Readers
// that only diagnose activity use it instead of reconstructing session state,
// so they never have to interpret the log's ordering rules.
type LedgerEntry struct {
	Version       int
	Type          string
	At            time.Time
	SessionID     string
	Session       string
	MessageID     string
	Sender        string
	Recipient     string
	ReplyTo       string
	Body          string
	RecipientPane string
	Accepted      *bool
	Detail        string
	AgentName     string
	PaneID        string
	Harness       string
	AuthorityHash string
	CanDelegate   bool
	ParentTaskID  string
	TaskID        string
	Assignee      string
	Assigner      string
	Description   string
	TaskStatus    TaskStatus
	WakeID        string
	WakeKind      string
	Actor         string
}

// DecodeLedgerLine decodes one ledger line for a diagnostic reader. It is
// deliberately lenient where decodeEvent is strict: a line written by a newer
// Fledge, or one whose contents the reconstruction rules would reject, is
// reported as undecodable rather than fatal. Ordering and referential rules are
// the store's business, not a trace reader's.
func DecodeLedgerLine(data []byte) (LedgerEntry, bool) {
	if len(bytes.TrimSpace(data)) == 0 {
		return LedgerEntry{}, false
	}
	var e event
	if err := json.Unmarshal(data, &e); err != nil {
		return LedgerEntry{}, false
	}
	if strings.TrimSpace(e.Type) == "" || e.At.IsZero() {
		return LedgerEntry{}, false
	}
	return LedgerEntry(e), true
}

type logState struct {
	sessionID string
	session   string
	messages  map[string]Message
	order     []string
	agents    map[string]Agent
	tasks     map[string]Task
	taskOrder []string
	wakes     map[string]Wake
	wakeOrder []string
}

func decodeEvent(data []byte, result *event) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("empty event")
	}
	// Unknown fields are tolerated so a dispatcher already running cannot be
	// killed by a line a newer Fledge wrote into the same session's ledger.
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(result); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return validateEvent(*result)
}

func validateEvent(e event) error {
	if e.Version != eventVersion {
		return fmt.Errorf("unsupported event version %d", e.Version)
	}
	if e.At.IsZero() {
		return errors.New("event timestamp is missing")
	}
	if strings.TrimSpace(e.SessionID) == "" {
		return errors.New("event session ID is missing")
	}
	switch e.Type {
	case eventSessionStart:
		if strings.TrimSpace(e.Session) == "" {
			return errors.New("session_start is missing Herdr session name")
		}
		if e.MessageID != "" || e.Sender != "" || e.Recipient != "" || e.ReplyTo != "" || e.Body != "" || e.RecipientPane != "" || e.Accepted != nil || e.Detail != "" || e.Actor != "" {
			return errors.New("session_start has unexpected fields")
		}
	case eventMessageCreated, eventMessageReplied, eventReplyCreated:
		if e.Session != "" {
			return errors.New("message event has unexpected session name")
		}
		if strings.TrimSpace(e.MessageID) == "" || strings.TrimSpace(e.Sender) == "" || strings.TrimSpace(e.Recipient) == "" {
			return errors.New("message event is missing identity fields")
		}
		if err := ValidateBody(e.Body); err != nil {
			return err
		}
		if e.Sender == e.Recipient {
			return errors.New("message sender and recipient must differ")
		}
		if e.Type == eventMessageCreated && e.ReplyTo != "" {
			return errors.New("message_created must not have reply_to")
		}
		if (e.Type == eventMessageReplied || e.Type == eventReplyCreated) && e.ReplyTo == "" {
			return errors.New("reply event is missing reply_to")
		}
		if e.Recipient == "user" && e.RecipientPane != "" {
			return errors.New("user message has a recipient pane")
		}
		if e.Recipient != "user" && e.RecipientPane == "" {
			return errors.New("agent message is missing recipient pane")
		}
		if e.Accepted != nil || e.Detail != "" || e.Actor != "" {
			return errors.New("message event has outcome fields")
		}
	case eventDeliveryAttempt:
		if e.MessageID == "" || e.Session != "" || e.Sender != "" || e.Recipient != "" || e.ReplyTo != "" || e.Body != "" || e.RecipientPane != "" || e.Accepted != nil || e.Detail != "" || e.Actor != "" {
			return errors.New("invalid delivery_attempt fields")
		}
	case eventDeliveryOutcome:
		if e.MessageID == "" || e.Session != "" || e.Accepted == nil || e.Sender != "" || e.Recipient != "" || e.ReplyTo != "" || e.Body != "" || e.RecipientPane != "" || e.Actor != "" {
			return errors.New("invalid delivery_outcome fields")
		}
		if *e.Accepted && e.Detail != "" {
			return errors.New("accepted delivery outcome has failure detail")
		}
	case eventAcknowledged:
		if e.MessageID == "" || e.Session != "" || e.Sender != "" || e.Recipient != "" || e.ReplyTo != "" || e.Body != "" || e.RecipientPane != "" || e.Accepted != nil || e.Detail != "" || e.Actor != "" {
			return errors.New("invalid acknowledged fields")
		}
	case eventAgentRegistered, eventAgentStopped, eventAgentStatus,
		eventTaskAssigned, eventTaskProgress, eventTaskBlocked, eventTaskDecision,
		eventTaskResumed, eventTaskCompleted, eventTaskFailed, eventTaskCanceled,
		eventTaskOrphaned, eventWakeRequested, eventWakeAttempt, eventWakeOutcome:
		return validateCoordinationEvent(e)
	default:
		return fmt.Errorf("unknown event type %q", e.Type)
	}
	return nil
}

func applyEvent(state *logState, e event) error {
	if err := validateEvent(e); err != nil {
		return err
	}
	if e.Type == eventSessionStart {
		if state.sessionID != "" || len(state.messages) != 0 {
			return errors.New("session_start must be the first and only session event")
		}
		state.sessionID = e.SessionID
		state.session = e.Session
		return nil
	}
	if state.sessionID == "" {
		return errors.New("first event is not session_start")
	}
	if state.sessionID != e.SessionID {
		return fmt.Errorf("event belongs to session %q, expected %q", e.SessionID, state.sessionID)
	}
	switch e.Type {
	case eventMessageCreated, eventMessageReplied, eventReplyCreated:
		if _, exists := state.messages[e.MessageID]; exists {
			return fmt.Errorf("duplicate message ID %q", e.MessageID)
		}
		if e.Type == eventMessageReplied || e.Type == eventReplyCreated {
			original, exists := state.messages[e.ReplyTo]
			if !exists {
				return fmt.Errorf("reply target %q does not exist", e.ReplyTo)
			}
			if e.Sender != original.Recipient || e.Recipient != original.Sender {
				return fmt.Errorf("reply %q does not reverse sender and recipient of %q", e.MessageID, e.ReplyTo)
			}
			if original.Status != StatusDelivered && original.Status != StatusUncertain {
				return fmt.Errorf("reply target %q is in status %s", e.ReplyTo, original.Status)
			}
			if e.Type == eventMessageReplied && original.Status == StatusUncertain {
				original.Status = StatusDelivered
				original.DeliveredAt = e.At
				original.UpdatedAt = e.At
				state.messages[e.ReplyTo] = original
			}
		}
		status := StatusPending
		var deliveredAt time.Time
		if e.Recipient == "user" {
			status = StatusDelivered
			deliveredAt = e.At
		}
		state.messages[e.MessageID] = Message{
			ID: e.MessageID, Sender: e.Sender, Recipient: e.Recipient, ReplyTo: e.ReplyTo,
			Body: e.Body, Status: status, RecipientPane: e.RecipientPane,
			CreatedAt: e.At, UpdatedAt: e.At, DeliveredAt: deliveredAt,
		}
		state.order = append(state.order, e.MessageID)
	case eventDeliveryAttempt:
		message, ok := state.messages[e.MessageID]
		if !ok {
			return fmt.Errorf("delivery attempt references unknown message %q", e.MessageID)
		}
		if message.Status != StatusPending {
			return fmt.Errorf("delivery attempt for message %q in status %s", e.MessageID, message.Status)
		}
		message.Status = StatusUncertain
		message.AttemptedAt = e.At
		message.UpdatedAt = e.At
		state.messages[e.MessageID] = message
	case eventDeliveryOutcome:
		message, ok := state.messages[e.MessageID]
		if !ok {
			return fmt.Errorf("delivery outcome references unknown message %q", e.MessageID)
		}
		if message.Status != StatusUncertain {
			return fmt.Errorf("delivery outcome for message %q in status %s", e.MessageID, message.Status)
		}
		if *e.Accepted {
			message.Status = StatusDelivered
			message.DeliveredAt = e.At
		} else {
			message.Status = StatusFailed
			message.Failure = e.Detail
		}
		message.UpdatedAt = e.At
		state.messages[e.MessageID] = message
	case eventAcknowledged:
		message, ok := state.messages[e.MessageID]
		if !ok {
			return fmt.Errorf("acknowledgement references unknown message %q", e.MessageID)
		}
		if message.Status != StatusDelivered && message.Status != StatusUncertain {
			return fmt.Errorf("acknowledgement for message %q in status %s", e.MessageID, message.Status)
		}
		if message.Status == StatusUncertain {
			message.Status = StatusDelivered
			message.DeliveredAt = e.At
			message.UpdatedAt = e.At
			state.messages[e.MessageID] = message
		}
	case eventAgentRegistered, eventAgentStopped, eventAgentStatus,
		eventTaskAssigned, eventTaskProgress, eventTaskBlocked, eventTaskDecision,
		eventTaskResumed, eventTaskCompleted, eventTaskFailed, eventTaskCanceled,
		eventTaskOrphaned, eventWakeRequested, eventWakeAttempt, eventWakeOutcome:
		return applyCoordinationEvent(state, e)
	}
	return nil
}
