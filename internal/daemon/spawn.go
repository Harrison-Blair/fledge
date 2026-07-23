package daemon

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/herdrwire"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/species"
)

// Lifecycle states of a spawned agent. Self-registered agents have none: their
// liveness is their pid. State only ever changes on an observed event — a
// launch, a readiness call, a stop — never on inference.
const (
	stateStarting = "starting"
	stateRunning  = "running"
	stateStopped  = "stopped"
	stateOrphaned = "orphaned"
)

// reservedPID marks a roster entry whose launch is in flight. Launching drops
// d.mu for seconds at a time, so the name is claimed before the lock is
// released and the real pid is filled in on the way back.
const reservedPID = -1

// metadataSource labels the pane metadata fledge sets. It is display-only:
// fledge never calls report_agent on a Claude pane, because native screen
// detection wins over a custom source anyway (EXP1, ADR-004).
const metadataSource = "custom:fledge"

var defaultReadinessTimeout = 2 * time.Minute

const bootstrapPrompt = "Complete startup now by running `fledge agent ready`. Do not begin other work until Fledge confirms readiness."

// assignedAgentPrompt is the complete Fledge-owned native instruction
// document. Raw profile and model spawns have no authored role, but still get
// their assigned identity and the mailbox guidance.
func assignedAgentPrompt(name, role string) string {
	instruction := fmt.Sprintf(
		"Fledge has assigned and already registered you as `%s`. Direct messages will arrive in this agent session. To reply, use `fledge agent msg send <recipient> <body>` with the recipient's assigned name.",
		name,
	)
	if role == "" {
		return instruction
	}
	return instruction + "\n\n" + role
}

// launchLatch lets readiness and stop calls observe a slow Herdr launch
// without holding d.mu. Its result is written while d.mu is held before done
// is closed.
type launchLatch struct {
	done  chan struct{}
	agent launched
	err   error
}

const (
	agentNameEnv  = protocol.AgentNameEnv
	agentTokenEnv = protocol.ReadyTokenEnv
)

type spawnResolution struct {
	cfg       agentcfg.Config
	agentType string
	agent     string
	profile   string
	source    string
	prompt    string
	workspace *agentcfg.Workspace
}

