// Package list implements agent list: enumerating every live Herdr agent.
package list

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

type Result struct {
	Agents []libagent.AgentRow `json:"agents"`
}

func Run(ctx context.Context, c libagent.Client) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.list", Status: "success", Effects: []libagent.Effect{}}
	var r herdr.AgentListResult
	err := c.Call(ctx, "agent.list", nil, &r)
	if err == nil && (r.Type != "agent_list" || r.Agents == nil) {
		err = libagent.Protocol("incomplete agent.list result")
	}
	rows := make([]libagent.AgentRow, 0, len(r.Agents))
	for _, p := range r.Agents {
		if !libagent.ValidAgent(p) {
			err = libagent.Protocol("incomplete agent info")
			break
		}
		rows = append(rows, libagent.NewAgentRow(p))
	}
	if err != nil {
		out.Fail(err, "agent.list", false)
		return out
	}
	out.Result = Result{Agents: rows}
	return out
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
	fmt.Fprintln(table, "NAME\tHARNESS\tSTATUS\tWORKSPACE\tTAB\tPANE\tCWD")
	for _, a := range r.Agents {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", display(a.Name), display(a.Harness), display(a.AgentStatus), display(a.WorkspaceID), display(a.TabID), display(a.PaneID), display(a.Cwd))
	}
	return table.Flush()
}
func display(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}
