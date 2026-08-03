package fledge

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/herdrtest"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

// okResult is what Herdr answers a method that reports nothing of its own.
var okResult = map[string]any{"type": "ok"}

type fakeLifecycle struct {
	mu               sync.Mutex
	snapshot         herdr.Snapshot
	workspaceCreates int
	workspaceCWD     string
	tabCreates       int
	tabCloses        int
	paneCloses       int
	tabRenames       int
	paneRenames      int
	paneSplits       int
	splitParams      map[string]any
	focusedPaneIDs   []string
	failMethod       string
	dropMethod       string
	// failPromptContaining fails agent.prompt calls whose text contains this
	// substring with a definite (non-transport) error.
	failPromptContaining string
	// bootingProcessInfoCalls makes the first N pane.process_info responses
	// report a shell-less booting pane; later calls report a ready shell.
	bootingProcessInfoCalls int
	processInfoCalls        int
	dropProcessInfoAfter    int
	// spawnAppearsAfterPolls delays the simulated exec transition by this many
	// pane.process_info polls (0 = immediately, negative = bootstrap forever).
	spawnAppearsAfterPolls int
	pendingSpawnPaneID     string
	pendingSpawnPolls      int
	childExeced            bool
	childExecutable        string
	staleAgentCaches       bool
	phantomAgentCache      bool
	foregroundByPane       map[string][]herdr.Process
	childClaim             func(string) string
	childWG                sync.WaitGroup
	// childPaneID names the pane whose foreground carries the simulated
	// bootstrap child (and then its harness), mirroring a real pane's process
	// timeline from the injection until the harness exits.
	childPaneID string
	// shellProbesAtSendInput records how many pane.process_info probes had
	// answered when the bootstrap command was injected, proving the spawn
	// waited for the pane's shell first.
	shellProbesAtSendInput int
	promptTarget           string
	promptText             string
	waitTarget             string
	readTarget             string
	sendKeysTarget         string
	sendKeys               []string
	sendKeyCalls           [][]string
	sendKeysTargets        []string
	methodCalls            []string
	sendInputCalls         int
	sendInputPaneID        string
	sendInputText          string
	sendInputKeys          []string
	serverStopped          bool
	serverStopError        string
	serverStopHook         func()
	runningMarker          string
	pongProtocol           int
	ignoreExit             bool
	agentExitHook          func()
}

func newFakeLifecycle(t *testing.T) (*Service, *fakeLifecycle) {
	t.Helper()
	fake := &fakeLifecycle{snapshot: herdrtest.EmptySnapshot(), pongProtocol: herdrtest.Protocol}
	socket := herdrtest.Server{
		Snapshot: &fake.snapshot,
		Mutex:    &fake.mu,
		IDs: herdrtest.IDs{
			Workspace: "w1", WorkspaceTab: "t1", WorkspacePane: "p1",
			Tab: "t-new", TabPane: "p-new", SplitPane: "p-right",
		},
		Observe: fake.record,
		Handle:  fake.handle,
		Unknown: okResult,
	}.Start(t)
	binary, runningMarker := fakeBinary(t, socket)
	// The socket has to exist before the binary can name it, so the marker is
	// the one field assigned once handle can already be reading it.
	fake.mu.Lock()
	fake.runningMarker = runningMarker
	fake.mu.Unlock()
	store, err := state.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{
		Project:           project.Info{Root: t.TempDir(), Session: "test-session"},
		Binary:            herdr.Binary{Path: binary},
		Store:             store,
		LaunchStopCleanup: func(StopCleanupRequest) error { return nil },
	}
	// The default deliverer runs the bounded delivery helper synchronously
	// in-process, so tests observe its effect as soon as the spawn returns.
	service.LaunchDeliveryHelper = func(name, activationID string, timeout time.Duration) error {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return service.DeliverActivation(ctx, name, activationID, timeout)
	}
	fake.mu.Lock()
	fake.childClaim = func(paneID string) string {
		executable := ""
		_ = store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
			for name, managed := range st.Agents {
				if managed.PaneID != paneID || managed.LaunchPhase != launchReserved {
					continue
				}
				managed.LaunchPhase = launchExecing
				managed.LaunchPID = 11
				managed.LaunchExecutable = "/usr/bin/" + managed.Kind
				executable = managed.LaunchExecutable
				st.Agents[name] = managed
			}
			return nil
		})
		return executable
	}
	fake.mu.Unlock()
	// Simulated children must finish before the test's state and socket
	// directories are torn down under them.
	t.Cleanup(fake.childWG.Wait)
	return service, fake
}

