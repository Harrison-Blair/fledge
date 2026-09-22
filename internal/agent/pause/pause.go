// Package pause implements agent pause: interrupting a live agent's foreground turn.
package pause

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

// Options selects a live agent and bounds interrupt delivery and settlement.
// Callers supply a positive Timeout; the CLI defaults to ten seconds.
type Options struct {
	Name, Pane, ID string
	Timeout        time.Duration
	NoWait         bool
}

// Result distinguishes acknowledged interruption from observed settlement.
type Result struct {
	libagent.AgentRow
	Submitted bool `json:"submitted"`
	Settled   bool `json:"settled"`
}
type pauser struct {
	libagent.Client
	Now func() time.Time
}

// Run interrupts the foreground turn once using the harness's default binding.
// Settlement observes readiness, not task completion or a persistent pause.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	return pauser{Client: c, Now: time.Now}.run(ctx, o)
}
func (s pauser) run(ctx context.Context, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.pause", Status: "success", Effects: []libagent.Effect{}}
	selected := identity.Target{Name: o.Name, Pane: o.Pane, ID: o.ID}
	err := selected.Validate()
	if err == nil && (strings.TrimSpace(o.Name+o.Pane+o.ID) == "" || o.Timeout <= 0) {
		err = libagent.Invalid("target must be nonempty and --timeout must be positive")
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	deadline := s.Now().Add(o.Timeout)
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	a, _, _, err := selected.Get(ctx, s.Client)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	result := Result{AgentRow: libagent.NewAgentRow(a.Pane)}
	out.Result = result
	if a.Agent == nil || !libagent.IsHarness(*a.Agent) {
		out.Fail(libagent.Invalid("pause requires a known harness"), "guard", false)
		return out
	}
	if (a.LaunchPending != nil && *a.LaunchPending) || (a.AgentStatus != "working" && a.AgentStatus != "idle" && a.AgentStatus != "done") {
		out.Fail(libagent.Invalid("cannot pause an agent that is %s or has a pending launch", a.AgentStatus), "guard", false)
		return out
	}
	if a.AgentStatus == "idle" || a.AgentStatus == "done" {
		result.Settled = true
		out.Result = result
		return out
	}
	keys := []string{"esc"}
	switch *a.Agent {
	case "amp", "copilot", "opencode", "kilo":
		keys = []string{"esc", "esc"}
	case "droid", "grok", "hermes", "mastracode", "qodercli":
		keys = []string{"ctrl+c"}
	}
	if deadline.Sub(s.Now()) <= 0 {
		out.Fail(&herdr.Error{Code: "timeout", Message: "pause timeout expired before interrupt"}, "agent.send_keys", false)
		return out
	}
	var ack struct {
		Type string `json:"type"`
	}
	err = s.Call(ctx, "agent.send_keys", map[string]any{"target": a.PaneID, "keys": keys}, &ack)
	if err == nil && ack.Type != "ok" {
		err = libagent.Protocol("incomplete agent.send_keys acknowledgement")
	}
	if err != nil {
		out.Fail(err, "agent.send_keys", true)
		return out
	}
	result.Submitted = true
	out.Result = result
	out.Effects = append(out.Effects, libagent.Effect{Action: "submitted", Kind: "interrupt", ID: a.PaneID})
	if o.NoWait {
		return out
	}
	remaining := deadline.Sub(s.Now())
	if remaining <= 0 {
		out.Fail(&herdr.Error{Code: "timeout", Message: "pause timeout expired before settlement"}, "agent.wait", false)
		return out
	}
	// Herdr's default settled set includes blocked so approval dialogs fail promptly.
	// The margin lets Herdr's own agent.wait timeout reach the caller.
	waitCtx, cancelWait := context.WithTimeout(parent, remaining+libagent.TransportMargin)
	defer cancelWait()
	var settled herdr.AgentResult
	err = s.Call(waitCtx, "agent.wait", map[string]any{"target": a.PaneID, "timeout_ms": remaining.Milliseconds()}, &settled)
	if err == nil && (settled.Type != "agent_info" || !libagent.ValidAgentInfo(settled.Agent) || settled.Agent.TerminalID != a.TerminalID || settled.Agent.PaneID != a.PaneID) {
		err = libagent.Protocol("agent.wait did not return the resolved terminal")
	}
	if err == nil {
		result.AgentRow = libagent.NewAgentRow(settled.Agent.Pane)
		out.Result = result
		switch {
		case settled.Agent.AgentStatus == "blocked":
			err = &herdr.Error{Code: "agent_blocked", Message: "agent is blocked after interruption"}
		case settled.Agent.LaunchPending != nil && *settled.Agent.LaunchPending:
			err = libagent.Protocol("agent launch is pending after interruption")
		case settled.Agent.AgentStatus != "idle" && settled.Agent.AgentStatus != "done":
			err = libagent.Protocol("agent did not settle after interruption")
		}
	}
	if err != nil {
		out.Fail(err, "agent.wait", false)
		return out
	}
	result.Settled = true
	out.Result = result
	return out
}

// Render writes a successful pause outcome.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	text := "Pause requested"
	if r.Settled {
		text = "Paused"
		if !r.Submitted {
			text = "Already idle or done"
		}
	}
	_, err := fmt.Fprintf(w, "%s: %s (%s) in %s.\n", text, display(r.Name), display(r.Harness), display(r.PaneID))
	return err
}
func display(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}
