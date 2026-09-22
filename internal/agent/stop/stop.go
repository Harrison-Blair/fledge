// Package stop implements agent stop: tearing down a live agent by closing its
// pane and ending its Fledge record.
package stop

import (
	"context"
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

type Options struct {
	Name, Pane, ID string
	Force          bool
}
type Result struct {
	libagent.AgentRow
	Stopped bool `json:"stopped"`
}

func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.stop", Status: "success", Effects: []libagent.Effect{}}
	selected := identity.Target{Name: o.Name, Pane: o.Pane, ID: o.ID}
	if err := selected.Validate(); err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	a, target, rec, err := selected.Get(ctx, c)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	out.Result = Result{AgentRow: libagent.NewAgentRow(a.Pane)}
	if status := a.AgentStatus; status != "idle" && status != "done" && !o.Force {
		out.Fail(libagent.Invalid("agent %s is %s; pass --force to stop it anyway", target, status), "guard", false)
		return out
	}
	var closed struct {
		Type string `json:"type"`
	}
	err = c.Call(ctx, "pane.close", map[string]any{"pane_id": a.PaneID}, &closed)
	if err == nil && closed.Type != "ok" {
		err = libagent.Protocol("incomplete pane.close result")
	}
	if err != nil {
		out.Fail(err, "pane.close", true)
		return out
	}
	out.Result = Result{AgentRow: libagent.NewAgentRow(a.Pane), Stopped: true}
	out.Effects = append(out.Effects, libagent.Effect{Action: "closed", Kind: "pane", ID: a.PaneID})
	ended, err := end(ctx, c, a, rec)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	if ended != "" {
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "agent_record", ID: ended})
	}
	return out
}

// end marks the stopped agent's live record ended and returns its id, or ""
// when the agent has no record. Like agent get, it treats an unavailable
// store as holding no record, so stop works outside a repository.
func end(ctx context.Context, c libagent.Client, a herdr.AgentDetails, rec *identity.Record) (string, error) {
	s, err := identity.Existing(ctx, c.Cwd)
	if err != nil || s == nil {
		return "", nil
	}
	if rec == nil {
		if rec, err = identity.Live(s, a.TerminalID); err != nil || rec == nil {
			return "", err
		}
	}
	return rec.ID, identity.End(s, rec.ID)
}

// Render writes a successful stop outcome.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := fmt.Fprintf(w, "Stopped %s (%s) in %s.\n", display(r.Name), display(r.Harness), display(r.PaneID))
	return err
}
func display(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}
