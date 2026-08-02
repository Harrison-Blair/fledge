package fledge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/state"
)

func viewFromInfo(name string, managed state.Agent, info herdr.AgentInfo) AgentView {
	status := info.AgentStatus
	if status == "" {
		status = StateUnknown
	}
	view := baseView(name, managed)
	view.State = status
	return view
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
	Agent     AgentView `json:"agent"`
	Forced    bool      `json:"forced"`
	TabClosed bool      `json:"tab_closed"`
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
		closeDedicatedTab, err := s.canCloseDedicatedAgentTab(ctx, client, name, managed)
		if err != nil {
			return AgentStopResult{}, err
		}
		method, params := "pane.close", map[string]any{"pane_id": managed.PaneID}
		if closeDedicatedTab {
			method, params = "tab.close", map[string]any{"tab_id": managed.TabID}
		}
		if err := client.Call(ctx, method, params, nil); err != nil {
			if closeDedicatedTab {
				return AgentStopResult{}, agentTabCloseError(name, managed, false, err)
			}
			return AgentStopResult{}, err
		}
		if err := s.messages().deactivateAgent(name, "agent force-stopped"); err != nil {
			return AgentStopResult{}, err
		}
		return stoppedResult(name, managed, true, closeDedicatedTab), nil
	}
	if stopped, _ := s.agentStopped(ctx, client, managed.PaneID); stopped {
		if err := s.messages().deactivateAgent(name, "agent already stopped"); err != nil {
			return AgentStopResult{}, err
		}
		return s.finishStoppedAgent(ctx, client, name, managed, false)
	}
	deadline := time.Now().Add(timeout)
	if s.gracefullyStopPane(ctx, client, managed.PaneID, deadline) {
		if err := s.messages().deactivateAgent(name, "agent stopped"); err != nil {
			return AgentStopResult{}, err
		}
		return s.finishStoppedAgent(ctx, client, name, managed, false)
	}
	return AgentStopResult{}, &Error{
		Code:    "agent_stop_timeout",
		Message: fmt.Sprintf("agent %q did not stop within %s; its pane was preserved", name, timeout),
		Details: map[string]any{"name": name, "pane_id": managed.PaneID},
	}
}

func (s *Service) canCloseDedicatedAgentTab(
	ctx context.Context,
	client *herdr.Client,
	name string,
	managed state.Agent,
) (bool, error) {
	if managed.Placement != "tab" || managed.TabID == "" {
		return false, nil
	}
	st, err := s.Store.Read(s.Project.Session, s.Project.Root)
	if err != nil {
		return false, err
	}
	if st.OrchestratorTabID == managed.TabID ||
		(name == "fledge-orchestrator" && st.OrchestratorPaneID == managed.PaneID) {
		return false, nil
	}
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		return false, err
	}
	panes := 0
	for _, pane := range snapshot.Panes {
		if pane.TabID != managed.TabID {
			continue
		}
		panes++
		if pane.PaneID != managed.PaneID {
			return false, nil
		}
	}
	return panes == 1, nil
}

func (s *Service) finishStoppedAgent(
	ctx context.Context,
	client *herdr.Client,
	name string,
	managed state.Agent,
	forced bool,
) (AgentStopResult, error) {
	// Eligibility must be checked after the agent exits. A user can split the
	// tab while graceful shutdown is in progress, and closing based on an
	// earlier snapshot would destroy that newly shared pane.
	closeDedicatedTab, err := s.canCloseDedicatedAgentTab(ctx, client, name, managed)
	if err != nil {
		return AgentStopResult{}, err
	}
	if closeDedicatedTab {
		if err := client.Call(ctx, "tab.close", map[string]any{"tab_id": managed.TabID}, nil); err != nil {
			return AgentStopResult{}, agentTabCloseError(name, managed, true, err)
		}
	}
	return stoppedResult(name, managed, forced, closeDedicatedTab), nil
}

func agentTabCloseError(name string, managed state.Agent, stopped bool, err error) error {
	message := fmt.Sprintf("agent %q could not be force-stopped by closing its dedicated tab %s: %v", name, managed.TabID, err)
	if stopped {
		message = fmt.Sprintf("agent %q stopped, but its dedicated tab %s could not be closed: %v", name, managed.TabID, err)
	}
	return &Error{
		Code:    "agent_tab_close_failed",
		Message: message,
		Details: map[string]any{"name": name, "pane_id": managed.PaneID, "tab_id": managed.TabID},
		Cause:   err,
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
	_ = client.Call(stopCtx, "agent.send_keys", map[string]any{"target": paneID, "keys": []string{"ctrl+d"}}, nil)
	firstBudget := time.Until(deadline) / 3
	if s.pollStopped(stopCtx, client, paneID, deadline, firstBudget) {
		return true
	}
	_ = client.Call(stopCtx, "agent.send_keys", map[string]any{"target": paneID, "keys": []string{"ctrl+c"}}, nil)
	remaining := time.Until(deadline)
	if remaining > 0 {
		settle := remaining / 2
		var ignored herdr.Result
		_ = client.Call(stopCtx, "agent.wait", map[string]any{
			"target": paneID, "until": stopSettleStates, "timeout_ms": settle.Milliseconds(),
		}, &ignored)
	}
	_ = client.Call(stopCtx, "agent.send_keys", map[string]any{"target": paneID, "keys": []string{"ctrl+d"}}, nil)
	return s.pollStopped(stopCtx, client, paneID, deadline, time.Until(deadline))
}

func stoppedResult(name string, managed state.Agent, forced, tabClosed bool) AgentStopResult {
	return AgentStopResult{
		Agent:     stoppedView(name, managed),
		Forced:    forced,
		TabClosed: tabClosed,
	}
}

func stoppedView(name string, managed state.Agent) AgentView {
	view := baseView(name, managed)
	view.State = StateStopped
	return view
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
