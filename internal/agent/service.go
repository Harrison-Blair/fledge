package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
	"unicode/utf8"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

// API is the socket boundary; tests supply a deterministic request script.
type API interface {
	Call(context.Context, string, any, any) error
}
type Service struct {
	API             API
	CallerPane, Cwd string
	// Wait pauses between agent.start retries; nil uses a real timer.
	Wait    func(context.Context, time.Duration) error
	initErr error
}

// FromEnvironment creates an operation-local service without contacting Herdr.
func FromEnvironment(timeout time.Duration) *Service {
	s := &Service{CallerPane: os.Getenv("HERDR_PANE_ID")}
	if os.Getenv("HERDR_ENV") != "1" || os.Getenv("HERDR_SOCKET_PATH") == "" {
		s.initErr = fmt.Errorf("run inside Herdr with HERDR_ENV=1 and HERDR_SOCKET_PATH set")
	}
	s.Cwd, _ = os.Getwd()
	s.API = herdr.Client{Socket: os.Getenv("HERDR_SOCKET_PATH"), Timeout: timeout + 15*time.Second}
	return s
}
func (s *Service) call(ctx context.Context, method string, params any, result any) error {
	if s.initErr != nil {
		return s.initErr
	}
	err := s.API.Call(ctx, method, params, result)
	if err != nil {
		return &phaseError{phase: method, cause: err}
	}
	return nil
}
func (s *Service) wait(ctx context.Context, d time.Duration) error {
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
func protocol(message string) error {
	return &herdr.Error{Code: "protocol_error", Message: message, Uncertain: true}
}
func validPane(p herdr.Pane) bool { return p.PaneID != "" && p.WorkspaceID != "" && p.TabID != "" }
func validAgent(p herdr.Pane) bool {
	if !validPane(p) {
		return false
	}
	switch p.AgentStatus {
	case "idle", "working", "blocked", "done", "unknown":
		return true
	}
	return false
}
func validAgentInfo(a herdr.AgentDetails) bool {
	if !validAgent(a.Pane) || a.TerminalID == "" || a.Focused == nil || a.Revision == nil {
		return false
	}
	if s := a.AgentSession; s != nil {
		return s.Source != nil && s.Agent != nil && s.Kind != nil && s.Value != nil &&
			(*s.Kind == "id" || *s.Kind == "path")
	}
	return true
}

// resolveTarget collapses the exactly-one-of --name/--pane choice into one agent.get target.
func resolveTarget(name, pane string) (string, error) {
	if (name == "") == (pane == "") {
		return "", invalid("exactly one of --name or --pane is required")
	}
	if name != "" {
		return name, nil
	}
	return pane, nil
}

// lookup fetches one agent by target and fails out on transport, protocol, or validation errors.
func (s *Service) lookup(ctx context.Context, target string, out *Outcome) (herdr.AgentDetails, error) {
	var r herdr.AgentResult
	err := s.call(ctx, "agent.get", map[string]any{"target": target}, &r)
	if err == nil && (r.Type != "agent_info" || !validAgentInfo(r.Agent)) {
		err = protocol("incomplete agent.get result")
	}
	if err != nil {
		out.fail(err, "agent.get", false)
	}
	return r.Agent, err
}
func (s *Service) snapshot(ctx context.Context) (*herdr.Snapshot, error) {
	var r herdr.SnapshotResult
	if err := s.call(ctx, "session.snapshot", nil, &r); err != nil {
		return nil, err
	}
	if r.Type != "session_snapshot" || r.Snapshot == nil || r.Snapshot.Workspaces == nil || r.Snapshot.Tabs == nil || r.Snapshot.Panes == nil || r.Snapshot.Layouts == nil || r.Snapshot.Agents == nil {
		return nil, protocol("incomplete session.snapshot result")
	}
	for _, w := range r.Snapshot.Workspaces {
		if w.ID == "" {
			return nil, protocol("workspace missing ID")
		}
	}
	for _, t := range r.Snapshot.Tabs {
		if t.ID == "" || t.WorkspaceID == "" {
			return nil, protocol("tab missing ownership")
		}
	}
	for _, p := range r.Snapshot.Panes {
		if !validPane(p) {
			return nil, protocol("pane missing ownership")
		}
	}
	return r.Snapshot, nil
}
func (s *Service) List(ctx context.Context) Outcome {
	out := Outcome{Operation: "agent.list", Status: "success", Effects: []Effect{}}
	var r herdr.AgentListResult
	err := s.call(ctx, "agent.list", nil, &r)
	if err == nil && (r.Type != "agent_list" || r.Agents == nil) {
		err = protocol("incomplete agent.list result")
	}
	rows := make([]AgentRow, 0, len(r.Agents))
	for _, p := range r.Agents {
		if !validAgent(p) {
			err = protocol("incomplete agent info")
			break
		}
		rows = append(rows, row(p))
	}
	if err != nil {
		out.fail(err, "agent.list", false)
		return out
	}
	out.Result = ListResult{Agents: rows}
	return out
}

type MessageOptions struct {
	Name, Pane, Body, File string
	BodySet, FileSet       bool
}

func (o MessageOptions) read(in io.Reader) (string, string, error) {
	target, err := resolveTarget(o.Name, o.Pane)
	if err != nil {
		return "", "", err
	}
	if o.BodySet == o.FileSet {
		return "", "", invalid("exactly one of --body or --file is required")
	}
	text := o.Body
	if o.FileSet {
		var b []byte
		var err error
		if o.File == "-" {
			b, err = io.ReadAll(in)
		} else {
			b, err = os.ReadFile(o.File)
		}
		if err != nil {
			return "", "", fmt.Errorf("read message: %w", err)
		}
		text = string(b)
	}
	if text == "" || !utf8.ValidString(text) {
		return "", "", invalid("message must be nonempty UTF-8")
	}
	return target, text, nil
}
func (s *Service) Message(ctx context.Context, o MessageOptions, in io.Reader) Outcome {
	out := Outcome{Operation: "agent.message", Status: "success", Effects: []Effect{}}
	target, text, err := o.read(in)
	if err != nil {
		out.fail(err, "validation", false)
		return out
	}
	a, err := s.lookup(ctx, target, &out)
	if err != nil {
		return out
	}
	out.Result = MessageResult{AgentRow: row(a.Pane)}
	var r herdr.AgentResult
	err = s.call(ctx, "agent.prompt", map[string]any{"target": target, "text": text}, &r)
	if err == nil && (r.Type != "agent_prompted" || !validAgent(r.Agent.Pane)) {
		err = protocol("incomplete agent.prompt result")
	}
	if err != nil {
		out.fail(err, "agent.prompt", true)
		return out
	}
	out.Result = MessageResult{AgentRow: row(r.Agent.Pane), Submitted: true}
	out.Effects = append(out.Effects, Effect{Action: "submitted", Kind: "message", ID: r.Agent.PaneID})
	return out
}

type StopOptions struct {
	Name, Pane string
	Force      bool
}

func (s *Service) Stop(ctx context.Context, o StopOptions) Outcome {
	out := Outcome{Operation: "agent.stop", Status: "success", Effects: []Effect{}}
	target, err := resolveTarget(o.Name, o.Pane)
	if err != nil {
		out.fail(err, "validation", false)
		return out
	}
	a, err := s.lookup(ctx, target, &out)
	if err != nil {
		return out
	}
	out.Result = StopResult{AgentRow: row(a.Pane)}
	if status := a.AgentStatus; status != "idle" && status != "done" && !o.Force {
		out.fail(invalid("agent %s is %s; pass --force to stop it anyway", target, status), "guard", false)
		return out
	}
	var closed struct {
		Type string `json:"type"`
	}
	err = s.call(ctx, "pane.close", map[string]any{"pane_id": a.PaneID}, &closed)
	if err == nil && closed.Type != "ok" {
		err = protocol("incomplete pane.close result")
	}
	if err != nil {
		out.fail(err, "pane.close", true)
		return out
	}
	out.Result = StopResult{AgentRow: row(a.Pane), Stopped: true}
	out.Effects = append(out.Effects, Effect{Action: "closed", Kind: "pane", ID: a.PaneID})
	return out
}
func (s *Service) Spawn(ctx context.Context, o SpawnOptions) Outcome {
	result := &SpawnResult{Name: o.Name, Harness: o.Harness}
	out := Outcome{Operation: "agent.spawn", Status: "success", Result: result, Effects: []Effect{}}
	args, err := o.Validate()
	if err != nil {
		out.fail(err, "validation", false)
		return out
	}
	snapshot, err := s.snapshot(ctx)
	if err != nil {
		out.fail(err, "session.snapshot", false)
		return out
	}
	for _, a := range snapshot.Agents {
		if a.Name != nil && *a.Name == o.Name {
			out.fail(invalid("agent name %q is already in use", o.Name), "preflight", false)
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
			out.fail(err, "placement", false)
		}
		return out
	}
	setPlacement(result, p)
	if err = s.customizePane(ctx, o, p, &out); err != nil {
		return out
	}

	var r herdr.AgentResult
	err = s.start(ctx, map[string]any{"name": o.Name, "kind": o.Harness, "pane_id": p.PaneID, "args": args, "timeout_ms": o.Timeout.Milliseconds()}, &r)
	if err == nil && (r.Type != "agent_started" || !validAgent(r.Agent.Pane) || !samePane(r.Agent.Pane, p) || r.Argv == nil) {
		err = protocol("incomplete agent.start result")
	}
	if err != nil {
		out.fail(err, "agent.start", true)
		return out
	}
	setPlacement(result, r.Agent.Pane)
	result.DetectedHarness = r.Agent.Agent
	result.AgentStatus = pointer(r.Agent.AgentStatus)
	result.Argv = r.Argv
	out.Effects = append(out.Effects, Effect{Action: "started", Kind: "agent", ID: r.Agent.PaneID})
	return out
}

// busyBackoff paces agent.start retries while a fresh shell reaches its prompt.
var busyBackoff = []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, 800 * time.Millisecond}

