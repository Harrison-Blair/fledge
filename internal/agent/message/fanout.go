package message

import (
	"context"
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
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
// unknown (may have been submitted), deferred (stored but its doorbell not
// rung), or rejected (nothing stored or submitted). MessageID is the row's
// inbox id, or for an unregistered recipient the header id shared by every
// such row; it, InboxID, and State are null for rejected rows.
type Row struct {
	Target    string             `json:"target"`
	Outcome   string             `json:"outcome"`
	MessageID *string            `json:"message_id"`
	InboxID   *string            `json:"inbox_id"`
	State     *string            `json:"state"`
	Agent     *libagent.AgentRow `json:"agent"`
	Error     *cli.Failure       `json:"error"`
}

// fanOut delivers the message to each target in turn, continuing past
// failures; each registered target gets its own record. Any failed row makes
// the outcome partial. Rereads take the store from d.open, the opener that
// selected the targets.
func fanOut(ctx context.Context, c libagent.Client, d delivery, targets []selector.Target) cli.Outcome {
	out := cli.NewOutcome("agent.message")
	result := FanOut{Mode: "fan-out", Targets: []Row{}}
	var failed []libagent.TargetFailure
	for _, t := range targets {
		one := reread(ctx, c, d.o, &t, d.id, d.sender, d.open)
		if one.Error == nil {
			one = deliver(ctx, c, d, t)
		}
		out.Effects = append(out.Effects, one.Effects...)
		r := one.Result.(Result)
		row := Row{Target: t.Label, Outcome: rowOutcome(one.Status, r), Agent: &r.AgentRow, Error: one.Error}
		if row.Outcome != "rejected" {
			row.MessageID, row.InboxID, row.State = &r.MessageID, r.InboxID, &r.State
		}
		if one.Error != nil {
			failed = append(failed, libagent.TargetFailure{Target: t.Label, Code: one.Error.Code})
		}
		result.Targets = append(result.Targets, row)
	}
	out.Result = result
	if out.Error = libagent.FanOutFailure(failed, len(targets), "failed", "agent.prompt"); out.Error != nil {
		out.Status = "partial"
	}
	return out
}

// reread refreshes t just before a message goes to it, as earlier targets'
// deliveries give it time to change; deliver then classifies it by terminal.
// A target selected by record id is resolved again
// from its record, following its terminal to a new pane and rejecting one
// gone or now running another harness, so the message never reaches whatever
// replaced it; store supplies the store for these rereads. Any other target's
// status is reread for a confirmed message; the already-working decision
// needs the status at sending. A failed read rejects the target.
func reread(ctx context.Context, c libagent.Client, o Options, t *selector.Target, id string, sender *libagent.Sender, store func() (*state.Store, error)) cli.Outcome {
	out := cli.NewOutcome("agent.message")
	a, pane, rec, err := t.Agent, t.Pane, t.Record, error(nil)
	switch {
	case t.Record != nil:
		a, pane, rec, err = identity.Target{ID: t.Record.ID}.GetWith(ctx, c, store)
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

func rowOutcome(status string, r Result) string {
	switch {
	case status == "partial" && r.State == "queued":
		return "deferred"
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
			where += " (" + cli.Display(row.Agent.PaneID) + ")"
		}
		var line string
		switch row.Outcome {
		case "deferred":
			line = fmt.Sprintf("Message %s stored for %s but its doorbell was not rung; the supervisor rings it when the agent is ready: %s", id, where, row.Error.Text())
		case "confirmed":
			line = fmt.Sprintf("Message %s submitted to %s; activity confirmed (%s)", id, where, cli.Display(row.Agent.AgentStatus))
		case "already_working":
			line = fmt.Sprintf("Message %s submitted to %s while the agent was already observed working; this prompt's start is not confirmed", id, where)
		case "unconfirmed":
			line = fmt.Sprintf("Message %s submitted to %s, but activity was not confirmed; do not resend it: %s", id, where, row.Error.Text())
		case "unknown":
			line = fmt.Sprintf("Message %s may have been submitted to %s; do not resend it: %s", id, where, row.Error.Text())
		case "rejected":
			line = fmt.Sprintf("Message not submitted to %s: %s", where, row.Error.Text())
		default:
			line = fmt.Sprintf("Message %s submitted to %s", id, where)
		}
		if _, err := fmt.Fprintln(w, line+"."); err != nil {
			return err
		}
	}
	return nil
}
