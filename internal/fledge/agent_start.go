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
	"github.com/Harrison-Blair/fledge/internal/processenv"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

type AgentStartOptions struct {
	Name              string
	Kind              string
	Model             string
	Profile           string `json:"profile,omitempty"`
	CWD               string
	Timeout           time.Duration
	Args              []string
	NewTab            bool
	CurrentPaneID     string
	Executable        string
	RememberSelection bool
}

type AgentStartResult struct {
	Agent AgentView `json:"agent"`
	Argv  []string  `json:"native_args"`
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
	if err := s.messages().activateAgent(ctx, client, opts.Name, managed.PaneID); err != nil {
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
		managed, err = s.ensureAgentPane(ctx, client, st, snapshot, opts.Name, opts.Kind, opts.Profile, cwd)
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
		managed.Kind, managed.Model, managed.Profile, managed.Placement, managed.CWD =
			opts.Kind, opts.Model, opts.Profile, "tab", cwd
		st.Agents[opts.Name] = managed
		rememberSpawnSelection(st, opts)
		return nil
	})
	return managed, startedInfo, err
}

// paneClaim records the pane an in-pane spawn took over, together with the
// state needed to undo the takeover.
type paneClaim struct {
	managed                state.Agent
	previousAgents         map[string]state.Agent
	previousSpawnSelection *state.SpawnSelection
	previousLabel          *string
}

func (s *Service) spawnAgentInCurrentPane(
	ctx context.Context,
	client *herdr.Client,
	opts AgentStartOptions,
	cwd string,
	forwardedArgs []string,
) (result AgentStartResult, err error) {
	if opts.Executable == "" {
		return AgentStartResult{}, NewError("harness_not_installed", fmt.Sprintf("harness %q has no executable", opts.Kind))
	}
	claim, err := s.claimCurrentPane(ctx, client, opts, cwd)
	if err != nil {
		return AgentStartResult{}, err
	}
	// Every failure past the claim leaves the pane renamed and the agent
	// recorded, so each one unwinds through the same rollback.
	defer func() {
		if err != nil {
			err = s.rollbackInPaneSpawn(ctx, client, claim, opts.CurrentPaneID, err)
		}
	}()
	if err = s.activateSpawnMessaging(opts, claim.managed.PaneID); err != nil {
		return AgentStartResult{}, err
	}
	if err = s.execIntoHarness(opts.Executable, forwardedArgs, cwd); err != nil {
		return AgentStartResult{}, err
	}
	return AgentStartResult{
		Agent: viewFromInfo(opts.Name, claim.managed, herdr.AgentInfo{AgentStatus: StateUnknown}),
		Argv:  forwardedArgs,
	}, nil
}

// claimCurrentPane records opts.Name as the owner of the caller's pane and
// renames the pane to match. A failure after the pane's previous owners were
// captured restores its label before reporting.
func (s *Service) claimCurrentPane(
	ctx context.Context,
	client *herdr.Client,
	opts AgentStartOptions,
	cwd string,
) (paneClaim, error) {
	var claim paneClaim
	err := s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		snapshot, err := client.Snapshot(ctx)
		if err != nil {
			return err
		}
		pane, err := s.spawnablePane(snapshot, st, opts.CurrentPaneID)
		if err != nil {
			return err
		}
		if err := agentNameAvailable(st, snapshot, opts.Name, opts.CurrentPaneID); err != nil {
			return err
		}
		claim.previousAgents = cloneAgents(st.Agents)
		claim.previousSpawnSelection = cloneSpawnSelection(st.LastSpawnSelection)
		claim.previousLabel = pane.Label
		if err := evictPaneOwners(st, pane, opts.Name); err != nil {
			return err
		}
		if err := client.Call(ctx, "pane.rename",
			map[string]any{"pane_id": opts.CurrentPaneID, "label": opts.Name}, nil); err != nil {
			return err
		}
		claim.managed = state.Agent{
			Name: opts.Name, Kind: opts.Kind, Model: opts.Model, Profile: opts.Profile, Placement: "pane",
			CWD: cwd, TabID: pane.TabID, PaneID: pane.PaneID,
		}
		st.Agents[opts.Name] = claim.managed
		rememberSpawnSelection(st, opts)
		return nil
	})
	if err != nil {
		if claim.previousAgents != nil {
			restorePaneLabel(ctx, client, opts.CurrentPaneID, claim.previousLabel)
		}
		return paneClaim{}, err
	}
	return claim, nil
}

// spawnablePane returns paneID when it belongs to this session's workspace.
func (s *Service) spawnablePane(snapshot herdr.Snapshot, st *state.Session, paneID string) (herdr.PaneInfo, error) {
	pane, ok := panesByID(snapshot)[paneID]
	if !ok {
		return herdr.PaneInfo{}, NewError("invalid_herdr_pane",
			fmt.Sprintf("HERDR_PANE_ID %q does not belong to the current Fledge session", paneID))
	}
	if workspaceID := s.selectedWorkspaceID(st); workspaceID != "" && pane.WorkspaceID != workspaceID {
		return herdr.PaneInfo{}, NewError("invalid_herdr_pane",
			fmt.Sprintf("HERDR_PANE_ID %q is outside the current Fledge workspace", paneID))
	}
	return pane, nil
}

