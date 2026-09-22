// Package get implements agent get: inspecting one live agent without side effects.
package get

import (
	"context"
	"fmt"
	"io"
	"strconv"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

// Options selects one live agent by name, hosting pane, or Fledge record ID.
type Options struct{ Name, Pane, ID string }

// Result exposes inspection details, preserving unavailable values as null.
type Result struct {
	libagent.AgentRow
	ForegroundCwd    *string          `json:"foreground_cwd"`
	InteractiveReady *bool            `json:"interactive_ready"`
	LaunchPending    *bool            `json:"launch_pending"`
	Focused          *bool            `json:"focused"`
	Title            *string          `json:"title"`
	AgentSession     *SessionIdentity `json:"agent_session"`
	Record           *identity.Record `json:"record"`
}
type SessionIdentity struct {
	Source  *string `json:"source"`
	Harness *string `json:"harness"`
	Kind    *string `json:"kind"`
	Value   *string `json:"value"`
}

// resolveTitle prefers the agent-reported title, else the stripped terminal title, else the raw terminal title.
func resolveTitle(a herdr.AgentDetails) *string {
	for _, t := range []*string{a.Title, a.TerminalTitleStripped, a.TerminalTitle} {
		if t != nil && *t != "" {
			return t
		}
	}
	return nil
}

// Run inspects an agent without focusing its pane or marking output seen.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.get", Status: "success", Effects: []libagent.Effect{}}
	target := identity.Target{Name: o.Name, Pane: o.Pane, ID: o.ID}
	if err := target.Validate(); err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	a, _, rec, err := target.Get(ctx, c)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	result := Result{AgentRow: libagent.NewAgentRow(a.Pane), ForegroundCwd: a.ForegroundCwd, InteractiveReady: a.InteractiveReady, LaunchPending: a.LaunchPending, Focused: a.Focused, Title: resolveTitle(a)}
	if session := a.AgentSession; session != nil {
		result.AgentSession = &SessionIdentity{Source: session.Source, Harness: session.Agent, Kind: session.Kind, Value: session.Value}
	}
	result.Record = rec
	if rec == nil {
		result.Record = liveRecord(ctx, c.Cwd, a.TerminalID)
	}
	out.Result = result
	return out
}

// liveRecord finds the agent's record for display; an unavailable store only
// means no record is shown.
func liveRecord(ctx context.Context, cwd, terminal string) *identity.Record {
	s, err := identity.Existing(ctx, cwd)
	if err != nil || s == nil {
		return nil
	}
	rec, _ := identity.Live(s, terminal)
	return rec
}

// Render writes a successful get outcome as labeled lines.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	session := r.AgentSession
	if session == nil {
		session = &SessionIdentity{}
	}
	for _, f := range []struct{ label, value string }{
		{"Name", libagent.Display(r.Name)}, {"Harness", libagent.Display(r.Harness)}, {"Status", libagent.Display(r.AgentStatus)},
		{"Workspace ID", libagent.Display(r.WorkspaceID)}, {"Tab ID", libagent.Display(r.TabID)}, {"Pane ID", libagent.Display(r.PaneID)},
		{"Working directory", libagent.Display(r.Cwd)}, {"Foreground working directory", libagent.Display(r.ForegroundCwd)},
		{"Interactive ready", displayBool(r.InteractiveReady)}, {"Launch pending", displayBool(r.LaunchPending)},
		{"Focused", displayBool(r.Focused)}, {"Title", libagent.Display(r.Title)},
		{"Session source", libagent.Display(session.Source)}, {"Session harness", libagent.Display(session.Harness)},
		{"Session reference kind", libagent.Display(session.Kind)}, {"Session reference value", libagent.Display(session.Value)},
	} {
		if _, err := fmt.Fprintf(w, "%s: %s\n", f.label, f.value); err != nil {
			return err
		}
	}
	if rec := r.Record; rec != nil {
		_, err := fmt.Fprintf(w, "Fledge ID: %s\nParent: %s\nRegistered at: %s\nRegistered by: %s\n", rec.ID, libagent.Display(rec.Parent), rec.RegisteredAt, rec.RegisteredBy)
		return err
	}
	return nil
}
func displayBool(b *bool) string {
	if b == nil {
		return "-"
	}
	return strconv.FormatBool(*b)
}
