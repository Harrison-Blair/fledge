package watchproc

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
)

// listHerdr is a Herdr stub whose List result is configurable, so
// socketSubscriber's session-resolution branches can each be driven directly.
type listHerdr struct {
	sessions []herdr.Session
	listErr  error
}

func (l *listHerdr) Protocol(context.Context) (int, error) { return RequiredHerdrProtocol, nil }
func (l *listHerdr) List(context.Context) ([]herdr.Session, error) {
	return l.sessions, l.listErr
}
func (l *listHerdr) Snapshot(context.Context, string) (herdr.Snapshot, error) {
	return herdr.Snapshot{}, nil
}
func (l *listHerdr) PromptAgent(context.Context, string, string, string) error { return nil }

func TestSocketSubscriberResolutionErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	missingPath := filepath.Join(dir, "absent.sock")
	regularPath := filepath.Join(dir, "regular")
	if err := os.WriteFile(regularPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		herdr    *listHerdr
		wantFrag string
	}{
		{
			name:     "list fails",
			herdr:    &listHerdr{listErr: errors.New("herdr offline")},
			wantFrag: "resolve Herdr dispatcher socket",
		},
		{
			name:     "session absent",
			herdr:    &listHerdr{sessions: nil},
			wantFrag: "has no event socket",
		},
		{
			name:     "session present but not running",
			herdr:    &listHerdr{sessions: []herdr.Session{{Name: testSession, Running: false, SocketPath: "/anything"}}},
			wantFrag: "has no event socket",
		},
		{
			name:     "running session with empty socket path",
			herdr:    &listHerdr{sessions: []herdr.Session{{Name: testSession, Running: true, SocketPath: ""}}},
			wantFrag: "has no event socket",
		},
		{
			name:     "socket path does not exist",
			herdr:    &listHerdr{sessions: []herdr.Session{{Name: testSession, Running: true, SocketPath: missingPath}}},
			wantFrag: "inspect Herdr event socket",
		},
		{
			name:     "socket path is a regular file",
			herdr:    &listHerdr{sessions: []herdr.Session{{Name: testSession, Running: true, SocketPath: regularPath}}},
			wantFrag: "is not a socket",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			subscribe, err := socketSubscriber(context.Background(), tc.herdr, testSession)
			if subscribe != nil {
				t.Fatalf("expected nil Subscribe, got non-nil")
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantFrag) {
				t.Fatalf("error = %v, want fragment %q", err, tc.wantFrag)
			}
		})
	}
}

// A real unix socket satisfies socketSubscriber's stat and socket-mode checks,
// so it returns a usable Subscribe. Invoking that Subscribe with an already
// canceled context exercises the dial closure and returned wrapper without
// requiring the full Herdr handshake.
func TestSocketSubscriberAcceptsARealUnixSocket(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "s.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	client := &listHerdr{sessions: []herdr.Session{{Name: testSession, Running: true, SocketPath: path}}}
	subscribe, err := socketSubscriber(context.Background(), client, testSession)
	if err != nil {
		t.Fatalf("socketSubscriber() = %v", err)
	}
	if subscribe == nil {
		t.Fatal("socketSubscriber returned a nil Subscribe for a real socket")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	// The dial closure runs and, seeing a canceled context, fails fast; the point
	// is that the returned wrapper is invocable, not that it streams.
	if err := subscribe(canceled, []string{"p1"}, func() {}, func(herdr.Event) {}); err == nil {
		t.Fatal("expected the canceled subscription to return an error")
	}
}

// reconcileSnapshot projects a present pane's non-blank status onto the registry
// and leaves the status untouched when the snapshot carries a blank one.
func TestReconcileSnapshotProjectsStatus(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := messaging.New(root, testSession)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{
		Name: "worker", PaneID: "p1", Harness: "codex", Caller: messaging.UserIdentity}); err != nil {
		t.Fatal(err)
	}

	nonBlank := herdr.Snapshot{Panes: []herdr.Pane{{PaneID: "p1", AgentStatus: "working"}}}
	if err := reconcileSnapshot(store, []string{"p1"}, nonBlank); err != nil {
		t.Fatalf("reconcileSnapshot(non-blank) = %v", err)
	}
	agent, err := store.AgentByPane("p1")
	if err != nil {
		t.Fatal(err)
	}
	if agent.Status != "working" {
		t.Fatalf("status after non-blank projection = %q, want working", agent.Status)
	}

	// A blank status in the snapshot must not overwrite the projected one.
	blank := herdr.Snapshot{Panes: []herdr.Pane{{PaneID: "p1", AgentStatus: ""}}}
	if err := reconcileSnapshot(store, []string{"p1"}, blank); err != nil {
		t.Fatalf("reconcileSnapshot(blank) = %v", err)
	}
	agent, err = store.AgentByPane("p1")
	if err != nil {
		t.Fatal(err)
	}
	if agent.Status != "working" {
		t.Fatalf("status after blank projection = %q, want it unchanged at working", agent.Status)
	}
}

