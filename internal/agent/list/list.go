// Package list implements agent list: enumerating every live Herdr agent.
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
)

type Result struct {
	Agents []Row `json:"agents"`
}

// Row is a live agent with its Fledge record ID, null when unregistered.
type Row struct {
	ID *string `json:"id"`
	libagent.AgentRow
}

func Run(ctx context.Context, c libagent.Client) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.list", Status: "success", Effects: []libagent.Effect{}}
	var r struct {
		Type   string               `json:"type"`
		Agents []herdr.AgentDetails `json:"agents"`
	}
	err := c.Call(ctx, "agent.list", nil, &r)
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
	records := liveRecords(ctx, c.Cwd)
	rows := make([]Row, 0, len(r.Agents))
	for _, a := range r.Agents {
		row := Row{AgentRow: libagent.NewAgentRow(a.Pane)}
		if rec, ok := records[a.TerminalID]; ok && a.TerminalID != "" {
			row.ID = &rec.ID
		}
		rows = append(rows, row)
	}
	out.Result = Result{Agents: rows}
	return out
}

// liveRecords loads records for the ID column; an unavailable store only
// leaves the column empty.
func liveRecords(ctx context.Context, cwd string) map[string]identity.Record {
	s, err := identity.Existing(ctx, cwd)
	if err != nil || s == nil {
		return nil
	}
	records, _ := identity.LiveByTerminal(s)
	return records
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
	fmt.Fprintln(table, "ID\tNAME\tHARNESS\tSTATUS\tWORKSPACE\tTAB\tPANE\tCWD")
	for _, a := range r.Agents {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", display(a.ID), display(a.Name), display(a.Harness), display(a.AgentStatus), display(a.WorkspaceID), display(a.TabID), display(a.PaneID), display(a.Cwd))
	}
	return table.Flush()
}
func display(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}