func (d *Daemon) spawn(req *protocol.Request) (protocol.Response, error) {
	resolved, err := d.resolveSpawnDetailed(req)
	if err != nil {
		return protocol.Response{}, err
	}
	cfg, agentType := resolved.cfg, resolved.agentType
	// The picker at `fledge start` chooses which model runs as the orchestrator,
	// so a picked config keeps everything it resolved — integration, model, cwd —
	// but takes the reserved identity. Naming it after the reserved type is the
	// whole of it: the bare name, the no-species rule and the one-at-a-time
	// collision are then the reserved branch of reserve, unchanged. Config still
	// records which entry it came from, so the roster names the model.
	if req.Orchestrator {
		agentType = agentcfg.ReservedOrchestrator
	}
	if err := validType(agentType); err != nil {
		return protocol.Response{}, err
	}
	if resolved.workspace != nil && cfg.Integration == "pi" {
		return protocol.Response{}, fmt.Errorf("agent %q requests dedicated workspace %q, but pi profiles do not support Herdr workspace placement; use a claude or codex profile", resolved.agent, resolved.workspace.Label)
	}
	if resolved.workspace != nil && req.AnchorPane != "" {
		return protocol.Response{}, fmt.Errorf("agent %q requests dedicated workspace %q and cannot also use anchored pane placement", resolved.agent, resolved.workspace.Label)
	}
	if d.session.SocketPath == "" {
		return protocol.Response{}, fmt.Errorf("flock is not bound to a herdr session; %s agents need a pane", cfg.Integration)
	}

	cwd, err := d.spawnCwd(cfg, req)
	if err != nil {
		return protocol.Response{}, err
	}

	res, err := d.reserve(agentType, req.Species, cfg.Integration)
	if err != nil {
		return protocol.Response{}, err
	}
	name, slug := res.name, res.slug
	authenticated := !d.skipReadiness
	if authenticated {
		defer os.Remove(ReadySignalPath(d.root, d.flockName, name))
	}
	var token, tokenHash string
	if authenticated {
		token, tokenHash, err = readinessToken()
		if err != nil {
			d.release(res)
			return protocol.Response{}, err
		}
		if cfg.Env == nil {
			cfg.Env = map[string]string{}
		} else {
			cfg.Env = cloneEnv(cfg.Env)
		}
		cfg.Env[agentNameEnv] = name
		cfg.Env[agentTokenEnv] = token
	}

	instructions := assignedAgentPrompt(name, resolved.prompt)
	instructionSum := sha256.Sum256([]byte(instructions))
	instructionHash := hex.EncodeToString(instructionSum[:])
	var sessionID string
	if cfg.Integration == "claude" {
		sessionID = agentcfg.NewSessionID()
	}
	argv := cfg.LaunchArgv(sessionID, instructions, bootstrapPrompt)
	placeholder := protocol.Agent{
		Name: name, Type: agentType, Species: slug, PID: reservedPID,
		Config: req.Config, Agent: resolved.agent, Profile: resolved.profile,
		Source: resolved.source, Model: cfg.Model, Integration: cfg.Integration,
		WorkspaceLabel: workspaceLabel(resolved.workspace), State: stateStarting,
	}
	launching := &launchLatch{done: make(chan struct{})}
	var ready chan struct{}
	if authenticated {
		ready = make(chan struct{})
	}

	d.mu.Lock()
	// Registration and resolved launch intent become authoritative together,
	// before the external CLI can run or report readiness.
	if err := d.appendAll(
		event{Event: evRegistered, Name: name, Type: agentType, Species: slug, PID: reservedPID},
		event{
			Event: evLaunching, Name: name, Type: agentType, Species: slug,
			Integration: cfg.Integration, Model: cfg.Model, Config: req.Config,
			Agent: resolved.agent, Profile: resolved.profile, Source: resolved.source,
			WorkspaceLabel: workspaceLabel(resolved.workspace), Cwd: cwd,
			SessionID: sessionID, TokenHash: tokenHash, InstructionHash: instructionHash,
		},
	); err != nil {
		d.releaseLocked(res)
		d.mu.Unlock()
		return protocol.Response{}, err
	}
	d.agents[name] = placeholder
	if d.launches == nil {
		d.launches = make(map[string]*launchLatch)
	}
	d.launches[name] = launching
	if authenticated {
		if d.readyTokens == nil {
			d.readyTokens = make(map[string]string)
		}
		if d.readyWaiters == nil {
			d.readyWaiters = make(map[string]chan struct{})
		}
		d.readyTokens[name] = tokenHash
		d.readyWaiters[name] = ready
	}
	d.mu.Unlock()

	agent, err := d.launch(name, cwd, cfg, argv, sessionID, req.Split, req.AnchorPane, resolved.workspace)
	if err != nil {
		d.failLaunching(name, launching, "launch failed", err)
		return protocol.Response{}, err
	}
	agent.Name = name
	agent.Type = agentType
	agent.Species = slug
	agent.Config = req.Config
	agent.Agent.Agent = resolved.agent
	agent.Profile = resolved.profile
	agent.Source = resolved.source
	agent.Model = cfg.Model
	agent.Integration = cfg.Integration
	agent.State = stateStarting
	if !authenticated {
		agent.State = stateRunning
	}

	d.mu.Lock()
	if err := d.append(event{
		Event: evSpawned, Name: name, PID: agent.PID, PaneID: agent.PaneID,
		WorkspaceID: agent.WorkspaceID, WorkspaceLabel: agent.WorkspaceLabel,
	}); err != nil {
		d.mu.Unlock()
		d.teardown(name, agent)
		d.failLaunching(name, launching, "spawn journal failed", err)
		return protocol.Response{}, err
	}
	d.agents[name] = agent.Agent
	launching.agent = agent
	close(launching.done)
	delete(d.launches, name)
	d.debug.Printf("spawned %s integration=%s pid=%d pane=%s", name, agent.Integration, agent.PID, agent.PaneID)
	d.mu.Unlock()
	if !authenticated {
		return protocol.Response{Name: name, PaneID: agent.PaneID}, nil
	}
	timeout := defaultReadinessTimeout
	if req.TimeoutMS > 0 {
		timeout = time.Duration(req.TimeoutMS) * time.Millisecond
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	readySignals := time.NewTicker(50 * time.Millisecond)
	defer readySignals.Stop()

readiness:
	for {
		select {
		case <-ready:
			if !d.agentReady(name) {
				return protocol.Response{}, fmt.Errorf("agent %s stopped before readiness", name)
			}
			break readiness
		case <-readySignals.C:
			consumed, signalErr := d.consumeReadySignal(name)
			if signalErr != nil {
				d.debug.Printf("ready signal %s: %v", name, signalErr)
				continue
			}
			if consumed {
				break readiness
			}
		case <-timer.C:
			if d.agentReady(name) {
				break readiness
			}
			if d.rollbackStarting(res, agent, "readiness timeout") {
				return protocol.Response{}, fmt.Errorf("agent %s did not become ready within %s", name, timeout)
			}
			return protocol.Response{}, fmt.Errorf("agent %s stopped before readiness", name)
		}
	}
	return protocol.Response{Name: name, PaneID: agent.PaneID}, nil
}

// resolveSpawn turns the request into the config to launch and the agent type
// to name it after: the config's own name when spawned from agents.json, the
// integration when spawned from a bare model id.
func (d *Daemon) resolveSpawnDetailed(req *protocol.Request) (spawnResolution, error) {
	var out spawnResolution
	selected := 0
	if req.Agent != "" {
		selected++
	}
	if req.Config != "" {
		selected++
	}
	if req.Model != "" {
		selected++
	}
	if req.Profile != "" && req.Agent == "" {
		selected++
	}
	if selected != 1 {
		return out, errors.New("spawn needs exactly one agent, profile, config, or model")
	}

	if req.Agent != "" {
		defs, profiles, err := agentcfg.LoadDefinitions(d.root)
		if err != nil {
			return out, err
		}
		def, ok := defs[req.Agent]
		if !ok {
			return out, fmt.Errorf("no agent definition %q", req.Agent)
		}
		profile := def.Profile
		if req.Profile != "" {
			if def.Profile != "" {
				return out, fmt.Errorf("agent %q already selects profile %q", def.Name, def.Profile)
			}
			profile = req.Profile
		}
		if profile == "" {
			return out, fmt.Errorf("agent %q is profile-agnostic; pass --profile", def.Name)
		}
		cfg, ok := profiles[profile]
		if !ok {
			return out, fmt.Errorf("no profile %q", profile)
		}
		out = spawnResolution{cfg: cfg, agentType: def.Name, agent: def.Name, profile: profile, source: def.Source, prompt: def.Prompt, workspace: def.Workspace}
	} else if req.Profile != "" {
		profiles, err := agentcfg.Load(d.root)
		if err != nil {
			return out, err
		}
		cfg, ok := profiles[req.Profile]
		if !ok {
			return out, fmt.Errorf("no profile %q", req.Profile)
		}
		out = spawnResolution{cfg: cfg, agentType: req.Profile, profile: req.Profile}
	} else if req.Config != "" {
		if req.Integration != "" {
			return out, errors.New("integration override applies to --model spawns; a config names its integration")
		}
		configs, err := agentcfg.Load(d.root)
		if err != nil {
			return out, err
		}
		entry, ok := configs[req.Config]
		if !ok {
			return out, fmt.Errorf("no agent config %q in %s", req.Config, agentcfg.FileName)
		}
		if err := entry.Validate(req.Config); err != nil {
			return out, err
		}
		out = spawnResolution{cfg: entry, agentType: req.Config, profile: req.Config}
	} else {
		integration, provider, err := agentcfg.Route(req.Model)
		if err != nil {
			return out, err
		}
		if req.Integration != "" && req.Integration != integration {
			// The routed provider is pi's; under any other integration it
			// would be a field ValidateFields rejects below.
			integration = req.Integration
			if integration != "pi" {
				provider = ""
			}
		}
		out.cfg = agentcfg.Config{Integration: integration, Model: req.Model, Provider: provider}
		out.agentType = integration
	}

	if req.Provider != "" {
		out.cfg.Provider = req.Provider
	}
	// An override or a provider flag can assemble a combination no config file
	// would have passed, so the assembled config takes the same field checks.
	if err := out.cfg.ValidateFields(); err != nil {
		return out, err
	}
	return out, nil
}

// resolveSpawn retains the focused resolver seam used by older package tests.
func (d *Daemon) resolveSpawn(req *protocol.Request) (agentcfg.Config, string, error) {
	r, err := d.resolveSpawnDetailed(req)
	return r.cfg, r.agentType, err
}

func readinessToken() (token, hash string, err error) {
	a, err := newID()
	if err != nil {
		return "", "", err
	}
	b, err := newID()
	if err != nil {
		return "", "", err
	}
	token = a + b
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:]), nil
}

