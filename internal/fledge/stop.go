package fledge

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/state"
)

const coordinatedStopTimeout = 10 * time.Second

// StopCleanupRequest contains the complete, caller-independent input needed
// by a detached process to finish a coordinated stop.
type StopCleanupRequest struct {
	ProjectRoot    string
	Session        string
	StateDir       string
	HerdrBinary    string
	BaseGeneration uint64
	Timeout        time.Duration
}

type StopResult struct {
	Session                 string   `json:"session"`
	Stopped                 bool     `json:"stopped"`
	Deleted                 bool     `json:"deleted"`
	Forced                  bool     `json:"forced"`
	Agents                  []string `json:"agents,omitempty"`
	GracefullyStoppedAgents []string `json:"gracefully_stopped_agents,omitempty"`
	ForcedAgents            []string `json:"forced_agents,omitempty"`
}

// StopAgentInspection describes an active agent that would be affected by a
// coordinated session shutdown.
type StopAgentInspection struct {
	Name        string `json:"name"`
	Harness     string `json:"harness"`
	State       string `json:"state"`
	WorkspaceID string `json:"workspace_id"`
	PaneID      string `json:"pane_id"`
}

// StopInspection is a read-only preview of the session and live agents that a
// subsequent Stop call would affect.
type StopInspection struct {
	Session    string                `json:"session"`
	Exists     bool                  `json:"exists"`
	Running    bool                  `json:"running"`
	LiveAgents []StopAgentInspection `json:"live_agents"`
}

// InspectStop returns the current coordinated-stop target without changing
// the Herdr session or Fledge's saved state.
func (s *Service) InspectStop(ctx context.Context) (StopInspection, error) {
	session, client, err := s.inspectStop(ctx)
	if err != nil {
		return StopInspection{}, err
	}
	out := StopInspection{
		Session: s.Project.Session,
		Exists:  session.Name != "",
		Running: session.Name != "" && session.Running,
	}
	if client != nil {
		out.LiveAgents, err = s.collectLiveAgentDetails(ctx, client)
		if err != nil {
			return StopInspection{}, err
		}
	}
	if out.LiveAgents == nil {
		out.LiveAgents = []StopAgentInspection{}
	}
	return out, nil
}

func (s *Service) Stop(ctx context.Context, force bool) (StopResult, error) {
	session, client, err := s.inspectStop(ctx)
	if err != nil {
		return StopResult{}, err
	}
	if client == nil {
		return s.cleanupStoppedSession(ctx, session, force)
	}
	liveDetails, err := s.collectLiveAgentDetails(ctx, client)
	if err != nil {
		return StopResult{}, err
	}
	live := liveAgentNames(liveDetails)
	if len(live) > 0 && !force {
		return StopResult{}, &Error{
			Code:    "live_agents",
			Message: fmt.Sprintf("refusing to stop session while agents are live: %s", strings.Join(live, ", ")),
			Details: map[string]any{"agents": live, "hint": "stop agents first or pass --force"},
		}
	}
	gracefullyStopped, forcedAgents := s.gracefullyStopAgents(ctx, client, liveDetails, coordinatedStopTimeout)
	baseline, err := s.prepareCoordinatedStop()
	if err != nil {
		return StopResult{}, err
	}
	if err := client.Call(ctx, "server.stop", nil, nil); err != nil {
		return StopResult{}, err
	}
	out := StopResult{
		Session:                 s.Project.Session,
		Stopped:                 true,
		Forced:                  force,
		Agents:                  live,
		GracefullyStoppedAgents: gracefullyStopped,
		ForcedAgents:            forcedAgents,
	}
	if err := s.finalizeStop(ctx, baseline, coordinatedStopTimeout, &out); err != nil {
		return StopResult{}, err
	}
	return out, nil
}

func (s *Service) inspectStop(ctx context.Context) (herdr.SessionInfo, *herdr.Client, error) {
	installed, err := s.inspect(ctx)
	if err != nil {
		return herdr.SessionInfo{}, nil, err
	}
	session, client, _, err := s.session(ctx, installed, false)
	return session, client, err
}

