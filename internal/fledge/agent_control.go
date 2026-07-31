package fledge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/state"
)

type PromptOptions struct {
	Name    string
	Text    string
	Wait    bool
	Until   []string
	Timeout time.Duration
}

func (s *Service) Prompt(ctx context.Context, opts PromptOptions) (AgentView, error) {
	_, _, client, err := s.running(ctx)
	if err != nil {
		return AgentView{}, err
	}
	managed, err := s.managed(ctx, client, opts.Name)
	if err != nil {
		return AgentView{}, err
	}
	params := map[string]any{"target": managed.PaneID, "text": opts.Text}
	if opts.Wait {
		wait := map[string]any{}
		if len(opts.Until) > 0 {
			wait["until"] = opts.Until
		}
		if timeout := herdr.Milliseconds(opts.Timeout); timeout != nil {
			wait["timeout_ms"] = *timeout
		}
		params["wait"] = wait
	}
	var result herdr.Result
	if err := client.Call(ctx, "agent.prompt", params, &result); err != nil {
		return AgentView{}, err
	}
	return viewFromInfo(opts.Name, managed, result.Agent), nil
}

func (s *Service) Wait(ctx context.Context, name string, until []string, timeout time.Duration) (AgentView, error) {
	_, _, client, err := s.running(ctx)
	if err != nil {
		return AgentView{}, err
	}
	managed, err := s.managed(ctx, client, name)
	if err != nil {
		return AgentView{}, err
	}
	params := map[string]any{"target": managed.PaneID}
	if len(until) > 0 {
		params["until"] = until
	}
	if value := herdr.Milliseconds(timeout); value != nil {
		params["timeout_ms"] = *value
	}
	var result herdr.Result
	if err := client.Call(ctx, "agent.wait", params, &result); err != nil {
		return AgentView{}, err
	}
	return viewFromInfo(name, managed, result.Agent), nil
}

func viewFromInfo(name string, managed state.Agent, info herdr.AgentInfo) AgentView {
	status := info.AgentStatus
	if status == "" {
		status = "unknown"
	}
	return AgentView{
		Name: name, Kind: managed.Kind, Model: managed.Model, Placement: managed.Placement,
		CWD: managed.CWD, State: status, PaneID: managed.PaneID, TabID: managed.TabID,
	}
}