func cloneEnv(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+2)
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ready authenticates the one-use token injected into a launched process.
func (d *Daemon) ready(req *protocol.Request) (protocol.Response, error) {
	if req.Name == "" || req.Token == "" {
		return protocol.Response{}, errors.New("agent ready requires its injected name and token")
	}
	sum := sha256.Sum256([]byte(req.Token))
	got := hex.EncodeToString(sum[:])
	return d.readyDigest(req.Name, got)
}

func (d *Daemon) readyDigest(name, got string) (protocol.Response, error) {
	for {
		d.mu.Lock()
		want, ok := d.readyTokens[name]
		if !ok {
			if a, exists := d.agents[name]; exists && a.State != stateStarting {
				d.mu.Unlock()
				return protocol.Response{}, fmt.Errorf("readiness token for %q was already used", name)
			}
			d.mu.Unlock()
			return protocol.Response{}, fmt.Errorf("no starting agent %q", name)
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			d.mu.Unlock()
			return protocol.Response{}, errors.New("invalid readiness token")
		}
		// The CLI can execute its initial prompt before agent.start returns.
		// Wait for the spawned event without keeping the daemon lock held.
		if launch := d.launches[name]; launch != nil {
			d.mu.Unlock()
			<-launch.done
			continue
		}
		if err := d.append(event{Event: evReady, Name: name}); err != nil {
			d.mu.Unlock()
			return protocol.Response{}, err
		}
		a := d.agents[name]
		a.State = stateRunning
		d.agents[name] = a
		delete(d.readyTokens, name)
		ch := d.readyWaiters[name]
		if ch != nil {
			close(ch)
			delete(d.readyWaiters, name)
		}
		d.debug.Printf("ready %s", name)
		d.mu.Unlock()
		return protocol.Response{Name: name}, nil
	}
}

