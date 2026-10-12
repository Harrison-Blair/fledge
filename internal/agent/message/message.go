package message

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

// Options selects one or more targets by name, pane, record ID, or filter.
// ReplyTo names an inbox record addressed to the caller that this message answers.
type Options struct {
	selector.Selection
	Body, File       string
	BodySet, FileSet bool
	// Confirm waits up to Timeout for observed activity after submission.
	Confirm    bool
	Timeout    time.Duration
	TimeoutSet bool
	ReplyTo    string
}

// Result reports one target. A registered recipient's message is an inbox
// record: MessageID and InboxID are its id and State is queued or rung. An
// unregistered recipient gets the header and body typed directly: InboxID is
// null, State is direct, and MessageID is the header's id.
type Result struct {
	libagent.AgentRow
	Submitted      bool             `json:"submitted"`
	MessageID      string           `json:"message_id"`
	InboxID        *string          `json:"inbox_id"`
	State          string           `json:"state"`
	Sender         *libagent.Sender `json:"sender"`
	Confirmed      *bool            `json:"confirmed,omitempty"`
	AlreadyWorking bool             `json:"already_working,omitempty"`
}

func (o Options) read(in io.Reader) (string, error) {
	if err := o.Selection.Validate(); err != nil {
		return "", err
	}
	if o.TimeoutSet && !o.Confirm {
		return "", cli.Invalid("--timeout requires --confirm")
	}
	if o.Confirm && o.Timeout < time.Millisecond {
		return "", cli.Invalid("--timeout must be at least 1ms")
	}
	if o.ReplyTo != "" {
		if err := libagent.ValidateID("reply-to", "inbox", o.ReplyTo); err != nil {
			return "", err
		}
	}
	return cli.ReadText(in, cli.TextInput{Body: o.Body, BodyFlag: "body", BodySet: o.BodySet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Required: true, Noun: "message"})
}

// delivery is what every target of one message shares.
type delivery struct {
	o      Options
	body   string // the message as given
	header string // header and body, typed to unregistered recipients
	id     string // the header's message id
	sender *libagent.Sender
	from   inbox.From
	open   func() (*state.Store, error)
}

// Run stores a message for each registered recipient and rings its doorbell,
// or types it with a sender header into an unregistered recipient's pane,
// without waiting for the agent to finish its turn. Several targets are
// handled one after another; a single target's result is unchanged.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) cli.Outcome {
	return run(ctx, c, o, in, libagent.NewMessageID())
}
func run(ctx context.Context, c libagent.Client, o Options, in io.Reader, id string) cli.Outcome {
	out := cli.NewOutcome("agent.message")
	text, err := o.read(in)
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	open := identity.OpenOnce(ctx, c.Cwd)
	targets, err := o.Selection.Targets(ctx, c, open)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	s, err := registry(open)
	var sender libagent.Sender
	var from inbox.From
	if err == nil {
		sender, from, err = inbox.Sender(ctx, c, s)
	}
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	if o.ReplyTo != "" {
		if err := checkReply(s, o.ReplyTo, from.ID); err != nil {
			out.Fail(err, "inbox", false)
			return out
		}
	}
	d := delivery{o: o, body: text, header: libagent.WithHeader(id, sender, text), id: id, sender: &sender, from: from, open: open}
	if len(targets) > 1 {
		out = fanOut(ctx, c, d, targets)
	} else {
		out = deliver(ctx, c, d, targets[0])
	}
	if o.ReplyTo != "" {
		answer(&out, s, o.ReplyTo, *from.ID)
	}
	return out
}

// registry opens the store that decides who is registered. Outside every
// repository there is none, so every recipient is unregistered; any other
// failure is returned, so a registered recipient never gets the body typed.
func registry(open func() (*state.Store, error)) (*state.Store, error) {
	s, err := open()
	if errors.Is(err, fledgedir.ErrNotRepository) {
		return nil, nil
	}
	return s, err
}

// classify returns t's live record and the store holding it, or a nil record
// for an unregistered recipient. A name or pane target is looked up by its
// terminal, as task assign does, and the record follows it to its pane.
func classify(t selector.Target, open func() (*state.Store, error)) (*identity.Record, *state.Store, error) {
	s, err := registry(open)
	if err != nil || t.Record != nil || s == nil {
		return t.Record, s, err
	}
	rec, err := identity.LiveEndingMismatched(s, t.Agent)
	if err != nil || rec == nil {
		return nil, s, err
	}
	moved, err := identity.Relocate(s, *rec, t.Agent)
	return &moved, s, err
}

func checkReply(s *state.Store, id string, caller *string) error {
	if s == nil || caller == nil {
		return &cli.Error{Code: "inbox_not_recipient", Message: fmt.Sprintf("--reply-to %s requires the caller to be that inbox record's registered recipient", id)}
	}
	_, err := inbox.CheckRecipient(s, id, *caller)
	return err
}

// answer marks the original answered by this message's first stored or typed
// id; a failure is a warning, as the reply itself went out.
func answer(out *cli.Outcome, s *state.Store, id, caller string) {
	var reply string
	switch r := out.Result.(type) {
	case Result:
		if out.Status != "rejected" {
			reply = r.MessageID
		}
	case FanOut:
		for _, row := range r.Targets {
			if row.MessageID != nil {
				reply = *row.MessageID
				break
			}
		}
	}
	if reply == "" {
		return
	}
	_, err := inbox.Answer(s, id, caller, reply)
	action := "updated"
	if err != nil {
		action = "warning"
	}
	out.Effects = append(out.Effects, cli.Effect{Action: action, Kind: "inbox", ID: id})
}