// start retries only agent_pane_busy, keeping the last busy error once attempts or the context run out.
func (s *Service) start(ctx context.Context, params map[string]any, r *herdr.AgentResult) error {
	for attempt := 0; ; attempt++ {
		*r = herdr.AgentResult{}
		err := s.call(ctx, "agent.start", params, r)
		var remote *herdr.Error
		if err == nil || attempt == len(busyBackoff) || !errors.As(err, &remote) || remote.Code != "agent_pane_busy" {
			return err
		}
		if s.wait(ctx, busyBackoff[attempt]) != nil {
			return err
		}
	}
}
func setPlacement(r *SpawnResult, p herdr.Pane) {
	r.WorkspaceID = pointer(p.WorkspaceID)
	r.TabID = pointer(p.TabID)
	r.PaneID = pointer(p.PaneID)
	r.Cwd = p.Cwd
}

func samePane(a, b herdr.Pane) bool {
	return a.PaneID == b.PaneID && a.WorkspaceID == b.WorkspaceID && a.TabID == b.TabID
}
func (s *Service) customizePane(ctx context.Context, o SpawnOptions, p herdr.Pane, out *Outcome) error {
	if o.Label != "" {
		var r herdr.PaneResult
		err := s.call(ctx, "pane.rename", map[string]any{"pane_id": p.PaneID, "label": o.Label}, &r)
		if err == nil && (r.Type != "pane_info" || !samePane(r.Pane, p)) {
			err = protocol("incomplete or mismatched pane.rename result")
		}
		if err != nil {
			out.fail(err, "pane.rename", true)
			return err
		}
		out.Effects = append(out.Effects, Effect{Action: "updated", Kind: "pane_label", ID: p.PaneID})
	}
	if o.Focus {
		var r herdr.PaneResult
		err := s.call(ctx, "pane.focus", map[string]any{"pane_id": p.PaneID}, &r)
		if err == nil && (r.Type != "pane_info" || !samePane(r.Pane, p)) {
			err = protocol("incomplete or mismatched pane.focus result")
		}
		if err != nil {
			out.fail(err, "pane.focus", true)
			return err
		}
		out.Effects = append(out.Effects, Effect{Action: "updated", Kind: "focus", ID: p.PaneID})
	}
	return nil
}

type phaseError struct {
	phase string
	cause error
}

func (e *phaseError) Error() string { return e.cause.Error() }
func (e *phaseError) Unwrap() error { return e.cause }
