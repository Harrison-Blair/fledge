// Package identity gives live Herdr agents a durable Fledge record: an id,
// the terminal it names, and the agent that registered it. The terminal is the
// record's identity; its pane is a locator that lookups refresh when Herdr
// moves the terminal to a new pane. A lookup fails closed when the terminal
// hosts no agent, and ends the record when the terminal is gone from Herdr or
// now hosts a different harness.
package identity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

// Kind is the state store kind holding agent records.
const Kind = "agents"

// Record is one registered agent. EndedAt stays null until agent stop closes
// its pane or a lookup observes its terminal gone; the record then moves to the
// store's archive. Records are never deleted.
type Record struct {
	ID           string  `json:"id"`
	Name         *string `json:"name"`
	Pane         string  `json:"pane"`
	WorkspaceID  string  `json:"workspace_id"`
	Harness      *string `json:"harness"`
	Session      *string `json:"session"`
	TerminalID   string  `json:"terminal_id"`
	Parent       *string `json:"parent"`
	RegisteredAt string  `json:"registered_at"`
	RegisteredBy string  `json:"registered_by"`
	WorktreePath *string `json:"worktree_path"`
	EndedAt      *string `json:"ended_at"`
}

// OpenStore opens the state store of the repository containing cwd, creating
// .fledge and its ignore file as needed and recording those effects on out.
func OpenStore(ctx context.Context, cwd string, out *libagent.Outcome) (*state.Store, error) {
	root, err := fledgedir.Root(ctx, cwd)
	if err != nil {
		return nil, err
	}
	dir, err := fledgedir.Ensure(root, out)
	if err != nil {
		return nil, err
	}
	return state.Open(filepath.Join(dir, "state"))
}

