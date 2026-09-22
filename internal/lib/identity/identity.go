// Package identity gives live Herdr agents a durable Fledge record: an id,
// the terminal it names, and the agent that registered it. A record is valid
// only while its terminal still occupies the recorded pane, so every lookup by
// id fails closed when the live terminal differs.
package identity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

// Kind is the state store kind holding agent records.
const Kind = "agents"

// Record is one registered agent. EndedAt stays null until a later command
// observes the agent gone; records are never deleted.
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
// with agent_already_registered naming it.
func Register(ctx context.Context, s *state.Store, c libagent.Client, details herdr.AgentDetails, by string, worktree *string) (Record, error) {
	if details.TerminalID == "" || details.PaneID == "" {
		return Record{}, fmt.Errorf("cannot register an agent without a pane and terminal id")
	}
	parent, err := Caller(ctx, s, c)
	if err != nil {
		return Record{}, err
	}
	var rec Record
	err = s.Exclusive(func() error {
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
// when a's terminal already has a live record.
func Unregistered(s *state.Store, a herdr.AgentDetails) error {
	existing, err := Live(s, a.TerminalID)
	if err != nil {
		return err
	}
	if existing != nil {
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
	rec, err := Live(s, caller.TerminalID)
	if err != nil || rec == nil || rec.Pane != caller.PaneID {
		return nil, err
	}
	return rec, nil
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

// Resolve loads record id and fetches the agent now in its pane, failing
// closed with agent_identity_stale unless that agent is the recorded terminal.
func Resolve(ctx context.Context, s *state.Store, c libagent.Client, id string) (Record, herdr.AgentDetails, error) {
	rec, err := load(s, id)
	if err != nil {
		return Record{}, herdr.AgentDetails{}, err
	}
	a, err := c.Get(ctx, rec.Pane)
	var remote *herdr.Error
	switch {
	case errors.As(err, &remote) && remote.Code == "agent_not_found":
		err = stale(id, "pane %s no longer hosts an agent", rec.Pane)
	case err == nil:
		err = Verify(rec, a)
	}
	if err != nil {
		return Record{}, herdr.AgentDetails{}, err
	}
	return rec, a, nil
}

// Verify fails closed unless a is the terminal rec names.
func Verify(rec Record, a herdr.AgentDetails) error {
	if a.TerminalID != rec.TerminalID || a.PaneID != rec.Pane {
		return stale(rec.ID, "pane %s now hosts a different terminal", rec.Pane)
	}
	return nil
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}$`)

func load(s *state.Store, id string) (Record, error) {
	if !idPattern.MatchString(id) {
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
