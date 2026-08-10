package watchproc

import (
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
)

// socketHerdr is a Herdr client for the assembly test: List advertises a real
// unix socket as a running session so socketSubscriber resolves it, Snapshot
// feeds onReady reconciliation, and PromptAgent records deliveries so a wake
// that travelled the real ledger watcher can be observed.
type socketHerdr struct {
	mu       sync.Mutex
	prompts  []string
	sessions []herdr.Session
	snapshot herdr.Snapshot
}

func (h *socketHerdr) Protocol(context.Context) (int, error) { return RequiredHerdrProtocol, nil }

func (h *socketHerdr) List(context.Context) ([]herdr.Session, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]herdr.Session(nil), h.sessions...), nil
}

func (h *socketHerdr) Snapshot(context.Context, string) (herdr.Snapshot, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.snapshot, nil
}

func (h *socketHerdr) PromptAgent(_ context.Context, _, _, prompt string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.prompts = append(h.prompts, prompt)
	return nil
}

func (h *socketHerdr) delivered() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.prompts...)
}

// herdrEventServer stands in for Herdr's event socket. It speaks the wire
// contract herdr.Subscribe expects: read the subscription request, write the
// acknowledgement, then push the queued status lines. It keeps the connection
// open afterwards so the dispatcher's stream never ends on its own; only the
// dispatcher's own context cancellation (which closes the client side) ends it,
// exactly as a healthy Herdr would behave.
type herdrEventServer struct {
	listener net.Listener
	path     string
	lines    [][]byte
	done     chan struct{}
}

const subscriptionAck = `{"result":{"type":"subscription_started"}}` + "\n"

// startHerdrEventServer binds a unix socket under a short temp path and serves a
// single subscriber, pushing lines after the ack. t.Cleanup tears it down and
// waits for the serving goroutine so nothing leaks under -race.
func startHerdrEventServer(t *testing.T, lines ...string) *herdrEventServer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "events.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen unix %q: %v", path, err)
	}
	raw := make([][]byte, 0, len(lines))
	for _, line := range lines {
		raw = append(raw, []byte(line))
	}
	s := &herdrEventServer{listener: listener, path: path, lines: raw, done: make(chan struct{})}
	go s.serve()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
			t.Error("herdr event server did not stop")
		}
	})
	return s
}

func (s *herdrEventServer) serve() {
	defer close(s.done)
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	// Consume the subscription request line before acknowledging, mirroring the
	// real server's request/response ordering.
	reader := bufio.NewReader(conn)
	if _, err := reader.ReadBytes('\n'); err != nil {
		return
	}
	if _, err := conn.Write([]byte(subscriptionAck)); err != nil {
		return
	}
	for _, line := range s.lines {
		if _, err := conn.Write(line); err != nil {
			return
		}
	}
	// Hold the stream open until the dispatcher closes its side on context
	// cancellation, so the stream never ends on its own and trips a fatal exit.
	_, _ = io.Copy(io.Discard, conn)
}

