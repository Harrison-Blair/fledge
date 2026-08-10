package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/watch"
)

const testSession = "fledge-test-1234abcd"

// dispatcherHarness runs one dispatcher against injected ledger and Herdr event
// sources so a test drives it entirely by events, never by elapsed time.
type dispatcherHarness struct {
	store  *messaging.Store
	client *fakeHerdr
	files  *fakeFiles
	ready  chan struct{}
	done   chan error
	cancel context.CancelFunc

	mu         sync.Mutex
	subscribed [][]string
	events     chan watch.Event
}

func newHarness(t *testing.T) *dispatcherHarness {
	t.Helper()
	root := t.TempDir()
	store := messaging.New(root, testSession)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	h := &dispatcherHarness{
		store:  store,
		client: &fakeHerdr{protocol: RequiredHerdrProtocol},
		files:  &fakeFiles{events: make(chan struct{}, 4), errs: make(chan error, 1)},
		ready:  make(chan struct{}, 4),
		done:   make(chan error, 1),
		events: make(chan watch.Event, 4),
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(5 * time.Second):
			t.Error("dispatcher did not stop")
		}
	})
	go func() {
		h.done <- Run(ctx, Options{
			Root: root, Session: testSession, Herdr: h.client,
			WatchFile: func(string) (FileWatcher, error) { return h.files, nil },
			Subscribe: func(streamCtx context.Context, panes []string, onReady func(), onEvent func(watch.Event)) error {
				h.mu.Lock()
				h.subscribed = append(h.subscribed, append([]string(nil), panes...))
				h.mu.Unlock()
				onReady()
				for {
					select {
					case <-streamCtx.Done():
						return streamCtx.Err()
					case event := <-h.events:
						onEvent(event)
					}
				}
			},
			Ready: func() { h.ready <- struct{}{} },
		})
	}()
	return h
}

// notifyLedger stands in for the filesystem notification an appended
// coordination event produces.
func (h *dispatcherHarness) notifyLedger() { h.files.events <- struct{}{} }

func (h *dispatcherHarness) awaitReady(t *testing.T) {
	t.Helper()
	select {
	case <-h.ready:
	case err := <-h.done:
		t.Fatalf("dispatcher exited before becoming ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher never became ready")
	}
}

// awaitDelivery waits for the dispatcher's asynchronous drain to reach the
// wanted number of prompts. Only a test needs this bound; the dispatcher itself
// is driven purely by events.
func (h *dispatcherHarness) awaitDelivery(t *testing.T, want int) []string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if prompts := h.client.delivered(); len(prompts) >= want {
			return prompts
		}
		select {
		case err := <-h.done:
			t.Fatalf("dispatcher exited with %v after %d deliveries", err, len(h.client.delivered()))
		case <-deadline:
			t.Fatalf("delivered %d prompts, want %d", len(h.client.delivered()), want)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (h *dispatcherHarness) subscriptions() [][]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([][]string(nil), h.subscribed...)
}

