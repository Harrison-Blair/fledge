package fledge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/processenv"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

type AgentStartOptions struct {
	Name    string
	Kind    string
	Model   string
	Profile string `json:"profile,omitempty"`
	// ProfileLocksHarness and ProfileLocksModel report that the spawned
	// profile pins the selection itself. The dedicated-tab bootstrap command
	// omits a locked flag because `fledge agent spawn <profile>` rejects
	// overrides of profile-owned fields.
	ProfileLocksHarness bool
	ProfileLocksModel   bool
	CWD                 string
	Timeout             time.Duration
	Args                []string
	NewTab              bool
	CurrentPaneID       string
	Executable          string
	RememberSelection   bool
	// LaunchID is hidden bootstrap plumbing. A non-empty value may only claim
	// the matching dedicated-pane reservation persisted by the parent spawn.
	LaunchID string
}

type AgentStartResult struct {
	Agent AgentView `json:"agent"`
	Argv  []string  `json:"native_args"`
}

func (s *Service) SpawnAgent(ctx context.Context, opts AgentStartOptions) (AgentStartResult, error) {
	if opts.CurrentPaneID == "" || opts.NewTab {
		// Preflight user-controlled argv before generic semantic validation so
		// terminal controls consistently report invalid_terminal_input. The
		// command is rendered again with the resolved executable and canonical
		// cwd immediately before any tab or state mutation.
		if _, err := spawnCommand("fledge", opts, opts.CWD); err != nil {
			return AgentStartResult{}, err
		}
	}
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
	managed, started, err := s.startAgentInDedicatedTab(ctx, client, opts, cwd)
	if err != nil {
		return AgentStartResult{}, err
	}
	// Messaging activation and the delivery helper are owned by the in-pane
	// child the injected command starts: it re-enters SpawnAgent through
	// spawnAgentInCurrentPane once it runs inside the prepared pane.
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

const (
	launchRetiring = "retiring"
	launchReserved = "reserved"
	launchClaimed  = "claimed"
	launchExecing  = "execing"
	launchFailed   = "failed"
)

// startAgentInDedicatedTab owns the dedicated launch transaction. Herdr's
// agent caches are deliberately absent from the handshake: state identifies
// the reservation and pane.process_info proves that the claimed PID execed
// into the recorded harness executable.
func (s *Service) startAgentInDedicatedTab(
	ctx context.Context,
	client *herdr.Client,
	opts AgentStartOptions,
	cwd string,
) (state.Agent, herdr.AgentInfo, error) {
	executable, err := s.fledgeExecutable()
	if err != nil {
		return state.Agent{}, herdr.AgentInfo{}, err
	}
	launchID, err := messaging.NewID("launch_")
	if err != nil {
		return state.Agent{}, herdr.AgentInfo{}, err
	}
	opts.LaunchID = launchID
	command, err := spawnCommand(executable, opts, cwd)
	if err != nil {
		return state.Agent{}, herdr.AgentInfo{}, err
	}
	begin := time.Now()
	managed, undo, err := s.reserveFreshDedicatedPane(ctx, client, opts, cwd, launchID)
	if err != nil {
		return state.Agent{}, herdr.AgentInfo{}, err
	}
	fail := func(cause error) (state.Agent, herdr.AgentInfo, error) {
		info, cleanupErr := s.cleanupOwnedLaunch(ctx, client, opts.Name, launchID, undo, cause)
		if cleanupErr == nil {
			clearLaunch(&managed)
			return managed, info, nil
		}
		return state.Agent{}, herdr.AgentInfo{}, cleanupErr
	}
	if err := awaitPaneShellOnly(ctx, client, managed.PaneID, opts.Timeout/3); err != nil {
		return fail(err)
	}
	if err := client.Call(ctx, "pane.send_input", map[string]any{
		"pane_id": managed.PaneID, "text": command, "keys": []string{"enter"},
	}, nil); err != nil {
		var apiErr *herdr.APIError
		if !errors.As(err, &apiErr) {
			_ = s.markReservedLaunchFailed(opts.Name, launchID)
			return state.Agent{}, herdr.AgentInfo{}, Wrap("agent_launch_unconfirmed",
				fmt.Sprintf("bootstrap delivery to managed pane %s was unverifiable; pane and launch ledger were preserved", managed.PaneID), err)
		}
		return fail(err)
	}
	started, err := s.awaitOwnedLaunch(ctx, client, opts.Name, launchID, opts.Timeout-time.Since(begin))
	if err != nil {
		return fail(err)
	}
	clearLaunch(&managed)
	return managed, started, nil
}

type dedicatedSpawnUndo struct {
	previousSelection   *state.SpawnSelection
	selectionGeneration uint64
}

// reserveFreshDedicatedPane retires only the pane named by Fledge state, then
// allocates and persists a fresh reservation before any bootstrap injection.
func (s *Service) reserveFreshDedicatedPane(
	ctx context.Context,
	client *herdr.Client,
	opts AgentStartOptions,
	cwd, launchID string,
) (state.Agent, dedicatedSpawnUndo, error) {
	for {
		var managed state.Agent
		var retiring *state.Agent
		var undo dedicatedSpawnUndo
		var already bool
		var createdFresh bool
		err := s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
			snapshot, err := client.Snapshot(ctx)
			if err != nil {
				return err
			}
			if existing, ok := st.Agents[opts.Name]; ok {
				if _, present := panesByID(snapshot)[existing.PaneID]; !present {
					existing.LaunchID, existing.LaunchPhase = launchID, launchRetiring
					st.Agents[opts.Name] = existing
					copy := existing
					retiring = &copy
					return nil
				} else {
					info, inspectErr := paneProcessInfo(ctx, client, existing.PaneID)
					if inspectErr != nil {
						return Wrap("agent_launch_unconfirmed", "could not inspect the state-owned pane", inspectErr)
					}
					if existing.LaunchID != "" {
						if launchHarnessProcess(existing, info) != nil {
							clearLaunch(&existing)
							st.Agents[opts.Name] = existing
							already = true
							return nil
						}
						if existing.LaunchPhase != launchFailed && existing.LaunchPhase != launchRetiring {
							return NewError("agent_spawn_in_progress", fmt.Sprintf("agent %q already has a launch in progress", opts.Name))
						}
						copy := existing
						retiring = &copy
						return nil
					}
					if paneHasNonShellForegroundProcess(info) {
						if processLooksLikeHarness(info, existing.Kind) {
							return NewError("agent_already_running", fmt.Sprintf("agent %q is already running", opts.Name))
						}
						return NewError("pane_occupied", fmt.Sprintf("managed pane %s has a foreground process", existing.PaneID))
					}
					existing.LaunchID, existing.LaunchPhase = launchID, launchRetiring
					st.Agents[opts.Name] = existing
					copy := existing
					retiring = &copy
					return nil
				}
			}
			undo.previousSelection = cloneSpawnSelection(st.LastSpawnSelection)
			managed, createdFresh, err = s.createAgentPane(ctx, client, st, snapshot, opts.Name, opts.Kind, opts.Profile, cwd)
			if err != nil {
				return err
			}
			managed.Model, managed.LaunchID, managed.LaunchPhase = opts.Model, launchID, launchReserved
			st.Agents[opts.Name] = managed
			undo.selectionGeneration, err = rememberSpawnSelection(st, opts)
			return err
		})
		if err != nil {
			if createdFresh {
				return state.Agent{}, dedicatedSpawnUndo{}, rollbackCreatedAgentTab(ctx, client, opts.Name, managed, true, err)
			}
			return state.Agent{}, dedicatedSpawnUndo{}, err
		}
		if already {
			return state.Agent{}, dedicatedSpawnUndo{}, NewError("agent_already_running", fmt.Sprintf("agent %q is already running", opts.Name))
		}
		if retiring == nil {
			return managed, undo, nil
		}
		if err := s.retireOwnedPane(ctx, client, opts.Name, *retiring); err != nil {
			return state.Agent{}, dedicatedSpawnUndo{}, err
		}
	}
}

