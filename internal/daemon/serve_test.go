package daemon

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/protocol"
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