// A non-pane.closed stream event is projected onto the registry through
// store.RecordAgentStatus by the dispatcher's onEvent path.
func TestDispatcherRecordsStatusFromStreamEvent(t *testing.T) {
	h := newHarness(t)
	h.awaitReady(t)
	if _, _, err := h.store.RegisterAgent(messaging.RegisterParams{
		Name: "worker", PaneID: "p1", Harness: "codex", Caller: messaging.UserIdentity}); err != nil {
		t.Fatal(err)
	}
	// p1 is live with a blank status at subscribe time, so reconciliation leaves it
	// untouched; the status event below is what must drive RecordAgentStatus.
	h.client.setSnapshotPanes("p1")
	h.notifyLedger()

	select {
	case h.events <- herdr.Event{Type: "agent.status_changed", PaneID: "p1", AgentStatus: "blocked"}:
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher never accepted the status event")
	}

	deadline := time.After(5 * time.Second)
	for {
		agent, err := h.store.AgentByPane("p1")
		if err == nil && agent.Status == "blocked" {
			return
		}
		select {
		case err := <-h.done:
			t.Fatalf("dispatcher exited: %v", err)
		case <-deadline:
			t.Fatalf("agent status never reached blocked")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// A canceled context makes a wake's delivery outcome uncertain: drain joins the
// context error with the delivery error, records no terminal outcome, and leaves
// the wake pending for the next dispatcher to replay.
func TestDrainCanceledContextLeavesWakePending(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := messaging.New(root, testSession)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{
		Name: "worker", PaneID: "p1", Harness: "codex", Caller: messaging.UserIdentity}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(messaging.CreateParams{
		Sender: messaging.UserIdentity, Recipient: "worker", RecipientPane: "p1", Body: "hello"}); err != nil {
		t.Fatal(err)
	}

	client := &fakeHerdr{
		protocol: RequiredHerdrProtocol,
		refuse:   map[string]error{"worker": errors.New("delivery boom")},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := drain(ctx, client, testSession, store)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("drain() = %v, want a joined context.Canceled", err)
	}
	if err == nil || !strings.Contains(err.Error(), "delivery boom") {
		t.Fatalf("drain() = %v, want it to carry the delivery error", err)
	}

	pending, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending wakes = %d, want the wake left pending for replay", len(pending))
	}
}

// A file-watcher error surfaces as a wrapped 'watch session ledger' failure that
// ends the dispatcher.
func TestRunDispatcherReturnsWrappedLedgerWatchError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := messaging.New(root, testSession)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	files := &fakeFiles{events: make(chan struct{}, 1), errs: make(chan error, 1)}
	client := &fakeHerdr{protocol: RequiredHerdrProtocol}
	ready := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runDispatcher(ctx, Options{Root: root, Session: testSession, Herdr: client,
			WatchFile: func(string) (FileWatcher, error) { return files, nil },
			Subscribe: func(ctx context.Context, _ []string, onReady func(), _ func(herdr.Event)) error {
				onReady()
				<-ctx.Done()
				return ctx.Err()
			},
			Ready: func() { ready <- struct{}{} }})
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("dispatcher exited before ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher never became ready")
	}

	files.errs <- errors.New("inotify exploded")
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "watch session ledger") {
			t.Fatalf("runDispatcher() = %v, want a wrapped 'watch session ledger' error", err)
		}
		if !strings.Contains(err.Error(), "inotify exploded") {
			t.Fatalf("runDispatcher() = %v, want it to wrap the watcher error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher did not exit on the watcher error")
	}
}