func (s *Service) retireOwnedPane(ctx context.Context, client *herdr.Client, name string, managed state.Agent) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	snapshot, err := client.Snapshot(cleanupCtx)
	if err != nil {
		return err
	}
	if _, present := panesByID(snapshot)[managed.PaneID]; !present {
		if err := s.messages().deactivateActivation(name, managed.ActivationID, managed.PaneID, "missing agent pane retired for a fresh launch"); err != nil {
			return err
		}
		return s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
			if current, ok := st.Agents[name]; ok && current.PaneID == managed.PaneID && current.LaunchID == managed.LaunchID {
				delete(st.Agents, name)
			}
			return nil
		})
	}
	info, err := paneProcessInfo(cleanupCtx, client, managed.PaneID)
	if err != nil {
		return Wrap("agent_launch_unconfirmed", "could not inspect the state-owned pane before retirement", err)
	}
	if paneHasUnexpectedLaunchProcess(managed, info) {
		return NewError("pane_occupied", fmt.Sprintf("managed pane %s became occupied before retirement", managed.PaneID))
	}
	if err := s.messages().deactivateActivation(name, managed.ActivationID, managed.PaneID, "agent pane retired for a fresh launch"); err != nil {
		return err
	}
	info, err = paneProcessInfo(cleanupCtx, client, managed.PaneID)
	if err != nil {
		return Wrap("agent_launch_unconfirmed", "could not recheck the state-owned pane before retirement", err)
	}
	if paneHasUnexpectedLaunchProcess(managed, info) {
		return NewError("pane_occupied", fmt.Sprintf("managed pane %s became occupied before retirement", managed.PaneID))
	}
	if err := s.closeOwnedPane(cleanupCtx, client, name, managed); err != nil {
		return err
	}
	return s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		if current, ok := st.Agents[name]; ok && current.PaneID == managed.PaneID && current.LaunchID == managed.LaunchID {
			delete(st.Agents, name)
		}
		return nil
	})
}

