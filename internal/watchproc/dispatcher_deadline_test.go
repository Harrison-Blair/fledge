package watchproc

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
)

type deadlineClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *deadlineClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *deadlineClock) Set(at time.Time) {
	c.mu.Lock()
	c.now = at
	c.mu.Unlock()
}

type manualDeadlineTimer struct {
	mu      sync.Mutex
	channel chan time.Time
	delay   time.Duration
	active  bool
	stops   chan struct{}
}

func newManualDeadlineTimer(delay time.Duration) *manualDeadlineTimer {
	return &manualDeadlineTimer{
		channel: make(chan time.Time, 1), delay: delay, active: true, stops: make(chan struct{}, 16),
	}
}

func (t *manualDeadlineTimer) C() <-chan time.Time { return t.channel }

func (t *manualDeadlineTimer) Stop() bool {
	t.mu.Lock()
	wasActive := t.active
	t.active = false
	t.mu.Unlock()
	t.stops <- struct{}{}
	return wasActive
}

func (t *manualDeadlineTimer) Reset(delay time.Duration) bool {
	t.mu.Lock()
	wasActive := t.active
	t.delay = delay
	t.active = true
	t.mu.Unlock()
	return wasActive
}

func (t *manualDeadlineTimer) Delay() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.delay
}

func (t *manualDeadlineTimer) Fire(at time.Time) bool {
	t.mu.Lock()
	if !t.active {
		t.mu.Unlock()
		return false
	}
	t.active = false
	t.mu.Unlock()
	t.channel <- at
	return true
}

func (t *manualDeadlineTimer) drainStops() {
	for {
		select {
		case <-t.stops:
		default:
			return
		}
	}
}

func (t *manualDeadlineTimer) awaitStop(test *testing.T) {
	test.Helper()
	select {
	case <-t.stops:
	case <-time.After(5 * time.Second):
		test.Fatal("dispatcher did not stop/reset its deadline timer")
	}
}

// deadlineSelectBarrier stops the run loop immediately before one select and
// reports whether the deadline channel is eligible in that exact selection.
// Tests arm it before a ledger event, so no elapsed-time inference is needed to
// prove that a replacement subscription's pending onReady gates an overdue
// timer.
type deadlineSelectBarrier struct {
	mu       sync.Mutex
	observed chan bool
	release  chan struct{}
}

func (b *deadlineSelectBarrier) arm() (<-chan bool, chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.observed != nil {
		panic("deadline select barrier already armed")
	}
	b.observed = make(chan bool, 1)
	b.release = make(chan struct{})
	return b.observed, b.release
}

func (b *deadlineSelectBarrier) observe(enabled bool) {
	b.mu.Lock()
	observed, release := b.observed, b.release
	if observed != nil {
		b.observed, b.release = nil, nil
	}
	b.mu.Unlock()
	if observed == nil {
		return
	}
	observed <- enabled
	<-release
}

type deadlineHerdr struct {
	mu                   sync.Mutex
	snapshot             herdr.Snapshot
	snapshotCalls        int
	blockedSnapshotCall  int
	capturedSnapshotCall int
	snapshotStarted      chan struct{}
	releaseSnapshot      chan struct{}
	promptCalls          int
	blockedPromptCall    int
	promptStarted        chan struct{}
	releasePrompt        chan struct{}
	refuse               map[string]error
	prompts              chan string
}

func newDeadlineHerdr(panes ...herdr.Pane) *deadlineHerdr {
	return &deadlineHerdr{snapshot: herdr.Snapshot{Panes: panes}, prompts: make(chan string, 32)}
}

func (h *deadlineHerdr) Protocol(context.Context) (int, error) {
	return RequiredHerdrProtocol, nil
}

func (h *deadlineHerdr) List(context.Context) ([]herdr.Session, error) { return nil, nil }

func (h *deadlineHerdr) Snapshot(ctx context.Context, _ string) (herdr.Snapshot, error) {
	h.mu.Lock()
	h.snapshotCalls++
	blocked := h.snapshotCalls == h.blockedSnapshotCall
	captured := h.snapshotCalls == h.capturedSnapshotCall
	result := herdr.Snapshot{}
	if captured {
		result = h.snapshot
		result.Panes = append([]herdr.Pane(nil), h.snapshot.Panes...)
	}
	started := h.snapshotStarted
	release := h.releaseSnapshot
	if blocked {
		h.mu.Unlock()
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return herdr.Snapshot{}, ctx.Err()
		}
		h.mu.Lock()
	}
	if !captured {
		result = h.snapshot
		result.Panes = append([]herdr.Pane(nil), h.snapshot.Panes...)
	}
	h.mu.Unlock()
	return result, nil
}

func (h *deadlineHerdr) PromptAgent(ctx context.Context, _, recipient, prompt string) error {
	h.mu.Lock()
	h.promptCalls++
	blocked := h.promptCalls == h.blockedPromptCall
	started := h.promptStarted
	release := h.releasePrompt
	err := h.refuse[recipient]
	if blocked {
		h.mu.Unlock()
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		h.mu.Lock()
	}
	h.mu.Unlock()
	if err != nil {
		return err
	}
	h.prompts <- prompt
	return nil
}