func fakeBinary(t *testing.T, socket string) (string, string) {
	t.Helper()
	temp := t.TempDir()
	existsMarker := filepath.Join(temp, "exists")
	runningMarker := filepath.Join(temp, "running")
	if err := os.WriteFile(existsMarker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runningMarker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	running := fmt.Sprintf(`{"sessions":[{"name":"test-session","running":true,"socket_path":%s}]}`, strconv.Quote(socket))
	stopped := `{"sessions":[{"name":"test-session","running":false}]}`
	path := herdrtest.WriteBinary(t, temp, herdrtest.Options{
		Version: herdrtest.VersionOutput,
		Sessions: []herdrtest.SessionCase{
			{Marker: runningMarker, Payload: running},
			{Marker: existsMarker, Payload: stopped},
		},
		DeleteRemoves: existsMarker,
	})
	return path, runningMarker
}

func serviceSessionSocket(t *testing.T, binary herdr.Binary) string {
	t.Helper()
	sessions, err := binary.Sessions(t.Context())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("read fake sessions: %v, %#v", err, sessions)
	}
	return sessions[0].SocketPath
}

// record instruments every request the shared server receives, including the
// ones handle then drops or fails.
func (f *fakeLifecycle) record(call herdrtest.Call) {
	f.methodCalls = append(f.methodCalls, call.Method)
	switch call.Method {
	case "workspace.create":
		f.workspaceCreates++
		f.workspaceCWD = call.Text("cwd")
	case "tab.create":
		f.tabCreates++
	case "tab.rename":
		f.tabRenames++
	case "pane.rename":
		f.paneRenames++
	case "pane.split":
		f.paneSplits++
		f.splitParams = call.Params
	case "pane.focus":
		f.focusedPaneIDs = append(f.focusedPaneIDs, call.Text("pane_id"))
	case "pane.send_input":
		f.sendInputCalls++
		f.sendInputPaneID = call.Text("pane_id")
		f.sendInputText = call.Text("text")
		f.sendInputKeys = appendStrings(f.sendInputKeys[:0], call.Params["keys"])
		f.shellProbesAtSendInput = f.processInfoCalls
		if strings.Contains(f.sendInputText, " agent spawn") &&
			f.failMethod != "pane.send_input" && f.dropMethod != "pane.send_input" {
			f.childPaneID = f.sendInputPaneID
			if f.phantomAgentCache {
				kind := "codex"
				for i := range f.snapshot.Panes {
					if f.snapshot.Panes[i].PaneID == f.sendInputPaneID {
						f.snapshot.Panes[i].Agent = &kind
						f.snapshot.Panes[i].AgentStatus = StateIdle
					}
				}
			}
			f.childWG.Add(1)
			go f.runInPaneChild(f.sendInputPaneID)
		}
	case "session.snapshot":
		if f.pendingSpawnPaneID != "" && f.pendingSpawnPolls > 0 {
			// Kept intentionally stale: launch authority comes only from
			// pane.process_info, not either Herdr cache view.
		}
	}
}

// runInPaneChild stands in for the injected `fledge agent spawn` child
// process. Like the real child's claimCurrentPane it first blocks on the
// session flock, so the harness can only ever appear after the parent has
// persisted the provisional record and released the lock — the only timeline
// production can produce. Once through the flock, the harness appears
// spawnAppearsAfterPolls session.snapshot polls later.
func (f *fakeLifecycle) runInPaneChild(paneID string) {
	defer f.childWG.Done()
	executable := f.childClaim(paneID)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pendingSpawnPaneID = paneID
	f.pendingSpawnPolls = f.spawnAppearsAfterPolls
	f.childExecutable = executable
	if f.pendingSpawnPolls == 0 {
		f.childExeced = true
		if !f.staleAgentCaches {
			f.simulateInPaneSpawn()
		}
	}
}