func (s *Service) closeOwnedPane(ctx context.Context, client *herdr.Client, name string, managed state.Agent) error {
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		return err
	}
	if _, ok := panesByID(snapshot)[managed.PaneID]; !ok {
		return nil
	}
	closeTab := managed.Placement == "tab" && managed.TabID != ""
	st, readErr := s.Store.Read(s.Project.Session, s.Project.Root)
	if readErr != nil {
		return readErr
	}
	if st.OrchestratorTabID == managed.TabID || st.OrchestratorPaneID == managed.PaneID {
		return NewError("pane_occupied", fmt.Sprintf("managed pane %s is the saved orchestrator pane and was preserved", managed.PaneID))
	}
	panes := 0
	for _, pane := range snapshot.Panes {
		if pane.TabID == managed.TabID {
			panes++
			if pane.PaneID != managed.PaneID {
				closeTab = false
			}
		}
	}
	if panes != 1 {
		closeTab = false
	}
	if closeTab {
		if err := client.Call(ctx, "tab.close", map[string]any{"tab_id": managed.TabID}, nil); err != nil {
			return agentTabCloseError(name, managed, false, err)
		}
		return nil
	}
	return client.Call(ctx, "pane.close", map[string]any{"pane_id": managed.PaneID}, nil)
}

// fledgeExecutable names the running fledge binary the dedicated-tab
// bootstrap re-invokes inside the prepared pane.
func (s *Service) fledgeExecutable() (string, error) {
	if s.FledgeExecutable != "" {
		return s.FledgeExecutable, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate fledge executable: %w", err)
	}
	return executable, nil
}

// spawnCommand renders the shell command a dedicated-tab spawn injects: the
// current fledge binary re-invoked as an in-pane `agent spawn`, carrying every
// selection plus --no-pickers so the child never blocks on a picker — a
// deliberately unset model launches the harness default instead of prompting
// into a pane nobody is watching. Profile-owned fields stay with the profile:
// the child re-derives cwd and native args from it, and locked harness or
// model flags are omitted because the child would reject them as overrides.
//
// The child re-receives the parent's full --timeout for its own claim and
// exec, so a pathological spawn can take ~2× the caller's timeout end to end;
// that bound is accepted rather than splitting one budget across two
// processes that each may legitimately need most of it.
func spawnCommand(executable string, opts AgentStartOptions, cwd string) (string, error) {
	args := []string{"agent", "spawn"}
	if opts.Profile != "" {
		args = append(args, opts.Profile)
	}
	args = append(args, "--name", opts.Name)
	if opts.Profile == "" || !opts.ProfileLocksHarness {
		args = append(args, "--harness", opts.Kind)
	}
	if opts.Model != "" && (opts.Profile == "" || !opts.ProfileLocksModel) {
		args = append(args, "--model", opts.Model)
	}
	if opts.Profile == "" {
		args = append(args, "--cwd", cwd)
	}
	args = append(args, "--timeout", opts.Timeout.String(), "--no-pickers")
	if opts.LaunchID != "" {
		args = append(args, "--launch-id", opts.LaunchID)
	}
	if opts.Profile == "" && len(opts.Args) > 0 {
		args = append(args, "--")
		args = append(args, opts.Args...)
	}
	return renderPTYCommand(append([]string{executable}, args...))
}