// TestDispatcherAssemblyDeliversWakeAndProjectsStatus wires the production
// dispatcher to its real seams: the native ledger file watcher (WatchFile nil)
// and the real Herdr event socket subscriber (Subscribe nil). A real unix
// socket speaks the subscribe protocol and pushes a status change; a real wake
// appended to the real ledger must be delivered through PromptAgent, and the
// pushed status must project onto the registry.
func TestDispatcherAssemblyDeliversWakeAndProjectsStatus(t *testing.T) {
	root := t.TempDir()
	store := messaging.New(root, testSession)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{
		Name: "worker", PaneID: "p1", Harness: "codex", Caller: messaging.UserIdentity}); err != nil {
		t.Fatal(err)
	}

	server := startHerdrEventServer(t,
		`{"event":"pane.agent_status_changed","data":{"pane_id":"p1","agent_status":"working"}}`+"\n")
	client := &socketHerdr{
		sessions: []herdr.Session{{Name: testSession, Running: true, SocketPath: server.path}},
		// p1 is live with a blank status at subscribe time, so onReady
		// reconciliation leaves it untouched and the pushed status is what must
		// drive the projection to "working".
		snapshot: herdr.Snapshot{Panes: []herdr.Pane{{PaneID: "p1"}}},
	}

	ready := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runDispatcher(ctx, Options{
			Root: root, Session: testSession, Herdr: client,
			Ready: func() { ready <- struct{}{} },
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("dispatcher did not stop")
		}
	})

	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("dispatcher exited before ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher never became ready")
	}

	// The status pushed over the real socket must project onto the registry.
	awaitStatus(t, store, done, "p1", "working")

	// Appending a real wake to the real ledger must reach the dispatcher through
	// the native file watcher and be delivered via PromptAgent.
	if _, err := store.Create(messaging.CreateParams{
		Sender: messaging.UserIdentity, Recipient: "worker", RecipientPane: "p1",
		Body: "integration-wake-body"}); err != nil {
		t.Fatal(err)
	}
	prompts := awaitDeliveredPrompt(t, client, done)
	if !strings.Contains(prompts[0], "integration-wake-body") {
		t.Fatalf("delivered prompt = %q, want it to carry the wake body", prompts[0])
	}
}

// awaitStatus polls until the agent on pane reaches want, or the dispatcher
// exits, or a bounded deadline elapses. Polling re-reads registry state the
// dispatcher owns, so no elapsed-time sleep is the correctness mechanism.
func awaitStatus(t *testing.T, store *messaging.Store, done <-chan error, pane, want string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		agent, err := store.AgentByPane(pane)
		if err == nil && agent.Status == want {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("dispatcher exited before %s reached %q: %v", pane, want, err)
		case <-deadline:
			t.Fatalf("agent %s status never reached %q", pane, want)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// awaitDeliveredPrompt polls until at least one wake has been delivered through
// PromptAgent, or the dispatcher exits, or a bounded deadline elapses.
func awaitDeliveredPrompt(t *testing.T, client *socketHerdr, done <-chan error) []string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if prompts := client.delivered(); len(prompts) >= 1 {
			return prompts
		}
		select {
		case err := <-done:
			t.Fatalf("dispatcher exited before delivering a wake: %v", err)
		case <-deadline:
			t.Fatal("no wake was delivered through the real ledger watcher")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestDispatcherAssemblyRejectsUnusableSocket drives the negative resolution
// paths through the full runDispatcher assembly (real ledger watcher, then the
// real socketSubscriber), confirming the glue surfaces a socket that cannot be
// used rather than coming up.
func TestDispatcherAssemblyRejectsUnusableSocket(t *testing.T) {
	cases := []struct {
		name     string
		sessions func(dir string) []herdr.Session
		wantFrag string
	}{
		{
			name:     "no running session",
			sessions: func(string) []herdr.Session { return nil },
			wantFrag: "has no event socket",
		},
		{
			name: "session present but not running",
			sessions: func(string) []herdr.Session {
				return []herdr.Session{{Name: testSession, Running: false, SocketPath: "/anything"}}
			},
			wantFrag: "has no event socket",
		},
		{
			name: "socket path is a regular file",
			sessions: func(dir string) []herdr.Session {
				regular := filepath.Join(dir, "not-a-socket")
				if err := os.WriteFile(regular, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return []herdr.Session{{Name: testSession, Running: true, SocketPath: regular}}
			},
			wantFrag: "is not a socket",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			store := messaging.New(root, testSession)
			if _, err := store.Initialize(); err != nil {
				t.Fatal(err)
			}
			client := &socketHerdr{sessions: tc.sessions(t.TempDir())}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := runDispatcher(ctx, Options{Root: root, Session: testSession, Herdr: client})
			if err == nil || !strings.Contains(err.Error(), tc.wantFrag) {
				t.Fatalf("runDispatcher() = %v, want fragment %q", err, tc.wantFrag)
			}
		})
	}
}
