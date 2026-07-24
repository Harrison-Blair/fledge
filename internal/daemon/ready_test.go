package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

func installStartingToken(t *testing.T, d *Daemon, name string) string {
	t.Helper()
	token, hash, err := readinessToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := d.appendAll(
		event{Event: evRegistered, Name: name, Type: "worker", Species: "emperor"},
		event{Event: evLaunching, Name: name, Integration: "claude", SessionID: "11111111-1111-4111-8111-111111111111", TokenHash: hash},
		event{Event: evSpawned, Name: name, Integration: "claude", PaneID: "w1:p2", TokenHash: hash},
	); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	d.agents[name] = protocol.Agent{
		Name: name, Type: "worker", Species: "emperor",
		Integration: "claude", PaneID: "w1:p2", State: stateStarting,
		SessionID: "11111111-1111-4111-8111-111111111111", Cwd: d.root,
	}
	d.order = append(d.order, name)
	d.readyTokens[name] = hash
	d.readyWaiters[name] = make(chan struct{})
	d.mu.Unlock()
	return token
}

func TestReadyValidInvalidAndReplayedTokens(t *testing.T) {
	d := newTestDaemon(t)
	token := installStartingToken(t, d, "worker-emperor")
	if _, err := d.ready(&protocol.Request{Name: "worker-emperor", Token: "wrong"}); err == nil {
		t.Fatal("invalid token succeeded")
	}
	if _, err := d.ready(&protocol.Request{Name: "worker-emperor", Token: token}); err != nil {
		t.Fatalf("valid token: %v", err)
	}
	if got := agentState(d, "worker-emperor"); got != stateRunning {
		t.Fatalf("state = %s", got)
	}
	if _, err := d.ready(&protocol.Request{Name: "worker-emperor", Token: token}); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("replayed token error = %v", err)
	}
	if got := countEvents(t, d, evReady, "worker-emperor"); got != 1 {
		t.Fatalf("ready events = %d", got)
	}
	replayed, err := replay(journalPath(d.root, d.flockName))
	if err != nil {
		t.Fatal(err)
	}
	if replayed.agents["worker-emperor"].State != stateRunning || replayed.tokens["worker-emperor"] != "" {
		t.Fatalf("replayed state = %+v tokens=%v", replayed.agents["worker-emperor"], replayed.tokens)
	}
	sum := sha256.Sum256([]byte(token))
	if got := replayed.credentials["worker-emperor"]; got != hex.EncodeToString(sum[:]) {
		t.Fatalf("replayed identity credential = %q", got)
	}
}

func TestSpawnedMessageIdentityUsesLaunchCredential(t *testing.T) {
	d := newTestDaemon(t)
	token := installStartingToken(t, d, "worker-emperor")
	if _, err := d.ready(&protocol.Request{Name: "worker-emperor", Token: token}); err != nil {
		t.Fatal(err)
	}
	receiver, err := d.register(&protocol.Request{Type: "receiver", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}

	for label, credential := range map[string]string{"missing": "", "wrong": "wrong"} {
		if _, err := d.send(&protocol.Request{
			From: "worker-emperor", To: receiver.Name, Body: label, Token: credential,
		}); err == nil || !strings.Contains(err.Error(), "identity token") {
			t.Fatalf("%s send error = %v", label, err)
		}
	}
	if _, err := d.send(&protocol.Request{
		From: "worker-emperor", To: receiver.Name, Body: "valid", Token: token,
	}); err != nil {
		t.Fatalf("authenticated send: %v", err)
	}

	if _, err := d.wait(&protocol.Request{
		As: "worker-emperor", TimeoutMS: 1,
	}, nil); err == nil || !strings.Contains(err.Error(), "identity token") {
		t.Fatalf("unauthenticated wait error = %v", err)
	}
	if _, err := d.wait(&protocol.Request{
		As: "worker-emperor", Token: token, TimeoutMS: 1,
	}, nil); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("authenticated wait error = %v, want timeout", err)
	}
}

