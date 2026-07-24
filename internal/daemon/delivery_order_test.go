package daemon

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/protocol"
)

type failDeliveredJournal struct {
	io.WriteCloser
	remaining atomic.Int32
}

func (f *failDeliveredJournal) Write(p []byte) (int, error) {
	if strings.Contains(string(p), `"event":"msg.delivered"`) {
		for remaining := f.remaining.Load(); remaining > 0; remaining = f.remaining.Load() {
			if f.remaining.CompareAndSwap(remaining, remaining-1) {
				return 0, errors.New("injected msg.delivered failure")
			}
		}
	}
	return f.WriteCloser.Write(p)
}

type deliveryWaitResult struct {
	resp protocol.Response
	err  error
}

func registerMessagingPair(t *testing.T, d *Daemon) (protocol.Agent, protocol.Agent) {
	t.Helper()
	sender, err := d.register(&protocol.Request{Type: "sender", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := d.register(&protocol.Request{Type: "receiver", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	return protocol.Agent{Name: sender.Name}, protocol.Agent{Name: receiver.Name}
}

func failNextDeliveredAppend(d *Daemon) {
	journal := &failDeliveredJournal{WriteCloser: d.journal}
	journal.remaining.Store(1)
	d.journal = journal
}

func waitUntilParked(t *testing.T, d *Daemon, as string) {
	t.Helper()
	waitFor(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		for _, w := range d.waiters {
			if w.as == as && !w.done {
				return true
			}
		}
		return false
	})
}

func TestSendToParkedWaiterDeliveryAppendFailureRemainsRetryable(t *testing.T) {
	d := newTestDaemon(t)
	sender, receiver := registerMessagingPair(t, d)

	parked := make(chan deliveryWaitResult, 1)
	go func() {
		resp, err := d.wait(&protocol.Request{As: receiver.Name}, nil)
		parked <- deliveryWaitResult{resp: resp, err: err}
	}()
	waitUntilParked(t, d, receiver.Name)

	failNextDeliveredAppend(d)
	if _, err := d.send(&protocol.Request{
		From: sender.Name, To: receiver.Name, Body: "durable",
	}); err == nil || !strings.Contains(err.Error(), "injected msg.delivered failure") {
		t.Fatalf("send error = %v, want injected delivery failure", err)
	}

	select {
	case result := <-parked:
		if result.err == nil || !strings.Contains(result.err.Error(), "injected msg.delivered failure") {
			t.Fatalf("parked wait result = %+v, %v", result.resp, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("no-timeout waiter was stranded after delivery append failure")
	}

	d.mu.Lock()
	if len(d.pending) != 1 {
		d.mu.Unlock()
		t.Fatalf("pending messages = %d, want durable send retained", len(d.pending))
	}
	msg := d.pending[0]
	if d.messageDelivered[msg.ID] {
		d.mu.Unlock()
		t.Fatalf("message %s marked delivered after failed append", msg.ID)
	}
	if len(d.waiters) != 0 {
		d.mu.Unlock()
		t.Fatalf("failed parked wait remains registered: %+v", d.waiters)
	}
	d.mu.Unlock()

	got, err := d.wait(&protocol.Request{As: receiver.Name, TimeoutMS: 1000}, nil)
	if err != nil {
		t.Fatalf("retry wait: %v", err)
	}
	if got.Message == nil || got.Message.ID != msg.ID || got.Message.Body != "durable" {
		t.Fatalf("retry delivered %+v, want %s", got.Message, msg.ID)
	}
	if _, err := d.wait(&protocol.Request{As: receiver.Name, TimeoutMS: 20}, nil); err == nil {
		t.Fatal("message was delivered more than once")
	}

	sentAt, deliveredAt, deliveredCount := -1, -1, 0
	for i, e := range events(t, d) {
		if e.ID != msg.ID {
			continue
		}
		switch e.Event {
		case evSent:
			sentAt = i
		case evDelivered:
			deliveredAt = i
			deliveredCount++
		}
	}
	if sentAt < 0 || deliveredAt <= sentAt || deliveredCount != 1 {
		t.Fatalf("journal order sent=%d delivered=%d delivered count=%d", sentAt, deliveredAt, deliveredCount)
	}
}

func TestReplyToParkedWaiterDeliveryAppendFailurePreservesCorrelation(t *testing.T) {
	d := newTestDaemon(t)
	sender, receiver := registerMessagingPair(t, d)

	inbound, err := d.send(&protocol.Request{
		From: sender.Name, To: receiver.Name, Body: "question",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.inbox(&protocol.Request{As: receiver.Name}); err != nil {
		t.Fatal(err)
	}

	parked := make(chan deliveryWaitResult, 1)
	go func() {
		resp, err := d.wait(&protocol.Request{
			As: sender.Name, From: receiver.Name, ReplyTo: inbound.ID,
		}, nil)
		parked <- deliveryWaitResult{resp: resp, err: err}
	}()
	waitUntilParked(t, d, sender.Name)

	failNextDeliveredAppend(d)
	if _, err := d.reply(&protocol.Request{
		From: receiver.Name, ID: inbound.ID, Body: "answer",
	}); err == nil || !strings.Contains(err.Error(), "injected msg.delivered failure") {
		t.Fatalf("reply error = %v, want injected delivery failure", err)
	}

	select {
	case result := <-parked:
		if result.err == nil || !strings.Contains(result.err.Error(), "injected msg.delivered failure") {
			t.Fatalf("parked reply wait result = %+v, %v", result.resp, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("reply failure stranded the exact no-timeout waiter")
	}

	d.mu.Lock()
	if len(d.pending) != 1 {
		d.mu.Unlock()
		t.Fatalf("pending replies = %d, want 1", len(d.pending))
	}
	reply := d.pending[0]
	d.mu.Unlock()
	if reply.From != receiver.Name || reply.To != sender.Name || reply.ReplyTo != inbound.ID {
		t.Fatalf("pending reply lost correlation: %+v", reply)
	}

	got, err := d.wait(&protocol.Request{
		As: sender.Name, From: receiver.Name, ReplyTo: inbound.ID, TimeoutMS: 1000,
	}, nil)
	if err != nil {
		t.Fatalf("retry exact reply wait: %v", err)
	}
	if got.Message == nil || got.Message.ID != reply.ID || got.Message.Body != "answer" {
		t.Fatalf("retry reply = %+v, want %s", got.Message, reply.ID)
	}
}

func TestWaitDeliveryAppendFailureLeavesPendingForRetry(t *testing.T) {
	d := newTestDaemon(t)
	sender, receiver := registerMessagingPair(t, d)
	sent, err := d.send(&protocol.Request{
		From: sender.Name, To: receiver.Name, Body: "queued",
	})
	if err != nil {
		t.Fatal(err)
	}

	failNextDeliveredAppend(d)
	if _, err := d.wait(&protocol.Request{
		As: receiver.Name, TimeoutMS: 1000,
	}, nil); err == nil || !strings.Contains(err.Error(), "injected msg.delivered failure") {
		t.Fatalf("wait error = %v, want injected delivery failure", err)
	}

	d.mu.Lock()
	if len(d.pending) != 1 || d.pending[0].ID != sent.ID {
		d.mu.Unlock()
		t.Fatalf("pending after failed wait = %+v, want %s", d.pending, sent.ID)
	}
	if d.messageDelivered[sent.ID] {
		d.mu.Unlock()
		t.Fatalf("message %s marked delivered after failed wait", sent.ID)
	}
	d.mu.Unlock()

	got, err := d.wait(&protocol.Request{As: receiver.Name, TimeoutMS: 1000}, nil)
	if err != nil {
		t.Fatalf("retry wait: %v", err)
	}
	if got.Message == nil || got.Message.ID != sent.ID {
		t.Fatalf("retry got %+v, want %s", got.Message, sent.ID)
	}
}

func TestDeliveryAppendFailureReplaysDurableSendAsPending(t *testing.T) {
	d := newTestDaemon(t)
	sender, receiver := registerMessagingPair(t, d)
	sent, err := d.send(&protocol.Request{
		From: sender.Name, To: receiver.Name, Body: "survive restart",
	})
	if err != nil {
		t.Fatal(err)
	}

	failNextDeliveredAppend(d)
	if _, err := d.wait(&protocol.Request{As: receiver.Name}, nil); err == nil {
		t.Fatal("wait succeeded despite injected delivery failure")
	}

	root, flockName := d.root, d.flockName
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(root, flockName)
	if err != nil {
		t.Fatalf("restart daemon: %v", err)
	}
	t.Cleanup(func() { restarted.Close() })

	if len(restarted.pending) != 1 || restarted.pending[0].ID != sent.ID {
		t.Fatalf("restart pending = %+v, want %s", restarted.pending, sent.ID)
	}
	got, err := restarted.wait(&protocol.Request{
		As: receiver.Name, TimeoutMS: 1000,
	}, nil)
	if err != nil {
		t.Fatalf("wait after restart: %v", err)
	}
	if got.Message == nil || got.Message.ID != sent.ID {
		t.Fatalf("restart delivered %+v, want %s", got.Message, sent.ID)
	}
	if _, err := restarted.wait(&protocol.Request{
		As: receiver.Name, TimeoutMS: 20,
	}, nil); err == nil {
		t.Fatal("restart delivered the durable message twice")
	}
}

func TestReceiveRequiresAckAndCanBeRetriedAfterOutputLoss(t *testing.T) {
	d := newTestDaemon(t)
	sender, receiver := registerMessagingPair(t, d)
	sent, err := d.send(&protocol.Request{
		From: sender.Name, To: receiver.Name, Body: "durable output",
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := d.receive(&protocol.Request{
		As: receiver.Name, From: sender.Name, TimeoutMS: 1000,
	}, nil)
	if err != nil {
		t.Fatalf("first receive: %v", err)
	}
	if first.Message == nil || first.Message.ID != sent.ID {
		t.Fatalf("first receive = %+v, want %s", first.Message, sent.ID)
	}
	d.mu.Lock()
	pendingBeforeAck := len(d.pending)
	deliveredBeforeAck := d.messageDelivered[sent.ID]
	d.mu.Unlock()
	if pendingBeforeAck != 1 || deliveredBeforeAck {
		t.Fatalf("before ack pending=%d delivered=%v, want pending and retryable", pendingBeforeAck, deliveredBeforeAck)
	}

	// A CLI that loses stdout never sends ack. Its exact retry must receive the
	// same durable message rather than block behind an eager msg.delivered.
	retry, err := d.receive(&protocol.Request{
		As: receiver.Name, From: sender.Name, TimeoutMS: 1000,
	}, nil)
	if err != nil {
		t.Fatalf("retry receive: %v", err)
	}
	if retry.Message == nil || retry.Message.ID != sent.ID || retry.Message.Body != "durable output" {
		t.Fatalf("retry receive = %+v, want %s", retry.Message, sent.ID)
	}

	failNextDeliveredAppend(d)
	if _, err := d.ack(&protocol.Request{As: receiver.Name, ID: sent.ID}); err == nil ||
		!strings.Contains(err.Error(), "injected msg.delivered failure") {
		t.Fatalf("failed ack = %v, want durable append error", err)
	}
	if _, err := d.ack(&protocol.Request{As: receiver.Name, ID: sent.ID}); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if _, err := d.ack(&protocol.Request{As: receiver.Name, ID: sent.ID}); err != nil {
		t.Fatalf("idempotent ack: %v", err)
	}
	d.mu.Lock()
	pendingAfterAck := len(d.pending)
	deliveredAfterAck := d.messageDelivered[sent.ID]
	d.mu.Unlock()
	if pendingAfterAck != 0 || !deliveredAfterAck {
		t.Fatalf("after ack pending=%d delivered=%v, want finalized once", pendingAfterAck, deliveredAfterAck)
	}
}
