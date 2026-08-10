package dispatcher

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/watch"
)

type fakeFiles struct {
	events chan struct{}
	errs   chan error
}

func (f *fakeFiles) Events() <-chan struct{} { return f.events }
func (f *fakeFiles) Errors() <-chan error    { return f.errs }
func (f *fakeFiles) Close() error            { return nil }

type fakeHerdr struct {
	protocol int
	mu       sync.Mutex
	prompts  []string
	// refuse reports a delivery failure for the named recipient, standing in for
	// a wedged or already-departed pane.
	refuse map[string]error
	// snapshot is the live session state returned to the dispatcher's onReady
	// reconciliation; snapshotErr forces a fail-closed snapshot read.
	snapshot    herdr.Snapshot
	snapshotErr error
}

func (f *fakeHerdr) Protocol(context.Context) (int, error)         { return f.protocol, nil }
func (f *fakeHerdr) List(context.Context) ([]herdr.Session, error) { return nil, nil }
func (f *fakeHerdr) Snapshot(context.Context, string) (herdr.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshot, f.snapshotErr
}

// setSnapshotPanes replaces the fake's live pane set with one Pane per id,
// modelling the panes the dispatcher's onReady reconciliation will observe.
func (f *fakeHerdr) setSnapshotPanes(ids ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	panes := make([]herdr.Pane, 0, len(ids))
	for _, id := range ids {
		panes = append(panes, herdr.Pane{PaneID: id})
	}
	f.snapshot = herdr.Snapshot{Panes: panes}
}

func (f *fakeHerdr) PromptAgent(_ context.Context, _, recipient, prompt string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.refuse[recipient]; ok {
		return err
	}
	f.prompts = append(f.prompts, prompt)
	return nil
}

func (f *fakeHerdr) delivered() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompts...)
}

func TestRunReplaysStableWakeAndRecordsOutcomes(t *testing.T) {
	root, session := t.TempDir(), "fledge-test-1234abcd"
	store := messaging.New(root, session)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{Name: "worker", PaneID: "p1", Harness: "codex", Caller: "user"}); err != nil {
		t.Fatal(err)
	}
	message, err := store.Create(messaging.CreateParams{Sender: "user", Recipient: "worker", RecipientPane: "p1", Body: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	wakes, _ := store.PendingWakes()
	if _, err := store.RecordAttempt(message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordWakeAttempt(wakes[0].ID); err != nil {
		t.Fatal(err)
	}
	files := &fakeFiles{events: make(chan struct{}, 1), errs: make(chan error, 1)}
	client := &fakeHerdr{protocol: RequiredHerdrProtocol}
	// The worker's pane is live, so the onReady snapshot must list it; otherwise
	// reconciliation would correctly retire a pane the fake claims is gone.
	client.setSnapshotPanes("p1")
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Root: root, Session: session, Herdr: client,
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
	case <-time.After(time.Second):
		t.Fatal("dispatcher not ready")
	}
	cancel()
	<-done
	client.mu.Lock()
	prompts := append([]string(nil), client.prompts...)
	client.mu.Unlock()
	if len(prompts) != 1 || !strings.Contains(prompts[0], "Delivery-ID: "+wakes[0].ID) {
		t.Fatalf("prompts = %#v", prompts)
	}
	message, _ = store.Get(message.ID)
	if message.Status != messaging.StatusDelivered {
		t.Fatalf("message status = %s", message.Status)
	}
	wakes, _ = store.PendingWakes()
	if len(wakes) != 0 {
		t.Fatalf("pending wakes = %#v", wakes)
	}
}

// A crash after a message's delivery outcome is persisted but before its wake
// outcome is recorded leaves a delivered message beside a still-pending wake. On
// relaunch the recipient's pane is gone, so drain's PromptAgent fails; recording
// a delivery outcome on the already-delivered message would be rejected by the
// store, surface as a drain error, and exit the dispatcher — permanently
// stalling every later wake. drain must instead skip the outcome for the
// terminal message and still terminalize the poison wake so the loop continues.
func TestDrainTerminalizesFailedWakeForAlreadyDeliveredMessage(t *testing.T) {
	root, session := t.TempDir(), "fledge-test-1234abcd"
	store := messaging.New(root, session)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	for _, params := range []messaging.RegisterParams{
		{Name: "poison", PaneID: "p1", Harness: "codex", Caller: messaging.UserIdentity},
		{Name: "healthy", PaneID: "p2", Harness: "codex", Caller: messaging.UserIdentity},
	} {
		if _, _, err := store.RegisterAgent(params); err != nil {
			t.Fatal(err)
		}
	}

	// Create the poison message first and drive it to the exact post-crash state:
	// the message is delivered, but its wake never got an outcome, so it is still
	// replayable.
	poison, err := store.Create(messaging.CreateParams{
		Sender: messaging.UserIdentity, Recipient: "poison", RecipientPane: "p1", Body: "first"})
	if err != nil {
		t.Fatal(err)
	}
	wakes, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	var poisonWake messaging.Wake
	for _, wake := range wakes {
		if wake.ReferenceID == poison.ID {
			poisonWake = wake
		}
	}
	if poisonWake.ID == "" {
		t.Fatalf("no wake for poison message; wakes = %#v", wakes)
	}
	if _, err := store.RecordAttempt(poison.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordWakeAttempt(poisonWake.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordDelivery(poison.ID, true, ""); err != nil {
		t.Fatal(err)
	} // Deliberately omit RecordWakeOutcome: this models the crash window.

	// A healthy message queued after the poison one proves drain kept going.
	healthy, err := store.Create(messaging.CreateParams{
		Sender: messaging.UserIdentity, Recipient: "healthy", RecipientPane: "p2", Body: "second"})
	if err != nil {
		t.Fatal(err)
	}

	client := &fakeHerdr{protocol: RequiredHerdrProtocol,
		refuse: map[string]error{"poison": errors.New("pane p1 is gone")}}
	if err := drain(context.Background(), client, session, store); err != nil {
		t.Fatalf("drain returned %v, want nil", err)
	}

	// The already-delivered message must be untouched, not force-failed.
	got, err := store.Get(poison.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != messaging.StatusDelivered {
		t.Fatalf("poison message status = %s, want delivered", got.Status)
	}
	// The healthy message went out and is the sole accepted prompt.
	delivered, err := store.Get(healthy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if delivered.Status != messaging.StatusDelivered {
		t.Fatalf("healthy message status = %s, want delivered", delivered.Status)
	}
	if prompts := client.delivered(); len(prompts) != 1 || !strings.Contains(prompts[0], "second") {
		t.Fatalf("prompts = %#v, want only the healthy wake", prompts)
	}
	// The poison wake was terminalized, so nothing is left to replay.
	pending, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending wakes = %#v, want none", pending)
	}
}

func TestRunRequiresProtocol19WithRestartGuidance(t *testing.T) {
	err := Run(context.Background(), Options{Root: t.TempDir(), Session: "fledge-test-1234abcd", Herdr: &fakeHerdr{protocol: 18}})
	if err == nil || !strings.Contains(err.Error(), "protocol 19") || !strings.Contains(err.Error(), "fledge stop and fledge start") {
		t.Fatalf("error = %v", err)
	}
}
