// Package usage implements agent usage: one token and cost summary per
// selected agent, read from the harness's own session store.
package usage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/harnessenv"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	libusage "github.com/Harrison-Blair/fledge/internal/lib/usage"
)

// noRef is the reason for a target without a session ref live or recorded.
const noRef = "no native session ref observed"

// Options selects agents by name, pane, record ID, or filter.
type Options struct {
	selector.Selection
}

type Result struct {
	Agents []Row `json:"agents"`
}

// Row is one agent's whole-session usage. AgentID and ElapsedSeconds are
// null for an agent without a record.
type Row struct {
	AgentID        *string `json:"agent_id"`
	Name           *string `json:"name"`
	Pane           *string `json:"pane"`
	ElapsedSeconds *int64  `json:"elapsed_seconds"`
	libusage.Summary
}

// now is replaceable so tests can fix elapsed time.
var now = time.Now

// liveByTerminal is identity.LiveByTerminal, replaceable so tests can count
// scans of the live records.
var liveByTerminal = identity.LiveByTerminal

// Run summarizes each selected agent's session. Its harness and session ref
// come from the live agent when present, else from its record, so an --id
// whose agent has ended still reports. A live ref is persisted on the agent's
// record; a failed write is only a warning effect.
func Run(ctx context.Context, c libagent.Client, d harnessenv.Env, o Options) libagent.Outcome {
	out := libagent.NewOutcome("agent.usage")
	if err := o.Selection.Validate(); err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	open := identity.OpenOnce(ctx, c.Cwd)
	targets, err := resolve(ctx, c, o.Selection, open)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	// Like agent get, an unavailable store holds no records.
	s, err := open()
	if err != nil {
		s = nil
	}
	records := attributable(s, targets)
	rows := make([]Row, 0, len(targets))
	for _, t := range targets {
		rows = append(rows, row(ctx, s, records, d, t, &out))
	}
	out.Result = Result{Agents: rows}
	return out
}

// resolve returns the selected targets, reading records from the store open
// supplies. An explicit --id whose agent is no longer live resolves to its
// record alone, with a zero Agent.
func resolve(ctx context.Context, c libagent.Client, sel selector.Selection, open func() (*state.Store, error)) ([]selector.Target, error) {
	if !sel.Filter.Empty() || len(sel.IDs) == 0 {
		return sel.Targets(ctx, c, open)
	}
	var targets []selector.Target
	if len(sel.Names)+len(sel.Panes) > 0 {
		var err error
		if targets, err = (selector.Selection{Names: sel.Names, Panes: sel.Panes}).Targets(ctx, c, open); err != nil {
			return nil, err
		}
	}
	for _, id := range sel.IDs {
		a, pane, rec, err := identity.Target{ID: id}.GetWith(ctx, c, open)
		var remote *herdr.Error
		if errors.As(err, &remote) && remote.Code == "agent_identity_stale" {
			if rec, err = stored(open, id); err == nil {
				a, pane = herdr.AgentDetails{}, rec.Pane
			}
		}
		if err != nil {
			return nil, err
		}
		targets = append(targets, selector.Target{Label: id, Pane: pane, Agent: a, Record: rec})
	}
	return targets, nil
}

// stored reads record id, live or archived, from the store open supplies.
func stored(open func() (*state.Store, error), id string) (*identity.Record, error) {
	s, err := open()
	if err != nil {
		return nil, libagent.AtPhase("identity", err)
	}
	var rec identity.Record
	if err := s.Get(identity.Kind, id, &rec); err != nil {
		return nil, libagent.AtPhase("identity", err)
	}
	return &rec, nil
}

// attributable maps each terminal to its live record, scanning the store once
// and only when a live target carries no record. A failed scan attributes
// nothing.
func attributable(s *state.Store, targets []selector.Target) map[string]identity.Record {
	if s == nil {
		return nil
	}
	for _, t := range targets {
		if t.Record == nil && t.Agent.TerminalID != "" {
			records, err := liveByTerminal(s)
			if err != nil {
				return nil
			}
			return records
		}
	}
	return nil
}