func (s *Service) awaitOwnedLaunch(
	ctx context.Context,
	client *herdr.Client,
	name, launchID string,
	budget time.Duration,
) (herdr.AgentInfo, error) {
	deadline := time.Now().Add(budget)
	for {
		if ctx.Err() != nil {
			return herdr.AgentInfo{}, NewError("agent_launch_unconfirmed",
				fmt.Sprintf("wait for agent %q launch was cancelled: %v", name, ctx.Err()))
		}
		st, err := s.Store.Read(s.Project.Session, s.Project.Root)
		if err != nil {
			return herdr.AgentInfo{}, Wrap("agent_launch_unconfirmed",
				fmt.Sprintf("could not read launch ledger for agent %q", name), err)
		}
		managed, ok := st.Agents[name]
		if !ok || managed.LaunchID != launchID {
			return herdr.AgentInfo{}, NewError("agent_launch_unconfirmed", fmt.Sprintf("agent %q no longer owns its launch reservation", name))
		}
		if managed.LaunchPhase == launchFailed {
			return herdr.AgentInfo{}, NewError("agent_launch_unconfirmed", fmt.Sprintf("agent %q bootstrap reported launch failure", name))
		}
		if managed.LaunchPID != 0 && managed.LaunchExecutable != "" {
			info, inspectErr := paneProcessInfo(ctx, client, managed.PaneID)
			if inspectErr != nil {
				return herdr.AgentInfo{}, Wrap("agent_launch_unconfirmed",
					fmt.Sprintf("could not inspect managed pane %s", managed.PaneID), inspectErr)
			}
			if launchHarnessProcess(managed, info) != nil {
				err := s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
					current, present := st.Agents[name]
					if !present || current.LaunchID != launchID || current.PaneID != managed.PaneID {
						return NewError("agent_launch_unconfirmed", fmt.Sprintf("agent %q launch ownership changed during confirmation", name))
					}
					clearLaunch(&current)
					st.Agents[name] = current
					return nil
				})
				if err != nil {
					return herdr.AgentInfo{}, err
				}
				return herdr.AgentInfo{AgentStatus: StateUnknown, PaneID: managed.PaneID}, nil
			}
		}
		if time.Now().After(deadline) {
			return herdr.AgentInfo{}, NewError("agent_launch_unconfirmed",
				fmt.Sprintf("managed pane %s did not exec the recorded harness after %s", managed.PaneID, budget))
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		timer.Stop()
	}
}

// cleanupOwnedLaunch destroys a failed bootstrap only when process inspection
// proves that no unexpected process owns the pane. A late exact exec is
// adopted as success. Inspection failure preserves both pane and ledger.
func (s *Service) cleanupOwnedLaunch(
	ctx context.Context,
	client *herdr.Client,
	name, launchID string,
	undo dedicatedSpawnUndo,
	cause error,
) (herdr.AgentInfo, error) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	st, err := s.Store.Read(s.Project.Session, s.Project.Root)
	if err != nil {
		return herdr.AgentInfo{}, cause
	}
	managed, ok := st.Agents[name]
	if !ok || managed.LaunchID != launchID {
		return herdr.AgentInfo{}, cause
	}
	info, inspectErr := paneProcessInfo(cleanupCtx, client, managed.PaneID)
	if inspectErr != nil {
		return herdr.AgentInfo{}, Wrap("agent_launch_unconfirmed",
			fmt.Sprintf("%v; process inspection failed, so managed pane %s was preserved", cause, managed.PaneID), inspectErr)
	}
	if launchHarnessProcess(managed, info) != nil {
		if err := s.finalizeOwnedLaunch(name, launchID); err != nil {
			return herdr.AgentInfo{}, err
		}
		return herdr.AgentInfo{AgentStatus: StateUnknown, PaneID: managed.PaneID}, nil
	}
	for _, process := range nonShellProcesses(info) {
		if managed.LaunchPID == 0 || process.PID != managed.LaunchPID || !processLooksLikeFledge(process) {
			return herdr.AgentInfo{}, NewError("agent_launch_unconfirmed",
				fmt.Sprintf("%v; unexpected process %d owns managed pane %s, which was preserved", cause, process.PID, managed.PaneID))
		}
	}
	_ = s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		current, present := st.Agents[name]
		if present && current.LaunchID == launchID {
			current.LaunchPhase = launchFailed
			st.Agents[name] = current
		}
		return nil
	})
	info, inspectErr = paneProcessInfo(cleanupCtx, client, managed.PaneID)
	if inspectErr != nil || paneHasUnexpectedLaunchProcess(managed, info) {
		return herdr.AgentInfo{}, NewError("agent_launch_unconfirmed",
			fmt.Sprintf("%v; managed pane %s changed during cleanup and was preserved", cause, managed.PaneID))
	}
	if err := s.closeOwnedPane(cleanupCtx, client, name, managed); err != nil {
		return herdr.AgentInfo{}, err
	}
	if err := s.messages().deactivateActivation(name, managed.ActivationID, managed.PaneID, "agent launch failed"); err != nil {
		// The failed record intentionally remains, even though its pane is now
		// gone, so a later spawn can retry the exact activation teardown.
		return herdr.AgentInfo{}, err
	}
	if err := s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		current, present := st.Agents[name]
		if present && current.LaunchID == launchID {
			delete(st.Agents, name)
		}
		return restoreSpawnSelectionIfOwned(st, undo.selectionGeneration, undo.previousSelection)
	}); err != nil {
		return herdr.AgentInfo{}, err
	}
	return herdr.AgentInfo{}, cause
}

