// Package adopt implements agent adopt: registering an already-running agent
// with a durable Fledge identity.
package adopt

import (
	"context"
	"fmt"
	"io"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Options selects the agent to adopt: Pane, or the caller's own pane when
// empty. Name names an unnamed agent, or must match an existing name.
type Options struct{ Name, Pane string }

// Result is the agent's record, new or named, and whether adopt named the
// agent.
type Result struct {
	identity.Record
	Renamed bool `json:"renamed"`
}

// Run registers the live agent in the selected pane. An unnamed agent whose
// terminal already has a live record is named and keeps that record, storing
// the new name. Naming an agent also labels its pane, and its tab when the
// pane is alone there. Run never renames a named agent, and refuses one whose
// terminal already has a live record.
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
	case o.Name != "":
		err = libagent.ValidateName(o.Name)
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	a, err := c.Get(ctx, target)
	if err != nil {
		out.Fail(err, "agent.get", false)
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
	// The store is created only once Herdr and the options allow adoption.
	store, err := identity.OpenStore(ctx, c.Cwd, &out)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	renamed := current == ""
	var existing *identity.Record
	if renamed {
		// Herdr calls never run under the store lock, so look for the
		// terminal's record before renaming; Register repeats its check under
		// the lock.
		if existing, err = registered(store, a); err != nil {
			out.Fail(err, "state", false)
			return out
		}
		if a, err = c.Rename(ctx, a, o.Name); err != nil {
			out.Fail(err, "agent.rename", true)
			return out
		}
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "agent_name", ID: a.PaneID})
	}
	if existing != nil {
		rec, err := identity.Rename(store, *existing, a)
		if err != nil {
			out.Fail(err, "state", false)
			return out
		}
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "agent_record", ID: rec.ID})
		out.Result = Result{Record: task.Observe(store, observeSession, rec, &a, time.Now(), &out), Renamed: true}
	} else {
		rec, err := identity.Register(ctx, store, c, a, "adopt", nil, nil)
		if err != nil {
			out.Fail(err, "state", false)
			return out
		}
		out.Effects = append(out.Effects, libagent.Effect{Action: "created", Kind: "agent_record", ID: rec.ID})
		out.Result = Result{Record: task.Observe(store, observeSession, rec, &a, time.Now(), &out), Renamed: renamed}
	}
	if renamed {
		// Label records its own failure on out.
		_ = c.Label(ctx, a.Pane, o.Name, &out)
	}
	return out
}

// registered is replaceable so tests can count adopt's scans of the agent
// records beyond Register's one.
var registered = identity.Registered

// observeSession is replaceable so tests can fail its write. The agent is
// already adopted, so a failed write is only a warning effect.
var observeSession task.Observer = identity.ObserveSession

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