// A session whose last agent has stopped is a resting state. Exiting there
// would leave nothing to deliver the very next spawn's wakes, and the spawn
// only relaunches a dispatcher it can find has died.
func TestDispatcherStaysUpWithNoRegisteredPanes(t *testing.T) {
	h := newHarness(t)
	h.awaitReady(t)
	// Readiness with no live pane is the resting idle state: the dispatcher
	// announced without opening any subscription.
	if subscriptions := h.subscriptions(); len(subscriptions) != 0 {
		t.Fatalf("subscribed with no panes: %#v", subscriptions)
	}

	if _, _, err := h.store.RegisterAgent(messaging.RegisterParams{
		Name: "worker", PaneID: "p1", Harness: "codex",
		Caller: messaging.UserIdentity, Task: "start working"}); err != nil {
		t.Fatal(err)
	}
	h.client.setSnapshotPanes("p1")
	h.notifyLedger()

	prompts := h.awaitDelivery(t, 1)
	if !strings.Contains(prompts[0], "start working") {
		t.Fatalf("prompt = %q", prompts[0])
	}
	deadline := time.After(5 * time.Second)
	for {
		if subscriptions := h.subscriptions(); len(subscriptions) == 1 {
			if len(subscriptions[0]) != 1 || subscriptions[0][0] != "p1" {
				t.Fatalf("subscription = %#v, want [p1]", subscriptions[0])
			}
			return
		}
		select {
		case err := <-h.done:
			t.Fatalf("dispatcher exited: %v", err)
		case <-deadline:
			t.Fatalf("subscriptions = %#v, want one for p1", h.subscriptions())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// One undeliverable recipient must not strand every other agent's wakes.
func TestDispatcherRecordsFailureAndDeliversRemainingWakes(t *testing.T) {
	h := newHarness(t)
	h.awaitReady(t)
	for _, params := range []messaging.RegisterParams{
		{Name: "wedged", PaneID: "p1", Harness: "codex", Caller: messaging.UserIdentity},
		{Name: "healthy", PaneID: "p2", Harness: "codex", Caller: messaging.UserIdentity},
	} {
		if _, _, err := h.store.RegisterAgent(params); err != nil {
			t.Fatal(err)
		}
	}
	h.client.mu.Lock()
	h.client.refuse = map[string]error{"wedged": errors.New("pane is not accepting input")}
	h.client.mu.Unlock()
	// Both panes are live for the subscription snapshot; a wedged pane still
	// exists, it simply refuses input.
	h.client.setSnapshotPanes("p1", "p2")

	stuck, err := h.store.Create(messaging.CreateParams{
		Sender: messaging.UserIdentity, Recipient: "wedged", RecipientPane: "p1", Body: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Create(messaging.CreateParams{
		Sender: messaging.UserIdentity, Recipient: "healthy", RecipientPane: "p2", Body: "second"}); err != nil {
		t.Fatal(err)
	}
	h.notifyLedger()

	prompts := h.awaitDelivery(t, 1)
	if !strings.Contains(prompts[0], "second") {
		t.Fatalf("prompt = %q, want the healthy agent's message", prompts[0])
	}
	failed, err := h.store.Get(stuck.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != messaging.StatusFailed {
		t.Fatalf("undeliverable message status = %s, want failed", failed.Status)
	}
	// A terminal outcome stops replay, so nothing is left pending.
	pending, err := h.store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending wakes = %#v, want none", pending)
	}
}

// A closed pane must deactivate its agent so the registry stops advertising it
// and the next subscription drops it.
func TestDispatcherRetiresClosedPanes(t *testing.T) {
	h := newHarness(t)
	h.awaitReady(t)
	if _, _, err := h.store.RegisterAgent(messaging.RegisterParams{
		Name: "worker", PaneID: "p1", Harness: "codex", Caller: messaging.UserIdentity}); err != nil {
		t.Fatal(err)
	}
	// p1 is live at subscribe time, so reconciliation leaves it alone; the
	// pane.closed event below is what must retire it.
	h.client.setSnapshotPanes("p1")
	h.notifyLedger()
	select {
	case h.events <- watch.Event{Type: "pane.closed", PaneID: "p1"}:
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher never accepted the pane.closed event")
	}
	deadline := time.After(5 * time.Second)
	for {
		if _, err := h.store.AgentByPane("p1"); errors.Is(err, messaging.ErrAgentNotFound) {
			return
		}
		select {
		case err := <-h.done:
			t.Fatalf("dispatcher exited: %v", err)
		case <-deadline:
			t.Fatal("closed pane is still registered as active")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// When the session's last live pane closes, restart tears down the active
// subscription and the dispatcher must rest idle — the resting state the next
// spawn relaunches into — not exit. The canceled stream still delivers a terminal
// streamResult tagged with its own generation; the per-teardown generation bump
// makes that result compare stale so the run loop discards it. Without the bump
// the result matches the current generation, is treated as a live stream failure,
// and the dispatcher returns "Herdr dispatcher event stream ended" and dies.
func TestDispatcherStaysIdleWhenLastPaneCloses(t *testing.T) {
	h := newHarness(t)
	h.awaitReady(t)

	// Bring a live pane up so the dispatcher holds an active subscription.
	if _, _, err := h.store.RegisterAgent(messaging.RegisterParams{
		Name: "worker", PaneID: "p1", Harness: "codex", Caller: messaging.UserIdentity}); err != nil {
		t.Fatal(err)
	}
	h.client.setSnapshotPanes("p1")
	h.notifyLedger()
	deadline := time.After(5 * time.Second)
	for {
		subs := h.subscriptions()
		if len(subs) == 1 && strings.Join(subs[0], ",") == "p1" {
			break
		}
		select {
		case err := <-h.done:
			t.Fatalf("dispatcher exited before subscribing: %v", err)
		case <-deadline:
			t.Fatalf("no subscription for p1: %#v", h.subscriptions())
		case <-time.After(10 * time.Millisecond):
		}
	}

	// The last pane closes: its agent deactivates so the next ledger recompute
	// sees zero live panes.
	select {
	case h.events <- watch.Event{Type: "pane.closed", PaneID: "p1"}:
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher never accepted the pane.closed event")
	}
	deadline = time.After(5 * time.Second)
	for {
		if _, err := h.store.AgentByPane("p1"); errors.Is(err, messaging.ErrAgentNotFound) {
			break
		}
		select {
		case err := <-h.done:
			t.Fatalf("dispatcher exited: %v", err)
		case <-deadline:
			t.Fatal("closed pane is still registered as active")
		case <-time.After(10 * time.Millisecond):
		}
	}
	// Recompute active panes -> empty -> restart tears down the p1 stream. Its
	// terminal result must be discarded as stale, not exit the dispatcher. Give
	// that terminal streamResult a window to race toward the run loop: on correct
	// code the per-teardown generation bump already made it stale, so nothing
	// exits and the dispatcher rests idle rather than reporting a stream-ended
	// failure.
	h.notifyLedger()
	select {
	case err := <-h.done:
		t.Fatalf("dispatcher exited when its last pane closed: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
}

// TestDispatcherStaysIdleWhenLastPaneCloses_Repeated is the deterministic
// regression guard for the same P1 fix. The reverted bug only surfaces when the
// torn-down stream's terminal streamResult wins the goroutine's random select
// against its already-canceled streamCtx.Done(): so a single zero-pane teardown
// catches the reverted bug only ~50% of the time, making one 'go test' run an
// unreliable regression signal. Driving many independent teardowns in one run
// lifts single-run detection to ~1-0.5^N. Each iteration brings a fresh pane
// live, closes it, asserts the dispatcher rests idle, and — before the next
// subscription can bump the generation and mask a stale-generation exit — grants
// a short grace window in which any raced stream-ended exit for THIS teardown
// must surface. On correct code no exit ever surfaces, so the test is a
// deterministic PASS.
func TestDispatcherStaysIdleWhenLastPaneCloses_Repeated(t *testing.T) {
	h := newHarness(t)
	h.awaitReady(t)

	const iterations = 30
	for i := range iterations {
		pane := fmt.Sprintf("p%d", i)
		name := fmt.Sprintf("worker%d", i)

		// Bring a live pane up so the dispatcher holds an active subscription.
		if _, _, err := h.store.RegisterAgent(messaging.RegisterParams{
			Name: name, PaneID: pane, Harness: "codex", Caller: messaging.UserIdentity}); err != nil {
			t.Fatalf("iteration %d: register %s: %v", i, name, err)
		}
		h.client.setSnapshotPanes(pane)
		h.notifyLedger()
		h.awaitSubscription(t, i, pane)

		// The last pane closes: its agent deactivates so the next ledger recompute
		// sees zero live panes and tears the p-i stream down.
		select {
		case h.events <- watch.Event{Type: "pane.closed", PaneID: pane}:
		case err := <-h.done:
			t.Fatalf("iteration %d: dispatcher exited before pane.closed: %v", i, err)
		case <-time.After(5 * time.Second):
			t.Fatalf("iteration %d: dispatcher never accepted pane.closed for %s", i, pane)
		}
		h.awaitPaneRetired(t, i, pane)
		// Recompute active panes -> empty -> restart tears down the p-i stream. Its
		// terminal result must be discarded as stale, not exit the dispatcher.
		h.notifyLedger()

		// The torn-down stream's terminal result may still be racing toward the run
		// loop. Give it a window to surface a stale-generation exit for THIS
		// teardown before the next iteration's subscribe bumps the generation and
		// would mask it. On correct code the bumped generation already made that
		// result stale, so nothing exits and the dispatcher rests idle.
		select {
		case err := <-h.done:
			t.Fatalf("iteration %d: dispatcher exited after resting idle: %v", i, err)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// awaitSubscription waits until the dispatcher has opened a subscription for the
// single given pane, so a later pane.closed exercises a genuinely live stream's
// teardown rather than a no-op.
func (h *dispatcherHarness) awaitSubscription(t *testing.T, iter int, pane string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		for _, sub := range h.subscriptions() {
			if len(sub) == 1 && sub[0] == pane {
				return
			}
		}
		select {
		case err := <-h.done:
			t.Fatalf("iteration %d: dispatcher exited before subscribing to %s: %v", iter, pane, err)
		case <-deadline:
			t.Fatalf("iteration %d: no subscription for %s: %#v", iter, pane, h.subscriptions())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// awaitPaneRetired waits until the closed pane's agent has been deactivated, the
// precondition for the next recompute seeing zero live panes.
func (h *dispatcherHarness) awaitPaneRetired(t *testing.T, iter int, pane string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if _, err := h.store.AgentByPane(pane); errors.Is(err, messaging.ErrAgentNotFound) {
			return
		}
		select {
		case err := <-h.done:
			t.Fatalf("iteration %d: dispatcher exited while retiring %s: %v", iter, pane, err)
		case <-deadline:
			t.Fatalf("iteration %d: closed pane %s still registered as active", iter, pane)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// gatedSub models one Herdr subscription whose acknowledgement the test releases
// on demand, so the vulnerable resubscribe window can be entered deterministically
// without timing sleeps.
type gatedSub struct {
	panes []string
	ack   chan struct{}
}

// A pane that closes after the old subscription is torn down but before the new
// one is acknowledged emits a pane.closed neither stream can carry. Only a
// snapshot taken from the new subscription's onReady still reflects the closure,
// so reconciliation — not an event — must retire the departed agent.
func TestDispatcherReconcilesPaneClosedDuringResubscribe(t *testing.T) {
	root := t.TempDir()
	store := messaging.New(root, testSession)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	// Worker A owns a task on p1, so its silent departure must orphan that task.
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{
		Name: "workerA", PaneID: "p1", Harness: "codex",
		Caller: messaging.UserIdentity, Task: "do work"}); err != nil {
		t.Fatal(err)
	}
	tasks, err := store.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	var taskID string
	for _, task := range tasks {
		if task.Assignee == "workerA" {
			taskID = task.ID
		}
	}
	if taskID == "" {
		t.Fatal("workerA has no task to orphan")
	}

	client := &fakeHerdr{protocol: RequiredHerdrProtocol}
	client.setSnapshotPanes("p1")
	files := &fakeFiles{events: make(chan struct{}, 4), errs: make(chan error, 1)}

	subs := make(chan *gatedSub, 8)
	subscribe := func(streamCtx context.Context, panes []string, onReady func(), _ func(watch.Event)) error {
		sub := &gatedSub{panes: append([]string(nil), panes...), ack: make(chan struct{})}
		select {
		case subs <- sub:
		case <-streamCtx.Done():
			return streamCtx.Err()
		}
		// Wait for the test to acknowledge before running onReady, exactly as
		// watch.Subscribe waits for Herdr's ack before calling it.
		select {
		case <-sub.ack:
		case <-streamCtx.Done():
			return streamCtx.Err()
		}
		onReady()
		<-streamCtx.Done()
		return streamCtx.Err()
	}

	ready := make(chan struct{}, 4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Root: root, Session: testSession, Herdr: client,
			WatchFile: func(string) (FileWatcher, error) { return files, nil },
			Subscribe: subscribe,
			Ready:     func() { ready <- struct{}{} }})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("dispatcher did not stop")
		}
	})

	awaitSub := func(want []string) *gatedSub {
		t.Helper()
		select {
		case sub := <-subs:
			if strings.Join(sub.panes, ",") != strings.Join(want, ",") {
				t.Fatalf("subscription = %#v, want %#v", sub.panes, want)
			}
			return sub
		case err := <-done:
			t.Fatalf("dispatcher exited before subscribing: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatalf("no subscription for %#v", want)
		}
		return nil
	}

	// First subscription covers p1; acknowledging it drives the initial snapshot
	// reconciliation, after which the dispatcher announces readiness.
	first := awaitSub([]string{"p1"})
	close(first.ack)
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("dispatcher exited before ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher never became ready")
	}

	// Registering B changes the pane set, tearing down the p1 stream and opening
	// a replacement for [p1, p2] that is not yet acknowledged: the window.
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{
		Name: "workerB", PaneID: "p2", Harness: "codex", Caller: messaging.UserIdentity}); err != nil {
		t.Fatal(err)
	}
	files.events <- struct{}{}
	second := awaitSub([]string{"p1", "p2"})

	// A closes inside the window: the snapshot loses p1 and NO pane.closed event
	// is delivered on either stream. Acknowledging the replacement now is the only
	// thing that can retire A, and only via snapshot reconciliation.
	client.setSnapshotPanes("p2")
	close(second.ack)

	deadline := time.After(5 * time.Second)
	for {
		if _, err := store.AgentByPane("p1"); errors.Is(err, messaging.ErrAgentNotFound) {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("dispatcher exited: %v", err)
		case <-deadline:
			t.Fatal("A was not reconciled to stopped during the resubscribe window")
		case <-time.After(10 * time.Millisecond):
		}
	}

	if _, err := store.AgentByPane("p2"); err != nil {
		t.Fatalf("workerB should still be active on p2: %v", err)
	}
	task, err := store.Task(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != messaging.TaskOrphaned {
		t.Fatalf("A's task status = %s, want orphaned", task.Status)
	}
	select {
	case err := <-done:
		t.Fatalf("dispatcher exited after reconciliation: %v", err)
	default:
	}
}