func TestStoppedSpawnedIdentityRemainsAuthenticatedButUnauthorized(t *testing.T) {
	d := newTestDaemon(t)
	token := installStartingToken(t, d, "worker-emperor")
	if _, err := d.ready(&protocol.Request{Name: "worker-emperor", Token: token}); err != nil {
		t.Fatal(err)
	}
	sender, err := d.register(&protocol.Request{Type: "sender", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	inbound, err := d.send(&protocol.Request{
		From: sender.Name, To: "worker-emperor", Body: "claim before stop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.inbox(&protocol.Request{As: "worker-emperor", Token: token}); err != nil {
		t.Fatal(err)
	}

	d.mu.Lock()
	if _, err := d.markStopped("worker-emperor", "test"); err != nil {
		d.mu.Unlock()
		t.Fatal(err)
	}
	credential := d.identityTokens["worker-emperor"]
	d.mu.Unlock()
	if credential == "" {
		t.Fatal("stop discarded identity credential")
	}

	operations := map[string]func() error{
		"send": func() error {
			_, err := d.send(&protocol.Request{
				From: "worker-emperor", To: sender.Name, Body: "stale", Token: token,
			})
			return err
		},
		"inbox": func() error {
			_, err := d.inbox(&protocol.Request{As: "worker-emperor", Token: token})
			return err
		},
		"wait": func() error {
			_, err := d.wait(&protocol.Request{
				As: "worker-emperor", Token: token, TimeoutMS: 1,
			}, nil)
			return err
		},
		"reply": func() error {
			_, err := d.reply(&protocol.Request{
				From: "worker-emperor", ID: inbound.ID, Body: "stale", Token: token,
			})
			return err
		},
	}
	for name, operation := range operations {
		if err := operation(); err == nil || !strings.Contains(err.Error(), "not authorized") {
			t.Fatalf("%s error = %v, want lifecycle authorization failure", name, err)
		}
	}
	if _, err := d.send(&protocol.Request{
		From: sender.Name, To: "worker-emperor", Body: "after stop",
	}); err == nil || !strings.Contains(err.Error(), "cannot receive") {
		t.Fatalf("send to stopped recipient error = %v", err)
	}

	replayed, err := replay(journalPath(d.root, d.flockName))
	if err != nil {
		t.Fatal(err)
	}
	if replayed.agents["worker-emperor"].State != stateStopped ||
		replayed.credentials["worker-emperor"] != credential {
		t.Fatalf("replayed stopped identity = %+v credential=%q",
			replayed.agents["worker-emperor"], replayed.credentials["worker-emperor"])
	}
}

func TestStoppingAgentCancelsParkedMessageWait(t *testing.T) {
	d := newTestDaemon(t)
	token := installStartingToken(t, d, "worker-emperor")
	if _, err := d.ready(&protocol.Request{Name: "worker-emperor", Token: token}); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() {
		_, err := d.wait(&protocol.Request{As: "worker-emperor", Token: token}, nil)
		waited <- err
	}()
	waitFor(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.waiters) == 1
	})
	d.mu.Lock()
	if _, err := d.markStopped("worker-emperor", "test"); err != nil {
		d.mu.Unlock()
		t.Fatal(err)
	}
	d.mu.Unlock()
	select {
	case err := <-waited:
		if err == nil || !strings.Contains(err.Error(), "stopped while waiting") {
			t.Fatalf("wait error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stopped agent wait was not canceled")
	}
}

func TestAuthenticatedReadyWaitDeliversBeforeAndAfterSendExactlyOnce(t *testing.T) {
	assertDeliveredOnce := func(t *testing.T, d *Daemon, id string) {
		t.Helper()
		deliveries := 0
		for _, e := range events(t, d) {
			if e.Event == evDelivered && e.ID == id {
				deliveries++
			}
		}
		if deliveries != 1 {
			t.Fatalf("delivery events for %s = %d, want 1", id, deliveries)
		}
	}

	t.Run("sent before ready", func(t *testing.T) {
		d := newTestDaemon(t)
		token := installStartingToken(t, d, "worker-emperor")
		sender, err := d.register(&protocol.Request{Type: "sender", PID: os.Getpid()})
		if err != nil {
			t.Fatal(err)
		}
		sent, err := d.send(&protocol.Request{
			From: sender.Name, To: "worker-emperor", Body: "queued before ready",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.ready(&protocol.Request{Name: "worker-emperor", Token: token}); err != nil {
			t.Fatal(err)
		}
		got, err := d.wait(&protocol.Request{
			As: "worker-emperor", Token: token, TimeoutMS: 1000,
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Message == nil || got.Message.ID != sent.ID {
			t.Fatalf("ready wait received %+v, want %s", got.Message, sent.ID)
		}
		if _, err := d.wait(&protocol.Request{
			As: "worker-emperor", Token: token, TimeoutMS: 20,
		}, nil); err == nil {
			t.Fatal("queued message was delivered twice")
		}
		assertDeliveredOnce(t, d, sent.ID)

		readyIndex, deliveredIndex := -1, -1
		for i, e := range events(t, d) {
			if e.Event == evReady && e.Name == "worker-emperor" {
				readyIndex = i
			}
			if e.Event == evDelivered && e.ID == sent.ID {
				deliveredIndex = i
			}
		}
		if readyIndex < 0 || deliveredIndex < 0 || readyIndex >= deliveredIndex {
			t.Fatalf("journal order ready=%d delivered=%d", readyIndex, deliveredIndex)
		}
	})

	t.Run("sent after ready wait parks", func(t *testing.T) {
		d := newTestDaemon(t)
		token := installStartingToken(t, d, "worker-emperor")
		sender, err := d.register(&protocol.Request{Type: "sender", PID: os.Getpid()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.ready(&protocol.Request{Name: "worker-emperor", Token: token}); err != nil {
			t.Fatal(err)
		}

		type waitResult struct {
			resp protocol.Response
			err  error
		}
		waited := make(chan waitResult, 1)
		go func() {
			resp, err := d.wait(&protocol.Request{
				As: "worker-emperor", Token: token, TimeoutMS: 2000,
			}, nil)
			waited <- waitResult{resp: resp, err: err}
		}()
		waitFor(t, func() bool {
			d.mu.Lock()
			defer d.mu.Unlock()
			return len(d.waiters) == 1
		})

		sent, err := d.send(&protocol.Request{
			From: sender.Name, To: "worker-emperor", Body: "sent after wait",
		})
		if err != nil {
			t.Fatal(err)
		}
		result := <-waited
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.resp.Message == nil || result.resp.Message.ID != sent.ID {
			t.Fatalf("parked ready wait received %+v, want %s", result.resp.Message, sent.ID)
		}
		if _, err := d.wait(&protocol.Request{
			As: "worker-emperor", Token: token, TimeoutMS: 20,
		}, nil); err == nil {
			t.Fatal("directly delivered message was delivered twice")
		}
		assertDeliveredOnce(t, d, sent.ID)
	})
}

func TestReadySignalAuthenticatesWithoutDaemonSocket(t *testing.T) {
	d := newTestDaemon(t)
	token := installStartingToken(t, d, "worker-emperor")
	if err := WriteReadySignal(d.root, d.flockName, "worker-emperor", token); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ReadySignalPath(d.root, d.flockName, "worker-emperor"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), token) {
		t.Fatal("readiness signal stored the raw token")
	}
	consumed, err := d.consumeReadySignal("worker-emperor")
	if err != nil {
		t.Fatal(err)
	}
	if !consumed || agentState(d, "worker-emperor") != stateRunning {
		t.Fatalf("consumed=%v state=%q", consumed, agentState(d, "worker-emperor"))
	}
	if _, err := os.Stat(ReadySignalPath(d.root, d.flockName, "worker-emperor")); !os.IsNotExist(err) {
		t.Fatalf("signal still exists: %v", err)
	}
}

func TestInvalidReadySignalDoesNotTransitionAgent(t *testing.T) {
	d := newTestDaemon(t)
	installStartingToken(t, d, "worker-emperor")
	if err := WriteReadySignal(d.root, d.flockName, "worker-emperor", "wrong"); err != nil {
		t.Fatal(err)
	}
	consumed, err := d.consumeReadySignal("worker-emperor")
	if !consumed || err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("consumed=%v error=%v", consumed, err)
	}
	if got := agentState(d, "worker-emperor"); got != stateStarting {
		t.Fatalf("state = %q", got)
	}
}

func TestEarlyReadyWaitsUntilSpawnedIsJournaled(t *testing.T) {
	gate := make(chan struct{})
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	f.mu.Lock()
	f.blocks = map[string]<-chan struct{}{"agent.start": gate}
	f.mu.Unlock()
	d := boundDaemon(t, f)
	d.skipReadiness = false

	spawned := make(chan error, 1)
	go func() {
		_, err := d.spawn(&protocol.Request{Model: "gpt-x", TimeoutMS: 2000})
		spawned <- err
	}()
	deadline := time.Now().Add(time.Second)
	for f.count("agent.start") == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	env := f.params("agent.start")["env"].(map[string]any)
	token := env[protocol.ReadyTokenEnv].(string)
	ready := make(chan error, 1)
	go func() {
		_, err := d.ready(&protocol.Request{Name: "pi-emperor", Token: token})
		ready <- err
	}()

	select {
	case err := <-ready:
		t.Fatalf("early readiness returned before launch resolution: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if hasEvent(t, d, evSpawned, "pi-emperor") || hasEvent(t, d, evReady, "pi-emperor") {
		t.Fatal("spawned or ready was journaled while agent.start was blocked")
	}
	close(gate)
	if err := <-ready; err != nil {
		t.Fatalf("ready: %v", err)
	}
	if err := <-spawned; err != nil {
		t.Fatalf("spawn: %v", err)
	}
	var order []string
	for _, e := range events(t, d) {
		if e.Name == "pi-emperor" {
			order = append(order, e.Event)
		}
	}
	want := []string{evRegistered, evLaunching, evSpawned, evReady}
	if len(order) != len(want) {
		t.Fatalf("event order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("event order = %v, want %v", order, want)
		}
	}
}

func TestConcurrentStopWaitsForLaunchThenClosesPane(t *testing.T) {
	gate := make(chan struct{})
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	f.mu.Lock()
	f.blocks = map[string]<-chan struct{}{"agent.start": gate}
	f.mu.Unlock()
	d := boundDaemon(t, f)
	d.skipReadiness = false

	spawned := make(chan error, 1)
	go func() {
		_, err := d.spawn(&protocol.Request{Model: "gpt-x", TimeoutMS: 2000})
		spawned <- err
	}()
	deadline := time.Now().Add(time.Second)
	for f.count("agent.start") == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	stopped := make(chan error, 1)
	go func() {
		_, err := d.stop(&protocol.Request{Name: "pi-emperor"})
		stopped <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if f.count("pane.close") != 0 {
		t.Fatal("stop closed a pane before launch resolution")
	}
	close(gate)
	if err := <-stopped; err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := <-spawned; err == nil || !strings.Contains(err.Error(), "stopped before readiness") {
		t.Fatalf("spawn error = %v", err)
	}
	if close := f.params("pane.close"); close["pane_id"] != "w1:p2" {
		t.Fatalf("pane.close = %+v", close)
	}
	if got := agentState(d, "pi-emperor"); got != stateStopped {
		t.Fatalf("state = %q, want stopped", got)
	}
}

func TestSpawnReadinessTimeoutRollsBackAndReusesSpecies(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	d.skipReadiness = false
	_, err := d.spawn(&protocol.Request{Model: "gpt-x", TimeoutMS: 10})
	if err == nil || !strings.Contains(err.Error(), "did not become ready") {
		t.Fatalf("error = %v", err)
	}
	if got := agentState(d, "pi-emperor"); got != stateStopped {
		t.Fatalf("state = %q", got)
	}
	d.skipReadiness = true
	resp, err := d.spawn(&protocol.Request{Model: "gpt-x"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Name != "pi-emperor" {
		t.Fatalf("reused name = %q", resp.Name)
	}
}

func TestStoppingStartingAgentDoesNotDeliverRolePrompt(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start":       paneStartedReply,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":4242}}}`,
	})
	d := boundDaemon(t, f)
	d.skipReadiness = false
	source := filepath.Join(d.root, scaffold.DirName, agentcfg.AgentsDir, agentcfg.UserDir, "reviewer", "reviewer.agent.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`---
name: reviewer
description: Review changes.
model: claude-opus-4
fledge:
  profile: reviewer-plan
  launch:
    permission_mode: plan
---
Do reviews.
`), 0o644); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := d.spawn(&protocol.Request{Agent: "reviewer", TimeoutMS: 2000})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for f.count("agent.start") < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if f.count("agent.start") != 1 {
		t.Fatal("agent was not launched")
	}
	if _, err := d.stop(&protocol.Request{Name: "reviewer-emperor"}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "stopped before readiness") {
			t.Fatalf("spawn error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("spawn did not wake when the starting agent stopped")
	}
	if got := f.count("pane.send_input"); got != 0 {
		t.Fatalf("pane inputs = %d, want no lifecycle input", got)
	}
}

func TestOrchestratorReadinessOnlyStartup(t *testing.T) {
	f := claudeHerdr(t)
	d := boundDaemon(t, f)
	d.skipReadiness = false
	writeAgents(t, d.root, map[string]agentcfg.Config{
		agentcfg.ReservedOrchestrator: {Integration: "claude", Model: "claude-opus-4"},
	})

	done := make(chan error, 1)
	go func() {
		_, err := d.spawn(&protocol.Request{Config: agentcfg.ReservedOrchestrator, TimeoutMS: 2000})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for f.count("agent.start") < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if f.count("agent.start") != 1 {
		t.Fatal("orchestrator was not launched")
	}
	env := f.params("agent.start")["env"].(map[string]any)
	if got := env[protocol.AgentNameEnv]; got != agentcfg.ReservedOrchestrator {
		t.Fatalf("agent name env = %v, want %q", got, agentcfg.ReservedOrchestrator)
	}
	token, _ := env[protocol.ReadyTokenEnv].(string)
	if token == "" {
		t.Fatal("start env has no readiness token")
	}
	if _, err := d.ready(&protocol.Request{Name: agentcfg.ReservedOrchestrator, Token: token}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := f.count("pane.send_input"); got != 0 {
		t.Fatalf("pane inputs after readiness = %d, want 0", got)
	}
}

func TestManagedOrchestratorReceivesAuthoritativeRoleNatively(t *testing.T) {
	f := claudeHerdr(t)
	d := boundDaemon(t, f)
	catalog, err := json.Marshal(agentcfg.Index{
		Version: agentcfg.IndexVersion, Agents: map[string]agentcfg.AgentRecord{},
		Profiles: map[string]agentcfg.Config{
			"haikucl":              {Integration: "claude", Model: "haiku"},
			"orchestrator-profile": {Integration: "claude", Model: "claude-opus-4"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d.root, scaffold.DirName, agentcfg.CatalogName), catalog, 0o644); err != nil {
		t.Fatal(err)
	}
	definition, _, err := agentcfg.FindDefinition(d.root, agentcfg.ReservedOrchestrator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.spawn(&protocol.Request{Agent: agentcfg.ReservedOrchestrator, Profile: "orchestrator-profile"}); err != nil {
		t.Fatal(err)
	}
	argv := strs(t, f.params("agent.start")["argv"])
	want := assignedAgentPrompt(agentcfg.ReservedOrchestrator, definition.Prompt)
	if len(argv) < 3 || argv[len(argv)-3] != "--append-system-prompt" || argv[len(argv)-2] != want || argv[len(argv)-1] != orchestratorBootstrapPrompt {
		t.Fatalf("orchestrator argv = %#v", argv)
	}
	sum := sha256.Sum256([]byte(want))
	launching := findEvent(t, d, evLaunching, agentcfg.ReservedOrchestrator)
	if launching.InstructionHash != hex.EncodeToString(sum[:]) {
		t.Fatalf("instruction hash = %q, want %x", launching.InstructionHash, sum)
	}
	if got := f.count("pane.send_input"); got != 0 {
		t.Fatalf("orchestrator lifecycle pane inputs = %d, want 0", got)
	}
}

func TestListAndSummaryLogTrackSpawnReadyStop(t *testing.T) {
	f := claudeHerdr(t)
	d := boundDaemon(t, f)
	d.skipReadiness = false
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"worker": {Integration: "claude", Model: "claude-opus-4"},
	})

	type spawnResult struct {
		resp protocol.Response
		err  error
	}
	spawned := make(chan spawnResult, 1)
	go func() {
		resp, err := d.spawn(&protocol.Request{Config: "worker", TimeoutMS: 2000})
		spawned <- spawnResult{resp: resp, err: err}
	}()

	waitFor(t, func() bool {
		for _, a := range d.list() {
			if a.Name == "worker-emperor" && a.State == stateStarting && a.PaneID != "" {
				return true
			}
		}
		return false
	})
	starting := d.list()
	if len(starting) != 1 || starting[0].Name != "worker-emperor" || starting[0].State != stateStarting {
		t.Fatalf("starting roster = %+v", starting)
	}

	logPath := filepath.Join(flock.Dir(d.root, d.flockName), protocol.LogName)
	readLog := func() string {
		t.Helper()
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if got := strings.Count(readLog(), "started: 0 agents, 0 pending"); got != 1 {
		t.Fatalf("initial summary count = %d, want 1", got)
	}
	if got := strings.Count(readLog(), "started: 1 agents, 0 pending"); got != 1 {
		t.Fatalf("spawn summary count = %d, want 1", got)
	}

	env := f.params("agent.start")["env"].(map[string]any)
	token, _ := env[protocol.ReadyTokenEnv].(string)
	if token == "" {
		t.Fatalf("start env has no readiness token: %v", env)
	}
	if _, err := d.ready(&protocol.Request{Name: "worker-emperor", Token: token}); err != nil {
		t.Fatal(err)
	}
	result := <-spawned
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.resp.Name != "worker-emperor" {
		t.Fatalf("spawn response = %+v", result.resp)
	}
	running := d.list()
	if len(running) != 1 || running[0].State != stateRunning {
		t.Fatalf("running roster = %+v", running)
	}
	if got := strings.Count(readLog(), "started: 1 agents, 0 pending"); got != 2 {
		t.Fatalf("spawn plus ready summary count = %d, want 2", got)
	}

	sender, err := d.register(&protocol.Request{Type: "sender", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.send(&protocol.Request{
		From: sender.Name, To: "worker-emperor", Body: "still pending at stop",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.stop(&protocol.Request{Name: "worker-emperor"}); err != nil {
		t.Fatal(err)
	}
	stopped := d.list()
	var workerState string
	for _, a := range stopped {
		if a.Name == "worker-emperor" {
			workerState = a.State
		}
	}
	if len(stopped) != 2 || workerState != stateStopped {
		t.Fatalf("stopped roster = %+v", stopped)
	}
	if got := strings.Count(readLog(), "started: 2 agents, 1 pending"); got != 1 {
		t.Fatalf("stop summary with current counts = %d, want 1", got)
	}
}

func TestOrchestratorReadinessRecoverySkipsRolePrompt(t *testing.T) {
	d := newTestDaemon(t)
	token := installStartingToken(t, d, agentcfg.ReservedOrchestrator)
	d.mu.Lock()
	delete(d.readyWaiters, agentcfg.ReservedOrchestrator)
	d.mu.Unlock()
	if _, err := d.ready(&protocol.Request{Name: agentcfg.ReservedOrchestrator, Token: token}); err != nil {
		t.Fatal(err)
	}
	if got := countEvents(t, d, evSent, agentcfg.ReservedOrchestrator); got != 0 {
		t.Fatalf("recovery sent events = %d, want 0", got)
	}
}

func TestSpawnInjectsClaudeRoleAndBootstrapAtLaunch(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start":       paneStartedReply,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":4242}}}`,
	})
	d := boundDaemon(t, f)
	d.skipReadiness = false
	source := filepath.Join(d.root, scaffold.DirName, agentcfg.AgentsDir, agentcfg.UserDir, "reviewer", "reviewer.agent.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`---
name: reviewer
description: Review changes.
model: claude-opus-4
fledge:
  profile: reviewer-plan
  launch:
    permission_mode: plan
---
Do reviews.
`), 0o644); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := d.spawn(&protocol.Request{Agent: "reviewer", TimeoutMS: 2000})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for f.count("agent.start") < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	start := f.params("agent.start")
	env := start["env"].(map[string]any)
	token, _ := env[protocol.ReadyTokenEnv].(string)
	if token == "" {
		t.Fatalf("start env has no readiness token: %v", env)
	}
	if _, err := d.ready(&protocol.Request{Name: "reviewer-emperor", Token: token}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	replayed, err := replay(journalPath(d.root, d.flockName))
	if err != nil {
		t.Fatal(err)
	}
	ra := replayed.agents["reviewer-emperor"]
	if ra.Agent != "reviewer" || ra.Profile != "reviewer-plan" || ra.Source != "user/reviewer/reviewer.agent.md" || ra.State != stateRunning {
		t.Fatalf("replayed agent metadata = %+v", ra)
	}

	argv := strs(t, start["argv"])
	wantInstructions := assignedAgentPrompt("reviewer-emperor", "Do reviews.\n")
	if len(argv) < 3 || argv[len(argv)-3] != "--append-system-prompt" || argv[len(argv)-2] != wantInstructions || argv[len(argv)-1] != bootstrapPrompt {
		t.Fatalf("launch argv tail = %#v", argv)
	}
	if got := f.count("pane.send_input"); got != 0 {
		t.Fatalf("lifecycle pane inputs = %d, want 0", got)
	}
	if got := countEvents(t, d, evSent, "reviewer-emperor"); got != 0 {
		t.Fatalf("lifecycle msg.sent events = %d, want 0", got)
	}
}

func TestSpawnInjectsPiRoleAndBootstrapAtLaunch(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start":       paneStartedReply,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":4242}}}`,
	})
	d := boundDaemon(t, f)
	d.skipReadiness = false
	source := filepath.Join(d.root, scaffold.DirName, agentcfg.AgentsDir, agentcfg.UserDir, "pi-reviewer", "pi-reviewer.agent.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`---
name: pi-reviewer
description: Review through Pi.
model: gpt-x
fledge:
  profile: pi-review
---
Review through Pi.
`), 0o644); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := d.spawn(&protocol.Request{Agent: "pi-reviewer", TimeoutMS: 2000})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for f.count("agent.start") < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	start := f.params("agent.start")
	if argv := start["argv"].([]any); argv[0] != "pi" {
		t.Fatalf("argv = %v, want a pi launch", argv)
	}
	env := start["env"].(map[string]any)
	if got := env[protocol.AgentNameEnv]; got != "pi-reviewer-emperor" {
		t.Fatalf("agent name env = %v, want pi-reviewer-emperor", got)
	}
	token, _ := env[protocol.ReadyTokenEnv].(string)
	if token == "" {
		t.Fatalf("start env has no readiness token: %v", env)
	}
	if _, err := d.ready(&protocol.Request{Name: "pi-reviewer-emperor", Token: token}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	argv := strs(t, start["argv"])
	wantInstructions := assignedAgentPrompt("pi-reviewer-emperor", "Review through Pi.\n")
	if len(argv) < 3 || argv[len(argv)-3] != "--append-system-prompt" || argv[len(argv)-2] != wantInstructions || argv[len(argv)-1] != bootstrapPrompt {
		t.Fatalf("launch argv tail = %#v", argv)
	}
	if got := f.count("pane.send_input"); got != 0 {
		t.Fatalf("lifecycle pane inputs = %d, want 0", got)
	}
}

func TestSpawnDoesNotPollPaneStatusOrSendLifecycleInput(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start": paneStartedReply,
		"agent.get":   `{"id":"1","result":{"type":"agent","agent":{"pane_id":"w1:p2","agent_status":"unknown"}}}`,
	})
	d := boundDaemon(t, f)
	d.skipReadiness = false

	done := make(chan error, 1)
	go func() {
		_, err := d.spawn(&protocol.Request{Model: "gpt-x", TimeoutMS: 2000})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for f.count("agent.start") < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	env := f.params("agent.start")["env"].(map[string]any)
	token, _ := env[protocol.ReadyTokenEnv].(string)
	if token == "" {
		t.Fatal("start env has no readiness token")
	}
	if _, err := d.ready(&protocol.Request{Name: "pi-emperor", Token: token}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("spawn failed though the readiness handshake completed: %v", err)
	}
	if f.count("agent.get") != 0 || f.count("pane.send_input") != 0 {
		t.Fatalf("startup methods = %v, want no status poll or pane input", f.methods())
	}
}

func TestRawSpawnReceivesAssignedNameAndMessageWaitPrompt(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"agent.start":       paneStartedReply,
		"pane.process_info": `{"id":"1","result":{"process_info":{"shell_pid":4242}}}`,
	})
	d := boundDaemon(t, f)
	d.skipReadiness = false
	writeCatalog(t, d.root, map[string]agentcfg.Config{
		"raw": {Integration: "claude", Model: "claude-opus-4"},
	})

	done := make(chan error, 1)
	go func() {
		_, err := d.spawn(&protocol.Request{Profile: "raw", TimeoutMS: 2000})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for f.count("agent.start") < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	env := f.params("agent.start")["env"].(map[string]any)
	token, _ := env[protocol.ReadyTokenEnv].(string)
	if token == "" {
		t.Fatalf("start env has no readiness token: %v", env)
	}
	if _, err := d.ready(&protocol.Request{Name: "raw-emperor", Token: token}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	want := assignedAgentPrompt("raw-emperor", "")
	argv := strs(t, f.params("agent.start")["argv"])
	if len(argv) < 3 || argv[len(argv)-2] != want || argv[len(argv)-1] != bootstrapPrompt {
		t.Fatalf("raw launch argv = %#v", argv)
	}
	for _, text := range []string{"already registered", "`raw-emperor`", "Direct messages arrive", "fledge agent msg inbox", "fledge agent msg reply <message-id> <body>"} {
		if !strings.Contains(want, text) {
			t.Errorf("assigned-name instructions missing %q: %q", text, want)
		}
	}
	for _, text := range []string{"fledge agent msg wait", "--as raw-emperor", "--from raw-emperor"} {
		if strings.Contains(want, text) {
			t.Errorf("assigned-name instructions still contain removed instruction %q: %q", text, want)
		}
	}
}
