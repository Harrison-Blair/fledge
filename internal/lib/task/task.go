package task

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

// Kind is the state store kind holding task records.
const Kind = "tasks"

// The six task states. Cancelled and landed are final; verified is final
// unless task land moves it to landed.
const (
	Created   = "created"
	Assigned  = "assigned"
	Completed = "completed"
	Verified  = "verified"
	Cancelled = "cancelled"
	Landed    = "landed"
)

// Statuses lists every state in lifecycle order.
var Statuses = []string{Created, Assigned, Completed, Verified, Cancelled, Landed}

// The delivery modes: how task land delivers a verified task's branch.
const (
	ModeLocal = "local"
	ModePR    = "pr"
	ModeAuto  = "auto"
	ModeNone  = "none"
)

// DeliveryModes lists every delivery mode.
var DeliveryModes = []string{ModeLocal, ModePR, ModeAuto, ModeNone}

// The parent shapes: how the subtasks of a parent task are delivered.
const (
	ShapeStack   = "stack"
	ShapeFeature = "feature"
)

// Shapes lists every parent shape.
var Shapes = []string{ShapeStack, ShapeFeature}

// Record is one task. Agent references are Fledge agent record ids. Parent
// is the task this one was created under; it never changes. After lists the
// prerequisite task ids in declaration order. UnmetAtAssign is set only when
// a forced assign bypassed prerequisites, naming the ones still unmet then.
// DeliveryMode is how task land delivers the task; a record written before it
// existed loads as none. Shape is one of Shapes, or empty when the task has
// no shape. Branch is set at creation or resolved and stored by task land;
// VerifiedCommit is the commit the latest verification bound, and
// VerifiedPatchID is its patch id against the task's base. CarriedFrom is
// the verified commit before a carry-forward to a head with an equal patch
// id. A new verification clears it. The landing
// fields stay null until task land records them; LandStep is the last land
// step reached, for refusals.
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
	DeliveryMode           string                  `json:"delivery_mode"`
	Shape                  string                  `json:"shape"`
	Branch                 *string                 `json:"branch"`
	BaseBranch             *string                 `json:"base_branch"`
	VerifiedCommit         *string                 `json:"verified_commit"`
	VerifiedPatchID        *string                 `json:"verified_patch_id"`
	CarriedFrom            *string                 `json:"carried_from"`
	LandedAt               *string                 `json:"landed_at"`
	LandedBy               *string                 `json:"landed_by"`
	MergeCommit            *string                 `json:"merge_commit"`
	TargetBranch           *string                 `json:"target_branch"`
	PRURL                  *string                 `json:"pr_url"`
	LandStep               *string                 `json:"land_step"`
}

// UnmarshalJSON decodes a record, defaulting an absent delivery_mode to none.
func (r *Record) UnmarshalJSON(b []byte) error {
	type plain Record
	p := plain{DeliveryMode: ModeNone}
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*r = Record(p)
	return nil
}

// CompletionNotification is the outcome of notifying a task's registered
// creator after completion. Pane is unknown when creator resolution fails;
// exactly one of DeliveredAt and Error is set when notification processing
// finishes.
type CompletionNotification struct {
	Recipient string  `json:"recipient"`
	MessageID string  `json:"message_id"`
	Pane      *string `json:"pane"`
	Attempt
}

// Delivery is the outcome of sending the brief to the owner on assignment.
type Delivery struct {
	MessageID string `json:"message_id"`
	Pane      string `json:"pane"`
	Attempt
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
	return libagent.ValidateID("id", "task", id)
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
	oldestFirst(rs)
	return rs, nil
}

// oldestFirst sorts rs by parsed creation time, then by id.
func oldestFirst(rs []Record) {
	slices.SortFunc(rs, func(a, b Record) int {
		return cmp.Or(created(a).Compare(created(b)), cmp.Compare(a.ID, b.ID))
	})
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
	return &cli.Error{Code: "task_invalid_state", Message: fmt.Sprintf("task %s is %s; %s requires %s", r.ID, r.Status, action, strings.Join(allowed, " or "))}
}

func notFound(id string) error {
	return &cli.Error{Code: "task_not_found", Message: fmt.Sprintf("no task with id %s", id)}
}

func mapMissing(err error, id string) error {
	var missing *state.NotFoundError
	if errors.As(err, &missing) {
		return notFound(id)
	}
	return err
}
