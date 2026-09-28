// Package rename implements agent rename: giving a live agent a new name and
// labeling its pane, and its tab when the pane is alone there, to match.
package rename

import (
	"context"
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

// Options selects the agent, or the caller's own agent when no selector is
// set, and names it To.
type Options struct {
	identity.Target
	To string
}

// Result is the renamed agent, its record id when registered, and the name
// it had before.
type Result struct {
	libagent.AgentRow
	ID       *string `json:"id"`
	Previous *string `json:"previous_name"`
}

// Run names the selected agent To through Herdr, stores the name on its
// record under the same id, then labels its pane and, when the pane is alone
// in its tab, the tab. An agent already named To is only labeled. An
// unavailable store means no record to update, as for agent get.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.rename", Status: "success", Effects: []libagent.Effect{}}
	if o.Name == "" && o.Pane == "" && o.ID == "" {
		o.Pane = c.CallerPane
	}
	var err error
	switch {
	case o.To == "":
		err = libagent.Invalid("--to is required")
	case o.Name == "" && o.Pane == "" && o.ID == "":
		err = libagent.Invalid("--name, --pane, or --id is required outside a Herdr pane")
	default:
		if err = libagent.ValidateName(o.To); err == nil {
			err = o.Target.Validate()
		}
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	a, _, rec, err := o.Target.Get(ctx, c)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	previous := a.Name
	if previous == nil || *previous != o.To {
		if a, err = c.Rename(ctx, a, o.To); err != nil {
			out.Fail(err, "agent.rename", true)
			return out
		}
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "agent_name", ID: a.PaneID})
	}
	result := Result{AgentRow: libagent.NewAgentRow(a.Pane), Previous: previous}
	out.Result = result
	s, err := identity.Existing(ctx, c.Cwd)
	if err == nil && s != nil && rec == nil {
		rec, err = identity.Match(s, a)
		if err != nil {
			out.Fail(err, "state", false)
			return out
		}
	}
	if rec != nil {
		if rec.Name == nil || *rec.Name != o.To {
			renamed, err := identity.Rename(s, *rec, a)
			if err != nil {
				out.Fail(err, "state", false)
				return out
			}
			out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "agent_record", ID: renamed.ID})
		}
		result.ID = &rec.ID
		out.Result = result
	}
	// Label records its own failure on out.
	_ = c.Label(ctx, a.Pane, o.To, &out)
	return out
}

// Render writes a successful rename.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := fmt.Fprintf(w, "Renamed %s to %s (%s).\n", libagent.Display(r.Previous), libagent.Display(r.Name), libagent.Display(r.PaneID))
	return err
}
