package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

func armOrchestrator(t *testing.T, d *Daemon, wake inboxWakeFunc) (string, protocol.Agent) {
	t.Helper()
	d.mu.Lock()
	d.inboxWake = wake
	d.mu.Unlock()
	token := installStartingToken(t, d, agentcfg.ReservedOrchestrator)
	if _, err := d.ready(&protocol.Request{
		Name: agentcfg.ReservedOrchestrator, Token: token, NoWait: true,
	}); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	agent := d.agents[agentcfg.ReservedOrchestrator]
	d.mu.Unlock()
	return token, agent
}

func TestOrchestratorReadinessAndArmingAreOneReplayAtomicEvent(t *testing.T) {
	d := newTestDaemon(t)
	armOrchestrator(t, d, func(context.Context, inboxWakeTarget, inboxWakeMetadata) error { return nil })

	var ready []event
	for _, e := range events(t, d) {
		if e.Event == evReady && e.Name == agentcfg.ReservedOrchestrator {
			ready = append(ready, e)
		}
		if e.Event == "inbox.notify.armed" {
			t.Fatalf("readiness emitted split arming event: %+v", e)
		}
	}
	if len(ready) != 1 || !ready[0].InboxNotifyArmed {
		t.Fatalf("ready events = %+v", ready)
	}
	replayed, err := replay(journalPath(d.root, d.flockName))
	if err != nil {
		t.Fatal(err)
	}
	if replayed.agents[agentcfg.ReservedOrchestrator].State != stateRunning ||
		!replayed.inboxNotifyArmed[agentcfg.ReservedOrchestrator] {
		t.Fatalf("replayed readiness/arming = agent %+v armed=%v",
			replayed.agents[agentcfg.ReservedOrchestrator],
			replayed.inboxNotifyArmed[agentcfg.ReservedOrchestrator])
	}
}

