package message

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
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
	for _, t := range targets {
		one := deliver(ctx, c, o, t, text, id, sender)
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