func (d *Daemon) agentReady(name string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.agents[name]
	return ok && a.State != stateStarting && a.State != stateStopped
}

func (d *Daemon) rollbackStarting(_ reservation, agent launched, reason string) bool {
	d.mu.Lock()
	if current, ok := d.agents[agent.Name]; ok && reason == "readiness timeout" && current.State != stateStarting {
		d.mu.Unlock()
		return false
	}
	delete(d.readyTokens, agent.Name)
	if ch := d.readyWaiters[agent.Name]; ch != nil {
		close(ch)
	}
	delete(d.readyWaiters, agent.Name)
	d.mu.Unlock()

	d.teardown(agent.Name, agent)

	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.markStopped(agent.Name, reason); err != nil {
		d.debug.Printf("%s: rollback journal: %v", agent.Name, err)
	}
	return true
}

// failLaunching resolves both startup latches after an attempt that was
// already recorded as launching cannot become a usable spawned agent.
func (d *Daemon) failLaunching(name string, launch *launchLatch, reason string, cause error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.append(event{Event: evStopped, Name: name, Reason: reason}); err != nil {
		d.debug.Printf("%s: failure journal: %v", name, err)
	}
	if a, ok := d.agents[name]; ok {
		a.State = stateStopped
		d.agents[name] = a
	}
	delete(d.readyTokens, name)
	if ch := d.readyWaiters[name]; ch != nil {
		close(ch)
	}
	delete(d.readyWaiters, name)
	launch.err = cause
	close(launch.done)
	if d.launches[name] == launch {
		delete(d.launches, name)
	}
}

// spawnCwd resolves the working directory the agent launches in: the request
// wins over the config, and the workspace root is the default.
func (d *Daemon) spawnCwd(cfg agentcfg.Config, req *protocol.Request) (string, error) {
	cwd := cfg.Cwd
	if req.Cwd != "" {
		cwd = req.Cwd
	}
	if cwd == "" {
		cwd = d.root
	} else if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(d.root, cwd)
	}
	return filepath.Abs(cwd)
}

// reservation is a claimed name plus whatever the roster held under it before,
// so that a failed launch can put the old entry back rather than erase it.
type reservation struct {
	name string
	slug string
	prev protocol.Agent
	had  bool
}

