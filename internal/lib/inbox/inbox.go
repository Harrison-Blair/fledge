// Package inbox stores the bodies of messages, task assignments, and
// supervisor wakes as durable records, one flat state store kind. A recipient
// gets only a one-line doorbell in its pane and reads the body with fledge
// inbox read. A body is never typed into a pane; a doorbell may be rung again.
package inbox

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/termtext"
)

// Kind is the state store kind holding inbox records.
const Kind = "inbox"

// The three record kinds.
const (
	Message    = "message"
	Assignment = "assignment"
	Wake       = "wake"
)

// SupervisorName is the sender name of every wake.
const SupervisorName = "fledge supervisor"

// From is the sender: its agent record id when registered, its pane name, and
// its pane. A wake has only Name, SupervisorName.
type From struct {
	ID   *string `json:"id"`
	Name *string `json:"name"`
	Pane *string `json:"pane"`
}

// Supervisor is the sender of wakes.
func Supervisor() From { return From{Name: cli.Pointer(SupervisorName)} }

// IsSupervisor reports whether f is the supervision daemon.
func (f From) IsSupervisor() bool {
	return f.ID == nil && f.Pane == nil && f.Name != nil && *f.Name == SupervisorName
}

// String describes the sender as doorbells and listings show it.
func (f From) String() string {
	switch {
	case f.IsSupervisor():
		return SupervisorName
	case f.Pane == nil:
		return "unknown sender"
	case f.Name != nil && *f.Name != "":
		return fmt.Sprintf("%s (%s)", *f.Name, *f.Pane)
	case f.ID != nil:
		return fmt.Sprintf("unnamed agent (%s)", *f.Pane)
	}
	return "pane " + *f.Pane
}

// Ring is one attempt to type the doorbell into Pane. A failed attempt has OK
// false and the error; it does not count as rung.
type Ring struct {
	At    string  `json:"at"`
	Pane  string  `json:"pane"`
	OK    bool    `json:"ok"`
	Error *string `json:"error"`
}

// Record is one stored message, assignment, or wake addressed to the agent
// record To. Task is set for assignments and task notices. ReplyTo links a
// reply to the record it answers; AnsweredAt and AnsweredBy mark a record that
// its recipient answered, AnsweredBy naming the reply's message id.
type Record struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	From       From    `json:"from"`
	To         string  `json:"to"`
	Task       *string `json:"task"`
	Body       string  `json:"body"`
	CreatedAt  string  `json:"created_at"`
	Rings      []Ring  `json:"rings"`
	ReadAt     *string `json:"read_at"`
	ReplyTo    *string `json:"reply_to"`
	AnsweredAt *string `json:"answered_at"`
	AnsweredBy *string `json:"answered_by"`
}

// View is a record with its derived state, for listings and JSON output.
type View struct {
	Record
	State string `json:"state"`
}

// NewView pairs r with its state.
func NewView(r Record) View { return View{Record: r, State: State(r)} }

// State derives queued (no successful ring), rung (rung and unread), read, or
// answered.
func State(r Record) string {
	switch {
	case r.AnsweredAt != nil:
		return "answered"
	case r.ReadAt != nil:
		return "read"
	case Rung(r):
		return "rung"
	}
	return "queued"
}

// Rung reports whether any doorbell attempt succeeded.
func Rung(r Record) bool { return slices.ContainsFunc(r.Rings, func(g Ring) bool { return g.OK }) }

// Unread reports whether r is queued or rung.
func Unread(r Record) bool { return r.ReadAt == nil && r.AnsweredAt == nil }

// LastRing returns the latest successful ring, or nil.
func LastRing(r Record) *Ring {
	for i := len(r.Rings) - 1; i >= 0; i-- {
		if r.Rings[i].OK {
			return &r.Rings[i]
		}
	}
	return nil
}

// Doorbell is the one line typed into the recipient's pane. It names the
// sender and the record, never the body. An assignment adds its task id and
// title. Dynamic text is cleaned and kept to one line.
func Doorbell(r Record, title string) string {
	line := fmt.Sprintf("ᛉ fledge inbox · %s from %s · id %s · read: fledge inbox read --id %s", r.Kind, oneLine(r.From.String()), r.ID, r.ID)
	if r.Kind == Assignment && r.Task != nil {
		line += fmt.Sprintf(" · task: %s · %s", *r.Task, oneLine(title))
	}
	return line
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(termtext.Clean(s)), " ")
}

// Now is the timestamp format of every inbox time field, as for tasks.
func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// Create stores r as a new record with a fresh id, its creation time, and no
// rings, and returns it.
func Create(s *state.Store, r Record) (Record, error) {
	r.CreatedAt, r.Rings = Now(), []Ring{}
	id, err := s.Create(Kind, func(id string) any { r.ID = id; return r })
	if err != nil {
		return Record{}, err
	}
	r.ID = id
	return r, nil
}

