package message

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

// FanOut reports a message to several targets: one row per target, in target order.
type FanOut struct {
	Mode    string `json:"mode"`
	Targets []Row  `json:"targets"`
}

// Row reports one target's delivery. Outcome is submitted, confirmed,
// already_working, unconfirmed (submitted without confirmed activity),
// unknown (may have been submitted), or rejected (not submitted). MessageID
// is null only for rejected rows.
type Row struct {
	Target    string             `json:"target"`
	Outcome   string             `json:"outcome"`
	MessageID *string            `json:"message_id"`
	Agent     *libagent.AgentRow `json:"agent"`
	Error     *libagent.Failure  `json:"error"`
}

// fanOut delivers text to each target in turn, continuing past failures.
// Any failed row makes the outcome partial.
func fanOut(ctx context.Context, c libagent.Client, o Options, targets []selector.Target, text, id string, sender *libagent.Sender) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.message", Status: "success", Effects: []libagent.Effect{}}
	result := FanOut{Mode: "fan-out", Targets: []Row{}}
	var failures, codes []string
	store := sync.OnceValues(func() (*state.Store, error) { return identity.Existing(ctx, c.Cwd) })
	for _, t := range targets {
		one := reread(ctx, c, o, &t, id, sender, store)
		if one.Error == nil {
			one = deliver(ctx, c, o, t, text, id, sender)
		}
		out.Effects = append(out.Effects, one.Effects...)
		r := one.Result.(Result)
		row := Row{Target: t.Label, Outcome: rowOutcome(one.Status, r), Agent: &r.AgentRow, Error: one.Error}
		if row.Outcome != "rejected" {
			row.MessageID = &id
		}
		if one.Error != nil {
			failures = append(failures, fmt.Sprintf("%s (%s)", t.Label, one.Error.Code))
			if !slices.Contains(codes, one.Error.Code) {
				codes = append(codes, one.Error.Code)
			}
		}
		result.Targets = append(result.Targets, row)
	}
	out.Result = result
	if len(failures) > 0 {
		code := "operation_failed"
		if len(codes) == 1 {
			code = codes[0]
		}
		out.Status = "partial"
		out.Error = &libagent.Failure{Code: code, Message: fmt.Sprintf("%d of %d targets failed: %s", len(failures), len(targets), strings.Join(failures, ", ")), Phase: "agent.prompt"}
	}
	return out
}

// reread refreshes t just before a message goes to it, as earlier targets'
// deliveries give it time to change. A registered target is resolved again
// from its record, following its terminal to a new pane and rejecting one
// gone or now running another harness, so the message never reaches whatever
// replaced it; store opens the store once for every such reread. Any other
// target's status is reread for a confirmed message; the already-working
// decision needs the status at sending. A failed read rejects the target.
func reread(ctx context.Context, c libagent.Client, o Options, t *selector.Target, id string, sender *libagent.Sender, store func() (*state.Store, error)) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.message", Status: "success", Effects: []libagent.Effect{}}
	a, pane, rec, err := t.Agent, t.Pane, t.Record, error(nil)
	switch {
	case t.Record != nil:
		a, pane, rec, err = resolve(ctx, c, t.Record.ID, store)
	case o.Confirm:
		a, err = c.Get(ctx, t.Pane)
	default:
		return out
	}
	if err != nil {
		r := Result{AgentRow: libagent.NewAgentRow(t.Agent.Pane), MessageID: id, Sender: sender}
		if o.Confirm {
			r.Confirmed = new(bool)
		}
		out.Result = r
		out.Fail(err, "agent.get", false)
		return out
	}
	t.Agent, t.Pane, t.Record = a, pane, rec
	return out
}

// resolve is identity.Target{ID: id}.Get with the store opened by store, so
// rereads share one store; its errors are located at the same phases.
func resolve(ctx context.Context, c libagent.Client, id string, store func() (*state.Store, error)) (herdr.AgentDetails, string, *identity.Record, error) {
	s, err := store()
	if err != nil {
		return herdr.AgentDetails{}, "", nil, libagent.AtPhase("identity", err)
	}
	rec, a, err := identity.Resolve(ctx, s, c, id)
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

func rowOutcome(status string, r Result) string {
	switch {
	case status == "partial":
		return "unconfirmed"
	case status == "unknown":
		return "unknown"
	case status != "success":
		return "rejected"
	case r.AlreadyWorking:
		return "already_working"
	case r.Confirmed != nil && *r.Confirmed:
		return "confirmed"
	}
	return "submitted"
}

func renderFanOut(w io.Writer, f FanOut) error {
	for _, row := range f.Targets {
		id, where := "", row.Target
		if row.MessageID != nil {
			id = *row.MessageID
		}
		if row.Agent != nil {
			where += " (" + libagent.Display(row.Agent.PaneID) + ")"
		}
		var line string
		switch row.Outcome {
		case "confirmed":
			line = fmt.Sprintf("Message %s submitted to %s; activity confirmed (%s)", id, where, libagent.Display(row.Agent.AgentStatus))
		case "already_working":
			line = fmt.Sprintf("Message %s submitted to %s while the agent was already observed working; this prompt's start is not confirmed", id, where)
		case "unconfirmed":
			line = fmt.Sprintf("Message %s submitted to %s, but activity was not confirmed; do not resend it: %s", id, where, row.Error.Message)
		case "unknown":
			line = fmt.Sprintf("Message %s may have been submitted to %s; do not resend it: %s", id, where, row.Error.Message)
		case "rejected":
			line = fmt.Sprintf("Message not submitted to %s: %s", where, row.Error.Message)
		default:
			line = fmt.Sprintf("Message %s submitted to %s", id, where)
		}
		if _, err := fmt.Fprintln(w, line+"."); err != nil {
			return err
		}
	}
	return nil
}
