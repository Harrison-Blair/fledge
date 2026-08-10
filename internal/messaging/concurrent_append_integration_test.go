package messaging_test

// This integration test drives the real event dispatcher (watchproc.Run ->
// runDispatcher -> drain) against a real on-disk ledger while N concurrent
// writers append message wakes through messaging.Store. Both the dispatcher's
// internal Store and the test's Store point at the same ledger file and
// coordinate solely through the unix flock in messaging/lock.go, so the test
// exercises the true "file lock vs drain + activePanes recompute" seam rather
// than a fake.
//
// Non-flakiness: every wait is a bounded poll on durable ledger state or a
// select against the dispatcher's done channel. No bare time.Sleep is used as a
// correctness mechanism, and the whole flow terminates deterministically because
// the poll loop keeps nudging the ledger watcher until PendingWakes drains.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/watchproc"
)

const concurrentSession = "fledge-concurrent-1234abcd"

// concurrentWatcher is the injected ledger watcher. The concurrent writers and
// the drain-forcing poll loop push notifications onto events; the dispatcher
// drains and recomputes activePanes on each one.
type concurrentWatcher struct {
	events chan struct{}
	errs   chan error
}

func (w *concurrentWatcher) Events() <-chan struct{} { return w.events }
func (w *concurrentWatcher) Errors() <-chan error    { return w.errs }
func (w *concurrentWatcher) Close() error            { return nil }

// recordingHerdr counts a delivery per wake ID parsed from the envelope so the
// test can assert every wake was delivered exactly once. Its snapshot lists a
// fixed live pane set so the dispatcher's onReady reconciliation never retires a
// pane the writers still rely on.
type recordingHerdr struct {
	mu        sync.Mutex
	delivered map[string]int
	panes     []string
}

func (h *recordingHerdr) Protocol(context.Context) (int, error) {
	return watchproc.RequiredHerdrProtocol, nil
}

func (h *recordingHerdr) List(context.Context) ([]herdr.Session, error) { return nil, nil }

func (h *recordingHerdr) Snapshot(context.Context, string) (herdr.Snapshot, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	panes := make([]herdr.Pane, 0, len(h.panes))
	for _, id := range h.panes {
		panes = append(panes, herdr.Pane{PaneID: id})
	}
	return herdr.Snapshot{Panes: panes}, nil
}

func (h *recordingHerdr) PromptAgent(_ context.Context, _, _, prompt string) error {
	id := deliveryID(prompt)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.delivered[id]++
	return nil
}

func (h *recordingHerdr) counts() map[string]int {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string]int, len(h.delivered))
	for id, n := range h.delivered {
		out[id] = n
	}
	return out
}