// row reports t's usage. A live target without a record is attributed to its
// record in records, an attributable map.
func row(ctx context.Context, s *state.Store, records map[string]identity.Record, d harnessenv.Env, t selector.Target, out *libagent.Outcome) Row {
	a, rec := t.Agent, t.Record
	live := a.TerminalID != ""
	if rec == nil {
		if found, ok := identity.Attributed(records, a); ok {
			rec = &found
		}
	}
	if live && rec != nil && a.AgentSession != nil {
		observed := identity.Observe(s, identity.ObserveSession, *rec, &a, now(), out)
		rec = &observed
	}
	var r Row
	var kind string
	var ref *libusage.Ref
	// t.Pane is the address used to reach the agent, a name for --name; the
	// row reports the resolved pane instead.
	if live {
		r.Pane = libagent.Pointer(a.PaneID)
		r.Name, kind = a.Name, deref(a.Agent)
		if as := a.AgentSession; as != nil && deref(as.Value) != "" {
			ref = &libusage.Ref{Kind: deref(as.Kind), Value: *as.Value, Cwd: deref(a.Cwd)}
		}
	}
	if rec != nil {
		r.AgentID, r.ElapsedSeconds = &rec.ID, elapsed(*rec)
		if !live {
			r.Pane = libagent.Pointer(rec.Pane)
			r.Name, kind = rec.Name, deref(rec.Harness)
		}
		if ns := rec.NativeSession; ref == nil && ns != nil {
			ref = &libusage.Ref{Kind: ns.Kind, Value: ns.Value, Cwd: deref(rec.WorktreePath)}
			if !live && ns.Harness != "" {
				kind = ns.Harness
			}
		}
	}
	if ref == nil {
		r.Summary = libusage.Summary{Harness: kind, Basis: libusage.Unavailable, Reason: noRef}
		return r
	}
	r.Summary = libusage.Read(ctx, d, kind, *ref, libusage.Window{})
	return r
}

// elapsed is the time from registration to the end, or to now while live.
func elapsed(rec identity.Record) *int64 {
	from, err := time.Parse(time.RFC3339, rec.RegisteredAt)
	if err != nil {
		return nil
	}
	to := now()
	if rec.EndedAt != nil {
		if to, err = time.Parse(time.RFC3339, *rec.EndedAt); err != nil {
			return nil
		}
	}
	seconds := int64(to.Sub(from) / time.Second)
	return &seconds
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Render writes one table row per agent, then any persist warnings.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	if len(r.Agents) > 0 {
		table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(table, "NAME\tHARNESS\tMODELS\tTURNS\tINPUT\tOUTPUT\tCACHE-R\tCACHE-W\tCOST\tELAPSED\tBASIS")
		for _, a := range r.Agents {
			cells := []string{libagent.Display(a.Name), libagent.DisplayString(a.Harness), libagent.DisplayString(strings.Join(a.Models, ","))}
			if a.Basis == libusage.Measured {
				t := a.Tokens
				cells = append(cells, strconv.Itoa(a.Turns), libusage.Count(t.Input), libusage.Count(t.Output), libusage.Count(t.CacheRead), libusage.Count(t.CacheWrite))
			} else {
				cells = append(cells, "-", "-", "-", "-", "-")
			}
			cost, span, basis := "-", "-", a.Basis
			if a.Cost != nil {
				cost = fmt.Sprintf("$%.2f (est)", a.Cost.Amount)
			}
			if a.ElapsedSeconds != nil {
				span = (time.Duration(*a.ElapsedSeconds) * time.Second).String()
			}
			if basis != libusage.Measured && a.Reason != "" {
				basis += " (" + a.Reason + ")"
			}
			fmt.Fprintln(table, strings.Join(append(cells, cost, span, basis), "\t"))
		}
		if err := table.Flush(); err != nil {
			return err
		}
	}
	for _, e := range o.Effects {
		if e.Action != "warning" {
			continue
		}
		if _, err := fmt.Fprintf(w, "warning: could not record the native session ref of agent %s\n", e.ID); err != nil {
			return err
		}
	}
	return nil
}
