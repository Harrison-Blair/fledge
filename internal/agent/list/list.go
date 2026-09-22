// Package list implements agent list: enumerating every live Herdr agent,
// optionally only the direct children of one registered agent.
package list

import (
	"context"
	"fmt"
	"io"
	"slices"
	"text/tabwriter"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

type Result struct {
	Agents []Row `json:"agents"`
}

// Row is a live agent with its Fledge record ID and that record's parent,
// each null when unregistered.
type Row struct {
	ID     *string `json:"id"`
	Parent *string `json:"parent"`
	libagent.AgentRow
}

// Options keeps only agents whose record's parent is Parent, or with Mine the
// caller's own live record. At most one may be set.
type Options struct {
	Mine   bool
	Parent string
}

func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.list", Status: "success", Effects: []libagent.Effect{}}
	var err error
	switch {
	case o.Mine && o.Parent != "":
		err = libagent.Invalid("--mine and --parent are mutually exclusive")
	case o.Parent != "" && !state.ValidID(o.Parent):
		err = libagent.Invalid("--parent must be an 8 lowercase hexadecimal agent id")
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
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
		o.Parent = caller.ID
	}
	var r struct {
		Type   string               `json:"type"`
		Agents []herdr.AgentDetails `json:"agents"`
	}
	err = c.Call(ctx, "agent.list", nil, &r)
	if err == nil && (r.Type != "agent_list" || r.Agents == nil) {
		err = libagent.Protocol("incomplete agent.list result")
	}
	if err == nil && slices.ContainsFunc(r.Agents, func(a herdr.AgentDetails) bool { return !libagent.ValidAgent(a.Pane) }) {
		err = libagent.Protocol("incomplete agent info")
	}
	if err != nil {
		out.Fail(err, "agent.list", false)
		return out
	}
	records, err := liveRecords(ctx, c.Cwd)
	if err != nil && o.Parent != "" {
		out.Fail(err, "state", false)
		return out
	}
	rows := make([]Row, 0, len(r.Agents))
	for _, a := range r.Agents {
		row := Row{AgentRow: libagent.NewAgentRow(a.Pane)}
		if rec, ok := records[a.TerminalID]; ok && a.TerminalID != "" {
			row.ID, row.Parent = &rec.ID, rec.Parent
		}
		if o.Parent != "" && (row.Parent == nil || *row.Parent != o.Parent) {
			continue
		}
		rows = append(rows, row)
	}
	out.Result = Result{Agents: rows}
	return out
}

// liveRecords loads records for the ID and PARENT columns. Unfiltered
// listings ignore its error, so an unavailable store only leaves them empty;
// a missing store is no records.
func liveRecords(ctx context.Context, cwd string) (map[string]identity.Record, error) {
	s, err := identity.Existing(ctx, cwd)
	if err != nil || s == nil {
		return nil, err
	}
	return identity.LiveByTerminal(s)
}

// Render writes a successful list outcome as a table.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	if len(r.Agents) == 0 {
		_, err := fmt.Fprintln(w, "No live agents.")
		return err
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tPARENT\tNAME\tHARNESS\tSTATUS\tWORKSPACE\tTAB\tPANE\tCWD")
	for _, a := range r.Agents {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", libagent.Display(a.ID), libagent.Display(a.Parent), libagent.Display(a.Name), libagent.Display(a.Harness), libagent.Display(a.AgentStatus), libagent.Display(a.WorkspaceID), libagent.Display(a.TabID), libagent.Display(a.PaneID), libagent.Display(a.Cwd))
	}
	return table.Flush()
}