func (s *Service) finalizeOwnedLaunch(name, launchID string) error {
	return s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		managed, ok := st.Agents[name]
		if ok && managed.LaunchID == launchID {
			clearLaunch(&managed)
			st.Agents[name] = managed
		}
		return nil
	})
}

func paneProcessInfo(ctx context.Context, client *herdr.Client, paneID string) (herdr.ProcessInfo, error) {
	var result herdr.Result
	err := client.Call(ctx, "pane.process_info", map[string]any{"pane_id": paneID}, &result)
	return result.ProcessInfo, err
}

func nonShellProcesses(info herdr.ProcessInfo) []herdr.Process {
	processes := make([]herdr.Process, 0, len(info.ForegroundProcesses))
	for _, process := range info.ForegroundProcesses {
		if info.ShellPID == nil || process.PID != *info.ShellPID {
			processes = append(processes, process)
		}
	}
	return processes
}

func launchHarnessProcess(managed state.Agent, info herdr.ProcessInfo) *herdr.Process {
	if managed.LaunchPID == 0 || managed.LaunchExecutable == "" {
		return nil
	}
	for i := range info.ForegroundProcesses {
		process := &info.ForegroundProcesses[i]
		if process.PID != managed.LaunchPID {
			continue
		}
		if process.Argv0 != nil && executableMatches(*process.Argv0, managed.LaunchExecutable) ||
			len(process.Argv) > 0 && executableMatches(process.Argv[0], managed.LaunchExecutable) {
			return process
		}
		// A shebang exec preserves the PID but exposes the interpreter as
		// argv[0]; the exact launched script remains a later argv element.
		for _, argument := range process.Argv[1:] {
			if executableMatches(argument, managed.LaunchExecutable) {
				return process
			}
		}
	}
	return nil
}

func executableMatches(got, want string) bool {
	if got == want || filepath.Clean(got) == filepath.Clean(want) {
		return true
	}
	gotResolved, gotErr := filepath.EvalSymlinks(got)
	wantResolved, wantErr := filepath.EvalSymlinks(want)
	return gotErr == nil && wantErr == nil && gotResolved == wantResolved
}

func processLooksLikeFledge(process herdr.Process) bool {
	values := append([]string{process.Name}, process.Argv...)
	if process.Argv0 != nil {
		values = append(values, *process.Argv0)
	}
	for _, value := range values {
		if filepath.Base(value) == "fledge" {
			return true
		}
	}
	return false
}

func paneHasUnexpectedLaunchProcess(managed state.Agent, info herdr.ProcessInfo) bool {
	for _, process := range nonShellProcesses(info) {
		if managed.LaunchPID == 0 || process.PID != managed.LaunchPID || !processLooksLikeFledge(process) {
			return true
		}
	}
	return false
}

func processLooksLikeHarness(info herdr.ProcessInfo, kind string) bool {
	for _, process := range nonShellProcesses(info) {
		if process.Name == kind || filepath.Base(process.Name) == kind {
			return true
		}
		if len(process.Argv) > 0 && filepath.Base(process.Argv[0]) == kind {
			return true
		}
	}
	return false
}