func TestInboxWakeUsesIntegrationSessionAndBodyFreeMetadata(t *testing.T) {
	d := newTestDaemon(t)
	wakes := make(chan struct {
		target inboxWakeTarget
		meta   inboxWakeMetadata
	}, 2)
	token, agent := armOrchestrator(t, d, func(_ context.Context, target inboxWakeTarget, meta inboxWakeMetadata) error {
		wakes <- struct {
			target inboxWakeTarget
			meta   inboxWakeMetadata
		}{target, meta}
		return nil
	})
	sender, err := d.register(&protocol.Request{Type: "sender", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	sent, err := d.send(&protocol.Request{
		From: sender.Name, To: agentcfg.ReservedOrchestrator, Body: "never leave the mailbox",
	})
	if err != nil {
		t.Fatal(err)
	}

	var wake struct {
		target inboxWakeTarget
		meta   inboxWakeMetadata
	}
	select {
	case wake = <-wakes:
	case <-time.After(time.Second):
		t.Fatal("no integration wake")
	}
	if wake.target.Integration != "claude" || wake.target.SessionID != agent.SessionID ||
		wake.target.Name != agentcfg.ReservedOrchestrator {
		t.Fatalf("target = %+v", wake.target)
	}
	raw, _ := json.Marshal(wake.meta)
	if strings.Contains(string(raw), "never leave the mailbox") {
		t.Fatalf("wake leaked body: %s", raw)
	}
	if len(wake.meta.Messages) != 1 || wake.meta.Messages[0].ID != sent.ID {
		t.Fatalf("metadata = %+v", wake.meta)
	}
	got, err := d.inbox(&protocol.Request{
		As: agentcfg.ReservedOrchestrator, Token: token,
	})
	if err != nil || got.Message == nil || got.Message.Body != "never leave the mailbox" {
		t.Fatalf("inbox = %+v, %v", got.Message, err)
	}
}

func TestOrchestratorReadinessDegradesWithoutOwnedSameSessionChannel(t *testing.T) {
	d := newTestDaemon(t)
	token := installStartingToken(t, d, agentcfg.ReservedOrchestrator)
	resp, err := d.ready(&protocol.Request{
		Name: agentcfg.ReservedOrchestrator, Token: token, NoWait: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.InboxDelivery != "manual" {
		t.Fatalf("inbox delivery = %q, want manual", resp.InboxDelivery)
	}
	d.mu.Lock()
	armed := d.inboxNotifyArmed[agentcfg.ReservedOrchestrator]
	tasks := len(d.inboxNotifyTasks)
	d.mu.Unlock()
	if armed || tasks != 0 {
		t.Fatalf("degraded readiness armed=%v tasks=%d", armed, tasks)
	}
	ready := findEvent(t, d, evReady, agentcfg.ReservedOrchestrator)
	if ready.InboxNotifyArmed {
		t.Fatalf("degraded ready event claimed arming: %+v", ready)
	}
}

func TestInboxWakeQueueCoalescesAndSendDoesNotBlock(t *testing.T) {
	d := newTestDaemon(t)
	block := make(chan struct{})
	started := make(chan struct{}, 1)
	armOrchestrator(t, d, func(ctx context.Context, _ inboxWakeTarget, _ inboxWakeMetadata) error {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-block:
			return nil
		}
	})
	sender, err := d.register(&protocol.Request{Type: "sender", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	before := runtime.NumGoroutine()
	for i := 0; i < 200; i++ {
		if _, err := d.send(&protocol.Request{
			From: sender.Name, To: agentcfg.ReservedOrchestrator, Body: "body",
		}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("wake did not start")
	}
	d.mu.Lock()
	queued := len(d.inboxNotifyTasks)
	d.mu.Unlock()
	if queued > 1 {
		t.Fatalf("coalesced queue length = %d, want at most 1", queued)
	}
	if growth := runtime.NumGoroutine() - before; growth > 4 {
		t.Fatalf("goroutine growth = %d for 200 messages", growth)
	}
	close(block)
}

type toggleJournal struct {
	io.WriteCloser
	fail atomic.Bool
}

func (w *toggleJournal) Write(p []byte) (int, error) {
	if w.fail.Load() {
		return 0, errors.New("injected journal failure")
	}
	return w.WriteCloser.Write(p)
}

func TestInboxWakeRetriesExternalSuccessJournalFailureAndClaimsExactlyOnce(t *testing.T) {
	d := newTestDaemon(t)
	base := d.journal
	journal := &toggleJournal{WriteCloser: base}
	d.journal = journal
	var calls atomic.Int32
	var claimed atomic.Int32
	armOrchestrator(t, d, func(_ context.Context, target inboxWakeTarget, _ inboxWakeMetadata) error {
		n := calls.Add(1)
		resp, err := d.inbox(&protocol.Request{
			As: target.Name, Credential: target.Credential,
		})
		if err != nil {
			return err
		}
		if resp.Message != nil {
			claimed.Add(1)
		}
		if n == 1 {
			journal.fail.Store(true)
			time.AfterFunc(2*inboxNotifyInitialBackoff, func() { journal.fail.Store(false) })
		}
		return nil
	})
	sender, err := d.register(&protocol.Request{Type: "sender", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.send(&protocol.Request{
		From: sender.Name, To: agentcfg.ReservedOrchestrator, Body: "once",
	}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return calls.Load() >= 2 })
	waitFor(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.inboxNotified) == 1
	})
	if got := claimed.Load(); got != 1 {
		t.Fatalf("message claims = %d, want exactly 1", got)
	}
}

func TestInboxWakeCloseCancelsAndJoinsWorker(t *testing.T) {
	d := newTestDaemon(t)
	started := make(chan struct{})
	var once sync.Once
	var calls atomic.Int32
	armOrchestrator(t, d, func(ctx context.Context, _ inboxWakeTarget, _ inboxWakeMetadata) error {
		calls.Add(1)
		once.Do(func() { close(started) })
		<-ctx.Done()
		return ctx.Err()
	})
	sender, err := d.register(&protocol.Request{Type: "sender", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.send(&protocol.Request{
		From: sender.Name, To: agentcfg.ReservedOrchestrator, Body: "shutdown",
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("wake did not start")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	before := calls.Load()
	for i := 0; i < 100; i++ {
		d.queueInboxWake(protocol.Message{ID: "late", To: agentcfg.ReservedOrchestrator})
	}
	time.Sleep(3 * inboxNotifyInitialBackoff)
	if got := calls.Load(); got != before {
		t.Fatalf("wake count after Close = %d, want %d", got, before)
	}
}

func TestStopCancelsAndJoinsAgentInboxWork(t *testing.T) {
	f := serveHerdr(t, nil)
	d := boundDaemon(t, f)
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	var cancelOnce sync.Once
	armOrchestrator(t, d, func(ctx context.Context, _ inboxWakeTarget, _ inboxWakeMetadata) error {
		startOnce.Do(func() { close(started) })
		<-ctx.Done()
		cancelOnce.Do(func() { close(canceled) })
		<-release
		return ctx.Err()
	})
	sender, err := d.register(&protocol.Request{Type: "sender", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.send(&protocol.Request{
		From: sender.Name, To: agentcfg.ReservedOrchestrator, Body: "cancel on stop",
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("inbox work did not start")
	}

	stopped := make(chan error, 1)
	go func() {
		_, err := d.stop(&protocol.Request{Name: agentcfg.ReservedOrchestrator})
		stopped <- err
	}()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel inbox work")
	}
	select {
	case err := <-stopped:
		t.Fatalf("stop returned before inbox work joined: %v", err)
	default:
	}
	close(release)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if got := agentState(d, agentcfg.ReservedOrchestrator); got != stateStopped {
		t.Fatalf("state = %q, want stopped", got)
	}
}

func TestReplayQueuesDurableUnnotifiedWake(t *testing.T) {
	const flockName = "test"
	root := t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(flock.Dir(root, flockName), 0o700); err != nil {
		t.Fatal(err)
	}
	journal := strings.Join([]string{
		`{"event":"agent.registered","name":"fledge-orchestrator","type":"fledge-orchestrator","pid":-1}`,
		`{"event":"agent.launching","name":"fledge-orchestrator","integration":"codex","cwd":"/tmp"}`,
		`{"event":"agent.spawned","name":"fledge-orchestrator","pane_id":"w1:p2"}`,
		`{"event":"agent.ready","name":"fledge-orchestrator","session_id":"019f9131-984b-7a33-b67d-85a17555033d","inbox_notify_armed":true}`,
		`{"event":"agent.registered","name":"sender-emperor","type":"sender","species":"emperor","pid":123}`,
		`{"event":"msg.sent","id":"m1","from":"sender-emperor","to":"fledge-orchestrator","body":"durable"}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(flock.Dir(root, flockName), protocol.JournalName), []byte(journal), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := New(root, flockName)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	woke := make(chan inboxWakeTarget, 1)
	d.mu.Lock()
	d.inboxWake = func(_ context.Context, target inboxWakeTarget, _ inboxWakeMetadata) error {
		woke <- target
		return nil
	}
	d.mu.Unlock()
	d.replayInboxNotifications()
	select {
	case target := <-woke:
		if target.Integration != "codex" || target.SessionID == "" {
			t.Fatalf("replayed target = %+v", target)
		}
	case <-time.After(time.Second):
		t.Fatal("replayed durable wake did not run")
	}
}

func TestUnsupportedIntegrationDegradesInsteadOfStartingBackgroundProcess(t *testing.T) {
	d := newTestDaemon(t)
	token := installStartingToken(t, d, agentcfg.ReservedOrchestrator)
	d.mu.Lock()
	a := d.agents[agentcfg.ReservedOrchestrator]
	a.SessionID = ""
	a.Integration = "unsupported"
	d.agents[agentcfg.ReservedOrchestrator] = a
	d.mu.Unlock()
	resp, err := d.ready(&protocol.Request{
		Name: agentcfg.ReservedOrchestrator, Token: token, NoWait: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.InboxDelivery != "manual" {
		t.Fatalf("inbox delivery = %q", resp.InboxDelivery)
	}
}

func TestOwnedAdapterRejectsUnsupportedIntegrationCapability(t *testing.T) {
	d := newTestDaemon(t)
	d.mu.Lock()
	d.inboxWake = func(context.Context, inboxWakeTarget, inboxWakeMetadata) error { return nil }
	d.mu.Unlock()
	token := installStartingToken(t, d, agentcfg.ReservedOrchestrator)
	d.mu.Lock()
	a := d.agents[agentcfg.ReservedOrchestrator]
	a.Integration = "unsupported"
	d.agents[agentcfg.ReservedOrchestrator] = a
	d.mu.Unlock()
	if _, err := d.ready(&protocol.Request{
		Name: agentcfg.ReservedOrchestrator, Token: token, NoWait: true,
	}); err == nil || !strings.Contains(err.Error(), "no same-session adapter") {
		t.Fatalf("ready error = %v", err)
	}
}

func TestCodexArmingPersistsRuntimeThreadIDFromReadiness(t *testing.T) {
	d := newTestDaemon(t)
	d.mu.Lock()
	d.inboxWake = func(context.Context, inboxWakeTarget, inboxWakeMetadata) error { return nil }
	d.mu.Unlock()
	token := installStartingToken(t, d, agentcfg.ReservedOrchestrator)
	d.mu.Lock()
	a := d.agents[agentcfg.ReservedOrchestrator]
	a.Integration = "codex"
	a.SessionID = ""
	d.agents[agentcfg.ReservedOrchestrator] = a
	d.mu.Unlock()
	const threadID = "019f9131-984b-7a33-b67d-85a17555033d"
	if _, err := d.ready(&protocol.Request{
		Name: agentcfg.ReservedOrchestrator, Token: token, NoWait: true, SessionID: threadID,
	}); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	got := d.agents[agentcfg.ReservedOrchestrator]
	d.mu.Unlock()
	if got.SessionID != threadID {
		t.Fatalf("runtime thread id = %q", got.SessionID)
	}
	replayed, err := replay(journalPath(d.root, d.flockName))
	if err != nil {
		t.Fatal(err)
	}
	if replayed.agents[agentcfg.ReservedOrchestrator].SessionID != threadID ||
		!replayed.inboxNotifyArmed[agentcfg.ReservedOrchestrator] {
		t.Fatalf("replayed codex = %+v armed=%v",
			replayed.agents[agentcfg.ReservedOrchestrator],
			replayed.inboxNotifyArmed[agentcfg.ReservedOrchestrator])
	}
}