// deliver stores and rings, or types directly, the message for one target.
func deliver(ctx context.Context, c libagent.Client, d delivery, t selector.Target) cli.Outcome {
	out := cli.NewOutcome("agent.message")
	o, a, target := d.o, t.Agent, t.Pane
	result := Result{AgentRow: libagent.NewAgentRow(a.Pane), MessageID: d.id, State: "direct", Sender: d.sender}
	if o.Confirm {
		result.Confirmed = new(bool)
	}
	out.Result = result
	rec, s, err := classify(t, d.open)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	text := d.header
	var stored inbox.Record
	if rec != nil {
		var reply *string
		if o.ReplyTo != "" {
			reply = &o.ReplyTo
		}
		stored, err = inbox.Create(s, inbox.Record{Kind: inbox.Message, From: d.from, To: rec.ID, Body: d.body, ReplyTo: reply})
		if err != nil {
			out.Fail(err, "inbox", false)
			return out
		}
		out.Effects = append(out.Effects, cli.Effect{Action: "created", Kind: "inbox", ID: stored.ID})
		result.MessageID, result.InboxID, result.State = stored.ID, &stored.ID, "queued"
		text = inbox.Doorbell(stored, "")
		out.Result = result
	}
	var agent herdr.AgentDetails
	if o.Confirm {
		agent, err = c.PromptConfirm(ctx, target, text, o.Timeout)
	} else {
		agent, err = c.Prompt(ctx, target, text)
	}
	if rec != nil {
		// A stalled confirmation was typed: the doorbell rang.
		ringErr := err
		if code, _ := cli.Coded(err); code == "agent_prompt_stalled" {
			ringErr = nil
		}
		updated, rerr := inbox.AddRing(s, stored.ID, a.PaneID, ringErr)
		switch {
		case rerr != nil:
			out.Effects = append(out.Effects, cli.Effect{Action: "warning", Kind: "inbox", ID: stored.ID})
		case inbox.Rung(updated):
			result.State = "rung"
		}
		out.Result = result
	}
	if err != nil {
		out.Fail(err, "agent.prompt", true)
		if o.Confirm {
			confirmFailure(&out, err, a.PaneID)
		}
		return out
	}
	out.Effects = append(out.Effects, cli.Effect{Action: "submitted", Kind: "message", ID: agent.PaneID})
	result.AgentRow, result.Submitted = libagent.NewAgentRow(agent.Pane), true
	out.Result = result
	if !o.Confirm {
		return out
	}
	switch {
	case agent.AgentStatus == "blocked":
		out.Status = "partial"
		out.Error = &cli.Failure{Code: "agent_prompt_blocked", Message: "agent became blocked after the message was submitted", Phase: "agent.prompt"}
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
func confirmFailure(out *cli.Outcome, err error, pane string) {
	var remote *herdr.Error
	if !errors.As(err, &remote) {
		return
	}
	switch remote.Code {
	case "agent_prompt_stalled":
		out.Status = "partial"
		out.Effects = append(out.Effects, cli.Effect{Action: "submitted", Kind: "message", ID: pane})
		r := out.Result.(Result)
		r.Submitted = true
		out.Result = r
	case "timeout":
		out.Status = "unknown"
	}
}

// Render writes a message outcome, adding a no-resend hint when a confirmed
// message may have been, or was, submitted without confirmed activity, and
// saying when a stored message waits for the supervisor to ring it.
func Render(w io.Writer, o cli.Outcome) error {
	if f, ok := o.Result.(FanOut); ok {
		return renderFanOut(w, f)
	}
	r, ok := o.Result.(Result)
	if !ok {
		return nil
	}
	if o.Error != nil {
		var err error
		switch {
		case r.InboxID != nil && r.State == "queued" && o.Status == "partial":
			_, err = fmt.Fprintf(w, "Message %s is stored for %s but its doorbell was not rung; the supervisor rings it when the agent is ready.\n", r.MessageID, cli.Display(r.PaneID))
		case r.InboxID != nil && o.Status == "unknown":
			_, err = fmt.Fprintf(w, "Message %s is stored for %s; its doorbell may have been rung, and the supervisor may ring it again. Do not resend it.\n", r.MessageID, cli.Display(r.PaneID))
		case r.Confirmed == nil || (o.Status != "partial" && o.Status != "unknown"):
		default:
			hint := "The message may have been submitted"
			if r.Submitted {
				hint = "The message was submitted, but activity was not confirmed"
			}
			_, err = fmt.Fprintf(w, "%s; do not resend it. Inspect: fledge agent read --pane %s\n", hint, cli.Display(r.PaneID))
		}
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
		suffix = fmt.Sprintf("; activity confirmed (%s).", cli.Display(r.AgentStatus))
	}
	if r.InboxID != nil {
		if suffix == "." {
			suffix = "; doorbell rung."
		} else {
			suffix = "; doorbell rung" + suffix
		}
		_, err := fmt.Fprintf(w, "Message %s stored for %s from %s%s\n", r.MessageID, cli.Display(r.PaneID), sender, suffix)
		return err
	}
	_, err := fmt.Fprintf(w, "Message %s submitted to %s from %s%s\nThe recipient has no Fledge record, so the message was typed into its pane directly.\n", r.MessageID, cli.Display(r.PaneID), sender, suffix)
	return err
}
