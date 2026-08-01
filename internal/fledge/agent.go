package fledge

import (
	"context"
	"fmt"
	"sort"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/state"
)

type AgentView struct {
	Name            string `json:"name"`
	Kind            string `json:"kind,omitempty"`
	Model           string `json:"model"`
	Profile         string `json:"profile,omitempty"`
	Placement       string `json:"placement,omitempty"`
	CWD             string `json:"cwd,omitempty"`
	State           string `json:"state"`
	PaneID          string `json:"pane_id"`
	TabID           string `json:"tab_id"`
	PendingMessages int    `json:"pending_messages"`
}

func (s *Service) ListAgents(ctx context.Context) ([]AgentView, error) {
	_, _, client, err := s.running(ctx)
	if err != nil {
		return nil, err
	}
	return s.listWithClient(ctx, client)
}

func (s *Service) listWithClient(ctx context.Context, client *herdr.Client) ([]AgentView, error) {
	var st state.Session
	snapshot, err := s.withReconciledState(ctx, client, func(current *state.Session) error {
		st = cloneState(*current)
		return nil
	})
	if err != nil {
		return nil, err
	}
	live := agentsByPane(snapshot)
	if err := s.deactivateExitedMessagingAgents(st.Agents, live); err != nil {
		return nil, err
	}
	panes := panesByID(snapshot)
	out := make([]AgentView, 0, len(st.Agents))
	pending, err := s.messages().pendingCounts(st.ActiveRunID)
	if err != nil {
		return nil, err
	}
	for name, managed := range st.Agents {
		view := baseView(name, managed)
		view.State = resolveAgentState(panes, live, managed.PaneID)
		view.PendingMessages = pending[name]
		out = append(out, view)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// withReconciledState snapshots the server, then reconciles the persisted
// mappings against it under the state lock before running fn.
func (s *Service) withReconciledState(
	ctx context.Context,
	client *herdr.Client,
	fn func(st *state.Session) error,
) (herdr.Snapshot, error) {
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		return herdr.Snapshot{}, err
	}
	err = s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		reconcileMappings(st, snapshot, s.Project.Root, s.Project.Session, s.WorkspaceID)
		return fn(st)
	})
	if err != nil {
		return herdr.Snapshot{}, err
	}
	return snapshot, nil
}

// deactivateExitedMessagingAgents retires the message activation of every
// agent whose pane no longer hosts a running harness.
func (s *Service) deactivateExitedMessagingAgents(agents map[string]state.Agent, live map[string]herdr.AgentInfo) error {
	messages := s.messages()
	for name, managed := range agents {
		if managed.ActivationID == "" {
			continue
		}
		if agent, ok := live[managed.PaneID]; ok && agent.Agent != nil {
			continue
		}
		if err := messages.deactivateAgent(name, "recipient agent exited"); err != nil {
			return err
		}
		managed.ActivationID = ""
		agents[name] = managed
	}
	return nil
}

// resolveAgentState reports the lifecycle state of paneID, preferring the
// agent record over the pane's cached status.
func resolveAgentState(panes map[string]herdr.PaneInfo, live map[string]herdr.AgentInfo, paneID string) string {
	pane, ok := panes[paneID]
	if !ok {
		return StateUnknown
	}
	status := pane.AgentStatus
	if pane.Agent == nil {
		status = StateStopped
	}
	if agent, ok := live[paneID]; ok && agent.AgentStatus != "" {
		status = agent.AgentStatus
		if agent.Agent == nil {
			status = StateStopped
		}
	}
	return status
}

// baseView projects the durable fields of a managed agent; callers supply the
// lifecycle state.
func baseView(name string, managed state.Agent) AgentView {
	return AgentView{
		Name: name, Kind: managed.Kind, Model: managed.Model, Profile: managed.Profile, Placement: managed.Placement,
		CWD: managed.CWD, PaneID: managed.PaneID, TabID: managed.TabID,
	}
}

func reconcileMappings(st *state.Session, snapshot herdr.Snapshot, root, session, preferredWorkspace string) {
	if preferredWorkspace != "" && hasWorkspace(snapshot, preferredWorkspace) {
		st.WorkspaceID = preferredWorkspace
	}
	if !hasWorkspace(snapshot, st.WorkspaceID) {
		st.WorkspaceID = ""
		if workspace, found := fallbackWorkspace(snapshot, root, session); found {
			st.WorkspaceID = workspace.WorkspaceID
		}
	}
	panes := panesByID(snapshot)
	for name, managed := range st.Agents {
		if pane, exists := panes[managed.PaneID]; exists &&
			(st.WorkspaceID == "" || pane.WorkspaceID == st.WorkspaceID) {
			continue
		}
		expected := agentLabelPrefix + name
		for _, pane := range snapshot.Panes {
			if pane.Label != nil && (*pane.Label == expected || *pane.Label == name) &&
				(st.WorkspaceID == "" || pane.WorkspaceID == st.WorkspaceID) {
				managed.PaneID, managed.TabID = pane.PaneID, pane.TabID
				st.Agents[name] = managed
				break
			}
		}
	}
}

func cloneState(st state.Session) state.Session {
	cloned := st
	cloned.Agents = make(map[string]state.Agent, len(st.Agents))
	for name, agent := range st.Agents {
		cloned.Agents[name] = agent
	}
	return cloned
}

func (s *Service) AgentStatus(ctx context.Context, name string) ([]AgentView, error) {
	agents, err := s.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return agents, nil
	}
	for _, agent := range agents {
		if agent.Name == name {
			return []AgentView{agent}, nil
		}
	}
	return nil, NewError("agent_not_found", fmt.Sprintf("managed agent %q does not exist", name))
}

func panesByID(snapshot herdr.Snapshot) map[string]herdr.PaneInfo {
	result := make(map[string]herdr.PaneInfo, len(snapshot.Panes))
	for _, pane := range snapshot.Panes {
		result[pane.PaneID] = pane
	}
	return result
}

func agentsByPane(snapshot herdr.Snapshot) map[string]herdr.AgentInfo {
	result := make(map[string]herdr.AgentInfo, len(snapshot.Agents))
	for _, agent := range snapshot.Agents {
		result[agent.PaneID] = agent
	}
	return result
}

func (s *Service) managed(ctx context.Context, client *herdr.Client, name string) (state.Agent, error) {
	var managed state.Agent
	var ok bool
	snapshot, err := s.withReconciledState(ctx, client, func(st *state.Session) error {
		managed, ok = st.Agents[name]
		return nil
	})
	if err != nil {
		return state.Agent{}, err
	}
	if !ok {
		return state.Agent{}, NewError("agent_not_found", fmt.Sprintf("managed agent %q does not exist", name))
	}
	if _, ok := panesByID(snapshot)[managed.PaneID]; !ok {
		return state.Agent{}, NewError("agent_pane_missing", fmt.Sprintf("managed pane for agent %q no longer exists", name))
	}
	return managed, nil
}

// AgentTarget resolves a logical name to its durable pane target.
func (s *Service) AgentTarget(ctx context.Context, name string) (state.Agent, error) {
	_, _, client, err := s.running(ctx)
	if err != nil {
		return state.Agent{}, err
	}
	return s.managed(ctx, client, name)
}
