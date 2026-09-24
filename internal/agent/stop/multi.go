package stop

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

// FanOut reports a multi-target stop (mode fan-out) or any dry run (mode
// dry-run): one row per target, in target order.
type FanOut struct {
	Mode    string `json:"mode"`
	Targets []Row  `json:"targets"`
}

// Row reports one target. A stop's Outcome is stopped, refused (needs
// --force), or failed; a dry run's is stop, refuse, or error. Error gives the
// reason for any other outcome, and for a stopped row whose record could not
// be ended.
type Row struct {
	Target  string             `json:"target"`
	Outcome string             `json:"outcome"`
	Agent   *libagent.AgentRow `json:"agent"`
	Error   *libagent.Failure  `json:"error"`
}

// pending is one target, looked up afresh when its turn comes.
type pending struct {
	label  string
	target identity.Target
	// match is a filter's listing of the agent, and terminal the terminal it
	// matched when no record follows it: a pane now hosting another terminal
	// is not the matched agent.
	match    *herdr.AgentDetails
	terminal string
}

// targets lists explicit targets in flag order, names then panes then ids, or
// resolves the filter, whose matches exclude the caller.
func (o Options) targets(ctx context.Context, c libagent.Client) ([]pending, error) {
	var ps []pending
	if o.Filter.Empty() {
		for _, v := range o.Names {
			ps = append(ps, pending{label: v, target: identity.Target{Name: v}})
		}
		for _, v := range o.Panes {
			ps = append(ps, pending{label: v, target: identity.Target{Pane: v}})
		}
		for _, v := range o.IDs {
			ps = append(ps, pending{label: v, target: identity.Target{ID: v}})
		}
		return ps, nil
	}
	matches, err := o.Selection.Targets(ctx, c)
	if err != nil {
		return nil, err
	}
	for _, m := range matches {
		p := pending{label: m.Label, target: identity.Target{Pane: m.Pane}, match: &m.Agent, terminal: m.Agent.TerminalID}
		if m.Record != nil {
			p.target, p.terminal = identity.Target{ID: m.Record.ID}, ""
		}
		ps = append(ps, p)
	}
	return ps, nil
}

func (p pending) get(ctx context.Context, c libagent.Client) (herdr.AgentDetails, string, *identity.Record, error) {
	a, target, rec, err := p.target.Get(ctx, c)
	if err == nil && p.terminal != "" && a.TerminalID != p.terminal {
		err = libagent.AtPhase("identity", &herdr.Error{Code: "agent_identity_stale", Message: fmt.Sprintf("pane %s now hosts a different agent than the one matched", target)})
	}
	return a, target, rec, err
}

// plan reports what stopping each target would do. It only reads: explicit
// targets are looked up, filter matches come from the listing.
func plan(ctx context.Context, c libagent.Client, o Options, targets []pending) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.stop", Status: "success", Effects: []libagent.Effect{}}
	result := FanOut{Mode: "dry-run", Targets: []Row{}}
	for _, p := range targets {
		row := Row{Target: p.label, Outcome: "stop"}
		var a herdr.AgentDetails
		var target string
		var err error
		if p.match != nil {
			a, target = *p.match, p.match.PaneID
		} else if a, target, _, err = p.target.Get(ctx, c); err != nil {
			row.Outcome, row.Error = "error", failure(err, "agent.get")
		}
		if err == nil {
			agent := libagent.NewAgentRow(a.Pane)
			row.Agent = &agent
			if err := o.guard(a, target); err != nil {
				row.Outcome, row.Error = "refuse", failure(err, "guard")
				if grace := o.EffectiveGrace(); a.AgentStatus == "working" && grace > 0 {
					row.Error.Message += fmt.Sprintf(" (stop first waits up to %s for it to finish its turn)", grace)
				}
			}
		}
		result.Targets = append(result.Targets, row)
	}
	out.Result = result
	summarize(&out, result.Targets, func(r Row) bool { return r.Outcome == "error" }, "rejected", "could not be planned")
	return out
}

// fanOut stops each target in turn, continuing past refusals and failures.
func fanOut(ctx context.Context, c libagent.Client, o Options, targets []pending) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.stop", Status: "success", Effects: []libagent.Effect{}}
	result := FanOut{Mode: "fan-out", Targets: []Row{}}
	for _, p := range targets {
		one := stopOne(ctx, c, o, p)
		out.Effects = append(out.Effects, one.Effects...)
		row := Row{Target: p.label, Outcome: "failed", Error: one.Error}
		if r, ok := one.Result.(Result); ok {
			row.Agent = &r.AgentRow
			if r.Stopped {
				row.Outcome = "stopped"
			}
		} else if p.match != nil {
			agent := libagent.NewAgentRow(p.match.Pane)
			row.Agent = &agent
		}
		if row.Outcome == "failed" && one.Error.Phase == "guard" {
			row.Outcome = "refused"
		}
		result.Targets = append(result.Targets, row)
	}
	out.Result = result
	summarize(&out, result.Targets, func(r Row) bool { return r.Error != nil }, "partial", "not stopped cleanly")
	return out
}

// failure classifies err as a single stop's outcome would.
func failure(err error, phase string) *libagent.Failure {
	var o libagent.Outcome
	o.Fail(err, phase, false)
	return o.Error
}

// summarize fails out with status when any row is bad, naming each bad row
// and its code; one shared code is kept, several become operation_failed.
func summarize(out *libagent.Outcome, rows []Row, bad func(Row) bool, status, what string) {
	var failures, codes []string
	for _, r := range rows {
		if !bad(r) {
			continue
		}
		failures = append(failures, fmt.Sprintf("%s (%s)", r.Target, r.Error.Code))
		if !slices.Contains(codes, r.Error.Code) {
			codes = append(codes, r.Error.Code)
		}
	}
	if len(failures) == 0 {
		return
	}
	code := "operation_failed"
	if len(codes) == 1 {
		code = codes[0]
	}
	out.Status = status
	out.Error = &libagent.Failure{Code: code, Message: fmt.Sprintf("%d of %d targets %s: %s", len(failures), len(rows), what, strings.Join(failures, ", ")), Phase: "agent.stop"}
}

var verbs = map[string]string{"dry-run": "Dry run: would stop %d of %d agents.\n", "fan-out": "Stopped %d of %d agents.\n"}

// renderFanOut writes a count line, then one line per target with the reason
// for any target not stopped.
func renderFanOut(w io.Writer, f FanOut) error {
	done, width := 0, 0
	for _, r := range f.Targets {
		if r.Outcome == "stop" || r.Outcome == "stopped" {
			done++
		}
		width = max(width, len(r.Outcome))
	}
	if _, err := fmt.Fprintf(w, verbs[f.Mode], done, len(f.Targets)); err != nil {
		return err
	}
	for _, r := range f.Targets {
		subject := r.Target
		if r.Agent != nil {
			where := libagent.Display(r.Agent.PaneID)
			if f.Mode == "dry-run" {
				where += ", " + libagent.Display(r.Agent.AgentStatus)
			}
			subject += " (" + where + ")"
		}
		if r.Error != nil {
			subject += ": " + r.Error.Message
		}
		if _, err := fmt.Fprintf(w, "  %-*s  %s\n", width, r.Outcome, subject); err != nil {
			return err
		}
	}
	return nil
}
