package fledge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

type AgentStartOptions struct {
	Name          string
	Kind          string
	Model         string
	CWD           string
	Timeout       time.Duration
	Args          []string
	NewTab        bool
	CurrentPaneID string
	Executable    string
}

type AgentStartResult struct {
	Agent AgentView `json:"agent"`
	Argv  []string  `json:"native_args"`
}

// StartAgent retains the dedicated-tab service entrypoint for callers that do
// not participate in interactive pane takeover.
func (s *Service) StartAgent(ctx context.Context, opts AgentStartOptions) (AgentStartResult, error) {
	opts.NewTab = true
	return s.SpawnAgent(ctx, opts)
}

func (s *Service) SpawnAgent(ctx context.Context, opts AgentStartOptions) (AgentStartResult, error) {
	if err := ValidateAgentName(opts.Name); err != nil {
		return AgentStartResult{}, err
	}
	_, _, client, err := s.running(ctx)
	if err != nil {
		return AgentStartResult{}, err
	}
	cwd, err := s.resolveAgentCWD(opts.CWD)
	if err != nil {
		return AgentStartResult{}, err
	}
	forwardedArgs := modelArgs(opts.Model, opts.Args)
	if opts.CurrentPaneID != "" && !opts.NewTab {
		return s.spawnAgentInCurrentPane(ctx, client, opts, cwd, forwardedArgs)
	}
	managed, started, err := s.startAgentLocked(ctx, client, opts, cwd, forwardedArgs)
	if err != nil {
		return AgentStartResult{}, err
	}
	if err := s.activateMessagingAgent(ctx, client, opts.Name, managed.PaneID); err != nil {
		return AgentStartResult{}, err
	}
	return AgentStartResult{
		Agent: viewFromInfo(opts.Name, managed, started),
		Argv:  forwardedArgs,
	}, nil
}

func modelArgs(model string, native []string) []string {
	args := make([]string, 0, len(native)+2)
	if model != "" {
		args = append(args, "--model", model)
	}
	return append(args, native...)
}

func (s *Service) resolveAgentCWD(requested string) (string, error) {
	cwd := requested
	if cwd == "" {
		cwd = s.Project.Root
	} else if !filepath.IsAbs(cwd) {
		var err error
		cwd, err = filepath.Abs(cwd)
		if err != nil {
			return "", Wrap("invalid_cwd", err.Error(), err)
		}
	}
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		return "", NewError("invalid_cwd", fmt.Sprintf("agent cwd %q is not a directory", cwd))
	}
	canonical, err := project.Canonical(cwd)
	if err != nil {
		return "", Wrap("invalid_cwd", err.Error(), err)
	}
	relative, err := filepath.Rel(s.Project.Root, canonical)
	if err != nil || relative == ".." || filepath.IsAbs(relative) ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", NewError("invalid_cwd",
			fmt.Sprintf("agent cwd %q must be inside project %s", canonical, s.Project.Root))
	}
	return canonical, nil
}

func (s *Service) startAgentLocked(
	ctx context.Context,
	client *herdr.Client,
	opts AgentStartOptions,
	cwd string,
	forwardedArgs []string,
) (state.Agent, herdr.AgentInfo, error) {
	var managed state.Agent
	var startedInfo herdr.AgentInfo
	err := s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		snapshot, err := client.Snapshot(ctx)
		if err != nil {
			return err
		}
		managed, err = s.ensureAgentPane(ctx, client, st, snapshot, opts.Name, opts.Kind, cwd)
		if err != nil {
			return err
		}
		if err := ensureAgentPaneAvailable(ctx, client, snapshot, managed, opts.Name); err != nil {
			return err
		}
		startedInfo, err = startAgentInPane(ctx, client, opts, managed.PaneID, forwardedArgs)
		if err != nil {
			return err
		}
		managed.Kind, managed.Model, managed.Placement, managed.CWD = opts.Kind, opts.Model, "tab", cwd
		st.Agents[opts.Name] = managed
		return nil
	})
	return managed, startedInfo, err
}

