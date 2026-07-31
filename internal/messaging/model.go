// Package messaging implements Fledge's project-local, append-only agent
// message audit stream. It deliberately has no Herdr dependency so archived
// conversations remain readable while the server is stopped.
package messaging

import (
	"encoding/json"
	"time"

	"github.com/Harrison-Blair/fledge/internal/buildinfo"
)

const SchemaVersion = 1

const (
	EventRunStarted          = "run.started"
	EventRunClosed           = "run.closed"
	EventAgentActivated      = "agent.activated"
	EventAgentDeactivated    = "agent.deactivated"
	EventMessageCreated      = "message.created"
	EventMessageReplied      = "message.replied"
	EventDeliveryAttempted   = "message.delivery.attempted"
	EventDeliveryInjected    = "message.delivery.injected"
	EventDeliveryFailed      = "message.delivery.failed"
	EventDeliveryUncertain   = "message.delivery.uncertain"
	EventMessageAcknowledged = "message.acknowledged"
	EventMessageCancelled    = "message.cancelled"
	EventMessageFailed       = "message.failed"
)

const (
	StatusQueued       = "queued"
	StatusAwaitingAck  = "awaiting_ack"
	StatusAcknowledged = "acknowledged"
	StatusFailed       = "failed"
	StatusUncertain    = "uncertain"
	StatusCancelled    = "cancelled"
)

type GitInfo struct {
	Head   string `json:"head,omitempty"`
	Branch string `json:"branch,omitempty"`
	Dirty  *bool  `json:"dirty,omitempty"`
	Error  string `json:"error,omitempty"`
}

type RunHeader struct {
	Fledge      buildinfo.Info `json:"fledge"`
	Herdr       string         `json:"herdr_version"`
	Protocol    int            `json:"protocol"`
	ProjectRoot string         `json:"project_root"`
	Session     string         `json:"session"`
	Git         GitInfo        `json:"git"`
	StartedAt   time.Time      `json:"started_at"`
}

// Event is the stable on-disk JSONL record. Optional fields allow new event
// types to be added without changing the framing or sequence contract.
type Event struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	RunID         string    `json:"run_id"`
	Sequence      uint64    `json:"sequence"`
	Timestamp     time.Time `json:"timestamp"`
	Type          string    `json:"type"`

	Header       *RunHeader      `json:"header,omitempty"`
	Agent        string          `json:"agent,omitempty"`
	ActivationID string          `json:"activation_id,omitempty"`
	PaneID       string          `json:"pane_id,omitempty"`
	MessageID    string          `json:"message_id,omitempty"`
	AttemptID    string          `json:"attempt_id,omitempty"`
	Sender       string          `json:"sender,omitempty"`
	Recipient    string          `json:"recipient,omitempty"`
	ReplyTo      string          `json:"reply_to,omitempty"`
	Body         string          `json:"body,omitempty"`
	Actor        string          `json:"actor,omitempty"`
	Error        string          `json:"error,omitempty"`
	ErrorKind    string          `json:"error_kind,omitempty"`
	Reason       string          `json:"reason,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

type DeliveryAttempt struct {
	ID           string     `json:"id"`
	Sequence     uint64     `json:"sequence"`
	Timestamp    time.Time  `json:"timestamp"`
	ActivationID string     `json:"activation_id,omitempty"`
	Outcome      string     `json:"outcome"`
	Error        string     `json:"error,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

type Acknowledgement struct {
	By        string    `json:"by"`
	Timestamp time.Time `json:"timestamp"`
	Sequence  uint64    `json:"sequence"`
	ViaReply  string    `json:"via_reply,omitempty"`
}

type Cancellation struct {
	By        string    `json:"by"`
	Reason    string    `json:"reason,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Sequence  uint64    `json:"sequence"`
}

type Failure struct {
	Reason       string    `json:"reason"`
	ActivationID string    `json:"activation_id,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	Sequence     uint64    `json:"sequence"`
}

// Message is the reconstructed public message view.
type Message struct {
	ID               string            `json:"id"`
	RunID            string            `json:"run_id"`
	Sequence         uint64            `json:"sequence"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	Sender           string            `json:"sender"`
	Recipient        string            `json:"recipient"`
	ReplyTo          string            `json:"reply_to,omitempty"`
	Body             string            `json:"body"`
	Status           string            `json:"status"`
	ActiveRun        bool              `json:"active_run"`
	DeliveryAttempts []DeliveryAttempt `json:"delivery_attempts"`
	Acknowledgement  *Acknowledgement  `json:"acknowledgement,omitempty"`
	Cancellation     *Cancellation     `json:"cancellation,omitempty"`
	Failure          *Failure          `json:"failure,omitempty"`
}

type Activation struct {
	ID            string     `json:"id"`
	Agent         string     `json:"agent"`
	PaneID        string     `json:"pane_id,omitempty"`
	ActivatedAt   time.Time  `json:"activated_at"`
	DeactivatedAt *time.Time `json:"deactivated_at,omitempty"`
}

type Run struct {
	ID           string                 `json:"id"`
	Header       RunHeader              `json:"header"`
	StartedAt    time.Time              `json:"started_at"`
	ClosedAt     *time.Time             `json:"closed_at,omitempty"`
	Active       bool                   `json:"active"`
	LastSequence uint64                 `json:"last_sequence"`
	Messages     []*Message             `json:"messages,omitempty"`
	Activations  map[string]*Activation `json:"activations,omitempty"`
}

type Collection struct {
	SelectedRuns []string   `json:"selected_runs"`
	Total        int        `json:"total"`
	Returned     int        `json:"returned"`
	Messages     []*Message `json:"messages"`
}

type RunCollection struct {
	Total    int    `json:"total"`
	Returned int    `json:"returned"`
	Runs     []*Run `json:"runs"`
}
