package dispatcher

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/trace"
	"github.com/Harrison-Blair/fledge/internal/watch"
)

type recorder struct {
	mu      sync.Mutex
	records []trace.Record
}

func (r *recorder) emit(record trace.Record) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
}

func (r *recorder) kinds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var kinds []string
	for _, record := range r.records {
		kinds = append(kinds, record.Kind)
	}
	return kinds
}

func (r *recorder) find(kind string) (trace.Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, record := range r.records {
		if record.Kind == kind {
			return record, true
		}
	}
	return trace.Record{}, false
}

// The trace has to account for a whole delivery: the wake the ledger recorded,
// the prompt the dispatcher sent, and the outcome it wrote back.
func TestRunEmitsTheRecordSequenceForAMessageWake(t *testing.T) {
	root, session := t.TempDir(), "fledge-test-1234abcd"
	store := messaging.New(root, session)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{Name: "worker", PaneID: "%12", Harness: "codex", Caller: "user"}); err != nil {
		t.Fatal(err)
	}
	files := &fakeFiles{events: make(chan struct{}, 4), errs: make(chan error, 1)}
	client := &fakeHerdr{protocol: RequiredHerdrProtocol}
	client.setSnapshotPanes("%12")
	collected := &recorder{}
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Root: root, Session: session, Herdr: client, Emit: collected.emit,
			WatchFile: func(string) (FileWatcher, error) { return files, nil },
			Subscribe: func(ctx context.Context, _ []string, onReady func(), events func(watch.Event)) error {
				onReady()
				close(ready)
				events(watch.Event{Type: "pane.agent_status_changed", PaneID: "%12", AgentStatus: "idle", Agent: "worker"})
				<-ctx.Done()
				return ctx.Err()
			}})
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher not ready")
	}
	// The message lands after the dispatcher is running, so its wake is only
	// reachable through the ledger notification.
	message, err := store.Create(messaging.CreateParams{Sender: "user", Recipient: "worker", RecipientPane: "%12", Body: "rerun the build"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	for {
		if _, ok := collected.find("wake.ok"); ok {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("records = %v", collected.kinds())
		case files.events <- struct{}{}:
		}
	}
	cancel()
	<-done

	for _, kind := range []string{"dispatcher.ready", "dispatcher.subscribe", "dispatcher.subscribed", "herdr.pane", "message", "wake.queued", "wake.send", "wake.attempt", "wake.ok"} {
		if _, ok := collected.find(kind); !ok {
			t.Fatalf("missing %s record; got %v", kind, collected.kinds())
		}
	}
	pane, _ := collected.find("herdr.pane")
	if pane.Pane != "%12" || pane.Status != "idle" || pane.Origin != "worker" {
		t.Fatalf("herdr.pane record = %#v", pane)
	}
	sent, _ := collected.find("wake.send")
	if sent.Target != "worker" || sent.Pane != "%12" || sent.Body != "" || sent.Note != "prompting pane %12" {
		t.Fatalf("wake.send record = %#v", sent)
	}
	exited, ok := collected.find("dispatcher.exit")
	if !ok || exited.Note != context.Canceled.Error() {
		t.Fatalf("dispatcher.exit record = %#v, found = %t", exited, ok)
	}
	folded, _ := collected.find("message")
	if folded.Ref != message.ID || folded.Origin != "user" || folded.Target != "worker" {
		t.Fatalf("message record = %#v", folded)
	}
}

// A failed prompt is the case an operator is watching for, so the transport
// error has to reach the trace rather than only the ledger.
func TestRunEmitsAFailedWakeWithItsTransportError(t *testing.T) {
	root, session := t.TempDir(), "fledge-test-1234abcd"
	store := messaging.New(root, session)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{Name: "worker", PaneID: "%12", Harness: "codex", Caller: "user"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(messaging.CreateParams{Sender: "user", Recipient: "worker", RecipientPane: "%12", Body: "hello"}); err != nil {
		t.Fatal(err)
	}
	files := &fakeFiles{events: make(chan struct{}, 1), errs: make(chan error, 1)}
	client := &fakeHerdr{protocol: RequiredHerdrProtocol, refuse: map[string]error{"worker": errPaneGone}}
	client.setSnapshotPanes("%12")
	collected := &recorder{}
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Root: root, Session: session, Herdr: client, Emit: collected.emit,
			WatchFile: func(string) (FileWatcher, error) { return files, nil },
			Subscribe: func(ctx context.Context, _ []string, onReady func(), _ func(watch.Event)) error {
				onReady()
				close(ready)
				<-ctx.Done()
				return ctx.Err()
			}})
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher not ready")
	}
	cancel()
	<-done
	failed, ok := collected.find("wake.failed")
	if !ok {
		t.Fatalf("records = %v", collected.kinds())
	}
	if failed.Target != "worker" || !strings.Contains(failed.Note, errPaneGone.Error()) {
		t.Fatalf("wake.failed record = %#v", failed)
	}
}

