package agent

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

// PauseOptions selects a live agent and bounds interrupt delivery and settlement.
// Callers supply a positive Timeout; the CLI defaults to ten seconds.
type PauseOptions struct {
	Name, Pane string
	Timeout    time.Duration
	NoWait     bool
}

// Pause interrupts the foreground turn once using the harness's default binding.
// Settlement observes readiness, not task completion or a persistent pause.
func (s *Service) Pause(ctx context.Context, o PauseOptions) Outcome {
	out := Outcome{Operation: "agent.pause", Status: "success", Effects: []Effect{}}
	target, err := resolveTarget(o.Name, o.Pane)
	if err == nil && (strings.TrimSpace(target) == "" || o.Timeout <= 0) {
		err = invalid("target must be nonempty and --timeout must be positive")
	}
	if err != nil {
		out.fail(err, "validation", false)
		return out
	}
	deadline := s.now().Add(o.Timeout)
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	a, err := s.lookup(ctx, target, &out)
	if err != nil {
		return out
	}
	result := PauseResult{AgentRow: row(a.Pane)}
	out.Result = result
	if a.Agent == nil || !slices.Contains(kinds, *a.Agent) {
		out.fail(invalid("pause requires a known harness"), "guard", false)
		return out
	}
	if (a.LaunchPending != nil && *a.LaunchPending) || (a.AgentStatus != "working" && a.AgentStatus != "idle" && a.AgentStatus != "done") {
		out.fail(invalid("cannot pause an agent that is %s or has a pending launch", a.AgentStatus), "guard", false)
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
	if deadline.Sub(s.now()) <= 0 {
		out.fail(&herdr.Error{Code: "timeout", Message: "pause timeout expired before interrupt"}, "agent.send_keys", false)
		return out
	}
	var ack struct {
		Type string `json:"type"`
	}
	err = s.call(ctx, "agent.send_keys", map[string]any{"target": a.PaneID, "keys": keys}, &ack)
	if err == nil && ack.Type != "ok" {
		err = protocol("incomplete agent.send_keys acknowledgement")
	}
	if err != nil {
		out.fail(err, "agent.send_keys", true)
		return out
	}
	result.Submitted = true
	out.Result = result
	out.Effects = append(out.Effects, Effect{Action: "submitted", Kind: "interrupt", ID: a.PaneID})
	if o.NoWait {
		return out
	}
	remaining := deadline.Sub(s.now())
	if remaining <= 0 {
		out.fail(&herdr.Error{Code: "timeout", Message: "pause timeout expired before settlement"}, "agent.wait", false)
		return out
	}
	// Herdr's default settled set includes blocked so approval dialogs fail promptly.
	var settled herdr.AgentResult
	err = s.call(ctx, "agent.wait", map[string]any{"target": a.PaneID, "timeout_ms": remaining.Milliseconds()}, &settled)
	if err == nil && (settled.Type != "agent_info" || !validAgentInfo(settled.Agent) || settled.Agent.TerminalID != a.TerminalID || settled.Agent.PaneID != a.PaneID) {
		err = protocol("agent.wait did not return the resolved terminal")
	}
	if err == nil {
		result.AgentRow = row(settled.Agent.Pane)
		out.Result = result
		switch {
		case settled.Agent.AgentStatus == "blocked":
			err = &herdr.Error{Code: "agent_blocked", Message: "agent is blocked after interruption"}
		case settled.Agent.LaunchPending != nil && *settled.Agent.LaunchPending:
			err = protocol("agent launch is pending after interruption")
		case settled.Agent.AgentStatus != "idle" && settled.Agent.AgentStatus != "done":
			err = protocol("agent did not settle after interruption")
		}
	}
	if err != nil {
		out.fail(err, "agent.wait", false)
		return out
	}
	result.Settled = true
	out.Result = result
	return out
}
