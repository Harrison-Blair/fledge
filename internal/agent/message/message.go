// Package message implements agent message: acknowledged prompt submission.
package message

import (
	"context"
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

type Options struct {
	identity.Target
	Body, File       string
	BodySet, FileSet bool
}
type Result struct {
	libagent.AgentRow
	Submitted bool             `json:"submitted"`
	MessageID string           `json:"message_id"`
	Sender    *libagent.Sender `json:"sender"`
}

func (o Options) read(in io.Reader) (identity.Target, string, error) {
	target := o.Target
	if err := target.Validate(); err != nil {
		return target, "", err
	}
	text, err := libagent.ReadText(in, libagent.TextInput{Body: o.Body, BodyFlag: "body", BodySet: o.BodySet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Required: true, Noun: "message"})
	if err != nil {
		return target, "", err
	}
	return target, text, nil
}

// Run submits a message, prefixed with a sender header, without waiting for
// the agent to finish its turn.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) libagent.Outcome {
	return run(ctx, c, o, in, libagent.NewMessageID())
}
func run(ctx context.Context, c libagent.Client, o Options, in io.Reader, id string) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.message", Status: "success", Effects: []libagent.Effect{}}
	selected, text, err := o.read(in)
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	a, target, _, err := selected.Get(ctx, c)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	sender := libagent.ResolveSender(ctx, c)
	out.Result = Result{AgentRow: libagent.NewAgentRow(a.Pane), MessageID: id, Sender: &sender}
	agent, err := c.Prompt(ctx, target, libagent.WithHeader(id, sender, text))
	if err != nil {
		out.Fail(err, "agent.prompt", true)
		return out
	}
	out.Effects = append(out.Effects, libagent.Effect{Action: "submitted", Kind: "message", ID: agent.PaneID})
	out.Result = Result{AgentRow: libagent.NewAgentRow(agent.Pane), Submitted: true, MessageID: id, Sender: &sender}
	return out
}

// Render writes a successful message outcome.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	sender := "unknown sender"
	if r.Sender != nil {
		sender = r.Sender.String()
	}
	_, err := fmt.Fprintf(w, "Message %s submitted to %s from %s.\n", r.MessageID, libagent.Display(r.PaneID), sender)
	return err
}
