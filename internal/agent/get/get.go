// Package get implements agent get: inspecting one live agent without side effects.
package get

import (
	"context"
	"fmt"
	"io"
	"strconv"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// Options selects one live agent by name or hosting pane.
type Options struct{ Name, Pane string }

// Result exposes inspection details, preserving unavailable values as null.
type Result struct {
	libagent.AgentRow
	ForegroundCwd    *string          `json:"foreground_cwd"`
	InteractiveReady *bool            `json:"interactive_ready"`
	LaunchPending    *bool            `json:"launch_pending"`
	Focused          *bool            `json:"focused"`
	Title            *string          `json:"title"`
	AgentSession     *SessionIdentity `json:"agent_session"`
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
	target, err := libagent.ResolveTarget(o.Name, o.Pane)
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	a, err := c.Get(ctx, target)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	result := Result{AgentRow: libagent.NewAgentRow(a.Pane), ForegroundCwd: a.ForegroundCwd, InteractiveReady: a.InteractiveReady, LaunchPending: a.LaunchPending, Focused: a.Focused, Title: resolveTitle(a)}
	if session := a.AgentSession; session != nil {
		result.AgentSession = &SessionIdentity{Source: session.Source, Harness: session.Agent, Kind: session.Kind, Value: session.Value}
	}
	out.Result = result
	return out
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
		{"Name", display(r.Name)}, {"Harness", display(r.Harness)}, {"Status", display(r.AgentStatus)},
		{"Workspace ID", display(r.WorkspaceID)}, {"Tab ID", display(r.TabID)}, {"Pane ID", display(r.PaneID)},
		{"Working directory", display(r.Cwd)}, {"Foreground working directory", display(r.ForegroundCwd)},
		{"Interactive ready", displayBool(r.InteractiveReady)}, {"Launch pending", displayBool(r.LaunchPending)},
		{"Focused", displayBool(r.Focused)}, {"Title", display(r.Title)},
		{"Session source", display(session.Source)}, {"Session harness", display(session.Harness)},
		{"Session reference kind", display(session.Kind)}, {"Session reference value", display(session.Value)},
	} {
		if _, err := fmt.Fprintf(w, "%s: %s\n", f.label, f.value); err != nil {
			return err
		}
	}
	return nil
}
func displayBool(b *bool) string {
	if b == nil {
		return "-"
	}
	return strconv.FormatBool(*b)
}
func display(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}