// reserve claims a name for a launch that has not happened yet, so that the
// slow part of spawn runs without holding d.mu. The placeholder holds the slug
// until launch either fills it in or release drops it.
func (d *Daemon) reserve(agentType, requested, integration string) (reservation, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// As in register, the pool is per-type. A spawned agent's liveness is its
	// state rather than its pid: a stopped pane's shell can outlive the stop,
	// and an orphaned agent's pid belonged to a daemon that is gone. Only a
	// self-registered agent, which has no state, is judged by its process.
	held := func(name string) bool {
		a, ok := d.agents[name]
		if !ok {
			return false
		}
		if a.PID == reservedPID {
			return a.State != stateStopped && a.State != stateOrphaned
		}
		if a.Integration != "" {
			return a.State != stateStopped && a.State != stateOrphaned
		}
		return alive(a.PID)
	}

	var name, slug string
	if agentType == agentcfg.ReservedOrchestrator {
		// The orchestrator runs under its bare config name, so its pool is
		// that one name instead of the species list — and a second one
		// collides with the first exactly as an exhausted species pool does.
		if requested != "" {
			return reservation{}, fmt.Errorf("%s takes no species", agentType)
		}
		if held(agentType) {
			return reservation{}, fmt.Errorf("%s is already running", agentType)
		}
		name = agentType
	} else {
		picked, err := species.Pick(func(slug string) bool {
			return held(agentType + "-" + slug)
		}, requested)
		if err != nil {
			return reservation{}, fmt.Errorf("%s: %w", agentType, err)
		}
		name, slug = agentType+"-"+picked, picked
	}

	r := reservation{name: name, slug: slug}
	r.prev, r.had = d.agents[r.name]
	if !r.had {
		d.order = append(d.order, r.name)
	}
	d.agents[r.name] = protocol.Agent{
		Name: r.name, Type: agentType, Species: slug,
		PID: reservedPID, Integration: integration, State: stateStarting,
	}
	return r, nil
}

// release drops a reservation whose launch failed.
func (d *Daemon) release(r reservation) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.releaseLocked(r)
}

// releaseLocked is release with d.mu already held. A slug reused from a stopped
// agent must give that agent's roster entry back: the journal still records it,
// so erasing it here would make the live roster disagree with a replay.
func (d *Daemon) releaseLocked(r reservation) {
	if r.had {
		d.agents[r.name] = r.prev
		return
	}
	delete(d.agents, r.name)
	for i, other := range d.order {
		if other == r.name {
			d.order = append(d.order[:i], d.order[i+1:]...)
			return
		}
	}
}

// teardown reaps a launch that could not be journaled. It must run WITHOUT
// d.mu: PaneClose is a socket round-trip — exactly the slow call the lock may
// not span.
func (d *Daemon) teardown(name string, agent launched) {
	if agent.WorkspaceID != "" {
		if err := herdrwire.WorkspaceClose(d.session.SocketPath, agent.WorkspaceID); err != nil {
			d.debug.Printf("%s: teardown workspace: %v", name, err)
		}
	} else if agent.PaneID != "" {
		if err := herdrwire.PaneClose(d.session.SocketPath, agent.PaneID); err != nil {
			d.debug.Printf("%s: teardown pane: %v", name, err)
		}
	}
}

// launched is a running agent before it is journaled: the roster entry plus the
// bits that ride on the journal line rather than the roster.
type launched struct {
	protocol.Agent
	sessionID string
}