// simulateInPaneSpawn stands in for the exec into the harness: the pane's
// harness starts and Herdr's detection reports it as a live,
// interactive-ready agent named after the pane's label.
func (f *fakeLifecycle) simulateInPaneSpawn() {
	paneID := f.pendingSpawnPaneID
	f.pendingSpawnPaneID = ""
	kind := filepath.Base(f.childExecutable)
	for i := range f.snapshot.Panes {
		if f.snapshot.Panes[i].PaneID != paneID {
			continue
		}
		f.snapshot.Panes[i].Agent = &kind
		f.snapshot.Panes[i].AgentStatus = "idle"
		name := ""
		if f.snapshot.Panes[i].Label != nil {
			name = *f.snapshot.Panes[i].Label
		}
		agents := f.snapshot.Agents[:0]
		for _, agent := range f.snapshot.Agents {
			if agent.PaneID != paneID {
				agents = append(agents, agent)
			}
		}
		// Like real Herdr process detection, the entry carries no launch
		// handshake fields; readiness comes from the settled status alone.
		f.snapshot.Agents = append(agents, herdr.AgentInfo{
			Agent: &kind, AgentStatus: "idle", Name: &name,
			PaneID: paneID, TabID: f.snapshot.Panes[i].TabID, WorkspaceID: f.snapshot.Panes[i].WorkspaceID,
		})
		return
	}
}

