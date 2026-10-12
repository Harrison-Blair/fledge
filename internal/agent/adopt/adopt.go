package adopt

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

// Options names an unnamed agent, or must match an existing name.
type Options struct{ Name string }

// Result is the agent's record, new or named, and whether adopt named the
// agent.
type Result struct {
	identity.Record
	Renamed bool `json:"renamed"`
}

// Run registers the caller's own live agent, which runs in the caller's pane.
// It first verifies that this command runs inside that agent (see owns) and
// verifies it again just before the first change, so a wrong or stale caller
// pane registers and names nothing. An unnamed agent whose terminal already
// has a live record is named and keeps that record, storing the new name.
// Naming an agent also labels its pane, and its tab when the pane is alone
// there. Run never renames a named agent, and refuses one whose terminal
// already has a live record. A new record has no parent. Herdr and the
// operating system offer no transaction: a change between the last check and
// a Herdr request is not detected.
func Run(ctx context.Context, c libagent.Client, o Options) cli.Outcome {
	out := cli.NewOutcome("agent.adopt")
	var err error
	switch {
	case strings.TrimSpace(c.CallerPane) == "":
		err = cli.Invalid("adopt registers the caller's own agent; run it inside that agent's Herdr pane")
	case o.Name != "":
		err = libagent.ValidateName(o.Name)
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	a, err := c.Get(ctx, c.CallerPane)
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
		err = cli.Invalid("the agent in %s is unnamed; pass --name", a.PaneID)
	case current != "" && o.Name != "" && o.Name != current:
		err = cli.Invalid("the agent in %s is already named %s", a.PaneID, current)
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	if err := owns(ctx, c, a); err != nil {
		out.Fail(err, "identity", false)
		return out
	}
	// The store is created only once Herdr, the options, and the caller's
	// processes allow adoption.
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
	}
	if a, err = recheck(ctx, c, a); err != nil {
		out.Fail(err, "identity", false)
		return out
	}
	if renamed {
		if a, err = c.Rename(ctx, a, o.Name); err != nil {
			out.Fail(err, "agent.rename", true)
			return out
		}
		out.Effects = append(out.Effects, cli.Effect{Action: "updated", Kind: "agent_name", ID: a.PaneID})
	}
	if existing != nil {
		rec, err := identity.Rename(store, *existing, a)
		if err != nil {
			out.Fail(err, "state", false)
			return out
		}
		out.Effects = append(out.Effects, cli.Effect{Action: "updated", Kind: "agent_record", ID: rec.ID})
		out.Result = Result{Record: identity.Observe(store, observeSession, rec, &a, time.Now(), &out), Renamed: true}
	} else {
		// The agent is the caller itself, so it is no parent of its record.
		self := c
		self.CallerPane = ""
		rec, err := identity.Register(ctx, store, self, a, "adopt", nil, nil)
		if err != nil {
			out.Fail(err, "state", false)
			return out
		}
		out.Effects = append(out.Effects, cli.Effect{Action: "created", Kind: "agent_record", ID: rec.ID})
		out.Result = Result{Record: identity.Observe(store, observeSession, rec, &a, time.Now(), &out), Renamed: renamed}
	}
	if renamed {
		// Label records its own failure on out.
		_ = c.Label(ctx, a.Pane, o.Name, &out)
	}
	return out
}

// recheck looks up the caller's agent again and verifies that it is the same
// terminal running the same harness, still owned by this command, and returns
// it as it is now, in the pane where it now runs.
func recheck(ctx context.Context, c libagent.Client, before herdr.AgentDetails) (herdr.AgentDetails, error) {
	a, err := c.Get(ctx, c.CallerPane)
	if err != nil {
		return a, err
	}
	if a.TerminalID != before.TerminalID || (a.Agent == nil) != (before.Agent == nil) || a.Agent != nil && *a.Agent != *before.Agent {
		return a, unverified("the agent in %s changed during adoption", c.CallerPane)
	}
	return a, owns(ctx, c, a)
}

// registered is replaceable so tests can count adopt's scans of the agent
// records beyond Register's one.
var registered = identity.Registered

// observeSession is replaceable so tests can fail its write. The agent is
// already adopted, so a failed write is only a warning effect.
var observeSession identity.Observer = identity.ObserveSession

// Render writes a successful adoption.
func Render(w io.Writer, o cli.Outcome) error {
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