func clearLaunch(managed *state.Agent) {
	managed.LaunchID = ""
	managed.LaunchPhase = ""
	managed.LaunchPID = 0
	managed.LaunchExecutable = ""
}

// paneClaim records the pane an in-pane spawn took over, together with the
// state needed to undo the takeover.
type paneClaim struct {
	managed                state.Agent
	previousAgents         map[string]state.Agent
	previousSpawnSelection *state.SpawnSelection
	selectionGeneration    uint64
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
	activationID, activationErr := s.activateSpawnMessaging(opts, claim.managed.PaneID)
	if activationErr != nil {
		err = activationErr
		if opts.LaunchID != "" {
			_ = s.markOwnedLaunchFailed(opts.Name, opts.LaunchID)
			return AgentStartResult{}, err
		}
		return AgentStartResult{}, s.rollbackInPaneSpawn(ctx, client, claim, opts.CurrentPaneID, err)
	}
	if opts.LaunchID != "" {
		if err = s.markOwnedLaunchExecing(opts.Name, opts.LaunchID, activationID); err != nil {
			_ = s.messages().deactivateActivation(opts.Name, activationID, claim.managed.PaneID, "agent launch failed before exec")
			_ = s.markOwnedLaunchFailed(opts.Name, opts.LaunchID)
			return AgentStartResult{}, err
		}
	}
	if err = s.execIntoHarness(opts.Executable, forwardedArgs, cwd); err != nil {
		if opts.LaunchID != "" {
			// Deactivation must precede the failed transition so a parent or later
			// spawn never deletes the record while its activation is still live.
			if deactivateErr := s.messages().deactivateActivation(
				opts.Name, activationID, claim.managed.PaneID, "agent harness exec failed",
			); deactivateErr != nil {
				_ = s.markOwnedLaunchFailed(opts.Name, opts.LaunchID)
				return AgentStartResult{}, Wrap("agent_exec_failed", fmt.Sprintf("exec agent harness: %v; deactivate activation: %v", err, deactivateErr), err)
			}
			_ = s.markOwnedLaunchFailed(opts.Name, opts.LaunchID)
			return AgentStartResult{}, Wrap("agent_exec_failed", fmt.Sprintf("exec agent harness: %v", err), err)
		}
		return AgentStartResult{}, s.rollbackInPaneSpawn(ctx, client, claim, opts.CurrentPaneID, err)
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
		if opts.LaunchID != "" {
			existing, ok := st.Agents[opts.Name]
			if !ok || existing.Name != opts.Name || existing.PaneID != opts.CurrentPaneID || existing.LaunchID != opts.LaunchID || existing.LaunchPhase != launchReserved {
				return NewError("agent_launch_unconfirmed", fmt.Sprintf("agent %q does not own the requested pane reservation", opts.Name))
			}
			resolved, resolveErr := resolveHarnessExecutable(opts.Executable)
			if resolveErr != nil {
				return resolveErr
			}
			existing.Kind, existing.Model, existing.Profile, existing.CWD = opts.Kind, opts.Model, opts.Profile, cwd
			existing.LaunchPhase, existing.LaunchPID, existing.LaunchExecutable = launchClaimed, os.Getpid(), resolved
			st.Agents[opts.Name] = existing
			claim.managed = existing
			return nil
		}
		for owner, existing := range st.Agents {
			if existing.PaneID == opts.CurrentPaneID && existing.LaunchID != "" {
				return NewError("agent_spawn_in_progress", fmt.Sprintf("pane %s is reserved for agent %q", opts.CurrentPaneID, owner))
			}
		}
		if err := agentNameAvailable(st, snapshot, opts.Name, opts.CurrentPaneID); err != nil {
			return err
		}
		claim.previousAgents = cloneAgents(st.Agents)
		claim.previousSpawnSelection = cloneSpawnSelection(st.LastSpawnSelection)
		claim.selectionGeneration, err = rememberSpawnSelection(st, opts)
		if err != nil {
			return err
		}
		claim.previousLabel = pane.Label
		if err := evictPaneOwners(st, pane, opts.Name); err != nil {
			return err
		}
		if err := client.Call(ctx, "pane.rename",
			map[string]any{"pane_id": opts.CurrentPaneID, "label": opts.Name}, nil); err != nil {
			return err
		}
		placement := "pane"
		// A dedicated-tab spawn pre-labels the pane and records the tab
		// placement before its in-pane child claims it; the claim keeps the
		// dedicated-tab semantics so `fledge agent stop` still closes the tab.
		if existing, ok := st.Agents[opts.Name]; ok &&
			existing.PaneID == pane.PaneID && existing.Placement == "tab" {
			placement = existing.Placement
		}
		claim.managed = state.Agent{
			Name: opts.Name, Kind: opts.Kind, Model: opts.Model, Profile: opts.Profile, Placement: placement,
			CWD: cwd, TabID: pane.TabID, PaneID: pane.PaneID,
		}
		st.Agents[opts.Name] = claim.managed
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
func (s *Service) activateSpawnMessaging(opts AgentStartOptions, paneID string) (string, error) {
	target, err := s.messages().prepareActivation(opts.Name, paneID)
	if err != nil {
		return "", err
	}
	if target.activationID == "" {
		return "", nil
	}
	if err := s.launchDeliveryHelper(opts.Name, target.activationID, opts.Timeout); err != nil {
		_ = s.messages().deactivateActivation(opts.Name, target.activationID, paneID, "delivery helper failed to launch")
		return "", err
	}
	return target.activationID, nil
}

func resolveHarnessExecutable(executable string) (string, error) {
	resolved := executable
	if !filepath.IsAbs(resolved) {
		path, err := exec.LookPath(resolved)
		if err != nil {
			return "", err
		}
		resolved = path
	}
	if absolute, err := filepath.Abs(resolved); err == nil {
		resolved = absolute
	}
	if canonical, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = canonical
	}
	return resolved, nil
}

func (s *Service) markOwnedLaunchExecing(name, launchID, activationID string) error {
	return s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		managed, ok := st.Agents[name]
		if !ok || managed.LaunchID != launchID || managed.LaunchPhase != launchClaimed || managed.LaunchPID != os.Getpid() {
			return NewError("agent_launch_unconfirmed", fmt.Sprintf("agent %q lost its launch claim before exec", name))
		}
		managed.LaunchPhase = launchExecing
		if activationID != "" {
			managed.ActivationID = activationID
		}
		st.Agents[name] = managed
		return nil
	})
}

