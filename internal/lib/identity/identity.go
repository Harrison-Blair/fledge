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
// its pane or a lookup observes its terminal gone; records are never deleted.
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
// Herdr lookups happen first; the store lock then covers only the check for an
// existing live record of the terminal and the create, so concurrent
// registrations of one terminal yield exactly one record and the others fail
// with agent_already_registered naming it. A live record of the terminal left
// by a different harness ends first.
func Register(ctx context.Context, s *state.Store, c libagent.Client, details herdr.AgentDetails, by string, worktree *string) (Record, error) {
	if details.TerminalID == "" || details.PaneID == "" {
		return Record{}, fmt.Errorf("cannot register an agent without a pane and terminal id")
	}
	parent, err := Caller(ctx, s, c)
	if err != nil {
		return Record{}, err
	}
	// End a different harness's record now: Unregistered, under the lock,
	// cannot write it.
	if _, err := Match(s, details); err != nil {
		return Record{}, err
	}
	var rec Record
	err = s.Exclusive(func(*state.Tx) error {
		if err := Unregistered(s, details); err != nil {
			return err
		}
		_, err := s.Create(Kind, func(id string) any {
			rec = Record{ID: id, Name: details.Name, Pane: details.PaneID, WorkspaceID: details.WorkspaceID, Harness: details.Agent,
				Session: session(), TerminalID: details.TerminalID, RegisteredAt: time.Now().UTC().Format(time.RFC3339), RegisteredBy: by, WorktreePath: worktree}
			if parent != nil {
				rec.Parent = &parent.ID
			}
			return rec
		})
		return err
	})
	if err != nil {
		return Record{}, err
	}
	return rec, nil
}

// Unregistered fails with agent_already_registered, naming the existing id,
// when a's terminal already has a live record of a's harness. It only reads,
// so it may run under the store lock. It tolerates a record left by a
// different harness because Register ends that record via Match before taking
// the lock; ending it under s.Exclusive would self-deadlock on the store flock.
func Unregistered(s *state.Store, a herdr.AgentDetails) error {
	existing, err := Live(s, a.TerminalID)
	if err != nil {
		return err
	}
	if existing != nil && !Mismatched(*existing, a) {
		return &herdr.Error{Code: "agent_already_registered", Message: fmt.Sprintf("the agent in %s is already registered as %s", a.PaneID, existing.ID)}
	}
	return nil
}

// Caller finds the live record of the agent in the caller's pane, if any. A
// caller outside Herdr, or whose pane hosts no agent, has none; any other
// lookup failure is returned.
func Caller(ctx context.Context, s *state.Store, c libagent.Client) (*Record, error) {
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
	rec, err := Match(s, caller)
	if err != nil || rec == nil {
		return nil, err
	}
	moved, err := Relocate(s, *rec, caller)
	if err != nil {
		return nil, err
	}
	return &moved, nil
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
	err := s.Update(Kind, rec.ID, &rec, func() error {
		if rec.EndedAt != nil {
			return stale(rec.ID, "the agent ended at %s", *rec.EndedAt)
		}
		rec.Pane, rec.WorkspaceID = a.PaneID, a.WorkspaceID
		return nil
	})
	return rec, err
}

// End records that the agent of record id is gone. An already ended record
// keeps its original time.
func End(s *state.Store, id string) error {
	var rec Record
	return s.Update(Kind, id, &rec, func() error {
		if rec.EndedAt == nil {
			now := time.Now().UTC().Format(time.RFC3339)
			rec.EndedAt = &now
		}
		return nil
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
	ids, err := s.List(Kind)
	if err != nil {
		return nil, err
	}
	records := map[string]Record{}
	for _, id := range ids {
		var rec Record
		if err := s.Get(Kind, id, &rec); err != nil {
			return nil, err
		}
		if rec.EndedAt == nil && sameSession(rec) {
			records[rec.TerminalID] = rec
		}
	}
	return records, nil
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
		agents, err := entries(ctx, c, "agent.list", "agent_list")
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
	panes, err := entries(ctx, c, "pane.list", "pane_list")
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

// entries calls method, whose kind result carries agents or panes, and
// validates every entry: a malformed one could be the terminal sought, which
// would make its absence unprovable.
func entries(ctx context.Context, c libagent.Client, method, kind string) ([]herdr.AgentDetails, error) {
	var r struct {
		Type   string               `json:"type"`
		Agents []herdr.AgentDetails `json:"agents"`
		Panes  []herdr.AgentDetails `json:"panes"`
	}
	err := c.Call(ctx, method, nil, &r)
	list := r.Agents
	if kind == "pane_list" {
		list = r.Panes
	}
	if err == nil && (r.Type != kind || list == nil) {
		err = libagent.Protocol("incomplete " + method + " result")
	}
	for _, a := range list {
		if err == nil && !libagent.ValidAgentInfo(a) {
			err = libagent.Protocol("incomplete " + method + " result")
		}
	}
	return list, err
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
		target, _ := libagent.ResolveTarget(t.Name, t.Pane)
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
