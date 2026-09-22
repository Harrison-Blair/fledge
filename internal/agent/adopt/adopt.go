// Package adopt implements agent adopt: registering an already-running agent
// with a durable Fledge identity.
package adopt

import (
	"context"
	"fmt"
	"io"
	"regexp"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

// Options selects the agent to adopt: Pane, or the caller's own pane when
// empty. Name names an unnamed agent, or must match an existing name.
type Options struct{ Name, Pane string }

// Result is the new record and whether adopt named the agent.
type Result struct {
	identity.Record
	Renamed bool `json:"renamed"`
}

// Run registers the live agent in the selected pane. It refuses a terminal
// that already has a live record and never renames a named agent.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.adopt", Status: "success", Effects: []libagent.Effect{}}
	target := o.Pane
	if target == "" {
		target = c.CallerPane
	}
	var err error
	switch {
	case target == "":
		err = libagent.Invalid("--pane is required outside a Herdr pane")
	case o.Name != "" && !regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`).MatchString(o.Name):
		err = libagent.Invalid("--name must match [a-z][a-z0-9_-]{0,31}")
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	store, err := identity.OpenStore(ctx, c.Cwd, &out)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	a, err := c.Get(ctx, target)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	// Refuse before renaming; Register repeats this check under the store lock.
	if err := identity.Unregistered(store, a); err != nil {
		out.Fail(err, "state", false)
		return out
	}
	current := ""
	if a.Name != nil {
		current = *a.Name
	}
	switch {
	case current == "" && o.Name == "":
		err = libagent.Invalid("the agent in %s is unnamed; pass --name", a.PaneID)
	case current != "" && o.Name != "" && o.Name != current:
		err = libagent.Invalid("the agent in %s is already named %s", a.PaneID, current)
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	renamed := current == ""
	if renamed {
		if a, err = rename(ctx, c, a, o.Name); err != nil {
			out.Fail(err, "agent.rename", true)
			return out
		}
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "agent_name", ID: a.PaneID})
	}
	rec, err := identity.Register(ctx, store, c, a, "adopt", nil)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	out.Effects = append(out.Effects, libagent.Effect{Action: "created", Kind: "agent_record", ID: rec.ID})
	out.Result = Result{Record: rec, Renamed: renamed}
	return out
}

// rename names the agent a and confirms the same terminal now carries name.
func rename(ctx context.Context, c libagent.Client, a herdr.AgentDetails, name string) (herdr.AgentDetails, error) {
	var r herdr.AgentResult
	err := c.Call(ctx, "agent.rename", map[string]any{"target": a.PaneID, "name": name}, &r)
	if err == nil && (r.Type != "agent_info" || !libagent.ValidAgentInfo(r.Agent) || r.Agent.TerminalID != a.TerminalID || r.Agent.Name == nil || *r.Agent.Name != name) {
		err = libagent.Protocol("incomplete or mismatched agent.rename result")
	}
	return r.Agent, err
}

// Render writes a successful adoption.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	name := "-"
	if r.Name != nil {
		name = *r.Name
	}
	_, err := fmt.Fprintf(w, "Adopted %s (%s) as %s.\n", name, r.Pane, r.ID)
	return err
}