// agentNameAvailable rejects a takeover that would steal a name whose recorded
// pane is still live elsewhere.
func agentNameAvailable(st *state.Session, snapshot herdr.Snapshot, name, paneID string) error {
	existing, ok := st.Agents[name]
	if !ok || existing.PaneID == paneID {
		return nil
	}
	if _, live := panesByID(snapshot)[existing.PaneID]; live {
		return NewError("agent_name_conflict",
			fmt.Sprintf("agent name %q is already owned by pane %s", name, existing.PaneID))
	}
	return nil
}

// evictPaneOwners drops the state entries of other agents recorded against
// pane, refusing while one of them still has a harness running there.
func evictPaneOwners(st *state.Session, pane herdr.PaneInfo, name string) error {
	for owner, existing := range st.Agents {
		if owner == name || existing.PaneID != pane.PaneID {
			continue
		}
		if pane.Agent != nil {
			return NewError("pane_occupied",
				fmt.Sprintf("pane %s is still owned by running agent %q", pane.PaneID, owner))
		}
		delete(st.Agents, owner)
	}
	return nil
}

// activateSpawnMessaging opens the agent's mailbox and starts the background
// deliverer that feeds it while the harness owns the pane.
func (s *Service) activateSpawnMessaging(opts AgentStartOptions, paneID string) error {
	target, err := s.messages().prepareActivation(opts.Name, paneID)
	if err != nil {
		return err
	}
	if target.activationID == "" {
		return nil
	}
	if err := s.launchDeliveryHelper(opts.Name, target.activationID, opts.Timeout); err != nil {
		_ = s.messages().deactivateAgent(opts.Name, "delivery helper failed to launch")
		return err
	}
	return nil
}

// execIntoHarness replaces this process with the harness, rooted at cwd. It
// returns only when the exec fails.
func (s *Service) execIntoHarness(executable string, forwardedArgs []string, cwd string) error {
	execAgent := s.ExecAgent
	if execAgent == nil {
		execAgent = syscall.Exec
	}
	oldCWD, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(cwd); err != nil {
		return err
	}
	err = execAgent(executable, append([]string{executable}, forwardedArgs...), processenv.WithoutNoColor(os.Environ()))
	_ = os.Chdir(oldCWD)
	return err
}

func (s *Service) launchDeliveryHelper(name, activationID string, timeout time.Duration) error {
	if s.LaunchDeliveryHelper != nil {
		return s.LaunchDeliveryHelper(name, activationID, timeout)
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate fledge executable: %w", err)
	}
	cmd := exec.Command(executable,
		"--herdr-bin", s.Binary.ResolvedPath(),
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
	claim paneClaim,
	paneID string,
	cause error,
) error {
	rollbackErr := s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		st.Agents = cloneAgents(claim.previousAgents)
		st.LastSpawnSelection = cloneSpawnSelection(claim.previousSpawnSelection)
		return nil
	})
	restorePaneLabel(ctx, client, paneID, claim.previousLabel)
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

func cloneSpawnSelection(selection *state.SpawnSelection) *state.SpawnSelection {
	if selection == nil {
		return nil
	}
	cloned := *selection
	return &cloned
}

func rememberSpawnSelection(st *state.Session, opts AgentStartOptions) {
	if !opts.RememberSelection {
		return
	}
	st.LastSpawnSelection = &state.SpawnSelection{Harness: opts.Kind, Model: opts.Model}
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

func (s *Service) ensureAgentPane(ctx context.Context, client *herdr.Client, st *state.Session, snapshot herdr.Snapshot, name, kind, profile, cwd string) (state.Agent, error) {
	if managed, found, err := s.reusableAgentPane(ctx, client, st, snapshot, name, kind, profile, cwd); found || err != nil {
		return managed, err
	}
	return s.createAgentPane(ctx, client, st, snapshot, name, kind, profile, cwd)
}

func (s *Service) reusableAgentPane(
	ctx context.Context,
	client *herdr.Client,
	st *state.Session,
	snapshot herdr.Snapshot,
	name, kind, profile, cwd string,
) (state.Agent, bool, error) {
	expected := agentLabelPrefix + name
	panes := panesByID(snapshot)
	selectedWorkspace := s.selectedWorkspaceID(st)
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
			existing.Profile = profile
			return existing, true, nil
		}
	}
	for _, pane := range snapshot.Panes {
		if pane.Label != nil && (*pane.Label == expected || (hadState && *pane.Label == name)) &&
			(selectedWorkspace == "" || pane.WorkspaceID == selectedWorkspace) {
			managed := state.Agent{Name: name, Kind: kind, Profile: profile, CWD: cwd, TabID: pane.TabID, PaneID: pane.PaneID}
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
	name, kind, profile, cwd string,
) (state.Agent, error) {
	expected := name
	// Unlike the orchestrator path, a stale non-empty s.WorkspaceID skips
	// st.WorkspaceID and goes straight to the project-wide search.
	workspaceID, err := resolveWorkspaceID(snapshot, s.Project, s.selectedWorkspaceID(st), fmt.Sprintf("agent %q", name))
	if err != nil {
		return state.Agent{}, err
	}
	workspaceID, tabID, paneID, err := s.allocateAgentPane(ctx, client, workspaceID, cwd)
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
	managed := state.Agent{Name: name, Kind: kind, Profile: profile, Placement: "tab", CWD: cwd, TabID: tabID, PaneID: paneID}
	st.Agents[name] = managed
	return managed, nil
}

func (s *Service) allocateAgentPane(
	ctx context.Context,
	client *herdr.Client,
	workspaceID, cwd string,
) (string, string, string, error) {
	if workspaceID == "" {
		created, err := createProjectWorkspace(ctx, client, s.Project.Root, cwd)
		if err != nil {
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
