package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/herdrwire"
	"github.com/Harrison-Blair/fledge/internal/pirpc"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/species"
)

// Lifecycle states of a spawned agent. Self-registered agents have none: their
// liveness is their pid. State only ever changes on an observed event — a
// launch, a pi frame, a stop — never on inference.
const (
	stateRunning  = "running"
	stateBusy     = "busy"
	stateSettled  = "settled"
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

// spawnLaunchDelay is a test seam, nil everywhere but in the one test that sets
// it. It widens the window between a launch returning and its runner being
// registered, which is the ordering an agent that dies immediately depends on.
// That window is microseconds wide in practice, and the regression it guards
// against is silent — a dead agent left reading as running, its slug held for
// good — so the invariant is untestable without it. pirpc's stopGrace is a var
// for the same reason.
var spawnLaunchDelay func()

func (d *Daemon) spawn(req *protocol.Request) (protocol.Response, error) {
	if req.Config == "" && req.Model == "" {
		return protocol.Response{}, errors.New("spawn needs a config name or a model")
	}
	if req.Config != "" && req.Model != "" {
		return protocol.Response{}, errors.New("spawn takes a config name or a model, not both")
	}

	cfg, agentType, err := d.resolveSpawn(req)
	if err != nil {
		return protocol.Response{}, err
	}
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
	if cfg.Integration == "claude" && d.session.SocketPath == "" {
		return protocol.Response{}, errors.New("flock is not bound to a herdr session; claude agents need a pane")
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

	agent, err := d.launch(name, cwd, cfg, req.Split)
	if err != nil {
		d.release(res)
		return protocol.Response{}, err
	}
	if spawnLaunchDelay != nil {
		spawnLaunchDelay()
	}
	agent.Name = name
	agent.Type = agentType
	agent.Species = slug
	agent.Config = req.Config
	agent.Model = cfg.Model
	agent.Integration = cfg.Integration
	agent.State = stateRunning

	d.mu.Lock()

	// Both lines in one write: agent.registered first, so a replay that only
	// knows the old events still rebuilds the roster entry, and agent.spawned
	// after it to upsert the launch metadata. Written separately, a failure
	// between them would replay as a live agent with no integration.
	if err := d.appendAll(
		event{Event: evRegistered, Name: name, Type: agentType, Species: slug, PID: agent.PID},
		event{
			Event:       evSpawned,
			Name:        name,
			Type:        agentType,
			Species:     slug,
			PID:         agent.PID,
			Integration: agent.Integration,
			Model:       agent.Model,
			Config:      agent.Config,
			PaneID:      agent.PaneID,
			Cwd:         cwd,
			SessionID:   agent.sessionID,
		},
	); err != nil {
		// The journal is the state authority, so an agent it does not record
		// must not be left running: nothing would ever reap it. Give the name
		// back under the lock, then tear the process down outside it.
		d.releaseLocked(res)
		d.mu.Unlock()
		d.teardown(name, agent)
		return protocol.Response{}, err
	}

	d.agents[name] = agent.Agent
	if agent.runner != nil {
		d.runners[name] = agent.runner
		// Only now is the runner findable, so only now may the watcher run: an
		// agent that dies this early would otherwise look to the watcher like
		// one the daemon had already stopped, and its death would go
		// unrecorded while the roster still called it running.
		go d.watchRunner(name, agent.runner, agent.files...)
	}
	d.debug.Printf("spawned %s integration=%s pid=%d pane=%s", name, agent.Integration, agent.PID, agent.PaneID)
	d.mu.Unlock()
	return protocol.Response{Name: name, PaneID: agent.PaneID}, nil
}

// resolveSpawn turns the request into the config to launch and the agent type
// to name it after: the config's own name when spawned from agents.json, the
// integration when spawned from a bare model id.
func (d *Daemon) resolveSpawn(req *protocol.Request) (agentcfg.Config, string, error) {
	var (
		cfg       agentcfg.Config
		agentType string
	)
	if req.Config != "" {
		configs, err := agentcfg.Load(d.root)
		if err != nil {
			return cfg, "", err
		}
		entry, ok := configs[req.Config]
		if !ok {
			return cfg, "", fmt.Errorf("no agent config %q in %s", req.Config, agentcfg.FileName)
		}
		if err := entry.Validate(req.Config); err != nil {
			return cfg, "", err
		}
		cfg, agentType = entry, req.Config
	} else {
		integration, provider, err := agentcfg.Route(req.Model)
		if err != nil {
			return cfg, "", err
		}
		cfg = agentcfg.Config{Integration: integration, Model: req.Model, Provider: provider}
		agentType = integration
	}

	if req.Provider != "" {
		cfg.Provider = req.Provider
	}
	return cfg, agentType, nil
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
			return true // a launch already in flight holds this name
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
		PID: reservedPID, Integration: integration, State: stateRunning,
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
// d.mu: Stop blocks until the process is reaped, and PaneClose is a socket
// round-trip — exactly the slow calls the lock may not span.
func (d *Daemon) teardown(name string, agent launched) {
	if agent.runner != nil {
		if err := agent.runner.Stop(); err != nil {
			d.debug.Printf("%s: teardown: %v", name, err)
		}
		for _, f := range agent.files {
			f.Close()
		}
		return
	}
	if agent.PaneID != "" {
		if err := herdrwire.PaneClose(d.session.SocketPath, agent.PaneID); err != nil {
			d.debug.Printf("%s: teardown pane: %v", name, err)
		}
	}
}

// launched is a running agent before it is journaled: the roster entry plus the
// bits that ride on the journal line rather than the roster.
type launched struct {
	protocol.Agent
	runner    *pirpc.Runner
	sessionID string
	files     []*os.File
}

// launch starts the agent process. It runs without d.mu held: a Herdr call or a
// process start can take seconds, and a spawn must not stall the whole flock.
func (d *Daemon) launch(name, cwd string, cfg agentcfg.Config, split string) (launched, error) {
	if cfg.Integration == "claude" {
		return d.launchClaude(name, cwd, cfg, split)
	}
	// A pi agent is a subprocess with no pane, so there is nothing to split.
	return d.launchPi(name, cwd, cfg)
}

func (d *Daemon) launchClaude(name, cwd string, cfg agentcfg.Config, split string) (launched, error) {
	sessionID := agentcfg.NewSessionID()

	env := make(map[string]string, len(cfg.Env)+1)
	for k, v := range cfg.Env {
		env[k] = v
	}
	// The pane inherits the flock so that `fledge agent msg` and `fledge
	// agent register` work from inside it.
	env[flock.Env] = d.flockName

	started, err := herdrwire.AgentStart(d.session.SocketPath, name, cwd, cfg.CommandArgv(sessionID), env, split)
	if err != nil {
		return launched{}, err
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
		Agent:     protocol.Agent{PID: pid, PaneID: started.PaneID},
		sessionID: sessionID,
	}, nil
}

func (d *Daemon) launchPi(name, cwd string, cfg agentcfg.Config) (launched, error) {
	env := make([]string, 0, len(cfg.Env)+1)
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}
	env = append(env, flock.Env+"="+d.flockName)

	frames, err := d.agentFile(name, ".jsonl")
	if err != nil {
		return launched{}, err
	}
	stderr, err := d.agentFile(name, ".stderr.log")
	if err != nil {
		frames.Close()
		return launched{}, err
	}

	runner, err := pirpc.Start(cfg.CommandArgv(""), cwd, env, stderr, d.piEvents(name, frames))
	if err != nil {
		frames.Close()
		stderr.Close()
		return launched{}, err
	}

	// The watcher is deliberately not started here: it tells a deliberate stop
	// from a crash by the runner's absence from d.runners, so it cannot run
	// before spawn has put the runner there. See spawn.
	return launched{
		Agent:  protocol.Agent{PID: runner.PID()},
		runner: runner,
		files:  []*os.File{frames, stderr},
	}, nil
}

// agentFile opens a per-agent log in the flock directory.
func (d *Daemon) agentFile(name, suffix string) (*os.File, error) {
	path := filepath.Join(flock.Dir(d.root, d.flockName), "pi-"+name+suffix)
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
}

// piEvents records every frame the agent emits and advances its state from the
// three frames that mean something. Everything else is logged, not interpreted.
func (d *Daemon) piEvents(name string, frames *os.File) func(pirpc.Event) {
	return func(ev pirpc.Event) {
		// Safe without synchronisation: pirpc calls onEvent only from its
		// reader goroutine, and watchRunner closes this file only after Done,
		// which the reader closes as its last act.
		if _, err := frames.Write(append([]byte(ev.Raw), '\n')); err != nil {
			d.debug.Printf("%s: frame log: %v", name, err)
		}

		switch ev.Type {
		case "agent_start":
			d.setState(name, stateBusy, event{})
		case "agent_settled":
			d.setState(name, stateSettled, event{Event: evSettled, Name: name, MsgID: ev.ID})
		case "agent_end":
			d.setState(name, stateSettled, event{})
		}
	}
}

// setState moves an agent's state, journaling e first when it carries an event.
func (d *Daemon) setState(name, state string, e event) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if e.Event != "" {
		if err := d.append(e); err != nil {
			d.debug.Printf("%s: journal %s: %v", name, e.Event, err)
		}
	}
	if a, ok := d.agents[name]; ok {
		a.State = state
		d.agents[name] = a
	}
}

// watchRunner records an agent that died on its own. A runner the daemon
// stopped on purpose is out of d.runners by the time this fires, which is what
// tells the two apart — so spawn must not start this watcher until the runner
// is in d.runners, or an agent that dies during launch reads as one already
// stopped and its death goes unrecorded.
//
// Closing the files here is only safe because Done closes after pirpc's reader
// goroutine returns, and that reader is the sole writer to them (see piEvents).
func (d *Daemon) watchRunner(name string, runner *pirpc.Runner, files ...*os.File) {
	<-runner.Done()
	for _, f := range files {
		f.Close()
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.runners[name] != runner {
		return
	}
	delete(d.runners, name)
	if _, err := d.markStopped(name, "exited"); err != nil {
		d.debug.Printf("%s: journal %s: %v", name, evStopped, err)
	}
	d.debug.Printf("%s exited", name)
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
	// Claiming the runner here is what marks this exit as deliberate, so the
	// watcher does not also journal it as a crash.
	runner := d.runners[req.Name]
	delete(d.runners, req.Name)
	d.mu.Unlock()

	if agent.Integration == "claude" {
		if err := herdrwire.PaneClose(d.session.SocketPath, agent.PaneID); err != nil {
			return protocol.Response{}, err
		}
	} else if runner != nil {
		// A non-nil error here says how the agent died, not that stopping it
		// failed: the process is reaped and Done is closed either way. The
		// agent is down, so the stop stands and the crash detail goes to the
		// log — the journal records that it stopped, which is the state.
		if err := runner.Stop(); err != nil {
			d.debug.Printf("%s: agent exited abnormally: %v", req.Name, err)
		}
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
// rather than into the pending queue.
func bridged(a protocol.Agent) bool {
	return a.Integration != "" && a.State != stateStopped && a.State != stateOrphaned
}

// bridge hands a message straight to a spawned agent. Called without d.mu:
// a Herdr call can block for seconds.
func (d *Daemon) bridge(to protocol.Agent, runner *pirpc.Runner, msg protocol.Message) error {
	if to.Integration == "claude" {
		return herdrwire.SendInput(d.session.SocketPath, to.PaneID, msg.Body, true)
	}
	if runner == nil {
		return fmt.Errorf("agent %q has no running process", to.Name)
	}
	return runner.Prompt(msg.ID, msg.Body)
}

// stopRunners shuts every pi subprocess down, so no agent outlives the daemon
// that owns its pipes.
func (d *Daemon) stopRunners() {
	d.mu.Lock()
	runners := make([]*pirpc.Runner, 0, len(d.runners))
	for name, r := range d.runners {
		runners = append(runners, r)
		delete(d.runners, name)
	}
	d.mu.Unlock()

	for _, r := range runners {
		r.Stop()
	}
}