// Get reads record id, live or archived.
func Get(s *state.Store, id string) (Record, error) {
	var r Record
	err := s.Get(Kind, id, &r)
	var missing *state.NotFoundError
	if errors.As(err, &missing) {
		return Record{}, &cli.Error{Code: "inbox_not_found", Message: fmt.Sprintf("no inbox record with id %s", id)}
	}
	return r, err
}

// List reads every unarchived record, oldest first.
func List(s *state.Store) ([]Record, error) {
	ids, err := s.List(Kind)
	if err != nil {
		return nil, err
	}
	rs := make([]Record, 0, len(ids))
	for _, id := range ids {
		var r Record
		if err := s.Get(Kind, id, &r); err != nil {
			return nil, err
		}
		rs = append(rs, r)
	}
	slices.SortStableFunc(rs, func(a, b Record) int { return compareCreated(a, b) })
	return rs, nil
}

// ForRecipient lists the unarchived records addressed to agent record to:
// unread first, then read, each oldest first.
func ForRecipient(s *state.Store, to string) ([]Record, error) {
	all, err := List(s)
	if err != nil {
		return nil, err
	}
	var unread, read []Record
	for _, r := range all {
		switch {
		case r.To != to:
		case Unread(r):
			unread = append(unread, r)
		default:
			read = append(read, r)
		}
	}
	return append(unread, read...), nil
}

func compareCreated(a, b Record) int {
	ta, _ := time.Parse(time.RFC3339Nano, a.CreatedAt)
	tb, _ := time.Parse(time.RFC3339Nano, b.CreatedAt)
	return cmp.Or(ta.Compare(tb), cmp.Compare(a.ID, b.ID))
}

// update applies mutate to record id under the store lock.
func update(s *state.Store, id string, mutate func(*Record) error) (Record, error) {
	var r Record
	err := s.Update(Kind, id, &r, func() error { return mutate(&r) })
	var missing *state.NotFoundError
	if errors.As(err, &missing) {
		return Record{}, &cli.Error{Code: "inbox_not_found", Message: fmt.Sprintf("no inbox record with id %s", id)}
	}
	return r, err
}

// AddRing records one doorbell attempt into pane; ringErr is its failure, or
// nil for a successful ring.
func AddRing(s *state.Store, id, pane string, ringErr error) (Record, error) {
	ring := Ring{At: Now(), Pane: pane, OK: ringErr == nil}
	if ringErr != nil {
		ring.Error = cli.Pointer(ringErr.Error())
	}
	return update(s, id, func(r *Record) error {
		r.Rings = append(r.Rings, ring)
		return nil
	})
}

// MarkRead sets ReadAt once; a later read keeps the first time.
func MarkRead(s *state.Store, id string) (Record, error) {
	return update(s, id, func(r *Record) error {
		if r.ReadAt == nil {
			r.ReadAt = cli.Pointer(Now())
		}
		return nil
	})
}

// CheckRecipient reads record id and fails with inbox_not_recipient unless
// caller is its To.
func CheckRecipient(s *state.Store, id, caller string) (Record, error) {
	r, err := Get(s, id)
	if err == nil && r.To != caller {
		err = notRecipient(id)
	}
	return r, err
}

// Answer marks record id answered by reply, the reply's message id, after
// checking under the lock that caller is its recipient.
func Answer(s *state.Store, id, caller, reply string) (Record, error) {
	return update(s, id, func(r *Record) error {
		if r.To != caller {
			return notRecipient(id)
		}
		r.AnsweredAt, r.AnsweredBy = cli.Pointer(Now()), &reply
		return nil
	})
}

func notRecipient(id string) error {
	return &cli.Error{Code: "inbox_not_recipient", Message: fmt.Sprintf("inbox record %s is addressed to another agent; only its recipient may reply to it", id)}
}

// Archive moves the records to or from any of agents out of List's view,
// keeping each readable by id, and returns the archived ids.
func Archive(s *state.Store, agents []string) ([]string, error) {
	var archived []string
	err := s.Exclusive(func(tx *state.Tx) error {
		ids, err := tx.List(Kind)
		if err != nil {
			return err
		}
		for _, id := range ids {
			var r Record
			if err := tx.Get(Kind, id, &r); err != nil {
				return err
			}
			if !slices.Contains(agents, r.To) && (r.From.ID == nil || !slices.Contains(agents, *r.From.ID)) {
				continue
			}
			if err := tx.Archive(Kind, id); err != nil {
				return err
			}
			archived = append(archived, id)
		}
		return nil
	})
	return archived, err
}
