package watchproc

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
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
		done <- runDispatcher(ctx, Options{Root: root, Session: session, Herdr: client,
			WatchFile: func(string) (FileWatcher, error) { return files, nil },
			Subscribe: func(ctx context.Context, _ []string, onReady func(), _ func(herdr.Event)) error {
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

func TestRunRequiresProtocol19WithRestartGuidance(t *testing.T) {
	err := runDispatcher(context.Background(), Options{Root: t.TempDir(), Session: "fledge-test-1234abcd", Herdr: &fakeHerdr{protocol: 18}})
	if err == nil || !strings.Contains(err.Error(), "protocol 19") || !strings.Contains(err.Error(), "fledge stop and fledge start") {
		t.Fatalf("error = %v", err)
	}
}