func (s *Service) markOwnedLaunchFailed(name, launchID string) error {
	return s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		managed, ok := st.Agents[name]
		if ok && managed.LaunchID == launchID {
			managed.LaunchPhase = launchFailed
			st.Agents[name] = managed
		}
		return nil
	})
}

func (s *Service) markReservedLaunchFailed(name, launchID string) error {
	return s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		managed, ok := st.Agents[name]
		if ok && managed.LaunchID == launchID && managed.LaunchPhase == launchReserved {
			managed.LaunchPhase = launchFailed
			st.Agents[name] = managed
		}
		return nil
	})
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
	err = execAgent(executable, append([]string{executable}, forwardedArgs...),
		processenv.Managed(os.Environ(), project.TempDir(s.Project.Root)))
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
		return restoreSpawnSelectionIfOwned(st, claim.selectionGeneration, claim.previousSpawnSelection)
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

func rememberSpawnSelection(st *state.Session, opts AgentStartOptions) (uint64, error) {
	if !opts.RememberSelection {
		return 0, nil
	}
	if st.SpawnSelectionGeneration == ^uint64(0) {
		return 0, NewError("state_generation_exhausted", "spawn selection generation is exhausted")
	}
	st.SpawnSelectionGeneration++
	st.LastSpawnSelection = &state.SpawnSelection{Harness: opts.Kind, Model: opts.Model}
	return st.SpawnSelectionGeneration, nil
}

// restoreSpawnSelectionIfOwned rolls back a remembered selection only while
// no later spawn has written one. Restoration is itself a write, so its
// generation advances and can never make a stale rollback look current.
func restoreSpawnSelectionIfOwned(
	st *state.Session,
	ownedGeneration uint64,
	previous *state.SpawnSelection,
) error {
	if ownedGeneration == 0 || st.SpawnSelectionGeneration != ownedGeneration {
		return nil
	}
	if st.SpawnSelectionGeneration == ^uint64(0) {
		return NewError("state_generation_exhausted", "spawn selection generation is exhausted")
	}
	st.SpawnSelectionGeneration++
	st.LastSpawnSelection = cloneSpawnSelection(previous)
	return nil
}

