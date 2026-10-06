package selector

import (
	"context"
	"slices"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

// Selection is either explicit targets or a filter; mixing is invalid.
type Selection struct {
	Names, Panes, IDs []string
	Filter            Filter
}

// Target is one agent a command addresses.
type Target struct {
	Label  string // the flag value as given, or the record id for filter matches
	Pane   string // Herdr target to address
	Agent  herdr.AgentDetails
	Record *identity.Record // nil for name and pane targets and unregistered matches
}

// Validate checks s without contacting Herdr or the store.
func (s Selection) Validate() error {
	explicit := slices.Concat(s.Names, s.Panes, s.IDs)
	switch {
	case len(explicit) > 0 && !s.Filter.Empty():
		return libagent.Invalid("explicit targets (--name, --pane, --id) and filter flags are mutually exclusive")
	case len(explicit) == 0 && s.Filter.Empty():
		return libagent.Invalid("at least one --name, --pane, --id, or filter flag is required")
	}
	for i, t := range explicit {
		if blank(t) {
			return libagent.Invalid("targets must be nonempty")
		}
		if slices.Contains(explicit[:i], t) {
			return libagent.Invalid("duplicate target %q", t)
		}
	}
	for _, id := range s.IDs {
		if err := libagent.ValidateID("id", "agent", id); err != nil {
			return err
		}
	}
	return s.Filter.Validate()
}

// Explicit lists s's explicit targets, unresolved, in flag order: names, then
// panes, then ids. It is nil for a filter.
func (s Selection) Explicit() []identity.Target {
	var targets []identity.Target
	for _, v := range s.Names {
		targets = append(targets, identity.Target{Name: v})
	}
	for _, v := range s.Panes {
		targets = append(targets, identity.Target{Pane: v})
	}
	for _, v := range s.IDs {
		targets = append(targets, identity.Target{ID: v})
	}
	return targets
}

// Targets resolves explicit targets in flag order, names then panes then ids,
// with identity.Target semantics and errors, stopping at the first failure.
// A filter's matches exclude the caller's own pane and fail with
// no_agents_matched when none remain. open supplies the store, as for
// identity.Target.GetWith and Resolve; identity.OpenOnce opens it once.
func (s Selection) Targets(ctx context.Context, c libagent.Client, open func() (*state.Store, error)) ([]Target, error) {
	if err := s.Validate(); err != nil {
		return nil, libagent.AtPhase("validation", err)
	}
	if s.Filter.Empty() {
		var targets []Target
		for _, t := range s.Explicit() {
			a, target, rec, err := t.GetWith(ctx, c, open)
			if err != nil {
				return nil, err
			}
			targets = append(targets, Target{Label: t.Name + t.Pane + t.ID, Pane: target, Agent: a, Record: rec})
		}
		return targets, nil
	}
	matches, err := resolve(ctx, c, s.Filter, open)
	if err != nil {
		return nil, err
	}
	var targets []Target
	for _, m := range matches {
		if c.CallerPane != "" && m.Agent.PaneID == c.CallerPane {
			continue
		}
		label := m.Agent.PaneID
		if m.Record != nil {
			label = m.Record.ID
		}
		targets = append(targets, Target{Label: label, Pane: m.Agent.PaneID, Agent: m.Agent, Record: m.Record})
	}
	if len(targets) == 0 {
		return nil, libagent.AtPhase("selection", &herdr.Error{Code: "no_agents_matched", Message: "no agents matched the selection"})
	}
	return targets, nil
}