// handle injects the failures a test asks for and answers the agent and server
// methods that only Fledge's service drives.
func (f *fakeLifecycle) handle(conn net.Conn, call herdrtest.Call) bool {
	if call.Method == f.dropMethod {
		return true
	}
	if call.Method == f.failMethod {
		herdrtest.WriteInjectedFailure(conn, call)
		return true
	}
	switch call.Method {
	case "ping":
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "pong", "version": herdrtest.Version, "protocol": f.pongProtocol,
		})
	case "pane.process_info":
		f.processInfoCalls++
		if f.dropProcessInfoAfter > 0 && f.processInfoCalls >= f.dropProcessInfoAfter {
			return true
		}
		if f.pendingSpawnPaneID == call.Text("pane_id") && f.pendingSpawnPolls > 0 {
			f.pendingSpawnPolls--
			if f.pendingSpawnPolls == 0 {
				f.childExeced = true
				if !f.staleAgentCaches {
					f.simulateInPaneSpawn()
				}
			}
		}
		info := map[string]any{"pane_id": call.Text("pane_id")}
		if f.processInfoCalls > f.bootingProcessInfoCalls {
			info["shell_pid"] = 10
			foreground := []map[string]any{{"pid": 10, "name": "bash", "argv": []string{"/bin/bash"}}}
			// From the injection until the harness exits, the pane's
			// foreground carries the bootstrap child (and then the harness),
			// exactly like a real injected spawn.
			if f.childPaneID != "" && f.childPaneID == call.Text("pane_id") {
				if f.childExeced {
					kind := filepath.Base(f.childExecutable)
					foreground = append(foreground, map[string]any{
						"pid": 11, "name": kind, "argv0": "/usr/bin/" + kind, "argv": []string{"/usr/bin/" + kind},
					})
				} else {
					foreground = append(foreground, map[string]any{"pid": 11, "name": "fledge", "argv0": "/usr/bin/fledge", "argv": []string{"/usr/bin/fledge"}})
				}
			} else {
				for _, pane := range f.snapshot.Panes {
					if pane.PaneID == call.Text("pane_id") && pane.Agent != nil {
						executable := "/usr/bin/" + *pane.Agent
						foreground = append(foreground, map[string]any{
							"pid": 12, "name": *pane.Agent, "argv0": executable, "argv": []string{executable},
						})
					}
				}
			}
			for _, process := range f.foregroundByPane[call.Text("pane_id")] {
				entry := map[string]any{"pid": process.PID, "name": process.Name, "argv": process.Argv}
				if process.Argv0 != nil {
					entry["argv0"] = *process.Argv0
				}
				foreground = append(foreground, entry)
			}
			info["foreground_processes"] = foreground
		}
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "pane_process_info", "process_info": info,
		})
	case "agent.prompt":
		if f.failPromptContaining != "" && strings.Contains(call.Text("text"), f.failPromptContaining) {
			herdrtest.WriteInjectedFailure(conn, call)
			return true
		}
		f.promptTarget = call.Text("target")
		f.promptText = call.Text("text")
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "agent_prompted", "agent": f.snapshot.Agents[0],
		})
	case "agent.read":
		f.readTarget = call.Text("target")
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "agent_read",
			"read": map[string]any{
				"pane_id": f.readTarget, "source": "recent_unwrapped",
				"format": "text", "text": "output", "truncated": false, "revision": 1,
			},
		})
	case "agent.send_keys":
		f.sendKeysTarget = call.Text("target")
		f.sendKeysTargets = append(f.sendKeysTargets, f.sendKeysTarget)
		f.sendKeys = appendStrings(f.sendKeys[:0], call.Params["keys"])
		f.sendKeyCalls = append(f.sendKeyCalls, append([]string(nil), f.sendKeys...))
		// Only shutdown keys stop the fake harness; a plain enter is the
		// prompt-submitting keypress and leaves the agent running.
		if !f.ignoreExit && slices.ContainsFunc(f.sendKeys, func(key string) bool {
			return key == "ctrl+d" || key == "ctrl+c"
		}) {
			f.exitAgent(f.sendKeysTarget)
			if f.agentExitHook != nil {
				f.agentExitHook()
			}
		}
		herdrtest.WriteResult(conn, call, okResult)
	case "agent.wait":
		f.waitTarget = call.Text("target")
		herdrtest.WriteResult(conn, call, map[string]any{"type": "agent_info"})
	case "pane.close":
		f.paneCloses++
		paneID := call.Text("pane_id")
		panes := f.snapshot.Panes[:0]
		for _, pane := range f.snapshot.Panes {
			if pane.PaneID != paneID {
				panes = append(panes, pane)
			}
		}
		f.snapshot.Panes = panes
		agents := f.snapshot.Agents[:0]
		for _, agent := range f.snapshot.Agents {
			if agent.PaneID != paneID {
				agents = append(agents, agent)
			}
		}
		f.snapshot.Agents = agents
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "pane_closed", "pane_id": paneID, "workspace_id": "w1",
		})
	case "tab.close":
		f.tabCloses++
		tabID := call.Text("tab_id")
		tabs := f.snapshot.Tabs[:0]
		for _, tab := range f.snapshot.Tabs {
			if tab.TabID != tabID {
				tabs = append(tabs, tab)
			}
		}
		f.snapshot.Tabs = tabs
		panes := f.snapshot.Panes[:0]
		for _, pane := range f.snapshot.Panes {
			if pane.TabID != tabID {
				panes = append(panes, pane)
			}
		}
		f.snapshot.Panes = panes
		agents := f.snapshot.Agents[:0]
		for _, agent := range f.snapshot.Agents {
			if agent.TabID != tabID {
				agents = append(agents, agent)
			}
		}
		f.snapshot.Agents = agents
		herdrtest.WriteResult(conn, call, map[string]any{
			"type": "tab_closed", "tab_id": tabID, "workspace_id": "w1",
		})
	case "server.stop":
		if f.serverStopError != "" {
			herdrtest.WriteError(conn, call, "stop_failed", f.serverStopError)
			return true
		}
		f.serverStopped = true
		_ = os.Remove(f.runningMarker)
		if f.serverStopHook != nil {
			f.serverStopHook()
		}
		herdrtest.WriteResult(conn, call, okResult)
	default:
		return false
	}
	return true
}

func (f *fakeLifecycle) exitAgent(paneID string) {
	if f.childPaneID == paneID {
		f.childPaneID = ""
		f.childExeced = false
	}
	for i := range f.snapshot.Panes {
		if f.snapshot.Panes[i].PaneID == paneID {
			f.snapshot.Panes[i].Agent = nil
			f.snapshot.Panes[i].AgentStatus = "unknown"
		}
	}
	remaining := f.snapshot.Agents[:0]
	for _, agent := range f.snapshot.Agents {
		if agent.PaneID != paneID {
			remaining = append(remaining, agent)
		}
	}
	f.snapshot.Agents = remaining
}

func appendStrings(into []string, raw any) []string {
	values, _ := raw.([]any)
	for _, value := range values {
		text, _ := value.(string)
		into = append(into, text)
	}
	return into
}