func (h *deadlineHerdr) setPanes(panes ...herdr.Pane) {
	h.mu.Lock()
	h.snapshot = herdr.Snapshot{Panes: append([]herdr.Pane(nil), panes...)}
	h.mu.Unlock()
}

func (h *deadlineHerdr) blockSnapshot(call int) (<-chan struct{}, chan<- struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.blockedSnapshotCall = call
	h.snapshotStarted = make(chan struct{})
	h.releaseSnapshot = make(chan struct{})
	return h.snapshotStarted, h.releaseSnapshot
}

func (h *deadlineHerdr) blockCapturedSnapshot(call int) (<-chan struct{}, chan<- struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.blockedSnapshotCall = call
	h.capturedSnapshotCall = call
	h.snapshotStarted = make(chan struct{})
	h.releaseSnapshot = make(chan struct{})
	return h.snapshotStarted, h.releaseSnapshot
}

func (h *deadlineHerdr) blockPrompt(call int) (<-chan struct{}, chan<- struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.blockedPromptCall = call
	h.promptStarted = make(chan struct{})
	h.releasePrompt = make(chan struct{})
	return h.promptStarted, h.releasePrompt
}

func (h *deadlineHerdr) setRefusal(recipient string, err error) {
	h.mu.Lock()
	if err == nil {
		delete(h.refuse, recipient)
	} else {
		if h.refuse == nil {
			h.refuse = make(map[string]error)
		}
		h.refuse[recipient] = err
	}
	h.mu.Unlock()
}

type deadlineHarness struct {
	root    string
	store   *messaging.Store
	client  *deadlineHerdr
	clock   *deadlineClock
	files   *fakeFiles
	events  chan herdr.Event
	queued  chan struct{}
	applied chan struct{}
	streams chan deadlineStream
	created chan *manualDeadlineTimer
	ready   chan struct{}
	done    chan error
	cancel  context.CancelFunc
	stop    sync.Once
}

type deadlineStream struct {
	panes []string
	event func(herdr.Event)
	done  <-chan struct{}
	ack   chan struct{}
}

func launchDeadlineHarness(t *testing.T, root string, clock *deadlineClock, client *deadlineHerdr) *deadlineHarness {
	return launchDeadlineHarnessWithConfig(t, root, clock, client, deadlineHarnessConfig{})
}

type deadlineHarnessConfig struct {
	gateSubscriptions bool
	selectPrepared    func(bool)
}