// launch starts the agent's herdr pane. It runs without d.mu held: a Herdr call
// can take seconds, and a spawn must not stall the whole flock.
func (d *Daemon) launch(name, cwd string, cfg agentcfg.Config, argv []string, sessionID, split, anchorPane string, workspace *agentcfg.Workspace) (launched, error) {
	env := make(map[string]string, len(cfg.Env)+1)
	for k, v := range cfg.Env {
		env[k] = v
	}
	// The pane inherits the flock so that `fledge agent msg` and `fledge
	// agent register` work from inside it.
	env[flock.Env] = d.flockName

	var (
		started herdrwire.StartedAgent
		created herdrwire.CreatedWorkspace
		err     error
	)
	if workspace == nil {
		started, err = herdrwire.AgentStart(d.session.SocketPath, name, cwd, argv, env, split)
	} else {
		started, created, err = d.createAgentWorkspace(name, cwd, argv, env, *workspace)
	}
	if err != nil {
		return launched{}, err
	}
	if anchorPane != "" {
		// Herdr 0.7.4 can create only right/down splits. Interactive start
		// therefore creates the orchestrator on the right, then immediately
		// swaps it left and restores focus before registration, readiness, or
		// bootstrap can make the temporary placement visible as a later jump.
		if err := herdrwire.PaneSwap(d.session.SocketPath, anchorPane, started.PaneID); err != nil {
			return launched{}, d.failPanePlacement(name, started.PaneID, "swap", err)
		}
		// pane.swap leaves focus with the slot rather than the pane.
		if err := herdrwire.PaneFocus(d.session.SocketPath, started.PaneID); err != nil {
			return launched{}, d.failPanePlacement(name, started.PaneID, "focus", err)
		}
	}

	// The pane exists either way; an unknown shell pid only costs the liveness
	// probe, so it is not worth failing a spawn over.
	pid, err := herdrwire.ProcessInfo(d.session.SocketPath, started.PaneID)
	if err != nil {
		d.debug.Printf("%s: process_info: %v", name, err)
	}
	if err := herdrwire.ReportMetadata(d.session.SocketPath, started.PaneID, metadataSource, name); err != nil {
		d.debug.Printf("%s: report_metadata: %v", name, err)
	}

	return launched{
		Agent: protocol.Agent{
			PID: pid, PaneID: started.PaneID,
			WorkspaceID: created.WorkspaceID, WorkspaceLabel: workspaceLabel(workspace),
		},
		sessionID: sessionID,
	}, nil
}

func workspaceLabel(workspace *agentcfg.Workspace) string {
	if workspace == nil {
		return ""
	}
	return workspace.Label
}

// createAgentWorkspace performs all placement steps before the launch can be
// journaled. Any failure closes the whole workspace, so no unnamed shell or
// untracked agent pane is left behind.
func (d *Daemon) createAgentWorkspace(name, cwd string, argv []string, env map[string]string, workspace agentcfg.Workspace) (herdrwire.StartedAgent, herdrwire.CreatedWorkspace, error) {
	focusedPane, err := herdrwire.PaneCurrent(d.session.SocketPath)
	if err != nil {
		return herdrwire.StartedAgent{}, herdrwire.CreatedWorkspace{}, fmt.Errorf("record focus before creating workspace for %s: %w", name, err)
	}
	if focusedPane == "" {
		return herdrwire.StartedAgent{}, herdrwire.CreatedWorkspace{}, fmt.Errorf("record focus before creating workspace for %s: Herdr returned no pane id", name)
	}

	created, err := herdrwire.WorkspaceCreate(d.session.SocketPath, cwd, workspace.Label, false)
	if err != nil {
		return herdrwire.StartedAgent{}, herdrwire.CreatedWorkspace{}, fmt.Errorf("create workspace for %s: %w", name, err)
	}
	fail := func(cause error) (herdrwire.StartedAgent, herdrwire.CreatedWorkspace, error) {
		if created.WorkspaceID != "" {
			if closeErr := herdrwire.WorkspaceClose(d.session.SocketPath, created.WorkspaceID); closeErr != nil {
				cause = fmt.Errorf("%w (closing workspace %s also failed: %v)", cause, created.WorkspaceID, closeErr)
			}
		}
		return herdrwire.StartedAgent{}, herdrwire.CreatedWorkspace{}, cause
	}
	if created.WorkspaceID == "" || created.TabID == "" || created.RootPaneID == "" {
		return fail(fmt.Errorf("create workspace for %s: Herdr returned incomplete workspace IDs", name))
	}
	if err := herdrwire.TabRename(d.session.SocketPath, created.TabID, workspace.Tab); err != nil {
		return fail(fmt.Errorf("rename workspace tab for %s: %w", name, err))
	}
	started, err := herdrwire.AgentStartInWorkspace(d.session.SocketPath, name, cwd, argv, env, created.WorkspaceID, created.TabID)
	if err != nil {
		return fail(fmt.Errorf("start %s in workspace %s: %w", name, workspace.Label, err))
	}
	if started.PaneID == "" {
		return fail(fmt.Errorf("start %s in workspace %s: Herdr returned no pane id", name, workspace.Label))
	}
	if err := herdrwire.PaneClose(d.session.SocketPath, created.RootPaneID); err != nil {
		return fail(fmt.Errorf("remove initial shell from workspace %s: %w", workspace.Label, err))
	}
	// Herdr 0.7.4 focuses the surviving pane's workspace when the initial
	// shell is closed, even though workspace.create and agent.start both used
	// focus:false. Put focus back where it was so dedicated placement is
	// observationally unfocused. A failed restoration makes the placement
	// incomplete and therefore rolls the whole workspace back.
	if err := herdrwire.PaneFocus(d.session.SocketPath, focusedPane); err != nil {
		return fail(fmt.Errorf("restore focus after creating workspace %s: %w", workspace.Label, err))
	}
	return started, created, nil
}