func (s *Service) spawnAgentInCurrentPane(
	ctx context.Context,
	client *herdr.Client,
	opts AgentStartOptions,
	cwd string,
	forwardedArgs []string,
) (AgentStartResult, error) {
	if opts.Executable == "" {
		return AgentStartResult{}, NewError("harness_not_installed", fmt.Sprintf("harness %q has no executable", opts.Kind))
	}
	var managed state.Agent
	var previousAgents map[string]state.Agent
	var previousLabel *string
	err := s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		snapshot, err := client.Snapshot(ctx)
		if err != nil {
			return err
		}
		pane, ok := panesByID(snapshot)[opts.CurrentPaneID]
		if !ok {
			return NewError("invalid_herdr_pane",
				fmt.Sprintf("HERDR_PANE_ID %q does not belong to the current Fledge session", opts.CurrentPaneID))
		}
		workspaceID := s.WorkspaceID
		if workspaceID == "" {
			workspaceID = st.WorkspaceID
		}
		if workspaceID != "" && pane.WorkspaceID != workspaceID {
			return NewError("invalid_herdr_pane",
				fmt.Sprintf("HERDR_PANE_ID %q is outside the current Fledge workspace", opts.CurrentPaneID))
		}
		if existing, ok := st.Agents[opts.Name]; ok && existing.PaneID != opts.CurrentPaneID {
			if _, live := panesByID(snapshot)[existing.PaneID]; live {
				return NewError("agent_name_conflict",
					fmt.Sprintf("agent name %q is already owned by pane %s", opts.Name, existing.PaneID))
			}
		}
		previousAgents = cloneAgents(st.Agents)
		previousLabel = pane.Label
		for name, existing := range st.Agents {
			if name != opts.Name && existing.PaneID == opts.CurrentPaneID {
				if pane.Agent != nil {
					return NewError("pane_occupied",
						fmt.Sprintf("pane %s is still owned by running agent %q", opts.CurrentPaneID, name))
				}
				delete(st.Agents, name)
			}
		}
		if err := client.Call(ctx, "pane.rename",
			map[string]any{"pane_id": opts.CurrentPaneID, "label": opts.Name}, nil); err != nil {
			return err
		}
		managed = state.Agent{
			Name: opts.Name, Kind: opts.Kind, Model: opts.Model, Placement: "pane",
			CWD: cwd, TabID: pane.TabID, PaneID: pane.PaneID,
		}
		st.Agents[opts.Name] = managed
		return nil
	})
	if err != nil {
		if previousAgents != nil {
			restorePaneLabel(ctx, client, opts.CurrentPaneID, previousLabel)
		}
		return AgentStartResult{}, err
	}
	_, activationID, err := s.prepareMessagingActivation(opts.Name, managed.PaneID)
	if err != nil {
		return AgentStartResult{}, s.rollbackInPaneSpawn(ctx, client, previousAgents, previousLabel, opts.CurrentPaneID, err)
	}
	if activationID != "" {
		if err := s.launchDeliveryHelper(opts.Name, activationID, opts.Timeout); err != nil {
			_ = s.deactivateMessagingAgent(opts.Name, "delivery helper failed to launch")
			return AgentStartResult{}, s.rollbackInPaneSpawn(ctx, client, previousAgents, previousLabel, opts.CurrentPaneID, err)
		}
	}

	execAgent := s.ExecAgent
	if execAgent == nil {
		execAgent = syscall.Exec
	}
	argv := append([]string{opts.Executable}, forwardedArgs...)
	oldCWD, cwdErr := os.Getwd()
	if cwdErr != nil {
		return AgentStartResult{}, s.rollbackInPaneSpawn(ctx, client, previousAgents, previousLabel, opts.CurrentPaneID, cwdErr)
	}
	if err := os.Chdir(cwd); err != nil {
		return AgentStartResult{}, s.rollbackInPaneSpawn(ctx, client, previousAgents, previousLabel, opts.CurrentPaneID, err)
	}
	err = execAgent(opts.Executable, argv, os.Environ())
	_ = os.Chdir(oldCWD)
	if err != nil {
		return AgentStartResult{}, s.rollbackInPaneSpawn(ctx, client, previousAgents, previousLabel, opts.CurrentPaneID, err)
	}
	return AgentStartResult{
		Agent: viewFromInfo(opts.Name, managed, herdr.AgentInfo{AgentStatus: "unknown"}),
		Argv:  forwardedArgs,
	}, nil
}