func launchDeadlineHarnessWithConfig(t *testing.T, root string, clock *deadlineClock, client *deadlineHerdr, config deadlineHarnessConfig) *deadlineHarness {
	t.Helper()
	h := &deadlineHarness{
		root: root, store: messaging.New(root, testSession, messaging.WithClock(clock.Now)),
		client: client, clock: clock,
		files:   &fakeFiles{events: make(chan struct{}, 8), errs: make(chan error, 1)},
		events:  make(chan herdr.Event, 8),
		queued:  make(chan struct{}, 32),
		applied: make(chan struct{}, 32),
		streams: make(chan deadlineStream, 8),
		created: make(chan *manualDeadlineTimer, 8),
		ready:   make(chan struct{}, 1), done: make(chan error, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() {
		h.done <- runDispatcher(ctx, Options{
			Root: root, Session: testSession, Herdr: client,
			WatchFile: func(string) (FileWatcher, error) { return h.files, nil },
			Subscribe: func(streamCtx context.Context, panes []string, onReady func(), onEvent func(herdr.Event)) error {
				ack := make(chan struct{})
				if !config.gateSubscriptions {
					close(ack)
				}
				select {
				case h.streams <- deadlineStream{panes: append([]string(nil), panes...), event: onEvent, done: streamCtx.Done(), ack: ack}:
				case <-streamCtx.Done():
					return streamCtx.Err()
				}
				select {
				case <-ack:
				case <-streamCtx.Done():
					return streamCtx.Err()
				}
				onReady()
				for {
					select {
					case <-streamCtx.Done():
						return streamCtx.Err()
					case event := <-h.events:
						onEvent(event)
						h.queued <- struct{}{}
					}
				}
			},
			Ready:          func() { h.ready <- struct{}{} },
			clock:          clock.Now,
			selectPrepared: config.selectPrepared,
			newTimer: func(delay time.Duration) dispatcherTimer {
				timer := newManualDeadlineTimer(delay)
				h.created <- timer
				return timer
			},
			eventApplied: func() { h.applied <- struct{}{} },
		})
	}()
	t.Cleanup(func() { h.stopDispatcher(t) })
	return h
}

func startDeadlineHarness(t *testing.T, root string, clock *deadlineClock, client *deadlineHerdr) *deadlineHarness {
	t.Helper()
	h := launchDeadlineHarness(t, root, clock, client)
	h.awaitReady(t)
	return h
}

func (h *deadlineHarness) awaitReady(t *testing.T) {
	t.Helper()
	select {
	case <-h.ready:
	case err := <-h.done:
		t.Fatalf("dispatcher exited before ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher did not become ready")
	}
}

func (h *deadlineHarness) queueEvent(t *testing.T, event herdr.Event) {
	t.Helper()
	select {
	case h.events <- event:
	case err := <-h.done:
		t.Fatalf("dispatcher exited before accepting stream event: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("subscription did not accept stream event")
	}
	select {
	case <-h.queued:
	case err := <-h.done:
		t.Fatalf("dispatcher exited before queueing stream event: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("subscription did not queue stream event")
	}
}

func (h *deadlineHarness) awaitApplied(t *testing.T) {
	t.Helper()
	select {
	case <-h.applied:
	case err := <-h.done:
		t.Fatalf("dispatcher exited before applying stream event: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher did not apply stream event")
	}
}

func (h *deadlineHarness) awaitStream(t *testing.T, wantPanes ...string) deadlineStream {
	t.Helper()
	select {
	case stream := <-h.streams:
		if strings.Join(stream.panes, "\x00") != strings.Join(wantPanes, "\x00") {
			t.Fatalf("subscription panes = %#v, want %#v", stream.panes, wantPanes)
		}
		return stream
	case err := <-h.done:
		t.Fatalf("dispatcher exited before subscribing to %#v: %v", wantPanes, err)
	case <-time.After(5 * time.Second):
		t.Fatalf("dispatcher did not subscribe to %#v", wantPanes)
	}
	return deadlineStream{}
}

func awaitSelectBoundary(t *testing.T, observed <-chan bool, release chan struct{}, wantDeadline bool) {
	t.Helper()
	select {
	case enabled := <-observed:
		close(release)
		if enabled != wantDeadline {
			t.Fatalf("deadline selectable = %v, want %v", enabled, wantDeadline)
		}
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("dispatcher did not reach the selected run-loop boundary")
	}
}

func (h *deadlineHarness) stopDispatcher(t *testing.T) {
	t.Helper()
	h.stop.Do(func() {
		h.cancel()
		select {
		case err := <-h.done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("dispatcher stopped with %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("dispatcher did not stop")
		}
	})
}

func (h *deadlineHarness) awaitTimer(t *testing.T) *manualDeadlineTimer {
	t.Helper()
	select {
	case timer := <-h.created:
		timer.drainStops()
		return timer
	case err := <-h.done:
		t.Fatalf("dispatcher exited before scheduling: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher did not schedule deadline")
	}
	return nil
}

func (h *deadlineHarness) awaitPrompt(t *testing.T, kind string) string {
	t.Helper()
	select {
	case prompt := <-h.client.prompts:
		if !strings.Contains(prompt, "Kind: "+kind) {
			t.Fatalf("prompt = %q, want kind %s", prompt, kind)
		}
		return prompt
	case err := <-h.done:
		t.Fatalf("dispatcher exited before %s prompt: %v", kind, err)
	case <-time.After(5 * time.Second):
		t.Fatalf("dispatcher did not deliver %s prompt", kind)
	}
	return ""
}

func seedDeadlineTask(t *testing.T, root string, clock *deadlineClock) (*messaging.Store, messaging.Task) {
	t.Helper()
	store := messaging.New(root, testSession, messaging.WithClock(clock.Now))
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	for _, params := range []messaging.RegisterParams{
		{Name: messaging.OrchestratorIdentity, PaneID: "p1", Harness: "codex", Caller: messaging.UserIdentity, CanDelegate: true},
		{Name: "worker", PaneID: "p2", Harness: "codex", Caller: messaging.OrchestratorIdentity},
	} {
		if _, _, err := store.RegisterAgent(params); err != nil {
			t.Fatal(err)
		}
	}
	task, err := store.AssignTask(messaging.OrchestratorIdentity, "worker", "", "do the thing")
	if err != nil {
		t.Fatal(err)
	}
	return store, task
}

func TestDispatcherDelaysPreStartIdleAndDeliversOneExactDeadlineAlert(t *testing.T) {
	clock := &deadlineClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	root := t.TempDir()
	store, task := seedDeadlineTask(t, root, clock)
	if err := store.RecordAgentStatus("p2", "idle"); err != nil {
		t.Fatal(err)
	}
	client := newDeadlineHerdr(herdr.Pane{PaneID: "p1"}, herdr.Pane{PaneID: "p2", AgentStatus: "idle"})
	h := startDeadlineHarness(t, root, clock, client)
	h.awaitPrompt(t, "task-assigned")
	timer := h.awaitTimer(t)
	if got := timer.Delay(); got != 5*time.Second {
		t.Fatalf("timer delay = %v, want 5s", got)
	}

	clock.Set(clock.Now().Add(5 * time.Second))
	if !timer.Fire(clock.Now()) {
		t.Fatal("deadline timer was not armed")
	}
	prompt := h.awaitPrompt(t, "agent-idle")
	if !strings.Contains(prompt, "did not start task "+task.ID) {
		t.Fatalf("deadline prompt = %q", prompt)
	}
	timer.awaitStop(t) // timer-path audit, drain, and reschedule completed
	if timer.Fire(clock.Now()) {
		t.Fatal("completed deadline timer remained armed")
	}

	entries, err := store.Tasks()
	if err != nil || len(entries) != 1 || entries[0].Status != messaging.TaskActive {
		t.Fatalf("task projection after alert = %#v, %v", entries, err)
	}
}

func TestDispatcherWorkingEvidenceCancelsDeadline(t *testing.T) {
	clock := &deadlineClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	root := t.TempDir()
	_, _ = seedDeadlineTask(t, root, clock)
	client := newDeadlineHerdr(herdr.Pane{PaneID: "p1"}, herdr.Pane{PaneID: "p2"})
	h := startDeadlineHarness(t, root, clock, client)
	h.awaitPrompt(t, "task-assigned")
	timer := h.awaitTimer(t)
	timer.drainStops()

	h.events <- herdr.Event{Type: "agent.status_changed", PaneID: "p2", AgentStatus: "working"}
	timer.awaitStop(t)
	clock.Set(clock.Now().Add(5 * time.Second))
	if timer.Fire(clock.Now()) {
		t.Fatal("working evidence did not cancel the timer")
	}
	select {
	case prompt := <-client.prompts:
		t.Fatalf("unexpected prompt after start: %q", prompt)
	default:
	}
}

func TestDispatcherDeadlineSnapshotClosesWorkingBoundaryRace(t *testing.T) {
	clock := &deadlineClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	root := t.TempDir()
	_, _ = seedDeadlineTask(t, root, clock)
	client := newDeadlineHerdr(herdr.Pane{PaneID: "p1"}, herdr.Pane{PaneID: "p2"})
	h := startDeadlineHarness(t, root, clock, client)
	h.awaitPrompt(t, "task-assigned")
	timer := h.awaitTimer(t)
	timer.drainStops()

	// Herdr has observed working exactly at the boundary, before the push event
	// is selected. The deadline snapshot must reconcile it before auditing.
	client.setPanes(herdr.Pane{PaneID: "p1"}, herdr.Pane{PaneID: "p2", AgentStatus: "working"})
	clock.Set(clock.Now().Add(5 * time.Second))
	if !timer.Fire(clock.Now()) {
		t.Fatal("deadline timer was not armed")
	}
	timer.awaitStop(t)
	select {
	case prompt := <-client.prompts:
		t.Fatalf("boundary race produced premature prompt: %q", prompt)
	default:
	}
	if deadline, err := h.store.NextAgentIdleDeadline(); err != nil || !deadline.IsZero() {
		t.Fatalf("boundary reconciliation deadline = %v, %v; want zero", deadline, err)
	}
}

func TestDispatcherDeadlinePreservesQueuedWorkingIdleAcrossLatestIdleSnapshot(t *testing.T) {
	clock := &deadlineClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	root := t.TempDir()
	_, task := seedDeadlineTask(t, root, clock)
	client := newDeadlineHerdr(herdr.Pane{PaneID: "p1"}, herdr.Pane{PaneID: "p2"})
	snapshotStarted, releaseSnapshot := client.blockSnapshot(2) // onReady is call one
	h := startDeadlineHarness(t, root, clock, client)
	h.awaitPrompt(t, "task-assigned")
	timer := h.awaitTimer(t)
	timer.drainStops()

	clock.Set(clock.Now().Add(5 * time.Second))
	if !timer.Fire(clock.Now()) {
		t.Fatal("deadline timer was not armed")
	}
	select {
	case <-snapshotStarted:
	case err := <-h.done:
		t.Fatalf("dispatcher exited before deadline snapshot: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("deadline snapshot did not start")
	}

	// Both callbacks return while the dispatcher is blocked in Snapshot, proving
	// that the acknowledged subscription queued working and then idle. Snapshot
	// reads its result only after release and therefore returns the latest idle
	// state; the working push remains the only start evidence.
	h.queueEvent(t, herdr.Event{Type: "agent.status_changed", PaneID: "p2", AgentStatus: "working"})
	h.queueEvent(t, herdr.Event{Type: "agent.status_changed", PaneID: "p2", AgentStatus: "idle"})
	client.setPanes(herdr.Pane{PaneID: "p1"}, herdr.Pane{PaneID: "p2", AgentStatus: "idle"})
	close(releaseSnapshot)

	prompt := h.awaitPrompt(t, "agent-idle")
	if strings.Contains(prompt, "did not start") {
		t.Fatalf("queued working evidence was lost: %q", prompt)
	}
	if want := "entered Herdr status idle while owning task " + task.ID; !strings.Contains(prompt, want) {
		t.Fatalf("agent-idle prompt = %q, want fragment %q", prompt, want)
	}

	// Stop is a deterministic barrier: every callback accepted above and every
	// wake produced by the deadline arm has finished before the remaining prompt
	// buffer is inspected.
	h.stopDispatcher(t)
	idlePrompts := []string{prompt}
	for {
		select {
		case extra := <-client.prompts:
			if strings.Contains(extra, "Kind: agent-idle") {
				idlePrompts = append(idlePrompts, extra)
			}
		default:
			if len(idlePrompts) != 1 {
				t.Fatalf("agent-idle prompts = %#v, want exactly one", idlePrompts)
			}
			for _, delivered := range idlePrompts {
				if strings.Contains(delivered, "did not start") {
					t.Fatalf("premature no-start prompt was delivered: %q", delivered)
				}
			}
			return
		}
	}
}

func TestDispatcherStartupDefersActivationUntilSubscriptionReady(t *testing.T) {
	clock := &deadlineClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	root := t.TempDir()
	store, task := seedDeadlineTask(t, root, clock)
	client := newDeadlineHerdr(
		herdr.Pane{PaneID: "p1"},
		herdr.Pane{PaneID: "p2", AgentStatus: "idle"},
	)
	selectBarrier := &deadlineSelectBarrier{}
	h := launchDeadlineHarnessWithConfig(t, root, clock, client, deadlineHarnessConfig{
		gateSubscriptions: true,
		selectPrepared:    selectBarrier.observe,
	})
	stream := h.awaitStream(t, "p1", "p2")

	// Receiving the subscription request is a deterministic barrier: startup's
	// pre-coverage drain has returned and Herdr acknowledgement is now the only
	// missing condition. The live assignee's activation must still be pending.
	select {
	case prompt := <-client.prompts:
		t.Fatalf("startup delivered activation before subscription acknowledgement: %q", prompt)
	default:
	}
	pending, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Kind != "task-assigned" || pending[0].ReferenceID != task.ID {
		t.Fatalf("pre-ack pending wakes = %#v, want the task activation", pending)
	}

	close(stream.ack)
	h.awaitReady(t)
	prompt := h.awaitPrompt(t, "task-assigned")
	if !strings.Contains(prompt, task.ID) {
		t.Fatalf("activation prompt = %q, want task %s", prompt, task.ID)
	}
	timer := h.awaitTimer(t)
	timer.drainStops()

	// Both transitions travel through the acknowledged subscription. Working is
	// durable start evidence; the following idle therefore emits one ordinary
	// post-start wake instead of arming the five-second no-start fallback.
	h.queueEvent(t, herdr.Event{Type: "agent.status_changed", PaneID: "p2", AgentStatus: "working"})
	h.awaitApplied(t)
	timer.awaitStop(t)
	h.queueEvent(t, herdr.Event{Type: "agent.status_changed", PaneID: "p2", AgentStatus: "idle"})
	h.awaitApplied(t)
	observed, release := selectBarrier.arm()
	h.files.events <- struct{}{}
	idlePrompt := h.awaitPrompt(t, "agent-idle")
	awaitSelectBoundary(t, observed, release, false)
	if strings.Contains(idlePrompt, "did not start") {
		t.Fatalf("startup lost acknowledged working evidence: %q", idlePrompt)
	}
	if want := "entered Herdr status idle while owning task " + task.ID; !strings.Contains(idlePrompt, want) {
		t.Fatalf("agent-idle prompt = %q, want fragment %q", idlePrompt, want)
	}

	h.stopDispatcher(t)
	select {
	case extra := <-client.prompts:
		t.Fatalf("startup delivered an extra prompt: %q", extra)
	default:
	}
}

func TestDispatcherLiveAddDefersOnlyUncoveredActivationUntilReplacementReady(t *testing.T) {
	clock := &deadlineClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	root := t.TempDir()
	store := messaging.New(root, testSession, messaging.WithClock(clock.Now))
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	for _, params := range []messaging.RegisterParams{
		{Name: messaging.OrchestratorIdentity, PaneID: "p1", Harness: "codex", Caller: messaging.UserIdentity, CanDelegate: true},
		{Name: "covered", PaneID: "p2", Harness: "codex", Caller: messaging.OrchestratorIdentity},
	} {
		if _, _, err := store.RegisterAgent(params); err != nil {
			t.Fatal(err)
		}
	}
	client := newDeadlineHerdr(herdr.Pane{PaneID: "p1"}, herdr.Pane{PaneID: "p2"})
	selectBarrier := &deadlineSelectBarrier{}
	h := launchDeadlineHarnessWithConfig(t, root, clock, client, deadlineHarnessConfig{
		gateSubscriptions: true,
		selectPrepared:    selectBarrier.observe,
	})
	first := h.awaitStream(t, "p1", "p2")
	close(first.ack)
	h.awaitReady(t)

	_, uncoveredTask, err := store.RegisterAgent(messaging.RegisterParams{
		Name: "new-worker", PaneID: "p3", Harness: "codex",
		Caller: messaging.OrchestratorIdentity, Task: "new pane activation",
	})
	if err != nil {
		t.Fatal(err)
	}
	coveredTask, err := store.AssignTask(messaging.OrchestratorIdentity, "covered", "", "covered pane activation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(messaging.CreateParams{
		Sender: messaging.UserIdentity, Recipient: messaging.OrchestratorIdentity,
		RecipientPane: "p1", Body: "unrelated covered message",
	}); err != nil {
		t.Fatal(err)
	}
	client.setPanes(
		herdr.Pane{PaneID: "p1"},
		herdr.Pane{PaneID: "p2", AgentStatus: "idle"},
		herdr.Pane{PaneID: "p3", AgentStatus: "idle"},
	)
	h.files.events <- struct{}{}
	replacement := h.awaitStream(t, "p1", "p2", "p3")

	// The drain skips p3's first wake but continues through the ledger: both the
	// covered pane's activation and the unrelated message are delivered before
	// replacement acknowledgement. This proves deferral causes no head-of-line
	// starvation and does not reduce established coverage.
	coveredPrompt := h.awaitPrompt(t, "task-assigned")
	if !strings.Contains(coveredPrompt, coveredTask.ID) {
		t.Fatalf("covered activation prompt = %q, want task %s", coveredPrompt, coveredTask.ID)
	}
	messagePrompt := h.awaitPrompt(t, "message")
	if !strings.Contains(messagePrompt, "unrelated covered message") {
		t.Fatalf("message prompt = %q", messagePrompt)
	}
	select {
	case prompt := <-client.prompts:
		t.Fatalf("new pane activation crossed unacknowledged replacement: %q", prompt)
	default:
	}
	pending, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ReferenceID != uncoveredTask.ID || pending[0].Kind != "task-assigned" {
		t.Fatalf("replacement pending wakes = %#v, want only new pane activation", pending)
	}

	close(replacement.ack)
	newPrompt := h.awaitPrompt(t, "task-assigned")
	if !strings.Contains(newPrompt, uncoveredTask.ID) {
		t.Fatalf("replacement-ready activation = %q, want task %s", newPrompt, uncoveredTask.ID)
	}
	h.queueEvent(t, herdr.Event{Type: "agent.status_changed", PaneID: "p3", AgentStatus: "working"})
	h.awaitApplied(t)
	h.queueEvent(t, herdr.Event{Type: "agent.status_changed", PaneID: "p3", AgentStatus: "idle"})
	h.awaitApplied(t)
	observed, release := selectBarrier.arm()
	h.files.events <- struct{}{}
	idlePrompt := h.awaitPrompt(t, "agent-idle")
	awaitSelectBoundary(t, observed, release, true)
	if strings.Contains(idlePrompt, "did not start") {
		t.Fatalf("live-add lost acknowledged working evidence: %q", idlePrompt)
	}
	if want := "entered Herdr status idle while owning task " + uncoveredTask.ID; !strings.Contains(idlePrompt, want) {
		t.Fatalf("agent-idle prompt = %q, want fragment %q", idlePrompt, want)
	}

	h.stopDispatcher(t)
	select {
	case extra := <-client.prompts:
		if strings.Contains(extra, "did not start") || strings.Contains(extra, "Kind: agent-idle") {
			t.Fatalf("live-add delivered an extra supervision prompt: %q", extra)
		}
	default:
	}
}

func TestDispatcherOverdueDeadlineWaitsForReadyReconciliation(t *testing.T) {
	clock := &deadlineClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	root := t.TempDir()
	store, task := seedDeadlineTask(t, root, clock)
	client := newDeadlineHerdr(
		herdr.Pane{PaneID: "p1"},
		herdr.Pane{PaneID: "p2"},
	)
	selectBarrier := &deadlineSelectBarrier{}
	h := launchDeadlineHarnessWithConfig(t, root, clock, client, deadlineHarnessConfig{
		selectPrepared: selectBarrier.observe,
	})
	h.awaitReady(t)
	h.awaitPrompt(t, "task-assigned")
	timer := h.awaitTimer(t)
	timer.drainStops()

	// Expanding the pane set replaces an already acknowledged subscription while
	// this task's no-start timer is armed. The replacement onReady snapshot
	// captures working and then remains blocked, leaving the run loop at a select
	// boundary where the timer must be disabled until reconciliation completes.
	snapshotStarted, releaseSnapshot := client.blockCapturedSnapshot(2)
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{
		Name: "worker2", PaneID: "p3", Harness: "codex", Caller: messaging.OrchestratorIdentity,
	}); err != nil {
		t.Fatal(err)
	}
	client.setPanes(
		herdr.Pane{PaneID: "p1"},
		herdr.Pane{PaneID: "p2", AgentStatus: "working"},
		herdr.Pane{PaneID: "p3"},
	)
	selectObserved, releaseSelect := selectBarrier.arm()
	h.files.events <- struct{}{}
	select {
	case enabled := <-selectObserved:
		if enabled {
			close(releaseSelect)
			t.Fatal("deadline remained selectable while replacement onReady was pending")
		}
	case err := <-h.done:
		t.Fatalf("dispatcher exited before replacement select boundary: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher did not reach replacement select boundary")
	}
	select {
	case <-snapshotStarted:
	case err := <-h.done:
		t.Fatalf("dispatcher exited before onReady captured its snapshot: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("onReady did not capture its snapshot")
	}

	// Herdr's acknowledged snapshot captured working, but production cannot
	// deliver either later push until onReady returns. Make the deadline overdue,
	// queue both pushes behind onReady, and fire the already-armed timer. Releasing
	// the explicit select barrier while onReady is still blocked proves the run
	// loop waits with the deadline channel nil; there is no scheduler-dependent
	// race in this regression.
	client.setPanes(
		herdr.Pane{PaneID: "p1"},
		herdr.Pane{PaneID: "p2", AgentStatus: "idle"},
		herdr.Pane{PaneID: "p3"},
	)
	clock.Set(clock.Now().Add(5 * time.Second))
	if !timer.Fire(clock.Now()) {
		t.Fatal("deadline timer was not armed")
	}
	h.events <- herdr.Event{Type: "agent.status_changed", PaneID: "p2", AgentStatus: "working"}
	h.events <- herdr.Event{Type: "agent.status_changed", PaneID: "p2", AgentStatus: "idle"}
	close(releaseSelect)
	close(releaseSnapshot)
	h.awaitApplied(t)
	h.awaitApplied(t)
	h.files.events <- struct{}{}

	prompt := h.awaitPrompt(t, "agent-idle")
	if strings.Contains(prompt, "did not start") {
		t.Fatalf("overdue deadline ran before ready reconciliation: %q", prompt)
	}
	if want := "entered Herdr status idle while owning task " + task.ID; !strings.Contains(prompt, want) {
		t.Fatalf("agent-idle prompt = %q, want fragment %q", prompt, want)
	}

	h.stopDispatcher(t)
	client.mu.Lock()
	snapshotCalls := client.snapshotCalls
	client.mu.Unlock()
	if snapshotCalls != 2 {
		t.Fatalf("snapshot calls = %d, want only the two onReady snapshots", snapshotCalls)
	}
	select {
	case extra := <-client.prompts:
		t.Fatalf("readiness boundary delivered an extra prompt: %q", extra)
	default:
	}
}

func TestDispatcherReplacementPreservesAcceptedOutgoingStatusEvents(t *testing.T) {
	clock := &deadlineClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	root := t.TempDir()
	store, task := seedDeadlineTask(t, root, clock)
	client := newDeadlineHerdr(herdr.Pane{PaneID: "p1"}, herdr.Pane{PaneID: "p2"})
	h := startDeadlineHarness(t, root, clock, client)
	firstStream := <-h.streams
	h.awaitPrompt(t, "task-assigned")
	_ = h.awaitTimer(t)

	promptStarted, releasePrompt := client.blockPrompt(2)
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{
		Name: "worker2", PaneID: "p3", Harness: "codex",
		Caller: messaging.OrchestratorIdentity, Task: "change the subscribed pane set",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(messaging.CreateParams{
		Sender: messaging.UserIdentity, Recipient: messaging.OrchestratorIdentity,
		RecipientPane: "p1", Body: "hold the ledger drain on covered work",
	}); err != nil {
		t.Fatal(err)
	}
	client.setPanes(
		herdr.Pane{PaneID: "p1"},
		herdr.Pane{PaneID: "p2", AgentStatus: "idle"},
		herdr.Pane{PaneID: "p3"},
	)
	h.files.events <- struct{}{}
	select {
	case <-promptStarted:
	case err := <-h.done:
		t.Fatalf("dispatcher exited before pane-set-changing delivery blocked: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("pane-set-changing delivery did not block")
	}

	// The newly added pane's activation is deferred, but the later message to a
	// covered pane is not starved and blocks this drain in PromptAgent. The old
	// acknowledged stream accepts both observations while the run loop is there.
	// Releasing delivery lets the ledger drain request replacement, which must
	// quiesce and apply these FIFO events before canceling or advancing generation.
	h.queueEvent(t, herdr.Event{Type: "agent.status_changed", PaneID: "p2", AgentStatus: "working"})
	h.queueEvent(t, herdr.Event{Type: "agent.status_changed", PaneID: "p2", AgentStatus: "idle"})
	close(releasePrompt)
	h.awaitPrompt(t, "message")
	prompt := h.awaitPrompt(t, "agent-idle")
	if strings.Contains(prompt, "did not start") {
		t.Fatalf("outgoing working evidence was discarded: %q", prompt)
	}
	if want := "entered Herdr status idle while owning task " + task.ID; !strings.Contains(prompt, want) {
		t.Fatalf("agent-idle prompt = %q, want fragment %q", prompt, want)
	}
	h.awaitPrompt(t, "task-assigned")
	h.awaitApplied(t)
	h.awaitApplied(t)
	select {
	case <-firstStream.done:
	case err := <-h.done:
		t.Fatalf("dispatcher exited before canceling outgoing stream: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("outgoing stream was not canceled during replacement")
	}
	var replacement deadlineStream
	select {
	case replacement = <-h.streams:
	case err := <-h.done:
		t.Fatalf("dispatcher exited before opening replacement stream: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher did not open replacement stream")
	}

	// A callback invoked only after cancellation is not accepted. Follow it with
	// one current-generation callback; observing that callback's application is a
	// FIFO barrier proving the stale close cannot still be waiting in ingress.
	firstStream.event(herdr.Event{Type: "pane.closed", PaneID: "p2"})
	replacement.event(herdr.Event{Type: "agent.status_changed", PaneID: "p1", AgentStatus: "working"})
	h.awaitApplied(t)
	if _, err := store.AgentByPane("p2"); err != nil {
		t.Fatalf("post-cancellation callback retired p2: %v", err)
	}

	h.stopDispatcher(t)
	select {
	case extra := <-client.prompts:
		if strings.Contains(extra, "Kind: agent-idle") {
			t.Fatalf("replacement delivered duplicate idle wake: %q", extra)
		}
	default:
	}
}

func TestDispatcherPauseCancelsAndResumeRearmsFreshTimer(t *testing.T) {
	clock := &deadlineClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	root := t.TempDir()
	store, task := seedDeadlineTask(t, root, clock)
	client := newDeadlineHerdr(herdr.Pane{PaneID: "p1"}, herdr.Pane{PaneID: "p2"})
	h := startDeadlineHarness(t, root, clock, client)
	h.awaitPrompt(t, "task-assigned")
	firstTimer := h.awaitTimer(t)
	firstTimer.drainStops()

	if _, err := store.TransitionTask("worker", task.ID, messaging.TaskBlocked, "waiting"); err != nil {
		t.Fatal(err)
	}
	h.files.events <- struct{}{}
	h.awaitPrompt(t, "task-blocked")
	firstTimer.awaitStop(t)
	if firstTimer.Fire(clock.Now()) {
		t.Fatal("paused task left its timer armed")
	}

	clock.Set(clock.Now().Add(time.Second))
	if _, err := store.TransitionTask(messaging.OrchestratorIdentity, task.ID, messaging.TaskActive, "continue"); err != nil {
		t.Fatal(err)
	}
	h.files.events <- struct{}{}
	h.awaitPrompt(t, "task-resumed")
	secondTimer := h.awaitTimer(t)
	if secondTimer == firstTimer || secondTimer.Delay() != 5*time.Second {
		t.Fatalf("resume timer = %p/%v, want a fresh 5s episode", secondTimer, secondTimer.Delay())
	}
	clock.Set(clock.Now().Add(5 * time.Second))
	if !secondTimer.Fire(clock.Now()) {
		t.Fatal("resume timer was not armed")
	}
	h.awaitPrompt(t, "agent-idle")
}

func TestDispatcherRestartReconstructsRemainingDeadline(t *testing.T) {
	clock := &deadlineClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	root := t.TempDir()
	_, _ = seedDeadlineTask(t, root, clock)
	client := newDeadlineHerdr(herdr.Pane{PaneID: "p1"}, herdr.Pane{PaneID: "p2"})
	first := startDeadlineHarness(t, root, clock, client)
	first.awaitPrompt(t, "task-assigned")
	_ = first.awaitTimer(t)
	first.stopDispatcher(t)

	clock.Set(clock.Now().Add(2 * time.Second))
	second := startDeadlineHarness(t, root, clock, client)
	timer := second.awaitTimer(t)
	if timer.Delay() != 3*time.Second {
		t.Fatalf("replayed timer delay = %v, want 3s", timer.Delay())
	}
	clock.Set(clock.Now().Add(3 * time.Second))
	if !timer.Fire(clock.Now()) {
		t.Fatal("replayed timer was not armed")
	}
	second.awaitPrompt(t, "agent-idle")
	timer.awaitStop(t)
	second.stopDispatcher(t)

	// A second restart replays the existing activation-scoped alert and has
	// neither a deadline to schedule nor another alert to deliver.
	third := startDeadlineHarness(t, root, clock, client)
	select {
	case extra := <-third.created:
		t.Fatalf("deduped restart scheduled timer %p", extra)
	default:
	}
	select {
	case prompt := <-client.prompts:
		t.Fatalf("deduped restart delivered prompt %q", prompt)
	default:
	}
}

func TestDispatcherFailedDeliveryUsesFailureTimeForGrace(t *testing.T) {
	clock := &deadlineClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	root := t.TempDir()
	_, _ = seedDeadlineTask(t, root, clock)
	client := newDeadlineHerdr(herdr.Pane{PaneID: "p1"}, herdr.Pane{PaneID: "p2"})
	client.setRefusal("worker", errors.New("prompt refused"))
	h := startDeadlineHarness(t, root, clock, client)
	timer := h.awaitTimer(t) // factory runs only after drain records the failure
	if timer.Delay() != 5*time.Second {
		t.Fatalf("failed-delivery timer = %v, want 5s", timer.Delay())
	}
	client.setRefusal("worker", nil)
	clock.Set(clock.Now().Add(5 * time.Second))
	if !timer.Fire(clock.Now()) {
		t.Fatal("failed-delivery deadline was not armed")
	}
	prompt := h.awaitPrompt(t, "agent-idle")
	if !strings.Contains(prompt, "activation wake delivery failed") {
		t.Fatalf("failed-delivery alert = %q", prompt)
	}
}

func TestDispatcherAuditsMultipleSimultaneousDeadlinesWithOneTimer(t *testing.T) {
	clock := &deadlineClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	root := t.TempDir()
	store, _ := seedDeadlineTask(t, root, clock)
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{Name: "worker2", PaneID: "p3", Harness: "codex", Caller: messaging.OrchestratorIdentity}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignTask(messaging.OrchestratorIdentity, "worker2", "", "second task"); err != nil {
		t.Fatal(err)
	}
	client := newDeadlineHerdr(herdr.Pane{PaneID: "p1"}, herdr.Pane{PaneID: "p2"}, herdr.Pane{PaneID: "p3"})
	h := startDeadlineHarness(t, root, clock, client)
	h.awaitPrompt(t, "task-assigned")
	h.awaitPrompt(t, "task-assigned")
	timer := h.awaitTimer(t)
	select {
	case extra := <-h.created:
		t.Fatalf("created more than one active timer: %p", extra)
	default:
	}

	clock.Set(clock.Now().Add(5 * time.Second))
	if !timer.Fire(clock.Now()) {
		t.Fatal("shared deadline timer was not armed")
	}
	h.awaitPrompt(t, "agent-idle")
	h.awaitPrompt(t, "agent-idle")
}
