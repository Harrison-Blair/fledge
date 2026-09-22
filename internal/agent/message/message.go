// Package message implements agent message: acknowledged prompt submission.
package message

import (
	"context"
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

type Options struct {
	Name, Pane, Body, File string
	BodySet, FileSet       bool
}
type Result struct {
	libagent.AgentRow
	Submitted bool `json:"submitted"`
}

func (o Options) read(in io.Reader) (string, string, error) {
	target, err := libagent.ResolveTarget(o.Name, o.Pane)
	if err != nil {
		return "", "", err
	}
	text, err := libagent.ReadText(in, libagent.TextInput{Body: o.Body, BodyFlag: "body", BodySet: o.BodySet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Required: true, Noun: "message"})
	if err != nil {
		return "", "", err
	}
	return target, text, nil
}

// Run submits a message without waiting for the agent to finish its turn.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.message", Status: "success", Effects: []libagent.Effect{}}
	target, text, err := o.read(in)
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	a, err := c.Get(ctx, target)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	out.Result = Result{AgentRow: libagent.NewAgentRow(a.Pane)}
	agent, err := c.Prompt(ctx, target, text)
	if err != nil {
		out.Fail(err, "agent.prompt", true)
		return out
	}
	out.Effects = append(out.Effects, libagent.Effect{Action: "submitted", Kind: "message", ID: agent.PaneID})
	out.Result = Result{AgentRow: libagent.NewAgentRow(agent.Pane), Submitted: true}
	return out
}

// Render writes a successful message outcome.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := fmt.Fprintf(w, "Message submitted to %s.\n", display(r.PaneID))
	return err
}
func display(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}