// deliveryID extracts the wake ID the dispatcher stamps into every wake envelope
// as "Delivery-ID: <id>". A blank result would collapse distinct deliveries into
// one bucket, so the test asserts it is non-empty.
func deliveryID(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		if rest, ok := strings.CutPrefix(line, "Delivery-ID: "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// TestConcurrentCLIAppendsDuringDrain fires N concurrent writers that each append
// a message wake for a distinct live pane while repeatedly notifying the ledger.
// The real dispatcher drains under the same flock the writers contend for. Every
// wake must reach a terminal delivered outcome, PendingWakes must drain to empty,
// and no wake may be delivered more than once.
func TestConcurrentCLIAppendsDuringDrain(t *testing.T) {
	const workers = 12

	root := t.TempDir()
	store := messaging.New(root, concurrentSession)
	if _, err := store.Initialize(); err != nil {
		t.Fatalf("initialize ledger: %v", err)
	}

	panes := make([]string, workers)
	for i := range workers {
		pane := fmt.Sprintf("p%d", i)
		panes[i] = pane
		if _, _, err := store.RegisterAgent(messaging.RegisterParams{
			Name: fmt.Sprintf("worker%d", i), PaneID: pane, Harness: "codex",
			Caller: messaging.UserIdentity}); err != nil {
			t.Fatalf("register worker%d: %v", i, err)
		}
	}

	client := &recordingHerdr{delivered: make(map[string]int), panes: panes}
	watcher := &concurrentWatcher{events: make(chan struct{}, 4*workers), errs: make(chan error, 1)}

	// A Subscribe seam that acknowledges once and then blocks until torn down,
	// mirroring herdr.Subscribe. The pane set never changes here, so the run loop
	// opens exactly one subscription; onReady drives the initial reconciliation.
	subscribe := func(streamCtx context.Context, _ []string, onReady func(), _ func(herdr.Event)) error {
		onReady()
		<-streamCtx.Done()
		return streamCtx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- watchproc.Run(ctx, watchproc.Options{
			Root: root, Session: concurrentSession, Herdr: client,
			WatchFile: func(string) (watchproc.FileWatcher, error) { return watcher, nil },
			Subscribe: subscribe,
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("dispatcher did not stop")
		}
	})

	notify := func() {
		select {
		case watcher.events <- struct{}{}:
		case <-ctx.Done():
		}
	}

	// Each writer appends its message wake, then nudges the ledger a few times so
	// a drain is very likely to run mid-flight against the other writers' appends.
	wakeIDs := make([]string, workers)
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			msg, err := store.Create(messaging.CreateParams{
				Sender:        messaging.UserIdentity,
				Recipient:     fmt.Sprintf("worker%d", i),
				RecipientPane: fmt.Sprintf("p%d", i),
				Body:          fmt.Sprintf("hello worker %d", i),
			})
			if err != nil {
				mu.Lock()
				wakeIDs[i] = "ERROR: " + err.Error()
				mu.Unlock()
				return
			}
			mu.Lock()
			wakeIDs[i] = "w-" + msg.ID
			mu.Unlock()
			for range 3 {
				notify()
			}
		}(i)
	}
	close(start)
	wg.Wait()

	for i, id := range wakeIDs {
		if strings.HasPrefix(id, "ERROR:") {
			t.Fatalf("writer %d failed to append: %s", i, id)
		}
	}

	// Drive drains until the ledger reports no pending or uncertain wakes. A fresh
	// notify each iteration guarantees a drain runs after the final append
	// committed, so termination does not depend on interleaving.
	deadline := time.After(30 * time.Second)
	for {
		select {
		case watcher.events <- struct{}{}:
		default:
		}
		pending, err := store.PendingWakes()
		if err != nil {
			t.Fatalf("read pending wakes: %v", err)
		}
		if len(pending) == 0 {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("dispatcher exited before draining, %d wakes pending: %v", len(pending), err)
		case <-deadline:
			t.Fatalf("PendingWakes did not drain: %d still pending", len(pending))
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Every message wake is delivered exactly once, keyed by its stable wake ID.
	counts := client.counts()
	if id := ""; counts[id] != 0 {
		t.Fatalf("a delivery carried a blank Delivery-ID: envelope parsing is wrong")
	}
	for i, id := range wakeIDs {
		switch counts[id] {
		case 1:
		case 0:
			t.Errorf("worker %d wake %s was never delivered", i, id)
		default:
			t.Errorf("worker %d wake %s was delivered %d times (duplicate)", i, id, counts[id])
		}
	}
	if len(counts) != workers {
		t.Errorf("delivered %d distinct wakes, want %d: %v", len(counts), workers, counts)
	}

	// Each message's projected status reflects the delivered wake outcome.
	inbox, err := store.Inbox(messaging.UserIdentity)
	if err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	if len(inbox) != workers {
		t.Fatalf("inbox has %d messages, want %d", len(inbox), workers)
	}
	for _, m := range inbox {
		if m.Status != messaging.StatusDelivered {
			t.Errorf("message %s status = %s, want delivered", m.ID, m.Status)
		}
	}
}