func (d *Daemon) failPanePlacement(name, paneID, operation string, placementErr error) error {
	err := fmt.Errorf("place %s pane: %s: %w", name, operation, placementErr)
	if closeErr := herdrwire.PaneClose(d.session.SocketPath, paneID); closeErr != nil {
		return fmt.Errorf("%w (closing pane also failed: %v)", err, closeErr)
	}
	return err
}

// markStopped records an agent as stopped, and reports whether this call was
// the one that recorded it. An agent that dies on its own while a stop is in
// flight is stopped by whichever path reaches the lock first; the other must
// not write a second agent.stopped line for the same death. Caller holds d.mu.
func (d *Daemon) markStopped(name, reason string) (bool, error) {
	a, ok := d.agents[name]
	if !ok || a.State == stateStopped {
		return false, nil
	}
	if err := d.append(event{Event: evStopped, Name: name, Reason: reason}); err != nil {
		return false, err
	}
	a.State = stateStopped
	d.agents[name] = a
	delete(d.readyTokens, name)
	if ch := d.readyWaiters[name]; ch != nil {
		close(ch)
	}
	delete(d.readyWaiters, name)
	return true, nil
}

func (d *Daemon) stop(req *protocol.Request) (protocol.Response, error) {
	if req.Name == "" {
		return protocol.Response{}, errors.New("missing agent name")
	}

	d.mu.Lock()
	agent, ok := d.agents[req.Name]
	if !ok {
		d.mu.Unlock()
		return protocol.Response{}, fmt.Errorf("no registered agent %q", req.Name)
	}
	if agent.Integration == "" {
		d.mu.Unlock()
		return protocol.Response{}, fmt.Errorf("agent %q was not spawned by fledge; stop it where it runs", req.Name)
	}
	launch := d.launches[req.Name]
	d.mu.Unlock()
	if launch != nil {
		<-launch.done
		if launch.err != nil {
			// The launch failure path already records the stopped attempt.
			return protocol.Response{Name: req.Name}, nil
		}
		agent = launch.agent.Agent
	}

	var err error
	if agent.WorkspaceID != "" {
		err = herdrwire.WorkspaceClose(d.session.SocketPath, agent.WorkspaceID)
	} else {
		err = herdrwire.PaneClose(d.session.SocketPath, agent.PaneID)
	}
	if err != nil {
		return protocol.Response{}, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.markStopped(req.Name, "requested"); err != nil {
		return protocol.Response{}, err
	}
	d.debug.Printf("stopped %s", req.Name)
	return protocol.Response{Name: req.Name}, nil
}

// bridged reports whether messages to this agent go to a process fledge drives
// rather than into the pending queue. The orchestrator is pane-hosted for
// interactive use, but remains a user-driven mailbox consumer.
func bridged(a protocol.Agent) bool {
	return a.Name != agentcfg.ReservedOrchestrator && a.Integration != "" && a.State != stateStopped && a.State != stateOrphaned
}

// bridge hands a message straight to a spawned agent. Called without d.mu:
// a Herdr call can block for seconds.
func (d *Daemon) bridge(to protocol.Agent, msg protocol.Message) error {
	return d.bridgePrompt(to, directMessagePrompt(msg))
}

func directMessagePrompt(msg protocol.Message) string {
	prompt := fmt.Sprintf("Fledge direct message\nid: %s\nfrom: %s", msg.ID, msg.From)
	if msg.ReplyTo != "" {
		prompt += "\nreply_to: " + msg.ReplyTo
	}
	return prompt + "\n\n" + msg.Body
}

func (d *Daemon) bridgePrompt(to protocol.Agent, body string) error {
	return herdrwire.SendInput(d.session.SocketPath, to.PaneID, body, true)
}