// awaitPaneShell polls until the pane's shell exists. A freshly created pane
// reports no shell for its first moments, and a spawn command injected before
// the shell runs would be lost.
func awaitPaneShell(
	ctx context.Context,
	client *herdr.Client,
	paneID string,
	budget time.Duration,
) (herdr.ProcessInfo, error) {
	deadline := time.Now().Add(budget)
	for {
		if ctx.Err() != nil {
			return herdr.ProcessInfo{}, NewError("agent_pane_unready",
				fmt.Sprintf("wait for a shell in managed pane %s was cancelled: %v", paneID, ctx.Err()))
		}
		var result herdr.Result
		if err := client.Call(ctx, "pane.process_info", map[string]any{"pane_id": paneID}, &result); err != nil {
			return herdr.ProcessInfo{}, err
		}
		if result.ProcessInfo.ShellPID != nil {
			return result.ProcessInfo, nil
		}
		if time.Now().After(deadline) {
			return herdr.ProcessInfo{}, NewError("agent_pane_unready",
				fmt.Sprintf("managed pane %s has no shell after %s", paneID, budget))
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		timer.Stop()
	}
}

func awaitPaneShellOnly(ctx context.Context, client *herdr.Client, paneID string, budget time.Duration) error {
	info, err := awaitPaneShell(ctx, client, paneID, budget)
	if err != nil {
		return err
	}
	if paneHasNonShellForegroundProcess(info) {
		return NewError("pane_occupied", fmt.Sprintf("managed pane %s has a foreground process before bootstrap injection", paneID))
	}
	return nil
}

// rollbackCreatedAgentTab closes the tab a failing spawn just created so a
// retry does not find an orphaned, half-configured tab and open another one.
// Reused panes are preserved for diagnosis.
func rollbackCreatedAgentTab(
	ctx context.Context,
	client *herdr.Client,
	name string,
	managed state.Agent,
	created bool,
	cause error,
) error {
	if !created {
		return cause
	}
	// The spawn may be failing precisely because ctx was cancelled; the close
	// must still run, or the freshly created tab is orphaned.
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := client.Call(closeCtx, "tab.close", map[string]any{"tab_id": managed.TabID}, nil); err != nil {
		message := fmt.Sprintf("agent %q failed to start (%v); its freshly created tab %s could not be closed: %v",
			name, cause, managed.TabID, err)
		return newAgentTabCloseError(name, managed, message, cause)
	}
	return cause
}

// paneHasNonShellForegroundProcess reports the observation — a foreground
// process other than the pane's shell — and leaves the judgment to callers:
// ensureAgentPaneAvailable reads it as "occupied by something else".
func paneHasNonShellForegroundProcess(info herdr.ProcessInfo) bool {
	for _, process := range info.ForegroundProcesses {
		if info.ShellPID == nil || process.PID != *info.ShellPID {
			return true
		}
	}
	return false
}

// createAgentPane reports created=true from the moment the tab exists, even
// when a rename below fails: a label-less orphan can never be re-adopted, so
// the caller's rollback must know to close it.
func (s *Service) createAgentPane(
	ctx context.Context,
	client *herdr.Client,
	st *state.Session,
	snapshot herdr.Snapshot,
	name, kind, profile, cwd string,
) (state.Agent, bool, error) {
	expected := name
	// Unlike the orchestrator path, a stale non-empty s.WorkspaceID skips
	// st.WorkspaceID and goes straight to the project-wide search.
	workspaceID, err := resolveWorkspaceID(snapshot, s.Project, s.selectedWorkspaceID(st), fmt.Sprintf("agent %q", name))
	if err != nil {
		return state.Agent{}, false, err
	}
	workspaceID, tabID, paneID, err := s.allocateAgentPane(ctx, client, workspaceID, cwd)
	if err != nil {
		return state.Agent{}, false, err
	}
	st.WorkspaceID = workspaceID
	managed := state.Agent{Name: name, Kind: kind, Profile: profile, Placement: "tab", CWD: cwd, TabID: tabID, PaneID: paneID}
	if err := client.Call(ctx, "tab.rename", map[string]any{"tab_id": tabID, "label": expected}, nil); err != nil {
		return managed, true, err
	}
	if err := client.Call(ctx, "pane.rename", map[string]any{"pane_id": paneID, "label": expected}, nil); err != nil {
		return managed, true, err
	}
	st.Agents[name] = managed
	return managed, true, nil
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
