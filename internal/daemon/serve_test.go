package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/filebridge"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/version"
)

// scriptedListener returns a queued sequence of Accept results, then blocks
// forever, so a test can drive Serve through specific error conditions.
type scriptedListener struct {
	mu      sync.Mutex
	results []error
	calls   int
	block   chan struct{}
}

func (l *scriptedListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	l.calls++
	if len(l.results) == 0 {
		l.mu.Unlock()
		<-l.block
		return nil, net.ErrClosed
	}
	err := l.results[0]
	l.results = l.results[1:]
	l.mu.Unlock()
	return nil, err
}

func (l *scriptedListener) Close() error   { return nil }
func (l *scriptedListener) Addr() net.Addr { return fakeAddr{} }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "fake" }
func (fakeAddr) String() string  { return "fake" }

// tempError is a net.Error that reports itself as temporary, like the EMFILE a
// busy host can return from accept(2).
type tempError struct{}

func (tempError) Error() string   { return "temporary accept failure" }
func (tempError) Timeout() bool   { return true }
func (tempError) Temporary() bool { return true }

type afterPassJournal struct {
	io.WriteCloser
	after  *atomic.Bool
	writes *atomic.Int32
}

func (j *afterPassJournal) Write(p []byte) (int, error) {
	if j.after.Load() {
		j.writes.Add(1)
	}
	return j.WriteCloser.Write(p)
}

func serveTestDaemon(ln net.Listener) *Daemon {
	d := &Daemon{ln: ln, debug: log.New(io.Discard, "", 0), done: make(chan struct{})}
	// Keep serveFileRequests from starting; these tests exercise Accept only.
	d.fileOnce.Do(func() {})
	return d
}

// A non-temporary Accept error must terminate Serve with that error, not a
// clean nil: reporting a clean exit on a real failure hides the fault.
func TestServeReturnsNonTemporaryAcceptError(t *testing.T) {
	ln := &scriptedListener{results: []error{syscall.EINVAL}, block: make(chan struct{})}
	d := serveTestDaemon(ln)

	err := d.Serve()
	if err == nil {
		t.Fatal("Serve returned nil on a non-temporary accept error; a real failure was reported as a clean exit")
	}
}

// A temporary Accept error (e.g. EMFILE) must be retried, not treated as a
// shutdown. An intentional listener close after it still returns nil.
func TestServeRetriesTemporaryAcceptError(t *testing.T) {
	ln := &scriptedListener{results: []error{tempError{}, net.ErrClosed}, block: make(chan struct{})}
	d := serveTestDaemon(ln)

	done := make(chan error, 1)
	go func() { done <- d.Serve() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v after a temporary error then clean close; want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after a temporary error then a listener close")
	}

	ln.mu.Lock()
	calls := ln.calls
	ln.mu.Unlock()
	if calls < 2 {
		t.Fatalf("Accept called %d times; a temporary error was not retried", calls)
	}
}

// A client that never reads its response must not pin the handler goroutine:
// the response write carries a deadline. net.Pipe is fully synchronous, so a
// write blocks until the deadline unless the peer reads.
func TestHandleWriteDeadlineFreesBlockedWriter(t *testing.T) {
	saved := writeTimeout
	writeTimeout = 100 * time.Millisecond
	defer func() { writeTimeout = saved }()

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	d := &Daemon{debug: log.New(io.Discard, "", 0)}

	done := make(chan struct{})
	go func() {
		d.handle(serverConn)
		close(done)
	}()

	// Send a request the handler will answer, then never read the response.
	if err := json.NewEncoder(clientConn).Encode(protocol.Request{Op: protocol.OpList}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handle pinned on a response write to a non-reading client")
	}
}

func TestStatusReportsDaemonProcessAndVersion(t *testing.T) {
	d := newTestDaemon(t)

	resp := d.dispatch(&protocol.Request{Op: protocol.OpStatus}, nil)
	if resp.Error != "" {
		t.Fatalf("status error = %q", resp.Error)
	}
	if resp.DaemonPID != os.Getpid() {
		t.Fatalf("daemon_pid = %d, want %d", resp.DaemonPID, os.Getpid())
	}
	if resp.DaemonVersion != version.Get() {
		t.Fatalf("daemon_version = %q, want %q", resp.DaemonVersion, version.Get())
	}
}

func TestFileBridgeStatusReportsDaemonProcessAndVersion(t *testing.T) {
	d := newTestDaemon(t)
	served := make(chan error, 1)
	go func() { served <- d.Serve() }()

	id, err := filebridge.Submit(d.root, d.flockName, protocol.Request{Op: protocol.OpStatus})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := filebridge.Await(d.root, d.flockName, id, time.Second, time.Second)
	if err != nil {
		t.Fatalf("filebridge status: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("status error = %q", resp.Error)
	}
	if resp.DaemonPID != os.Getpid() {
		t.Fatalf("daemon_pid = %d, want %d", resp.DaemonPID, os.Getpid())
	}
	if resp.DaemonVersion != version.Get() {
		t.Fatalf("daemon_version = %q, want %q", resp.DaemonVersion, version.Get())
	}

	d.Close()
	if err := <-served; err != nil {
		t.Fatalf("Serve after close = %v", err)
	}
}

func TestFileBridgeShutdownRespondsBeforeClose(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprintf("attempt-%02d", i), func(t *testing.T) {
			d := newTestDaemon(t)
			served := make(chan error, 1)
			go func() { served <- d.Serve() }()

			id, err := filebridge.Submit(d.root, d.flockName, protocol.Request{Op: protocol.OpShutdown})
			if err != nil {
				t.Fatal(err)
			}
			resp, err := filebridge.Await(d.root, d.flockName, id, time.Second, time.Second)
			if err != nil {
				t.Fatalf("filebridge shutdown await: %v", err)
			}
			if resp.Error != "" {
				t.Fatalf("shutdown error = %q", resp.Error)
			}
			if resp.DaemonPID != os.Getpid() || resp.DaemonVersion != version.Get() {
				t.Fatalf("shutdown response = %+v", resp)
			}
			if err := <-served; err != nil {
				t.Fatalf("Serve after shutdown = %v", err)
			}
		})
	}
}