func (s *Service) launchDeliveryHelper(name, activationID string, timeout time.Duration) error {
	if s.LaunchDeliveryHelper != nil {
		return s.LaunchDeliveryHelper(name, activationID, timeout)
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate fledge executable: %w", err)
	}
	herdrPath := s.Binary.Path
	if herdrPath == "" {
		herdrPath = "herdr"
	}
	cmd := exec.Command(executable,
		"--herdr-bin", herdrPath,
		"agent", "message", "deliver", name, activationID,
		"--timeout", timeout.String(),
	)
	cmd.Dir = s.Project.Root
	cmd.Stdin = nil
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devNull.Close()
	cmd.Stdout, cmd.Stderr = devNull, devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func (s *Service) rollbackInPaneSpawn(
	ctx context.Context,
	client *herdr.Client,
	agents map[string]state.Agent,
	label *string,
	paneID string,
	cause error,
) error {
	rollbackErr := s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		st.Agents = cloneAgents(agents)
		return nil
	})
	restorePaneLabel(ctx, client, paneID, label)
	message := fmt.Sprintf("exec agent harness: %v", cause)
	if rollbackErr != nil {
		message += fmt.Sprintf("; rollback state: %v", rollbackErr)
	}
	return Wrap("agent_exec_failed", message, cause)
}

func restorePaneLabel(ctx context.Context, client *herdr.Client, paneID string, label *string) {
	restoredLabel := ""
	if label != nil {
		restoredLabel = *label
	}
	_ = client.Call(ctx, "pane.rename", map[string]any{"pane_id": paneID, "label": restoredLabel}, nil)
}

func cloneAgents(agents map[string]state.Agent) map[string]state.Agent {
	cloned := make(map[string]state.Agent, len(agents))
	for name, agent := range agents {
		cloned[name] = agent
	}
	return cloned
}

func ensureAgentPaneAvailable(
	ctx context.Context,
	client *herdr.Client,
	snapshot herdr.Snapshot,
	managed state.Agent,
	name string,
) error {
	live := agentsByPane(snapshot)
	if agent, ok := live[managed.PaneID]; ok && agent.Agent != nil {
		if agent.Name != nil && *agent.Name == name {
			return NewError("agent_already_running", fmt.Sprintf("agent %q is already running", name))
		}
		return NewError("pane_occupied", fmt.Sprintf("managed pane %s is occupied by another agent", managed.PaneID))
	}
	var processResult herdr.Result
	if err := client.Call(ctx, "pane.process_info", map[string]any{"pane_id": managed.PaneID}, &processResult); err != nil {
		return err
	}
	if paneHasUnrelatedForegroundProcess(processResult.ProcessInfo) {
		return NewError("pane_occupied", fmt.Sprintf("managed pane %s has a foreground process", managed.PaneID))
	}
	return nil
}

func startAgentInPane(
	ctx context.Context,
	client *herdr.Client,
	opts AgentStartOptions,
	paneID string,
	forwardedArgs []string,
) (herdr.AgentInfo, error) {
	params := map[string]any{
		"name": opts.Name, "kind": opts.Kind, "pane_id": paneID,
		"args": forwardedArgs,
	}
	if timeout := herdr.Milliseconds(opts.Timeout); timeout != nil {
		params["timeout_ms"] = *timeout
	}
	var started herdr.Result
	if err := client.Call(ctx, "agent.start", params, &started); err != nil {
		return herdr.AgentInfo{}, err
	}
	return started.Agent, nil
}

func paneHasUnrelatedForegroundProcess(info herdr.ProcessInfo) bool {
	for _, process := range info.ForegroundProcesses {
		if info.ShellPID == nil || process.PID != *info.ShellPID {
			return true
		}
	}
	return false
}

func (s *Service) ensureAgentPane(ctx context.Context, client *herdr.Client, st *state.Session, snapshot herdr.Snapshot, name, kind, cwd string) (state.Agent, error) {
	if managed, found, err := s.reusableAgentPane(ctx, client, st, snapshot, name, kind, cwd); found || err != nil {
		return managed, err
	}
	return s.createAgentPane(ctx, client, st, snapshot, name, kind, cwd)
}