type ReadResult struct {
	Name      string `json:"name"`
	PaneID    string `json:"pane_id"`
	Source    string `json:"source"`
	Format    string `json:"format"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
	Revision  uint64 `json:"revision"`
}

func (s *Service) ReadAgent(ctx context.Context, name, source string, lines int, ansi bool) (ReadResult, error) {
	_, _, client, err := s.running(ctx)
	if err != nil {
		return ReadResult{}, err
	}
	managed, err := s.managed(ctx, client, name)
	if err != nil {
		return ReadResult{}, err
	}
	format := "text"
	if ansi {
		format = "ansi"
	}
	params := map[string]any{"target": managed.PaneID, "source": strings.ReplaceAll(source, "-", "_"), "format": format, "strip_ansi": !ansi}
	if lines > 0 {
		params["lines"] = lines
	}
	var raw struct {
		Type string `json:"type"`
		Read struct {
			PaneID    string `json:"pane_id"`
			Source    string `json:"source"`
			Format    string `json:"format"`
			Text      string `json:"text"`
			Truncated bool   `json:"truncated"`
			Revision  uint64 `json:"revision"`
		} `json:"read"`
	}
	if err := client.Call(ctx, "agent.read", params, &raw); err != nil {
		return ReadResult{}, err
	}
	return ReadResult{
		Name: name, PaneID: managed.PaneID, Source: raw.Read.Source, Format: raw.Read.Format,
		Text: raw.Read.Text, Truncated: raw.Read.Truncated, Revision: raw.Read.Revision,
	}, nil
}

type AgentStopResult struct {
	Agent  AgentView `json:"agent"`
	Forced bool      `json:"forced"`
}

func (s *Service) StopAgent(ctx context.Context, name string, timeout time.Duration, force bool) (AgentStopResult, error) {
	_, _, client, err := s.running(ctx)
	if err != nil {
		return AgentStopResult{}, err
	}
	managed, err := s.managed(ctx, client, name)
	if err != nil {
		return AgentStopResult{}, err
	}
	if force {
		if name == "fledge-orchestrator" {
			st, readErr := s.Store.Read(s.Project.Session, s.Project.Root)
			if readErr != nil {
				return AgentStopResult{}, readErr
			}
			if st.OrchestratorPaneID != "" && st.OrchestratorPaneID == managed.PaneID {
				return AgentStopResult{}, NewError("orchestrator_force_stop_forbidden",
					"cannot force-close the saved orchestrator pane; use `fledge stop --force` for coordinated shutdown")
			}
		}
		if err := client.Call(ctx, "pane.close", map[string]any{"pane_id": managed.PaneID}, nil); err != nil {
			return AgentStopResult{}, err
		}
		if err := s.deactivateMessagingAgent(name, "agent force-stopped"); err != nil {
			return AgentStopResult{}, err
		}
		return stoppedResult(name, managed, true), nil
	}
	if stopped, _ := s.agentStopped(ctx, client, managed.PaneID); stopped {
		if err := s.deactivateMessagingAgent(name, "agent already stopped"); err != nil {
			return AgentStopResult{}, err
		}
		return AgentStopResult{Agent: stoppedView(name, managed)}, nil
	}
	deadline := time.Now().Add(timeout)
	if s.gracefullyStopPane(ctx, client, managed.PaneID, deadline) {
		if err := s.deactivateMessagingAgent(name, "agent stopped"); err != nil {
			return AgentStopResult{}, err
		}
		return stoppedResult(name, managed, false), nil
	}
	return AgentStopResult{}, &Error{
		Code:    "agent_stop_timeout",
		Message: fmt.Sprintf("agent %q did not stop within %s; its pane was preserved", name, timeout),
		Details: map[string]any{"name": name, "pane_id": managed.PaneID},
	}
}

// gracefullyStopPane asks the active agent in paneID to exit without closing
// its pane. All phases share deadline so callers can coordinate several panes
// under one shutdown budget.
func (s *Service) gracefullyStopPane(ctx context.Context, client *herdr.Client, paneID string, deadline time.Time) bool {
	stopCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if stopped, _ := s.agentStopped(stopCtx, client, paneID); stopped {
		return true
	}
	_ = client.Call(stopCtx, "agent.send_keys", map[string]any{"target": paneID, "keys": []string{"Ctrl+D"}}, nil)
	firstBudget := time.Until(deadline) / 3
	if s.pollStopped(stopCtx, client, paneID, deadline, firstBudget) {
		return true
	}
	_ = client.Call(stopCtx, "agent.send_keys", map[string]any{"target": paneID, "keys": []string{"Ctrl+C"}}, nil)
	remaining := time.Until(deadline)
	if remaining > 0 {
		settle := remaining / 2
		var ignored herdr.Result
		_ = client.Call(stopCtx, "agent.wait", map[string]any{
			"target": paneID, "until": []string{"idle", "done", "blocked"}, "timeout_ms": settle.Milliseconds(),
		}, &ignored)
	}
	_ = client.Call(stopCtx, "agent.send_keys", map[string]any{"target": paneID, "keys": []string{"Ctrl+D"}}, nil)
	return s.pollStopped(stopCtx, client, paneID, deadline, time.Until(deadline))
}

func stoppedResult(name string, managed state.Agent, forced bool) AgentStopResult {
	return AgentStopResult{
		Agent:  stoppedView(name, managed),
		Forced: forced,
	}
}

func stoppedView(name string, managed state.Agent) AgentView {
	return AgentView{
		Name: name, Kind: managed.Kind, Model: managed.Model, Placement: managed.Placement,
		CWD: managed.CWD, State: "stopped", PaneID: managed.PaneID, TabID: managed.TabID,
	}
}

func (s *Service) pollStopped(ctx context.Context, client *herdr.Client, paneID string, deadline time.Time, budget time.Duration) bool {
	end := time.Now().Add(budget)
	if deadline.Before(end) {
		end = deadline
	}
	for {
		stopped, err := s.agentStopped(ctx, client, paneID)
		if err == nil && stopped {
			return true
		}
		if time.Now().After(end) || ctx.Err() != nil {
			return false
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}

func (s *Service) agentStopped(ctx context.Context, client *herdr.Client, paneID string) (bool, error) {
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		return false, err
	}
	if agent, ok := agentsByPane(snapshot)[paneID]; ok && agent.Agent != nil {
		return false, nil
	}
	pane, ok := panesByID(snapshot)[paneID]
	return !ok || pane.Agent == nil, nil
}
