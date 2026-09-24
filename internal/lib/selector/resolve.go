package selector

import (
	"context"
	"path/filepath"
	"slices"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Match is one live agent with its record when attributed.
type Match struct {
	Agent  herdr.AgentDetails
	Record *identity.Record
}

// liveByTerminal is replaceable so tests can count reads of the agent records.
var liveByTerminal = identity.LiveByTerminal

// Resolve lists Herdr agents once, reads the agent records once, loads task
// owners only when f.Tasks is set, and returns the matches in Herdr order. It
// never writes. A repository without state has no records; an unavailable
// store fails only a record-backed filter and otherwise leaves every match
// unattributed. Mine finds the caller in the listing, so a caller outside
// Herdr or without a live record fails with caller_unregistered.
func Resolve(ctx context.Context, c libagent.Client, f Filter) ([]Match, error) {
	if err := f.Validate(); err != nil {
		return nil, libagent.AtPhase("validation", err)
	}
	agents, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	s, records, err := load(ctx, c.Cwd)
	if err != nil && f.NeedsRecords() {
		return nil, libagent.AtPhase("state", err)
	}
	match := func(a herdr.AgentDetails) *identity.Record {
		if rec, ok := identity.Attributed(records, a); ok {
			return &rec
		}
		return nil
	}
	parent := f.Parent
	if f.Mine {
		var caller *identity.Record
		if i := slices.IndexFunc(agents, func(a herdr.AgentDetails) bool { return c.CallerPane != "" && a.PaneID == c.CallerPane }); i >= 0 {
			caller = match(agents[i])
		}
		if caller == nil {
			// Without a store, RequireCaller fails with caller_unregistered.
			_, err := identity.RequireCaller(ctx, nil, c)
			return nil, libagent.AtPhase("identity", err)
		}
		parent = caller.ID
	}
	var owners []string
	for _, id := range f.Tasks {
		t, err := task.Get(s, id)
		if err != nil {
			return nil, libagent.AtPhase("selection", err)
		}
		if t.Owner != nil {
			owners = append(owners, *t.Owner)
		}
	}
	worktrees := make([]string, len(f.Worktrees))
	for i, w := range f.Worktrees {
		if worktrees[i], err = filepath.Abs(w); err != nil {
			return nil, libagent.AtPhase("validation", libagent.Invalid("--worktree %q: %v", w, err))
		}
	}
	matches := []Match{}
	for _, a := range agents {
		rec := match(a)
		if !live(f, a) || f.NeedsRecords() && (rec == nil || !recorded(f, *rec, parent, owners, worktrees)) {
			continue
		}
		matches = append(matches, Match{Agent: a, Record: rec})
	}
	return matches, nil
}

// load opens the store without creating it and reads its live records. A
// repository without state has neither.
func load(ctx context.Context, cwd string) (*state.Store, map[string]identity.Record, error) {
	s, err := identity.Existing(ctx, cwd)
	if err != nil || s == nil {
		return nil, nil, err
	}
	records, err := liveByTerminal(s)
	return s, records, err
}

// live applies the fields compared with live Herdr values.
func live(f Filter, a herdr.AgentDetails) bool {
	return (len(f.States) == 0 || slices.Contains(f.States, a.AgentStatus)) &&
		(len(f.Harnesses) == 0 || a.Agent != nil && slices.Contains(f.Harnesses, *a.Agent))
}

// recorded applies the record-backed fields to rec.
func recorded(f Filter, rec identity.Record, parent string, owners, worktrees []string) bool {
	return (parent == "" || rec.Parent != nil && *rec.Parent == parent) &&
		(len(f.Profiles) == 0 || rec.Profile != nil && slices.Contains(f.Profiles, *rec.Profile)) &&
		(len(f.Tasks) == 0 || slices.Contains(owners, rec.ID)) &&
		(len(f.Worktrees) == 0 || rec.WorktreePath != nil && slices.Contains(worktrees, *rec.WorktreePath))
}