func (s *Service) reusableAgentPane(
	ctx context.Context,
	client *herdr.Client,
	st *state.Session,
	snapshot herdr.Snapshot,
	name, kind, cwd string,
) (state.Agent, bool, error) {
	expected := agentLabelPrefix + name
	panes := panesByID(snapshot)
	selectedWorkspace := s.WorkspaceID
	if selectedWorkspace == "" {
		selectedWorkspace = st.WorkspaceID
	}
	_, hadState := st.Agents[name]
	if existing, ok := st.Agents[name]; ok {
		if pane, valid := panes[existing.PaneID]; valid &&
			(selectedWorkspace == "" || pane.WorkspaceID == selectedWorkspace) {
			if existing.CWD != "" && existing.CWD != cwd {
				return state.Agent{}, true, NewError("agent_cwd_conflict",
					fmt.Sprintf("agent %q has a retained pane rooted at %s; use that cwd or force-stop it and choose a new name", name, existing.CWD))
			}
			if pane.Label != nil && *pane.Label == expected {
				if err := renameAgentLabels(ctx, client, pane.TabID, pane.PaneID, name); err != nil {
					return state.Agent{}, true, err
				}
			}
			return existing, true, nil
		}
	}
	for _, pane := range snapshot.Panes {
		if pane.Label != nil && (*pane.Label == expected || (hadState && *pane.Label == name)) &&
			(selectedWorkspace == "" || pane.WorkspaceID == selectedWorkspace) {
			managed := state.Agent{Name: name, Kind: kind, CWD: cwd, TabID: pane.TabID, PaneID: pane.PaneID}
			st.Agents[name] = managed
			if err := renameAgentLabels(ctx, client, pane.TabID, pane.PaneID, name); err != nil {
				return state.Agent{}, true, err
			}
			return managed, true, nil
		}
	}
	return state.Agent{}, false, nil
}

func (s *Service) createAgentPane(
	ctx context.Context,
	client *herdr.Client,
	st *state.Session,
	snapshot herdr.Snapshot,
	name, kind, cwd string,
) (state.Agent, error) {
	expected := name
	workspaceID := s.WorkspaceID
	if workspaceID == "" {
		workspaceID = st.WorkspaceID
	}
	if !hasWorkspace(snapshot, workspaceID) {
		workspaceID = ""
		if matched, found, err := matchingWorkspace(snapshot, s.Project.Root); err != nil {
			return state.Agent{}, fmt.Errorf("resolve Herdr workspace for agent %q: %w", name, err)
		} else if found {
			workspaceID = matched.WorkspaceID
		}
	}
	if workspaceID == "" {
		if workspace, found := fallbackWorkspace(snapshot, s.Project.Root, s.Project.Session); found {
			workspaceID = workspace.WorkspaceID
		}
	}
	workspaceID, tabID, paneID, err := s.allocateAgentPane(ctx, client, snapshot, workspaceID, cwd)
	if err != nil {
		return state.Agent{}, err
	}
	st.WorkspaceID = workspaceID
	if err := client.Call(ctx, "tab.rename", map[string]any{"tab_id": tabID, "label": expected}, nil); err != nil {
		return state.Agent{}, err
	}
	if err := client.Call(ctx, "pane.rename", map[string]any{"pane_id": paneID, "label": expected}, nil); err != nil {
		return state.Agent{}, err
	}
	managed := state.Agent{Name: name, Kind: kind, Placement: "tab", CWD: cwd, TabID: tabID, PaneID: paneID}
	st.Agents[name] = managed
	return managed, nil
}

func (s *Service) allocateAgentPane(
	ctx context.Context,
	client *herdr.Client,
	snapshot herdr.Snapshot,
	workspaceID, cwd string,
) (string, string, string, error) {
	if workspaceID == "" {
		var created herdr.Result
		if err := client.Call(ctx, "workspace.create", map[string]any{
			"cwd": cwd, "focus": false, "label": project.WorkspaceLabel(s.Project.Root),
		}, &created); err != nil {
			return "", "", "", err
		}
		return created.Workspace.WorkspaceID, created.Tab.TabID, created.RootPane.PaneID, nil
	}
	var created herdr.Result
	if err := client.Call(ctx, "tab.create", map[string]any{
		"workspace_id": workspaceID, "cwd": cwd, "focus": false,
	}, &created); err != nil {
		return "", "", "", err
	}
	return workspaceID, created.Tab.TabID, created.RootPane.PaneID, nil
}

func renameAgentLabels(ctx context.Context, client *herdr.Client, tabID, paneID, name string) error {
	if err := client.Call(ctx, "tab.rename", map[string]any{"tab_id": tabID, "label": name}, nil); err != nil {
		return err
	}
	return client.Call(ctx, "pane.rename", map[string]any{"pane_id": paneID, "label": name}, nil)
}
