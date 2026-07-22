package daemon

import (
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

// newTestDaemon builds a daemon over a scratch workspace.
func newTestDaemon(t *testing.T) *Daemon {
	t.Helper()
	root := t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	d, err := New(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	d.skipReadiness = true
	t.Cleanup(func() { d.Close() })
	return d
}

// A watch whose probe reports the session gone must stop Serve.
func TestWatchSessionExitsWhenSessionGone(t *testing.T) {
	d := newTestDaemon(t)

	gone := make(chan struct{})
	go d.WatchSession(func() bool { close(gone); return true }, time.Millisecond)

	served := make(chan error, 1)
	go func() { served <- d.Serve() }()

	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve returned %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after the session went away")
	}

	select {
	case <-gone:
	default:
		t.Fatal("probe was never called")
	}
}

// While the session is up the daemon must keep serving.
func TestWatchSessionKeepsServingWhileSessionUp(t *testing.T) {
	d := newTestDaemon(t)

	probed := make(chan struct{}, 100)
	go d.WatchSession(func() bool {
		select {
		case probed <- struct{}{}:
		default:
		}
		return false
	}, time.Millisecond)

	served := make(chan error, 1)
	go func() { served <- d.Serve() }()

	select {
	case <-served:
		t.Fatal("Serve returned while the session was still up")
	case <-time.After(100 * time.Millisecond):
	}

	if len(probed) == 0 {
		t.Fatal("probe was never called")
	}
}

const titleMethod = "client.window_title.set"

// The watch tick must brand the session's window as fledge-managed, and stop
// re-asserting the title once herdr reports it landed on an attached client.
func TestWatchSessionSetsWindowTitleOnce(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		titleMethod: `{"id":"1","result":{"changed":true,"reason":"set"}}`,
	})
	d := boundDaemon(t, f)

	go d.WatchSession(func() bool { return false }, time.Millisecond)
	defer d.Close()

	waitFor(t, func() bool { return f.count(titleMethod) > 0 })
	if p := f.params(titleMethod); p["title"] != "fledge · test" {
		t.Errorf("title = %v, want %q", p["title"], "fledge · test")
	}

	// Many ticks have passed by now; a landed title must not be re-sent.
	time.Sleep(50 * time.Millisecond)
	if n := f.count(titleMethod); n != 1 {
		t.Errorf("herdr saw %d title calls, want 1 after the title landed", n)
	}
}

// Until a client attaches, herdr changes nothing and the daemon must keep
// retrying — the operator attaches after the daemon is already up.
func TestWatchSessionRetriesWindowTitleUntilAttached(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		titleMethod: `{"id":"1","result":{"changed":false,"reason":"no_foreground_client"}}`,
	})
	d := boundDaemon(t, f)

	go d.WatchSession(func() bool { return false }, time.Millisecond)
	defer d.Close()

	waitFor(t, func() bool { return f.count(titleMethod) > 2 })
}

// An unbound daemon has no session to title and must not dial anything.
func TestWatchSessionUnboundSetsNoTitle(t *testing.T) {
	d := boundDaemon(t, nil)

	go d.WatchSession(func() bool { return false }, time.Millisecond)
	defer d.Close()

	time.Sleep(20 * time.Millisecond)
	if d.titled {
		t.Error("unbound daemon reported a title set")
	}
}

// waitFor blocks until cond holds, failing the test if it never does.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition never held")
}

// Closing the daemon must stop the watch goroutine rather than leaking it.
func TestWatchSessionStopsOnClose(t *testing.T) {
	d := newTestDaemon(t)

	returned := make(chan struct{})
	go func() {
		d.WatchSession(func() bool { return false }, time.Millisecond)
		close(returned)
	}()

	d.Close()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("WatchSession did not return after Close")
	}
}