// Existing opens the store for lookups without creating anything. It returns
// a nil store when the repository has no state directory yet.
func Existing(ctx context.Context, cwd string) (*state.Store, error) {
	root, err := fledgedir.Root(ctx, cwd)
	if err != nil {
		return nil, err
	}
	s, err := state.OpenExisting(filepath.Join(root, ".fledge", "state"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return s, err
}

// Register records details as a new agent. The parent is the caller's live
// record when the caller's pane hosts a registered terminal; otherwise null.
// Herdr lookups happen first; one store lock then covers a single scan of the
// records, the caller's relocation, the check for a live record of the
// terminal, and the create, so concurrent registrations of one terminal yield
// exactly one live record and the others fail with agent_already_registered
// naming it. A live record of the terminal left by a different harness ends
// under the same lock.
func Register(ctx context.Context, s *state.Store, c libagent.Client, details herdr.AgentDetails, by string, worktree *string) (Record, error) {
	if details.TerminalID == "" || details.PaneID == "" {
		return Record{}, fmt.Errorf("cannot register an agent without a pane and terminal id")
	}
	caller, err := callerAgent(ctx, c)
	if err != nil {
		return Record{}, err
	}
	var rec Record
	err = s.Exclusive(func(tx *state.Tx) error {
		records, err := live(tx)
		if err != nil {
			return err
		}
		var parent *string
		if caller != nil {
			if p, ok := records[caller.TerminalID]; ok {
				if parent, err = attach(tx, p, *caller); err != nil {
					return err
				}
			}
		}
		if existing, ok := records[details.TerminalID]; ok {
			if !Mismatched(existing, details) {
				return alreadyRegistered(details, existing)
			}
			if _, err := end(tx, existing.ID); err != nil {
				return err
			}
		}
		_, err = tx.Create(Kind, func(id string) any {
			rec = Record{ID: id, Name: details.Name, Pane: details.PaneID, WorkspaceID: details.WorkspaceID, Harness: details.Agent,
				Session: session(), TerminalID: details.TerminalID, RegisteredAt: time.Now().UTC().Format(time.RFC3339), RegisteredBy: by, WorktreePath: worktree, Parent: parent}
			return rec
		})
		return err
	})
	if err != nil {
		return Record{}, err
	}
	return rec, nil
}

// attach returns the id of rec, the caller's live record, after pointing it at
// the caller's current pane, or nil after ending it when the caller now runs a
// different harness.
func attach(tx *state.Tx, rec Record, caller herdr.AgentDetails) (*string, error) {
	if Mismatched(rec, caller) {
		_, err := end(tx, rec.ID)
		return nil, err
	}
	if err := relocate(tx, &rec, caller); err != nil {
		return nil, err
	}
	return &rec.ID, nil
}

// live is liveRecords under the store lock, which also archives the ended
// records it finds among the live ones.
func live(tx *state.Tx) (map[string]Record, error) {
	records, ended, err := scan(tx)
	for _, id := range ended {
		if err == nil {
			err = tx.Archive(Kind, id)
		}
	}
	return records, err
}

// Unregistered fails with agent_already_registered, naming the existing id,
// when a's terminal already has a live record of a's harness. It lets callers
// refuse before acting; Register repeats the check under the store lock and
// ends a record left by a different harness, which Unregistered tolerates.
func Unregistered(s *state.Store, a herdr.AgentDetails) error {
	existing, err := Live(s, a.TerminalID)
	if err != nil {
		return err
	}
	if existing != nil && !Mismatched(*existing, a) {
		return alreadyRegistered(a, *existing)
	}
	return nil
}

func alreadyRegistered(a herdr.AgentDetails, existing Record) error {
	return &herdr.Error{Code: "agent_already_registered", Message: fmt.Sprintf("the agent in %s is already registered as %s", a.PaneID, existing.ID)}
}

// Caller finds the live record of the agent in the caller's pane, if any. A
// caller outside Herdr, or whose pane hosts no agent, has none; any other
// lookup failure is returned.
func Caller(ctx context.Context, s *state.Store, c libagent.Client) (*Record, error) {
	caller, err := callerAgent(ctx, c)
	if err != nil || caller == nil {
		return nil, err
	}
	rec, err := Match(s, *caller)
	if err != nil || rec == nil {
		return nil, err
	}
	moved, err := Relocate(s, *rec, *caller)
	if err != nil {
		return nil, err
	}
	return &moved, nil
}

// callerAgent fetches the agent in the caller's pane, or nil for a caller
// outside Herdr or whose pane hosts no agent.
func callerAgent(ctx context.Context, c libagent.Client) (*herdr.AgentDetails, error) {
	if c.CallerPane == "" {
		return nil, nil
	}
	caller, err := c.Get(ctx, c.CallerPane)
	var remote *herdr.Error
	if errors.As(err, &remote) && remote.Code == "agent_not_found" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &caller, nil
}

// RequireCaller is Caller for commands that act on the caller's own record:
// a caller without a live record, including one in a repository with no
// store yet (nil s), fails with caller_unregistered.
func RequireCaller(ctx context.Context, s *state.Store, c libagent.Client) (Record, error) {
	var rec *Record
	var err error
	if s != nil {
		rec, err = Caller(ctx, s, c)
	}
	if err != nil {
		return Record{}, err
	}
	if rec == nil {
		return Record{}, &herdr.Error{Code: "caller_unregistered", Message: "the caller has no live Fledge record; register with fledge agent adopt"}
	}
	return *rec, nil
}

// Relocate points rec at the pane where a, rec's terminal, now runs, storing
// the new pane and workspace under the same record id. Callers pass an a whose
// terminal is rec's.
func Relocate(s *state.Store, rec Record, a herdr.AgentDetails) (Record, error) {
	if rec.Pane == a.PaneID && rec.WorkspaceID == a.WorkspaceID {
		return rec, nil
	}
	err := s.Exclusive(func(tx *state.Tx) error { return relocate(tx, &rec, a) })
	return rec, err
}

// relocate is Relocate under the store lock, rereading rec first. An ended
// rec is stale.
func relocate(tx *state.Tx, rec *Record, a herdr.AgentDetails) error {
	if err := tx.Get(Kind, rec.ID, rec); err != nil {
		return err
	}
	if rec.EndedAt != nil {
		return stale(rec.ID, "the agent ended at %s", *rec.EndedAt)
	}
	rec.Pane, rec.WorkspaceID = a.PaneID, a.WorkspaceID
	return tx.Put(Kind, rec.ID, rec)
}

// End records that the agent of record id is gone and archives the record, so
// scans for live agents no longer read it; it stays loadable by id. An already
// ended record keeps its original time.
func End(s *state.Store, id string) error {
	_, err := EndOnce(s, id)
	return err
}

// EndOnce is End that also reports whether this call ended the record: false
// when it had already ended. A caller that must roll back its own end passes
// only a record it ended to Reopen, so it never undoes another caller's end.
func EndOnce(s *state.Store, id string) (bool, error) {
	var ended bool
	err := s.Exclusive(func(tx *state.Tx) error {
		var err error
		ended, err = end(tx, id)
		return err
	})
	return ended, err
}

func end(tx *state.Tx, id string) (bool, error) {
	var rec Record
	if err := tx.Get(Kind, id, &rec); err != nil {
		return false, err
	}
	ended := rec.EndedAt == nil
	if ended {
		now := time.Now().UTC().Format(time.RFC3339)
		rec.EndedAt = &now
		if err := tx.Put(Kind, id, rec); err != nil {
			return false, err
		}
	}
	return ended, tx.Archive(Kind, id)
}

// Reopen undoes the end of record id under one store lock: it returns the
// record from the archive to the live records and clears ended_at. It refuses
// with agent_already_registered, naming the other record and changing nothing,
// when another live record in the current Herdr session now holds the
// terminal, and with agent_identity_stale for a record of another Herdr
// session. An unknown id fails with agent_record_not_found. A record that has
// not ended is left as is without error; that is the only no-op.
func Reopen(s *state.Store, id string) error {
	if !state.ValidID(id) {
		return libagent.Invalid("--id must be 8 lowercase hexadecimal characters")
	}
	return s.Exclusive(func(tx *state.Tx) error {
		var rec Record
		var missing *state.NotFoundError
		if err := tx.Get(Kind, id, &rec); errors.As(err, &missing) {
			return &herdr.Error{Code: "agent_record_not_found", Message: fmt.Sprintf("no agent record with id %s", id)}
		} else if err != nil {
			return err
		}
		switch {
		case rec.EndedAt == nil:
			return nil
		case !sameSession(rec):
			return stale(id, "it belongs to another Herdr session")
		}
		records, err := live(tx)
		if err != nil {
			return err
		}
		if other, ok := records[rec.TerminalID]; ok {
			return &herdr.Error{Code: "agent_already_registered", Message: fmt.Sprintf("cannot reopen agent record %s: terminal %s is registered as %s", id, rec.TerminalID, other.ID)}
		}
		// Unarchive first: interrupted here, the record is live but ended,
		// which the next scan under the lock archives again.
		if err := tx.Unarchive(Kind, id); err != nil {
			return err
		}
		rec.EndedAt = nil
		return tx.Put(Kind, id, rec)
	})
}

// Match returns the live record of a's terminal, or nil when none exists. A
// record left by a different harness is not a's: Match ends it and returns nil.
func Match(s *state.Store, a herdr.AgentDetails) (*Record, error) {
	rec, err := Live(s, a.TerminalID)
	if err != nil || rec == nil || !Mismatched(*rec, a) {
		return rec, err
	}
	return nil, End(s, rec.ID)
}

// Mismatched reports whether a runs a different harness than rec recorded.
// An unknown harness on either side is no mismatch.
func Mismatched(rec Record, a herdr.AgentDetails) bool {
	return rec.Harness != nil && a.Agent != nil && *rec.Harness != *a.Agent
}

// Attributed returns a's record from records, a LiveByTerminal map, unless a
// has no terminal or runs a different harness than the record. It never
// writes, for listings that only display records.
func Attributed(records map[string]Record, a herdr.AgentDetails) (Record, bool) {
	rec, ok := records[a.TerminalID]
	return rec, ok && a.TerminalID != "" && !Mismatched(rec, a)
}

// Live returns the unended record naming terminal in the current Herdr
// session, or nil when none exists.
func Live(s *state.Store, terminal string) (*Record, error) {
	records, err := LiveByTerminal(s)
	if rec, ok := records[terminal]; ok {
		return &rec, nil
	}
	return nil, err
}

// LiveByTerminal maps each terminal id to its unended record in the current
// Herdr session.
func LiveByTerminal(s *state.Store) (map[string]Record, error) {
	records, _, err := scan(s)
	return records, err
}

// reader is the read side shared by state.Store and state.Tx.
type reader interface {
	Get(kind, id string, v any) error
	List(kind string) ([]string, error)
}

// scan is replaceable so tests can count scans of the agent records.
var scan = liveRecords

// liveRecords reads every unarchived agent record once, mapping each terminal
// to its unended record in the current Herdr session. It also returns the ids
// of ended records not yet archived, which predate archiving.
func liveRecords(r reader) (map[string]Record, []string, error) {
	ids, err := r.List(Kind)
	if err != nil {
		return nil, nil, err
	}
	records, ended := map[string]Record{}, []string{}
	for _, id := range ids {
		var rec Record
		if err := r.Get(Kind, id, &rec); err != nil {
			return nil, nil, err
		}
		switch {
		case rec.EndedAt != nil:
			ended = append(ended, id)
		case sameSession(rec):
			records[rec.TerminalID] = rec
		}
	}
	return records, ended, nil
}

// Resolve loads record id and fetches its terminal's live agent. When the
// recorded pane no longer hosts the terminal, Herdr's agent list locates it
// and the record follows it to its new pane. A terminal hosting no agent fails
// closed with agent_identity_stale; if it is gone from every pane, or hosts a
// different harness, the record also ends.
func Resolve(ctx context.Context, s *state.Store, c libagent.Client, id string) (Record, herdr.AgentDetails, error) {
	rec, err := load(s, id)
	if err != nil {
		return Record{}, herdr.AgentDetails{}, err
	}
	a, err := c.Get(ctx, rec.Pane)
	var remote *herdr.Error
	if err != nil && !(errors.As(err, &remote) && remote.Code == "agent_not_found") {
		return Record{}, herdr.AgentDetails{}, err
	}
	if err != nil || a.TerminalID != rec.TerminalID {
		agents, err := c.List(ctx)
		if err != nil {
			return Record{}, herdr.AgentDetails{}, err
		}
		var found bool
		if a, found = lookup(agents, rec.TerminalID); !found {
			return Record{}, herdr.AgentDetails{}, gone(ctx, s, c, rec)
		}
	}
	if Mismatched(rec, a) {
		if err := End(s, rec.ID); err != nil {
			return Record{}, herdr.AgentDetails{}, err
		}
		return Record{}, herdr.AgentDetails{}, harnessChanged(rec, a)
	}
	if rec, err = Relocate(s, rec, a); err != nil {
		return Record{}, herdr.AgentDetails{}, err
	}
	return rec, a, nil
}

// gone explains why rec's terminal hosts no agent. Only a terminal missing
// from every pane ends the record; one still in a pane may host an agent again.
func gone(ctx context.Context, s *state.Store, c libagent.Client, rec Record) error {
	panes, err := paneList(ctx, c)
	if err != nil {
		return err
	}
	if p, ok := lookup(panes, rec.TerminalID); ok {
		return stale(rec.ID, "terminal %s in pane %s no longer hosts an agent", rec.TerminalID, p.PaneID)
	}
	if err := End(s, rec.ID); err != nil {
		return err
	}
	return stale(rec.ID, "terminal %s is gone from Herdr", rec.TerminalID)
}

// paneList fetches every pane and validates each entry, as Client.List does
// for agents: a malformed one could be the terminal sought, which would make
// its absence unprovable.
func paneList(ctx context.Context, c libagent.Client) ([]herdr.AgentDetails, error) {
	var r struct {
		Type  string               `json:"type"`
		Panes []herdr.AgentDetails `json:"panes"`
	}
	err := c.Call(ctx, "pane.list", nil, &r)
	if err == nil && (r.Type != "pane_list" || r.Panes == nil) {
		err = libagent.Protocol("incomplete pane.list result")
	}
	for _, a := range r.Panes {
		if err == nil && !libagent.ValidAgentInfo(a) {
			err = libagent.Protocol("incomplete pane.list result")
		}
	}
	return r.Panes, err
}

func lookup(list []herdr.AgentDetails, terminal string) (herdr.AgentDetails, bool) {
	for _, a := range list {
		if a.TerminalID == terminal {
			return a, true
		}
	}
	return herdr.AgentDetails{}, false
}

// Verify fails closed unless a is the terminal rec names, in whatever pane,
// running the harness rec recorded.
func Verify(rec Record, a herdr.AgentDetails) error {
	if a.TerminalID != rec.TerminalID {
		return stale(rec.ID, "pane %s now hosts a different terminal", a.PaneID)
	}
	if Mismatched(rec, a) {
		return harnessChanged(rec, a)
	}
	return nil
}

func harnessChanged(rec Record, a herdr.AgentDetails) error {
	return stale(rec.ID, "terminal %s now hosts a different harness: %s, not the recorded %s", rec.TerminalID, *a.Agent, *rec.Harness)
}

func load(s *state.Store, id string) (Record, error) {
	if !state.ValidID(id) {
		return Record{}, libagent.Invalid("--id must be 8 lowercase hexadecimal characters")
	}
	var rec Record
	var missing *state.NotFoundError
	err := errors.New("no agent records exist in this repository")
	if s != nil {
		err = s.Get(Kind, id, &rec)
	}
	if s == nil || errors.As(err, &missing) {
		return Record{}, &herdr.Error{Code: "agent_record_not_found", Message: fmt.Sprintf("no agent record with id %s", id)}
	}
	if err != nil {
		return Record{}, err
	}
	switch {
	case rec.EndedAt != nil:
		return Record{}, stale(id, "the agent ended at %s", *rec.EndedAt)
	case !sameSession(rec):
		return Record{}, stale(id, "it belongs to another Herdr session")
	}
	return rec, nil
}

func stale(id, format string, args ...any) error {
	return &herdr.Error{Code: "agent_identity_stale", Message: fmt.Sprintf("agent record %s is stale: ", id) + fmt.Sprintf(format, args...)}
}

// session names the caller's Herdr session, or nil outside one.
func session() *string {
	if s := os.Getenv("HERDR_SESSION"); s != "" {
		return &s
	}
	return nil
}
func sameSession(rec Record) bool {
	current, recorded := os.Getenv("HERDR_SESSION"), ""
	if rec.Session != nil {
		recorded = *rec.Session
	}
	return current == recorded
}

// Target selects one live agent by exactly one of name, pane, or record id.
type Target struct{ Name, Pane, ID string }

// Validate enforces exactly one selector without contacting Herdr.
func (t Target) Validate() error {
	set := 0
	for _, v := range []string{t.Name, t.Pane, t.ID} {
		if v != "" {
			set++
		}
	}
	if set != 1 {
		return libagent.Invalid("exactly one of --name, --pane, or --id is required")
	}
	return nil
}

// Get fetches the selected agent and returns the Herdr target that addressed
// it: the name or pane as given, or for an id the verified pane. An id lookup
// also returns its record. Record lookup failures are located at phase identity.
func (t Target) Get(ctx context.Context, c libagent.Client) (herdr.AgentDetails, string, *Record, error) {
	if t.ID == "" {
		target := t.Name
		if target == "" {
			target = t.Pane
		}
		a, err := c.Get(ctx, target)
		return a, target, nil, err
	}
	s, err := Existing(ctx, c.Cwd)
	if err != nil {
		return herdr.AgentDetails{}, "", nil, libagent.AtPhase("identity", err)
	}
	rec, a, err := Resolve(ctx, s, c, t.ID)
	var remote *herdr.Error
	var input *libagent.InputError
	if err != nil && (errors.As(err, &input) || errors.As(err, &remote) && (remote.Code == "agent_identity_stale" || remote.Code == "agent_record_not_found")) {
		err = libagent.AtPhase("identity", err)
	}
	if err != nil {
		return herdr.AgentDetails{}, "", nil, err
	}
	return a, rec.Pane, &rec, nil
}
