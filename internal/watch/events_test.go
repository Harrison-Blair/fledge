package watch

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	ackLine   = `{"id":"fledge-watch","result":{"type":"subscription_started"}}` + "\n"
	eventLine = `{"event":"pane.agent_status_changed","data":{"pane_id":"p1","workspace_id":"w1","agent_status":"blocked","agent":"reviewer"}}` + "\n"
)

func TestSubscribeRequest(t *testing.T) {
	t.Parallel()

	request, err := subscribeRequest([]string{"p1", "p2"})
	if err != nil {
		t.Fatalf("subscribeRequest() error = %v", err)
	}

	want := `{"id":"fledge-watch","method":"events.subscribe","params":{"subscriptions":[{"type":"pane.agent_status_changed","pane_id":"p1"},{"type":"pane.closed","pane_id":"p1"},{"type":"pane.agent_status_changed","pane_id":"p2"},{"type":"pane.closed","pane_id":"p2"}]}}` + "\n"
	if string(request) != want {
		t.Errorf("subscribeRequest() =\n%s\nwant\n%s", request, want)
	}
}

func TestSubscribeRequestWithoutPanes(t *testing.T) {
	t.Parallel()

	if _, err := subscribeRequest(nil); err == nil {
		t.Error("subscribeRequest(nil) error = nil, want an error")
	}
}

func TestDecodeEventLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		line   string
		want   Event
		wantOK bool
	}{
		{
			name:   "complete event",
			line:   eventLine,
			want:   Event{Type: agentStatusEvt, PaneID: "p1", AgentStatus: "blocked", Agent: "reviewer"},
			wantOK: true,
		},
		{
			name:   "extra fields ignored",
			line:   `{"event":"pane.agent_status_changed","data":{"pane_id":"p1","agent_status":"working","mystery":7}}`,
			want:   Event{Type: agentStatusEvt, PaneID: "p1", AgentStatus: "working"},
			wantOK: true,
		},
		{name: "wrong event type", line: `{"event":"pane.output","data":{"pane_id":"p1","agent_status":"blocked"}}`},
		{name: "subscription ack", line: ackLine},
		{name: "malformed json", line: "{not json"},
		{name: "blank line", line: "\n"},
		{name: "missing pane", line: `{"event":"pane.agent_status_changed","data":{"agent_status":"blocked"}}`},
		{name: "missing status", line: `{"event":"pane.agent_status_changed","data":{"pane_id":"p1"}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := decodeEventLine([]byte(test.line))
			if ok != test.wantOK {
				t.Fatalf("decodeEventLine(%q) ok = %t, want %t", test.line, ok, test.wantOK)
			}
			if ok && got != test.want {
				t.Errorf("decodeEventLine(%q) = %+v, want %+v", test.line, got, test.want)
			}
		})
	}
}

func TestSubscribeStreamsEventsUntilTheServerCloses(t *testing.T) {
	t.Parallel()

	requests := make(chan string, 1)
	dial := fakeHerdr(t, func(conn net.Conn) {
		requests <- readLine(conn)
		writeAll(conn,
			ackLine,
			eventLine,
			"{not json\n",
			`{"event":"pane.output","data":{"pane_id":"p1"}}`+"\n",
			`{"event":"pane.agent_status_changed","data":{"pane_id":"p2","workspace_id":"w1","agent_status":"working","agent":"migrator"}}`+"\n",
		)
	})

	var events []Event
	err := Subscribe(t.Context(), dial, []string{"p1", "p2"}, func() {}, func(event Event) {
		events = append(events, event)
	})
	if err == nil {
		t.Fatal("Subscribe() error = nil, want an error when the server closes the stream")
	}

	want := []Event{
		{Type: agentStatusEvt, PaneID: "p1", AgentStatus: "blocked", Agent: "reviewer"},
		{Type: agentStatusEvt, PaneID: "p2", AgentStatus: "working", Agent: "migrator"},
	}
	if len(events) != len(want) {
		t.Fatalf("Subscribe() delivered %d events (%+v), want %d", len(events), events, len(want))
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("Subscribe() event %d = %+v, want %+v", i, events[i], want[i])
		}
	}

	select {
	case request := <-requests:
		want := `{"id":"fledge-watch","method":"events.subscribe","params":{"subscriptions":[{"type":"pane.agent_status_changed","pane_id":"p1"},{"type":"pane.closed","pane_id":"p1"},{"type":"pane.agent_status_changed","pane_id":"p2"},{"type":"pane.closed","pane_id":"p2"}]}}`
		if request != want {
			t.Errorf("Subscribe() sent %s, want %s", request, want)
		}
	default:
		t.Error("Subscribe() sent no subscription request")
	}
}

func TestSubscribeSignalsReadinessAfterTheAck(t *testing.T) {
	t.Parallel()

	dial := fakeHerdr(t, func(conn net.Conn) {
		readLine(conn)
		writeAll(conn, ackLine, eventLine)
	})

	var order []string
	onReady := func() { order = append(order, "ready") }
	onEvent := func(Event) { order = append(order, "event") }

	if err := Subscribe(t.Context(), dial, []string{"p1"}, onReady, onEvent); err == nil {
		t.Fatal("Subscribe() error = nil, want an error when the server closes the stream")
	}

	want := []string{"ready", "event"}
	if len(order) != len(want) {
		t.Fatalf("Subscribe() callbacks = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("Subscribe() callbacks = %v, want %v", order, want)
		}
	}
}

func TestSubscribeSkipsReadinessWithoutAnAck(t *testing.T) {
	t.Parallel()

	dial := fakeHerdr(t, func(conn net.Conn) {
		readLine(conn)
		writeAll(conn, eventLine, eventLine)
	})

	ready := 0
	if err := Subscribe(t.Context(), dial, []string{"p1"}, func() { ready++ }, func(Event) {}); err == nil {
		t.Fatal("Subscribe() error = nil, want an error when the ack is missing")
	}
	if ready != 0 {
		t.Errorf("Subscribe() signalled readiness %d times, want 0 without an ack", ready)
	}
}

func TestSubscribeWaitsForASlowAck(t *testing.T) {
	t.Parallel()

	dial := fakeHerdr(t, func(conn net.Conn) {
		readLine(conn)
		time.Sleep(300 * time.Millisecond)
		writeAll(conn, ackLine, eventLine)
	})

	var events []Event
	if err := Subscribe(t.Context(), dial, []string{"p1"}, func() {}, func(event Event) {
		events = append(events, event)
	}); err == nil {
		t.Fatal("Subscribe() error = nil, want an error when the server closes the stream")
	}
	if len(events) != 1 {
		t.Fatalf("Subscribe() delivered %d events (%+v), want 1", len(events), events)
	}
}

func TestSubscribeRejectsAMissingAck(t *testing.T) {
	t.Parallel()

	// Two events and then a hang-up: an implementation that wrongly accepts
	// the first line as the ack goes on to deliver the second, so it fails
	// this test on the delivery count instead of blocking on a read.
	dial := fakeHerdr(t, func(conn net.Conn) {
		readLine(conn)
		writeAll(conn, eventLine, eventLine)
	})

	delivered := 0
	if err := Subscribe(t.Context(), dial, []string{"p1"}, func() {}, func(Event) { delivered++ }); err == nil {
		t.Fatal("Subscribe() error = nil, want an error when the ack is missing")
	}
	if delivered != 0 {
		t.Errorf("Subscribe() delivered %d events, want 0 before the ack", delivered)
	}
}

func TestSubscribeGivesUpOnASilentServer(t *testing.T) {
	t.Parallel()

	dial := fakeHerdr(t, func(conn net.Conn) {
		_, _ = io.Copy(io.Discard, conn)
	})

	const budget = 50 * time.Millisecond
	started := time.Now()
	err := subscribe(t.Context(), dial, []string{"p1"}, func() {}, func(Event) {}, budget)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("subscribe() error = nil, want an error when the ack never arrives")
	}
	if elapsed < budget {
		t.Errorf("subscribe() returned after %s, want it to wait out the %s ack budget", elapsed, budget)
	}
	// Generous ceiling on purpose: the mutant this catches (ignoring the
	// budget for the 5s default) returns at ~5s, so the headroom costs no
	// detection and keeps the bound from flaking on a loaded box.
	if elapsed > time.Second {
		t.Errorf("subscribe() returned after %s, want the %s ack budget to bound the wait", elapsed, budget)
	}
}

func TestSubscribeStopsWhenTheContextExpiresAfterTheAck(t *testing.T) {
	t.Parallel()

	// A wedged Herdr holds the connection open and sends neither events nor
	// EOF; only the caller's deadline ends the wait.
	dial := fakeHerdr(t, func(conn net.Conn) {
		readLine(conn)
		writeAll(conn, ackLine)
		_, _ = io.Copy(io.Discard, conn)
	})

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	returned := make(chan error, 1)
	go func() {
		returned <- Subscribe(ctx, dial, []string{"p1"}, func() {}, func(Event) {})
	}()

	select {
	case err := <-returned:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Subscribe() error = %v, want %v", err, context.DeadlineExceeded)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe() did not return after the deadline passed")
	}
}

func TestSubscribeStopsWhenTheContextIsCanceled(t *testing.T) {
	t.Parallel()

	dial := fakeHerdr(t, func(conn net.Conn) {
		readLine(conn)
		writeAll(conn, ackLine)
		_, _ = io.Copy(io.Discard, conn)
	})

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	returned := make(chan error, 1)
	go func() {
		returned <- Subscribe(ctx, dial, []string{"p1"}, func() {}, func(Event) {})
	}()

	select {
	case err := <-returned:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Subscribe() error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe() did not return after the context was canceled")
	}
}

func TestSubscribeReportsADialFailure(t *testing.T) {
	t.Parallel()

	dialErr := errors.New("no socket")
	dial := func(context.Context) (net.Conn, error) { return nil, dialErr }

	if err := Subscribe(t.Context(), dial, []string{"p1"}, func() {}, func(Event) {}); !errors.Is(err, dialErr) {
		t.Errorf("Subscribe() error = %v, want %v", err, dialErr)
	}
}

func TestSubscribeRejectsAnOversizedAcknowledgementLine(t *testing.T) {
	t.Parallel()

	dial := fakeHerdr(t, func(conn net.Conn) {
		readLine(conn)
		writeAll(conn, strings.Repeat("x", maxEventLineBytes)+"\n")
	})

	err := Subscribe(t.Context(), dial, []string{"p1"}, func() {}, func(Event) {})
	if err == nil || !strings.Contains(err.Error(), "exceeds 65536 bytes") {
		t.Errorf("Subscribe() error = %v, want an oversized-line transport failure", err)
	}
}

func TestSubscribeAcceptsLinesAtThe64KiBLimit(t *testing.T) {
	t.Parallel()

	ack := strings.TrimSuffix(ackLine, "\n")
	ack += strings.Repeat(" ", maxEventLineBytes-len(ack)-1) + "\n"
	event := strings.TrimSuffix(eventLine, "\n")
	event += strings.Repeat(" ", maxEventLineBytes-len(event)-1) + "\n"

	dial := fakeHerdr(t, func(conn net.Conn) {
		readLine(conn)
		writeAll(conn, ack, event)
	})

	ready := 0
	delivered := 0
	err := Subscribe(t.Context(), dial, []string{"p1"}, func() { ready++ }, func(Event) { delivered++ })
	if ready != 1 || delivered != 1 {
		t.Errorf("Subscribe() readiness/events = %d/%d, want 1/1 at the line limit", ready, delivered)
	}
	if err == nil || strings.Contains(err.Error(), "exceeds") {
		t.Errorf("Subscribe() error = %v, want only the eventual stream-close error", err)
	}
}

func TestSubscribeRejectsAnOversizedEventLine(t *testing.T) {
	t.Parallel()

	dial := fakeHerdr(t, func(conn net.Conn) {
		readLine(conn)
		writeAll(conn, ackLine, strings.Repeat("x", maxEventLineBytes)+"\n")
	})

	ready := 0
	err := Subscribe(t.Context(), dial, []string{"p1"}, func() { ready++ }, func(Event) {})
	if ready != 1 {
		t.Errorf("Subscribe() readiness calls = %d, want 1 before the oversized event", ready)
	}
	if err == nil || !strings.Contains(err.Error(), "exceeds 65536 bytes") {
		t.Errorf("Subscribe() error = %v, want an oversized-line transport failure", err)
	}
}

func TestSubscribeRejectsAnUnterminatedOversizedEventLine(t *testing.T) {
	t.Parallel()

	dial := fakeHerdr(t, func(conn net.Conn) {
		readLine(conn)
		writeAll(conn, ackLine, strings.Repeat("x", maxEventLineBytes+1))
	})

	err := Subscribe(t.Context(), dial, []string{"p1"}, func() {}, func(Event) {})
	if err == nil || !strings.Contains(err.Error(), "exceeds 65536 bytes") {
		t.Errorf("Subscribe() error = %v, want an unterminated oversized-line failure", err)
	}
}

// fakeHerdr serves one connection with script and returns a dialer for it. The
// socket lives under os.MkdirTemp rather than t.TempDir because the temporary
// directories tests get are long enough to overflow sun_path.
func fakeHerdr(t *testing.T, script func(conn net.Conn)) func(context.Context) (net.Conn, error) {
	t.Helper()

	dir, err := os.MkdirTemp("", "w")
	if err != nil {
		t.Fatalf("create socket directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	path := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen on %s: %v", path, err)
	}

	served := make(chan struct{})
	go func() {
		defer close(served)

		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		script(conn)
	}()

	// Waiting for the script keeps a stray goroutine from outliving the test,
	// but the wait is bounded: a Subscribe that leaks its connection leaves
	// the script blocked, and that must fail the test rather than hang it.
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Error("fake Herdr server is still serving; the connection was never closed")
		}
	})

	return func(ctx context.Context) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "unix", path)
	}
}

func readLine(conn net.Conn) string {
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return ""
	}
	return line[:len(line)-1]
}

func writeAll(conn net.Conn, lines ...string) {
	for _, line := range lines {
		if _, err := io.WriteString(conn, line); err != nil {
			return
		}
	}
}
