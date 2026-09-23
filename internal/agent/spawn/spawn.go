package spawn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

type spawner struct {
	libagent.Client
	// Wait pauses between agent.start retries; nil uses a real timer.
	Wait func(context.Context, time.Duration) error
	// Now reports the current time for the spawn timeout budget; nil uses time.Now.
	Now func() time.Time
	// NewID generates the first prompt's message ID; nil uses libagent.NewMessageID.
	NewID func() string
	// checkout is the placement's checkout, recorded on the agent's record.
	checkout *identity.Checkout
}

func (s *spawner) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func (s *spawner) newID() string {
	if s.NewID != nil {
		return s.NewID()
	}
	return libagent.NewMessageID()
}
func (s *spawner) wait(ctx context.Context, d time.Duration) error {
	if s.Wait != nil {
		return s.Wait(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *spawner) snapshot(ctx context.Context) (*herdr.Snapshot, error) {
	var r herdr.SnapshotResult
	if err := s.Call(ctx, "session.snapshot", nil, &r); err != nil {
		return nil, err
	}
	if r.Type != "session_snapshot" || r.Snapshot == nil || r.Snapshot.Workspaces == nil || r.Snapshot.Tabs == nil || r.Snapshot.Panes == nil || r.Snapshot.Layouts == nil || r.Snapshot.Agents == nil {
		return nil, libagent.Protocol("incomplete session.snapshot result")
	}
	for _, w := range r.Snapshot.Workspaces {
		if w.ID == "" {
			return nil, libagent.Protocol("workspace missing ID")
		}
	}
	for _, t := range r.Snapshot.Tabs {
		if t.ID == "" || t.WorkspaceID == "" {
			return nil, libagent.Protocol("tab missing ownership")
		}
	}
	for _, p := range r.Snapshot.Panes {
		if !libagent.ValidPane(p) {
			return nil, libagent.Protocol("pane missing ownership")
		}
	}
	return r.Snapshot, nil
}

// prompt submits the first prompt to target, recording the submission effect or failure.
func (s *spawner) prompt(ctx context.Context, target, text string, out *libagent.Outcome) (herdr.AgentDetails, error) {
	agent, err := s.Prompt(ctx, target, text)
	if err != nil {
		out.Fail(err, "agent.prompt", true)
		return herdr.AgentDetails{}, err
	}
	out.Effects = append(out.Effects, libagent.Effect{Action: "submitted", Kind: "message", ID: agent.PaneID})
	return agent, nil
}

// Run launches an agent, optionally waits for readiness, and submits a first prompt.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) libagent.Outcome {
	return (&spawner{Client: c}).run(ctx, o, in)
}
func (s *spawner) run(ctx context.Context, o Options, in io.Reader) libagent.Outcome {
	result := &Result{Name: o.Name, Harness: o.Harness}
	out := libagent.Outcome{Operation: "agent.spawn", Status: "success", Result: result, Effects: []libagent.Effect{}}
	args, err := o.Validate()
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	prompt, err := libagent.ReadText(in, libagent.TextInput{Body: o.Prompt, BodyFlag: "prompt", BodySet: o.PromptSet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Required: false, Noun: "prompt"})
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	// The effective first prompt is the only input to PromptRequested.
	result.PromptRequested = prompt != ""
	if o.Cwd != "" && !filepath.IsAbs(o.Cwd) {
		o.Cwd = filepath.Join(s.Cwd, o.Cwd)
	}
	snapshot, err := s.snapshot(ctx)
	if err != nil {
		out.Fail(err, "session.snapshot", false)
		return out
	}
	for _, a := range snapshot.Agents {
		if a.Name != nil && *a.Name == o.Name {
			out.Fail(libagent.Invalid("agent name %q is already in use", o.Name), "preflight", false)
			return out
		}
	}
	var p herdr.Pane
	if o.Worktree != "" {
		p, err = s.worktreePlacement(ctx, o, snapshot, &out)
	} else {
		p, err = s.ordinaryPlacement(ctx, o, snapshot, &out)
	}
	if err != nil {
		if out.Error == nil {
			out.Fail(err, "placement", false)
		}
		return out
	}
	setPlacement(result, p)
	if err = s.customizePane(ctx, o, p, &out); err != nil {
		return out
	}

	started := s.now()
	var r herdr.AgentResult
	err = s.start(ctx, map[string]any{"name": o.Name, "kind": o.Harness, "pane_id": p.PaneID, "args": args, "timeout_ms": o.Timeout.Milliseconds()}, &r)
	if err == nil && (r.Type != "agent_started" || !libagent.ValidAgent(r.Agent.Pane) || !samePane(r.Agent.Pane, p) || r.Argv == nil) {
		err = libagent.Protocol("incomplete agent.start result")
	}
	if err != nil {
		out.Fail(err, "agent.start", true)
		return out
	}
	setPlacement(result, r.Agent.Pane)
	result.DetectedHarness = r.Agent.Agent
	result.AgentStatus = libagent.Pointer(r.Agent.AgentStatus)
	result.Argv = r.Argv
	out.Effects = append(out.Effects, libagent.Effect{Action: "started", Kind: "agent", ID: r.Agent.PaneID})
	if o.NoWait {
		s.register(ctx, withHarness(r.Agent, o.Harness), &out)
		return out
	}

	remaining := max(o.Timeout-s.now().Sub(started), 0)
	var w herdr.AgentResult
	err = s.Call(ctx, "agent.wait", map[string]any{"target": o.Name, "timeout_ms": remaining.Milliseconds()}, &w)
	if err == nil && (w.Type != "agent_info" || !libagent.ValidAgentInfo(w.Agent) || !samePane(w.Agent.Pane, r.Agent.Pane)) {
		err = libagent.Protocol("incomplete agent.wait result")
	}
	if err != nil {
		out.Fail(err, "agent.wait", true)
		return out
	}
	setPlacement(result, w.Agent.Pane)
	result.DetectedHarness = w.Agent.Agent
	result.AgentStatus = libagent.Pointer(w.Agent.AgentStatus)
	s.register(ctx, withHarness(w.Agent, o.Harness), &out)
	if w.Agent.AgentStatus == "blocked" {
		out.Fail(&herdr.Error{Code: "agent_blocked", Message: fmt.Sprintf("agent %s is waiting on a startup prompt", o.Name)}, "agent.wait", true)
		return out
	}
	if !result.PromptRequested {
		return out
	}
	id, sender := s.newID(), libagent.ResolveSender(ctx, s.Client)
	result.MessageID, result.Sender = &id, &sender
	if _, err := s.prompt(ctx, o.Name, libagent.WithHeader(id, sender, prompt), &out); err != nil {
		return out
	}
	result.Prompted = true
	return out
}

// busyBackoff paces agent.start retries while a fresh shell reaches its prompt.
var busyBackoff = []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, 800 * time.Millisecond}