func TestShutdownReleasesParkedWaiters(t *testing.T) {
	d := newTestDaemon(t)
	sender, err := d.register(&protocol.Request{Type: "sender", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := d.register(&protocol.Request{Type: "receiver", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}

	type waitResult struct {
		resp protocol.Response
		err  error
	}
	done := make(chan waitResult, 1)
	go func() {
		d.beginRequest()
		resp, err := d.wait(&protocol.Request{As: receiver.Name, From: sender.Name}, nil)
		d.endRequest()
		done <- waitResult{resp: resp, err: err}
	}()
	waitFor(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.waiters) == 1
	})

	resp := d.dispatch(&protocol.Request{Op: protocol.OpShutdown}, nil)
	if resp.Error != "" {
		t.Fatalf("shutdown error = %q", resp.Error)
	}

	select {
	case result := <-done:
		if result.err == nil || !strings.Contains(result.err.Error(), "shutting down") {
			t.Fatalf("wait result = %+v, %v; want shutdown error", result.resp, result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not release parked waiter")
	}
}

func TestShutdownHoldsOwnershipUntilActiveRequestsDrain(t *testing.T) {
	d := newTestDaemon(t)
	served := make(chan error, 1)
	go func() { served <- d.Serve() }()

	d.beginRequest()
	resp := d.dispatch(&protocol.Request{Op: protocol.OpShutdown}, nil)
	if resp.Error != "" {
		t.Fatalf("shutdown error = %q", resp.Error)
	}

	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve after shutdown = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not close the listener")
	}

	type startResult struct {
		d   *Daemon
		err error
	}
	started := make(chan startResult, 1)
	go func() {
		next, err := New(d.root, d.flockName)
		started <- startResult{d: next, err: err}
	}()

	select {
	case result := <-started:
		if result.d != nil {
			result.d.Close()
		}
		t.Fatalf("replacement New returned before the active request drained: %v", result.err)
	case <-time.After(150 * time.Millisecond):
	}

	d.endRequest()

	select {
	case result := <-started:
		if result.err != nil {
			t.Fatalf("replacement New after drain: %v", result.err)
		}
		result.d.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("replacement New did not proceed after active request drained")
	}
}

func TestFileBridgeDrainCompletesBeforeOwnershipPasses(t *testing.T) {
	d := newTestDaemon(t)
	var after atomic.Bool
	var writes atomic.Int32
	d.journal = &afterPassJournal{WriteCloser: d.journal, after: &after, writes: &writes}

	served := make(chan error, 1)
	go func() { served <- d.Serve() }()

	d.fileMu.Lock()
	if _, err := filebridge.Submit(d.root, d.flockName, protocol.Request{
		Op: protocol.OpRegister, Type: "bridge", PID: os.Getpid(),
	}); err != nil {
		d.fileMu.Unlock()
		t.Fatal(err)
	}

	shutdown := make(chan protocol.Response, 1)
	go func() {
		shutdown <- d.dispatch(&protocol.Request{Op: protocol.OpShutdown}, nil)
	}()
	var serveErr error
	select {
	case serveErr = <-served:
		if serveErr != nil {
			d.fileMu.Unlock()
			t.Fatalf("Serve after shutdown listener close = %v", serveErr)
		}
	case <-time.After(2 * time.Second):
		d.fileMu.Unlock()
		t.Fatal("shutdown did not close listener while bridge drain was blocked")
	}

	type startResult struct {
		d   *Daemon
		err error
	}
	started := make(chan startResult, 1)
	go func() {
		next, err := New(d.root, d.flockName)
		started <- startResult{d: next, err: err}
	}()

	select {
	case result := <-started:
		if result.d != nil {
			result.d.Close()
		}
		d.fileMu.Unlock()
		t.Fatalf("replacement New returned while old bridge drain was blocked: %v", result.err)
	case <-time.After(100 * time.Millisecond):
	}

	d.fileMu.Unlock()

	select {
	case resp := <-shutdown:
		if resp.Error != "" {
			t.Fatalf("shutdown error = %q", resp.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not finish after bridge drain unblocked")
	}

	select {
	case result := <-started:
		if result.err != nil {
			t.Fatalf("replacement New: %v", result.err)
		}
		after.Store(true)
		time.Sleep(100 * time.Millisecond)
		if got := writes.Load(); got != 0 {
			result.d.Close()
			t.Fatalf("old daemon wrote journal after ownership passed: %d writes", got)
		}
		result.d.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("replacement New did not proceed after bridge drain")
	}

	_ = serveErr
}
