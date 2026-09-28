// Package list implements agent list: enumerating every live Herdr agent,
// optionally only those a selector filter picks.
package list

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
)

type Result struct {
	Agents   []Row `json:"agents"`
	filtered bool
	ids      bool
}

// Row is a live agent with its Fledge record ID and that record's parent and
// profile, each null when unregistered.
type Row struct {
	ID      *string `json:"id"`
	Parent  *string `json:"parent"`
	Profile *string `json:"profile"`
	libagent.AgentRow
}

// Options keeps only the agents Filter selects. With IDs the human output is
// one record id per registered match; JSON reports that structured output was
// requested, which IDs excludes.
type Options struct {
	selector.Filter
	IDs, JSON bool
}

func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.list", Status: "success", Effects: []libagent.Effect{}}
	err := o.Validate()
	if err == nil && o.IDs && o.JSON {
		err = libagent.Invalid("--ids and --json are mutually exclusive")
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	// --mine resolves the caller through Herdr's agent.get, as it always has,
	// then filters like --parent.
	if o.Mine {
		s, err := identity.Existing(ctx, c.Cwd)
		if err != nil {
			out.Fail(err, "state", false)
			return out
		}
		caller, err := identity.RequireCaller(ctx, s, c)
		if err != nil {
			out.Fail(err, "identity", false)
			return out
		}
		o.Mine, o.Parent = false, caller.ID
	}
	matches, err := selector.Resolve(ctx, c, o.Filter)
	if err != nil {
		out.Fail(err, "agent.list", false)
		return out
	}
	rows := make([]Row, 0, len(matches))
	for _, m := range matches {
		row := Row{AgentRow: libagent.NewAgentRow(m.Agent.Pane)}
		if rec := m.Record; rec != nil {
			row.ID, row.Parent, row.Profile = &rec.ID, rec.Parent, rec.Profile
		}
		rows = append(rows, row)
	}
	out.Result = Result{Agents: rows, filtered: !o.Filter.Empty(), ids: o.IDs}
	return out
}

// Render writes a successful list outcome as a table.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	if r.ids {
		for _, a := range r.Agents {
			if a.ID == nil {
				continue
			}
			if _, err := fmt.Fprintln(w, *a.ID); err != nil {
				return err
			}
		}
		return nil
	}
	if len(r.Agents) == 0 {
		empty := "No live agents."
		if r.filtered {
			empty = "No agents match."
		}
		_, err := fmt.Fprintln(w, empty)
		return err
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tPARENT\tNAME\tHARNESS\tPROFILE\tSTATUS\tWORKSPACE\tTAB\tPANE\tCWD")
	for _, a := range r.Agents {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", libagent.Display(a.ID), libagent.Display(a.Parent), libagent.Display(a.Name), libagent.Display(a.Harness), libagent.Display(a.Profile), libagent.Display(a.AgentStatus), libagent.Display(a.WorkspaceID), libagent.Display(a.TabID), libagent.Display(a.PaneID), libagent.Display(a.Cwd))
	}
	return table.Flush()
}
