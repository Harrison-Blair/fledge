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
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	var st state.Session
	err = s.Store.WithLocked(s.Project.Session, s.Project.Root, func(current *state.Session) error {
		reconcileMappings(current, snapshot, s.Project.Root, s.Project.Session, s.WorkspaceID)
		st = cloneState(*current)
		return nil
	})
	if err != nil {
		return nil, err
	}
	live := agentsByPane(snapshot)
	for name, managed := range st.Agents {
		if managed.ActivationID != "" {
			if agent, ok := live[managed.PaneID]; !ok || agent.Agent == nil {
				if err := s.deactivateMessagingAgent(name, "recipient agent exited"); err != nil {
					return nil, err
				}
				managed.ActivationID = ""
				st.Agents[name] = managed
			}
		}
	}
	panes := panesByID(snapshot)
	out := make([]AgentView, 0, len(st.Agents))
	pending, err := s.pendingMessageCounts(st.ActiveRunID)
	if err != nil {
		return nil, err
	}
	for name, managed := range st.Agents {
		view := AgentView{
			Name: name, Kind: managed.Kind, Model: managed.Model, Placement: managed.Placement,
			CWD: managed.CWD, State: "unknown", PaneID: managed.PaneID, TabID: managed.TabID,
			PendingMessages: pending[name],
		}
		if pane, ok := panes[managed.PaneID]; ok {
			view.State = pane.AgentStatus
			if pane.Agent == nil {
				view.State = "stopped"
			}
			if agent, ok := live[managed.PaneID]; ok && agent.AgentStatus != "" {
				view.State = agent.AgentStatus
				if agent.Agent == nil {
					view.State = "stopped"
				}
			}
		}
		out = append(out, view)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
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
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		return state.Agent{}, err
	}
	var managed state.Agent
	var ok bool
	err = s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		reconcileMappings(st, snapshot, s.Project.Root, s.Project.Session, s.WorkspaceID)
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
