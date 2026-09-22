package task

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

// Kind is the state store kind holding task records.
const Kind = "tasks"

// The five task states. Verified and cancelled are terminal.
const (
	Created   = "created"
	Assigned  = "assigned"
	Completed = "completed"
	Verified  = "verified"
	Cancelled = "cancelled"
)

// Statuses lists every state in lifecycle order.
var Statuses = []string{Created, Assigned, Completed, Verified, Cancelled}

// Record is one task. Agent references are Fledge agent record ids. Parent
// is the task this one was created under; it never changes. After lists the
// prerequisite task ids in declaration order. UnmetAtAssign is set only when
// a forced assign bypassed prerequisites, naming the ones still unmet then.
type Record struct {
	ID                     string                  `json:"id"`
	Title                  string                  `json:"title"`
	Brief                  string                  `json:"brief"`
	Parent                 *string                 `json:"parent"`
	After                  []string                `json:"after"`
	Owner                  *string                 `json:"owner"`
	Status                 string                  `json:"status"`
	Result                 *string                 `json:"result"`
	Verifier               *string                 `json:"verifier"`
	VerificationNote       *string                 `json:"verification_note"`
	Forced                 bool                    `json:"forced"`
	CancelReason           *string                 `json:"cancel_reason"`
	CreatedAt              string                  `json:"created_at"`
	CreatedBy              *string                 `json:"created_by"`
	AssignedAt             *string                 `json:"assigned_at"`
	UnmetAtAssign          []string                `json:"unmet_at_assign"`
	CompletedAt            *string                 `json:"completed_at"`
	CompletionNotification *CompletionNotification `json:"completion_notification"`
	VerifiedAt             *string                 `json:"verified_at"`
	CancelledAt            *string                 `json:"cancelled_at"`
	Delivery               *Delivery               `json:"delivery"`
}

// CompletionNotification is the outcome of notifying a task's registered
// creator after completion. Pane is unknown when creator resolution fails;
// exactly one of DeliveredAt and Error is set when notification processing
// finishes.
type CompletionNotification struct {
	Recipient   string  `json:"recipient"`
	MessageID   string  `json:"message_id"`
	Pane        *string `json:"pane"`
	DeliveredAt *string `json:"delivered_at"`
	Error       *string `json:"error"`
	Uncertain   bool    `json:"uncertain"`
}

// Delivery is the outcome of sending the brief to the owner on assignment.
// Exactly one of DeliveredAt and Error is set once the attempt has finished;
// Uncertain marks an Error after which the brief may still have arrived.
type Delivery struct {
	MessageID   string  `json:"message_id"`
	Pane        string  `json:"pane"`
	DeliveredAt *string `json:"delivered_at"`
	Error       *string `json:"error"`
	Uncertain   bool    `json:"uncertain"`
}

// Now is the timestamp format of every task time field: UTC RFC 3339 with
// nanoseconds, so tasks created within one second still order correctly.
func Now() *string {
	s := time.Now().UTC().Format(time.RFC3339Nano)
	return &s
}

// Existing opens the repository's store for lookups; it is nil before any
// Fledge state exists.
func Existing(ctx context.Context, cwd string) (*state.Store, error) {
	return identity.Existing(ctx, cwd)
}

// ValidateID checks a --id value without touching the store.
func ValidateID(id string) error {
	if !state.ValidID(id) {
		return libagent.Invalid("--id must be 8 lowercase hexadecimal characters")
	}
	return nil
}

// Get loads task id, failing with task_not_found when it does not exist.
func Get(s *state.Store, id string) (Record, error) {
	if err := ValidateID(id); err != nil {
		return Record{}, err
	}
	var r Record
	if s == nil {
		return r, notFound(id)
	}
	return r, mapMissing(s.Get(Kind, id, &r), id)
}

// List returns every task, oldest first by parsed creation time, so records
// written at whole-second precision order correctly among newer ones; equal
// times fall back to id order. A nil s has none.
func List(s *state.Store) ([]Record, error) {
	rs := []Record{}
	if s == nil {
		return rs, nil
	}
	ids, err := s.List(Kind)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		r, err := Get(s, id)
		if err != nil {
			return nil, err
		}
		rs = append(rs, r)
	}
	slices.SortFunc(rs, func(a, b Record) int {
		return cmp.Or(created(a).Compare(created(b)), cmp.Compare(a.ID, b.ID))
	})
	return rs, nil
}

// created parses r's creation time; an unparsable one sorts first.
func created(r Record) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, r.CreatedAt)
	return t
}

// Update runs mutate on task id under the store lock and stores the result.
// A mutate error leaves the stored record unchanged.
func Update(s *state.Store, id string, mutate func(*Record) error) (Record, error) {
	if err := ValidateID(id); err != nil {
		return Record{}, err
	}
	var r Record
	if s == nil {
		return r, notFound(id)
	}
	err := s.Update(Kind, id, &r, func() error { return mutate(&r) })
	return r, mapMissing(err, id)
}

// Require fails with task_invalid_state unless r is in one of allowed.
func Require(r *Record, action string, allowed ...string) error {
	if slices.Contains(allowed, r.Status) {
		return nil
	}
	return &herdr.Error{Code: "task_invalid_state", Message: fmt.Sprintf("task %s is %s; %s requires %s", r.ID, r.Status, action, strings.Join(allowed, " or "))}
}

func notFound(id string) error {
	return &herdr.Error{Code: "task_not_found", Message: fmt.Sprintf("no task with id %s", id)}
}

func mapMissing(err error, id string) error {
	var missing *state.NotFoundError
	if errors.As(err, &missing) {
		return notFound(id)
	}
	return err
}