// A dispatcher restarted mid-session must not replay the ledger as if it had
// just happened.
func TestRunDoesNotReplayLedgerHistoryOnStart(t *testing.T) {
	root, session := t.TempDir(), "fledge-test-1234abcd"
	store := messaging.New(root, session)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{Name: "worker", PaneID: "%12", Harness: "codex", Caller: "user"}); err != nil {
		t.Fatal(err)
	}
	files := &fakeFiles{events: make(chan struct{}, 1), errs: make(chan error, 1)}
	client := &fakeHerdr{protocol: RequiredHerdrProtocol}
	client.setSnapshotPanes("%12")
	collected := &recorder{}
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Root: root, Session: session, Herdr: client, Emit: collected.emit,
			WatchFile: func(string) (FileWatcher, error) { return files, nil },
			Subscribe: func(ctx context.Context, _ []string, onReady func(), _ func(watch.Event)) error {
				onReady()
				close(ready)
				<-ctx.Done()
				return ctx.Err()
			}})
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher not ready")
	}
	// Pump the ledger tail with something new. Once the new line is traced, the
	// same pass would have carried the registration that preceded it.
	if _, err := store.Create(messaging.CreateParams{Sender: "user", Recipient: "worker", RecipientPane: "%12", Body: "hello"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	for {
		if _, ok := collected.find("message"); ok {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("records = %v", collected.kinds())
		case files.events <- struct{}{}:
		}
	}
	cancel()
	<-done
	if _, ok := collected.find("agent.start"); ok {
		t.Fatalf("history was replayed: %v", collected.kinds())
	}
}

// The diagnostic tail is secondary to delivery. A malformed or temporarily
// unreadable trace must be visible to the operator without ending dispatch.
func TestRunSurvivesTraceReadFailure(t *testing.T) {
	root, session := t.TempDir(), "fledge-test-1234abcd"
	store := messaging.New(root, session)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{Name: "worker", PaneID: "%12", Harness: "codex", Caller: "user"}); err != nil {
		t.Fatal(err)
	}
	files := &fakeFiles{events: make(chan struct{}, 1), errs: make(chan error, 1)}
	client := &fakeHerdr{protocol: RequiredHerdrProtocol}
	client.setSnapshotPanes("%12")
	collected := &recorder{}
	records := make(chan trace.Record, 16)
	ready := make(chan struct{})
	readErr := errors.New("diagnostic tail is unreadable")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Root: root, Session: session, Herdr: client,
			Emit: func(record trace.Record) {
				collected.emit(record)
				records <- record
			},
			WatchFile: func(string) (FileWatcher, error) { return files, nil },
			Subscribe: func(ctx context.Context, _ []string, onReady func(), _ func(watch.Event)) error {
				onReady()
				close(ready)
				<-ctx.Done()
				return ctx.Err()
			},
			ReadTrace: func(_ string, offset int64) ([]messaging.LedgerEntry, int64, error) {
				return nil, offset, readErr
			}})
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("dispatcher exited before becoming ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher not ready")
	}
	files.events <- struct{}{}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case record := <-records:
			if record.Kind == "dispatcher.trace-degraded" {
				if record.Note != readErr.Error() {
					t.Fatalf("dispatcher.trace-degraded record = %#v", record)
				}
				goto degraded
			}
		case err := <-done:
			t.Fatalf("dispatcher exited after trace read failure: %v", err)
		case <-deadline:
			t.Fatalf("records = %v", collected.kinds())
		}
	}

degraded:
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher did not stop after cancellation")
	}
}

// errPaneGone stands in for the transport reporting an undeliverable pane.
var errPaneGone = errors.New("pane %12 is gone")