// start retries only agent_pane_busy, keeping the last busy error once attempts or the context run out.
func (s *spawner) start(ctx context.Context, params map[string]any, r *herdr.AgentResult) error {
	for attempt := 0; ; attempt++ {
		*r = herdr.AgentResult{}
		err := s.Call(ctx, "agent.start", params, r)
		var remote *herdr.Error
		if err == nil || attempt == len(busyBackoff) || !errors.As(err, &remote) || remote.Code != "agent_pane_busy" {
			return err
		}
		if s.wait(ctx, busyBackoff[attempt]) != nil {
			return err
		}
	}
}
func setPlacement(r *Result, p herdr.Pane) {
	r.WorkspaceID = libagent.Pointer(p.WorkspaceID)
	r.TabID = libagent.Pointer(p.TabID)
	r.PaneID = libagent.Pointer(p.PaneID)
	r.Cwd = p.Cwd
}

func samePane(a, b herdr.Pane) bool {
	return a.PaneID == b.PaneID && a.WorkspaceID == b.WorkspaceID && a.TabID == b.TabID
}
func (s *spawner) customizePane(ctx context.Context, o Options, p herdr.Pane, out *libagent.Outcome) error {
	if o.Label != "" {
		var r herdr.PaneResult
		err := s.Call(ctx, "pane.rename", map[string]any{"pane_id": p.PaneID, "label": o.Label}, &r)
		if err == nil && (r.Type != "pane_info" || !samePane(r.Pane, p)) {
			err = libagent.Protocol("incomplete or mismatched pane.rename result")
		}
		if err != nil {
			out.Fail(err, "pane.rename", true)
			return err
		}
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "pane_label", ID: p.PaneID})
	}
	if o.Focus {
		var r herdr.PaneResult
		err := s.Call(ctx, "pane.focus", map[string]any{"pane_id": p.PaneID}, &r)
		if err == nil && (r.Type != "pane_info" || !samePane(r.Pane, p)) {
			err = libagent.Protocol("incomplete or mismatched pane.focus result")
		}
		if err != nil {
			out.Fail(err, "pane.focus", true)
			return err
		}
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "focus", ID: p.PaneID})
	}
	return nil
}
