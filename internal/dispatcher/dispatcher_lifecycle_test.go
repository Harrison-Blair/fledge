package dispatcher

import (
	"context"
	"errors"
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
	if subscriptions := h.subscriptions(); len(subscriptions) != 0 {
		t.Fatalf("subscribed with no panes: %#v", subscriptions)
	}

	if _, _, err := h.store.RegisterAgent(messaging.RegisterParams{
		Name: "worker", PaneID: "p1", Harness: "codex",
		Caller: messaging.UserIdentity, Task: "start working"}); err != nil {
		t.Fatal(err)
	}
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