func (s *Service) cleanupStoppedSession(ctx context.Context, session herdr.SessionInfo, force bool) (StopResult, error) {
	out := StopResult{Session: s.Project.Session, Stopped: false, Forced: force}
	if session.Name != "" && session.Running {
		return StopResult{}, NewError("session_not_running",
			fmt.Sprintf("Fledge session %q is reported running but is not reachable; cannot coordinate shutdown", s.Project.Session))
	}
	if session.Name != "" {
		if err := s.deleteSession(ctx, &out); err != nil {
			return StopResult{}, err
		}
	}
	if err := s.closeActiveMessageRun("run closed during stopped-session cleanup"); err != nil {
		return StopResult{}, err
	}
	if err := s.clearDisposableState(); err != nil {
		return StopResult{}, Wrap("state_persist_failed",
			fmt.Sprintf("Herdr session was deleted but Fledge state could not be cleared: %v", err), err)
	}
	return out, nil
}

func (s *Service) collectLiveAgentDetails(ctx context.Context, client *herdr.Client) ([]StopAgentInspection, error) {
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	liveByPane := make(map[string]StopAgentInspection)
	for _, pane := range snapshot.Panes {
		if pane.Agent == nil {
			continue
		}
		liveByPane[pane.PaneID] = StopAgentInspection{
			Harness:     stringValue(pane.Agent),
			State:       pane.AgentStatus,
			WorkspaceID: pane.WorkspaceID,
			PaneID:      pane.PaneID,
		}
	}
	for _, agent := range snapshot.Agents {
		if agent.Agent == nil {
			continue
		}
		name := stringValue(agent.Name)
		harness := stringValue(agent.Agent)
		state := agent.AgentStatus
		workspaceID := agent.WorkspaceID
		if pane, ok := liveByPane[agent.PaneID]; ok {
			if harness == "" {
				harness = pane.Harness
			}
			if state == "" {
				state = pane.State
			}
			if workspaceID == "" {
				workspaceID = pane.WorkspaceID
			}
		}
		liveByPane[agent.PaneID] = StopAgentInspection{
			Name:        name,
			Harness:     harness,
			State:       state,
			WorkspaceID: workspaceID,
			PaneID:      agent.PaneID,
		}
	}
	managedNames := s.savedAgentNamesByPane(snapshot)
	live := make([]StopAgentInspection, 0, len(liveByPane))
	for _, agent := range liveByPane {
		if agent.Name == "" {
			agent.Name = managedNames[agent.PaneID]
		}
		live = append(live, normalizeStopAgentInspection(agent))
	}
	sort.Slice(live, func(i, j int) bool {
		if live[i].Name != live[j].Name {
			return live[i].Name < live[j].Name
		}
		return live[i].PaneID < live[j].PaneID
	})
	return live, nil
}

func (s *Service) savedAgentNamesByPane(snapshot herdr.Snapshot) map[string]string {
	names := map[string]string{}
	if s.Store == nil {
		return names
	}
	st, found, err := s.Store.ReadExisting(s.Project.Session, s.Project.Root)
	if err != nil || !found {
		return names
	}
	reconcileMappings(&st, snapshot, s.Project.Root, s.Project.Session, s.WorkspaceID)
	managedNames := make([]string, 0, len(st.Agents))
	for name := range st.Agents {
		managedNames = append(managedNames, name)
	}
	sort.Strings(managedNames)
	for _, name := range managedNames {
		if name != "" && st.Agents[name].PaneID != "" {
			names[st.Agents[name].PaneID] = name
		}
	}
	return names
}

