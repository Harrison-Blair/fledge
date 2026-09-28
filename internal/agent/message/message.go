package message

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
)

// Options selects one or more targets by name, pane, record ID, or filter.
type Options struct {
	selector.Selection
	Body, File       string
	BodySet, FileSet bool
	// Confirm waits up to Timeout for observed activity after submission.
	Confirm    bool
	Timeout    time.Duration
	TimeoutSet bool
}
type Result struct {
	libagent.AgentRow
	Submitted      bool             `json:"submitted"`
	MessageID      string           `json:"message_id"`
	Sender         *libagent.Sender `json:"sender"`
	Confirmed      *bool            `json:"confirmed,omitempty"`
	AlreadyWorking bool             `json:"already_working,omitempty"`
}

func (o Options) read(in io.Reader) (string, error) {
	if err := o.Selection.Validate(); err != nil {
		return "", err
	}
	if o.TimeoutSet && !o.Confirm {
		return "", libagent.Invalid("--timeout requires --confirm")
	}
	if o.Confirm && o.Timeout < time.Millisecond {
		return "", libagent.Invalid("--timeout must be at least 1ms")
	}
	return libagent.ReadText(in, libagent.TextInput{Body: o.Body, BodyFlag: "body", BodySet: o.BodySet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Required: true, Noun: "message"})
}

// Run submits a message, prefixed with a sender header, without waiting for
// the agent to finish its turn. Several targets receive the same header and
// message ID, one after another; a single target's result is unchanged.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) libagent.Outcome {
	return run(ctx, c, o, in, libagent.NewMessageID())
}
func run(ctx context.Context, c libagent.Client, o Options, in io.Reader, id string) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.message", Status: "success", Effects: []libagent.Effect{}}
	text, err := o.read(in)
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	targets, err := o.Selection.Targets(ctx, c)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	sender := libagent.ResolveSender(ctx, c)
	text = libagent.WithHeader(id, sender, text)
	if len(targets) > 1 {
		return fanOut(ctx, c, o, targets, text, id, &sender)
	}
	return deliver(ctx, c, o, targets[0], text, id, &sender)
}

// deliver submits text, which already carries its header, to one target.
func deliver(ctx context.Context, c libagent.Client, o Options, t selector.Target, text, id string, sender *libagent.Sender) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.message", Status: "success", Effects: []libagent.Effect{}}
	a, target := t.Agent, t.Pane
	result := Result{AgentRow: libagent.NewAgentRow(a.Pane), MessageID: id, Sender: sender}
	if o.Confirm {
		result.Confirmed = new(bool)
	}
	out.Result = result
	var agent herdr.AgentDetails
	var err error
	if o.Confirm {
		agent, err = c.PromptConfirm(ctx, target, text, o.Timeout)
	} else {
		agent, err = c.Prompt(ctx, target, text)
	}
	if err != nil {
		out.Fail(err, "agent.prompt", true)
		if o.Confirm {
			confirmFailure(&out, err, a.PaneID)
		}
		return out
	}
	out.Effects = append(out.Effects, libagent.Effect{Action: "submitted", Kind: "message", ID: agent.PaneID})
	result.AgentRow, result.Submitted = libagent.NewAgentRow(agent.Pane), true
	out.Result = result
	if !o.Confirm {
		return out
	}
	switch {
	case agent.AgentStatus == "blocked":
		out.Status = "partial"
		out.Error = &libagent.Failure{Code: "agent_prompt_blocked", Message: "agent became blocked after the message was submitted", Phase: "agent.prompt"}
	case a.AgentStatus == "working":
		result.AlreadyWorking = true
	default:
		*result.Confirmed = true
	}
	out.Result = result
	return out
}

// confirmFailure corrects Fail's classification for failures Herdr raises only
// after typing the message (stalled) or that cannot tell whether it was typed
// (a wait timeout). Neither may be reported as unsent, and neither is retried.
func confirmFailure(out *libagent.Outcome, err error, pane string) {
	var remote *herdr.Error
	if !errors.As(err, &remote) {
		return
	}
	switch remote.Code {
	case "agent_prompt_stalled":
		out.Status = "partial"
		out.Effects = append(out.Effects, libagent.Effect{Action: "submitted", Kind: "message", ID: pane})
		r := out.Result.(Result)
		r.Submitted = true
		out.Result = r
	case "timeout":
		out.Status = "unknown"
	}
}

// Render writes a message outcome, adding a no-resend hint when a confirmed
// message may have been, or was, submitted without confirmed activity.
func Render(w io.Writer, o libagent.Outcome) error {
	if f, ok := o.Result.(FanOut); ok {
		return renderFanOut(w, f)
	}
	r, ok := o.Result.(Result)
	if !ok {
		return nil
	}
	if o.Error != nil {
		if r.Confirmed == nil || (o.Status != "partial" && o.Status != "unknown") {
			return nil
		}
		hint := "The message may have been submitted"
		if r.Submitted {
			hint = "The message was submitted, but activity was not confirmed"
		}
		_, err := fmt.Fprintf(w, "%s; do not resend it. Inspect: fledge agent read --pane %s\n", hint, libagent.Display(r.PaneID))
		return err
	}
	sender := "unknown sender"
	if r.Sender != nil {
		sender = r.Sender.String()
	}
	suffix := "."
	switch {
	case r.AlreadyWorking:
		suffix = " while the agent was already observed working; this prompt's start is not confirmed."
	case r.Confirmed != nil:
		suffix = fmt.Sprintf("; activity confirmed (%s).", libagent.Display(r.AgentStatus))
	}
	_, err := fmt.Fprintf(w, "Message %s submitted to %s from %s%s\n", r.MessageID, libagent.Display(r.PaneID), sender, suffix)
	return err
}
