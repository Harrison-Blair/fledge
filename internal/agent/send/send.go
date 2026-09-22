// Package send implements agent send: raw terminal input without a sender header.
package send

import (
	"context"
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

// Options selects a live agent and the input to deliver: Text as a paste,
// then Keys pressed in order.
type Options struct {
	Name, Pane, ID, Text string
	Keys                 []string
}

// Result reports the agent as observed before sending.
type Result struct {
	libagent.AgentRow
	Submitted bool `json:"submitted"`
}

// Run delivers text and keys verbatim in one pane.send_input call, in any agent
// state. Nothing is prefixed, so the recipient has no reply channel.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.send", Status: "success", Effects: []libagent.Effect{}}
	selected := identity.Target{Name: o.Name, Pane: o.Pane, ID: o.ID}
	err := selected.Validate()
	if err == nil && o.Text == "" && len(o.Keys) == 0 {
		err = libagent.Invalid("at least one of --text or --key is required")
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	a, _, _, err := selected.Get(ctx, c)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	result := Result{AgentRow: libagent.NewAgentRow(a.Pane)}
	out.Result = result
	params := map[string]any{"pane_id": a.PaneID}
	if o.Text != "" {
		params["text"] = o.Text
	}
	if len(o.Keys) > 0 {
		params["keys"] = o.Keys
	}
	var ack struct {
		Type string `json:"type"`
	}
	err = c.Call(ctx, "pane.send_input", params, &ack)
	if err == nil && ack.Type != "ok" {
		err = libagent.Protocol("incomplete pane.send_input acknowledgement")
	}
	if err != nil {
		out.Fail(err, "pane.send_input", true)
		return out
	}
	result.Submitted = true
	out.Result = result
	out.Effects = append(out.Effects, libagent.Effect{Action: "submitted", Kind: "input", ID: a.PaneID})
	return out
}

// Render writes a successful send outcome.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := fmt.Fprintf(w, "Sent input to %s (%s) in %s; it was %s before sending.\n", libagent.Display(r.Name), libagent.Display(r.Harness), libagent.Display(r.PaneID), libagent.Display(r.AgentStatus))
	return err
}