func normalizeStopAgentInspection(agent StopAgentInspection) StopAgentInspection {
	if agent.Name == "" {
		agent.Name = agent.PaneID
	}
	if agent.Harness == "" {
		agent.Harness = "unknown"
	}
	if agent.State == "" {
		agent.State = "unknown"
	}
	return agent
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func liveAgentNames(live []StopAgentInspection) []string {
	names := make([]string, 0, len(live))
	for _, agent := range live {
		names = append(names, agent.Name)
	}
	sort.Strings(names)
	return names
}

func (s *Service) gracefullyStopAgents(
	ctx context.Context,
	client *herdr.Client,
	live []StopAgentInspection,
	timeout time.Duration,
) ([]string, []string) {
	if len(live) == 0 {
		return nil, nil
	}
	type outcome struct {
		name    string
		stopped bool
	}
	deadline := time.Now().Add(timeout)
	outcomes := make(chan outcome, len(live))
	for _, agent := range live {
		go func(agent StopAgentInspection) {
			outcomes <- outcome{
				name:    agent.Name,
				stopped: s.gracefullyStopPane(ctx, client, agent.PaneID, deadline),
			}
		}(agent)
	}
	graceful := make([]string, 0, len(live))
	forced := make([]string, 0, len(live))
	for range live {
		result := <-outcomes
		if result.stopped {
			graceful = append(graceful, result.name)
		} else {
			forced = append(forced, result.name)
		}
	}
	sort.Strings(graceful)
	sort.Strings(forced)
	return graceful, forced
}

func (s *Service) prepareCoordinatedStop() (uint64, error) {
	baseline, err := s.StopGeneration()
	if err != nil {
		return 0, Wrap("state_unavailable",
			fmt.Sprintf("cannot prepare coordinated shutdown state: %v; check access to the Fledge state directory", err), err)
	}
	if s.LaunchStopCleanup == nil {
		return 0, NewError("cleanup_worker_launch_failed",
			"cannot launch the detached shutdown cleanup worker: no launcher is configured")
	}
	if err := s.LaunchStopCleanup(StopCleanupRequest{
		ProjectRoot:    s.Project.Root,
		Session:        s.Project.Session,
		StateDir:       s.Store.Root,
		HerdrBinary:    s.Binary.Path,
		BaseGeneration: baseline,
		Timeout:        coordinatedStopTimeout,
	}); err != nil {
		return 0, Wrap("cleanup_worker_launch_failed",
			fmt.Sprintf("cannot launch the detached shutdown cleanup worker: %v", err), err)
	}
	return baseline, nil
}

// FinalizeStop completes a coordinated shutdown after server.stop has been
// accepted. It is safe for the detached worker and the original caller to run
// concurrently.
func (s *Service) FinalizeStop(ctx context.Context, baseline uint64, timeout time.Duration) error {
	out := StopResult{Session: s.Project.Session, Stopped: true}
	return s.finalizeStop(ctx, baseline, timeout, &out)
}

func (s *Service) finalizeStop(ctx context.Context, baseline uint64, timeout time.Duration, out *StopResult) error {
	if err := s.waitForSessionStopped(ctx, timeout); err != nil {
		return err
	}
	if err := s.deleteSession(ctx, out); err != nil {
		return err
	}
	if err := s.closeActiveMessageRun("run closed by fledge stop"); err != nil {
		return err
	}
	if err := s.finalizeStopState(baseline); err != nil {
		return Wrap("state_persist_failed",
			fmt.Sprintf("Herdr session was deleted but coordinated shutdown state could not be persisted: %v", err), err)
	}
	s.WorkspaceID = ""
	return nil
}

func (s *Service) waitForSessionStopped(ctx context.Context, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		session, found, err := s.Binary.FindSession(ctx, s.Project.Session)
		if err == nil && (!found || !session.Running) {
			return nil
		}
		if err != nil {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return Wrap("session_stop_timeout",
				fmt.Sprintf("wait for Herdr session %q to stop: %v", s.Project.Session, ctx.Err()), ctx.Err())
		case <-timer.C:
			message := fmt.Sprintf("Herdr session %q did not report stopped within %s", s.Project.Session, timeout)
			if lastErr != nil {
				message += ": " + lastErr.Error()
			}
			return NewError("session_stop_timeout", message)
		case <-ticker.C:
		}
	}
}

func (s *Service) deleteSession(ctx context.Context, out *StopResult) error {
	if err := s.Binary.DeleteSession(ctx, s.Project.Session); err != nil {
		if _, found, findErr := s.Binary.FindSession(ctx, s.Project.Session); findErr == nil && !found {
			out.Deleted = true
			return nil
		}
		return &Error{
			Code:    "session_delete_failed",
			Message: fmt.Sprintf("Herdr session %q stopped but could not be deleted: %v", s.Project.Session, err),
			Details: *out,
			Cause:   err,
		}
	}
	out.Deleted = true
	return nil
}

func (s *Service) clearDisposableState() error {
	return s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		clearDisposableSessionState(st)
		return nil
	})
}

func (s *Service) finalizeStopState(baseline uint64) error {
	return s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		if st.StopGeneration < baseline {
			return fmt.Errorf("stop generation regressed from %d to %d", baseline, st.StopGeneration)
		}
		if st.StopGeneration == baseline {
			st.StopGeneration = baseline + 1
		}
		clearDisposableSessionState(st)
		return nil
	})
}

func clearDisposableSessionState(st *state.Session) {
	st.Socket = ""
	st.WorkspaceID = ""
	st.OrchestratorTabID = ""
	st.OrchestratorPaneID = ""
	st.OrchestratorInitialized = false
	st.ActiveRunID = ""
	st.Agents = map[string]state.Agent{}
}
