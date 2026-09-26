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

// Run summarizes each selected agent's session. Its harness and session ref
// come from the live agent when present, else from its record, so an --id
// whose agent has ended still reports. A live ref is persisted on the agent's
// record; a failed write is only a warning effect.
func Run(ctx context.Context, c libagent.Client, d libusage.Discovery, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.usage", Status: "success", Effects: []libagent.Effect{}}
	if err := o.Selection.Validate(); err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	targets, err := resolve(ctx, c, o.Selection)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	// Like agent get, an unavailable store holds no records.
	s, err := identity.Existing(ctx, c.Cwd)
	if err != nil {
		s = nil
	}
	rows := make([]Row, 0, len(targets))
	for _, t := range targets {
		rows = append(rows, row(ctx, s, d, t, &out))
	}
	out.Result = Result{Agents: rows}
	return out
}

// resolve returns the selected targets. An explicit --id whose agent is no
// longer live resolves to its record alone, with a zero Agent.
func resolve(ctx context.Context, c libagent.Client, sel selector.Selection) ([]selector.Target, error) {
	if !sel.Filter.Empty() || len(sel.IDs) == 0 {
		return sel.Targets(ctx, c)
	}
	var targets []selector.Target
	if len(sel.Names)+len(sel.Panes) > 0 {
		var err error
		if targets, err = (selector.Selection{Names: sel.Names, Panes: sel.Panes}).Targets(ctx, c); err != nil {
			return nil, err
		}
	}
	for _, id := range sel.IDs {
		a, pane, rec, err := identity.Target{ID: id}.Get(ctx, c)
		var remote *herdr.Error
		if errors.As(err, &remote) && remote.Code == "agent_identity_stale" {
			if rec, err = stored(ctx, c.Cwd, id); err == nil {
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

// stored reads record id, live or archived.
func stored(ctx context.Context, cwd, id string) (*identity.Record, error) {
	s, err := identity.Existing(ctx, cwd)
	if err != nil {
		return nil, libagent.AtPhase("identity", err)
	}
	var rec identity.Record
	if err := s.Get(identity.Kind, id, &rec); err != nil {
		return nil, libagent.AtPhase("identity", err)
	}
	return &rec, nil
}

func row(ctx context.Context, s *state.Store, d libusage.Discovery, t selector.Target, out *libagent.Outcome) Row {
	a, rec := t.Agent, t.Record
	live := a.TerminalID != ""
	if rec == nil && live && s != nil {
		if found, err := identity.Live(s, a.TerminalID); err == nil && found != nil && !identity.Mismatched(*found, a) {
			rec = found
		}
	}
	if live && rec != nil && a.AgentSession != nil {
		rec = observe(s, *rec, *a.AgentSession, out)
	}
	r := Row{Pane: libagent.Pointer(t.Pane)}
	var kind string
	var ref *libusage.Ref
	if live {
		r.Name, kind = a.Name, deref(a.Agent)
		if as := a.AgentSession; as != nil && deref(as.Value) != "" {
			ref = &libusage.Ref{Kind: deref(as.Kind), Value: *as.Value, Cwd: deref(a.Cwd)}
		}
	}
	if rec != nil {
		r.AgentID, r.ElapsedSeconds = &rec.ID, elapsed(*rec)
		if !live {
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

// observe persists session on rec and returns the updated record, or rec as
// it was when the write fails, which is only a warning.
func observe(s *state.Store, rec identity.Record, session herdr.AgentSession, out *libagent.Outcome) *identity.Record {
	updated, changed, err := identity.ObserveSession(s, rec.ID, session, now())
	switch {
	case err != nil:
		out.Effects = append(out.Effects, libagent.Effect{Action: "warning", Kind: "native_session", ID: rec.ID})
		return &rec
	case changed:
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "native_session", ID: rec.ID})
	}
	return &updated
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
			cells := []string{libagent.Display(a.Name), dash(a.Harness), dash(strings.Join(a.Models, ","))}
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

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
