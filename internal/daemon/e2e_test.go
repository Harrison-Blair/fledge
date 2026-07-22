package daemon_test

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/client"
	"github.com/Harrison-Blair/fledge/internal/daemon"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

// testFlock is the flock every single-flock test works in.
const testFlock = "test"

// workspace creates a scaffolded temp workspace.
func workspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	return root
}

// start runs a daemon for root in the background and returns a stop func that
// blocks until it has fully released the socket.
func start(t *testing.T, root, flockName string) (stop func()) {
	t.Helper()
	d, err := daemon.New(root, flockName)
	if err != nil {
		t.Fatalf("daemon.New: %v", err)
	}

	done := make(chan struct{})
	go func() {
		d.Serve()
		close(done)
	}()

	var once bool
	return func() {
		if once {
			return
		}
		once = true
		d.Close()
		<-done
	}
}

func register(t *testing.T, root, flockName, typ string) string {
	t.Helper()
	resp, err := client.Do(root, flockName, protocol.Request{
		Op: protocol.OpRegister, Type: typ, PID: os.Getpid(),
	})
	if err != nil {
		t.Fatalf("register %s: %v", typ, err)
	}
	return resp.Name
}

func TestDaemonDownIsHardError(t *testing.T) {
	root := workspace(t)

	if client.Running(root, testFlock) {
		t.Fatal("no daemon started, but Running reports up")
	}
	_, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpList})
	if !errors.Is(err, client.ErrNotRunning) {
		t.Fatalf("err = %v, want ErrNotRunning", err)
	}
}

func TestRegisterAssignsSpeciesPerType(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	if got := register(t, root, testFlock, "engineer"); got != "engineer-emperor" {
		t.Fatalf("first engineer = %q, want engineer-emperor", got)
	}
	// A second live engineer takes the next slug; a reviewer starts the pool
	// over, because the pool is per-type.
	if got := register(t, root, testFlock, "engineer"); got != "engineer-king" {
		t.Fatalf("second engineer = %q, want engineer-king", got)
	}
	if got := register(t, root, testFlock, "reviewer"); got != "reviewer-emperor" {
		t.Fatalf("first reviewer = %q, want reviewer-emperor", got)
	}

	resp, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpRegister, Type: "engineer", Species: "gentoo", PID: os.Getpid()})
	if err != nil {
		t.Fatalf("requested species: %v", err)
	}
	if resp.Name != "engineer-gentoo" {
		t.Fatalf("requested species gave %q", resp.Name)
	}
}

func TestRegisterRejectsBadType(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	for _, typ := range []string{"", "-code-engineer", "code--engineer", "Engineer", "code_engineer"} {
		if _, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpRegister, Type: typ, PID: os.Getpid()}); err == nil {
			t.Fatalf("type %q was accepted", typ)
		}
	}
}

func TestDeadAgentReleasesItsName(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	// PID 0 is never a live process, so this registration is born dead.
	resp, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpRegister, Type: "engineer", PID: 0})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if resp.Name != "engineer-emperor" {
		t.Fatalf("name = %q", resp.Name)
	}

	if got := register(t, root, testFlock, "engineer"); got != "engineer-emperor" {
		t.Fatalf("reclaimed name = %q, want engineer-emperor", got)
	}

	list, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpList})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Agents) != 1 {
		t.Fatalf("roster = %+v, want one entry", list.Agents)
	}
	if !list.Agents[0].Alive {
		t.Fatal("reclaimed agent should report alive")
	}
}

func TestPoolExhaustsAtNineteen(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	for i := 0; i < 18; i++ {
		register(t, root, testFlock, "engineer")
	}
	if _, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpRegister, Type: "engineer", PID: os.Getpid()}); err == nil {
		t.Fatal("19th engineer registered; want pool exhaustion error")
	}
	// A different type is unaffected.
	register(t, root, testFlock, "reviewer")
}

func TestSendToUnknownAgentFails(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	a := register(t, root, testFlock, "engineer")
	if _, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpSend, From: a, To: "nobody-emperor", Body: "hi"}); err == nil {
		t.Fatal("send to unregistered name succeeded")
	}
}

func TestSendThenWaitDelivers(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	a := register(t, root, testFlock, "engineer")
	b := register(t, root, testFlock, "reviewer")

	sent, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpSend, From: a, To: b, Body: "look at this"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	got, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpWait, As: b, TimeoutMS: 5000})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if got.Message == nil {
		t.Fatal("wait returned no message")
	}
	if got.Message.ID != sent.ID || got.Message.From != a || got.Message.Body != "look at this" {
		t.Fatalf("delivered %+v", got.Message)
	}

	// Delivered exactly once: nothing is left pending for b.
	if _, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpWait, As: b, TimeoutMS: 100}); err == nil {
		t.Fatal("second wait returned a message; delivery was not once-only")
	}
}

func TestWaitBlocksUntilSend(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	a := register(t, root, testFlock, "engineer")
	b := register(t, root, testFlock, "reviewer")

	type result struct {
		resp protocol.Response
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpWait, As: b, TimeoutMS: 5000})
		done <- result{resp, err}
	}()

	// The wait must still be blocked before anything is sent.
	select {
	case r := <-done:
		t.Fatalf("wait returned before any send: %+v %v", r.resp, r.err)
	case <-time.After(150 * time.Millisecond):
	}

	if _, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpSend, From: a, To: b, Body: "now"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("wait: %v", r.err)
		}
		if r.resp.Message == nil || r.resp.Message.Body != "now" {
			t.Fatalf("delivered %+v", r.resp.Message)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait did not wake on send")
	}
}

func TestWaitReplyToOnlyTakesCorrelatedReply(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	a := register(t, root, testFlock, "engineer")
	b := register(t, root, testFlock, "reviewer")

	ask, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpSend, From: a, To: b, Body: "question"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	type result struct {
		resp protocol.Response
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpWait, As: a, ReplyTo: ask.ID, TimeoutMS: 5000})
		done <- result{resp, err}
	}()

	// An uncorrelated message to the same agent must not satisfy the wait.
	if _, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpSend, From: b, To: a, Body: "unrelated"}); err != nil {
		t.Fatalf("send unrelated: %v", err)
	}
	select {
	case r := <-done:
		t.Fatalf("reply wait took an uncorrelated message: %+v %v", r.resp.Message, r.err)
	case <-time.After(150 * time.Millisecond):
	}

	if _, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpSend, From: b, To: a, Body: "answer", ReplyTo: ask.ID}); err != nil {
		t.Fatalf("send reply: %v", err)
	}
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("wait: %v", r.err)
		}
		if r.resp.Message == nil || r.resp.Message.Body != "answer" {
			t.Fatalf("delivered %+v", r.resp.Message)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reply wait did not wake on the correlated reply")
	}

	// The uncorrelated message was never delivered, so it is still pending.
	rest, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpWait, As: a, TimeoutMS: 1000})
	if err != nil {
		t.Fatalf("wait for the skipped message: %v", err)
	}
	if rest.Message == nil || rest.Message.Body != "unrelated" {
		t.Fatalf("skipped message = %+v, want it still pending", rest.Message)
	}
}

func TestWaitTimesOut(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	b := register(t, root, testFlock, "reviewer")
	started := time.Now()
	if _, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpWait, As: b, TimeoutMS: 200}); err == nil {
		t.Fatal("wait with nothing pending returned success")
	}
	if elapsed := time.Since(started); elapsed < 150*time.Millisecond {
		t.Fatalf("wait returned after %v; it did not honor the timeout", elapsed)
	}
}

func TestWaitAsUnknownAgentFails(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	if _, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpWait, As: "nobody-emperor", TimeoutMS: 100}); err == nil {
		t.Fatal("wait as an unregistered name succeeded")
	}
}

func TestRestartReplaysRosterAndPending(t *testing.T) {
	root := workspace(t)
	stop := start(t, root, testFlock)

	a := register(t, root, testFlock, "engineer")
	b := register(t, root, testFlock, "reviewer")
	sent, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpSend, From: a, To: b, Body: "survive this"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	stop()
	if client.Running(root, testFlock) {
		t.Fatal("daemon still reachable after stop")
	}

	defer start(t, root, testFlock)()

	list, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpList})
	if err != nil {
		t.Fatalf("list after restart: %v", err)
	}
	if len(list.Agents) != 2 {
		t.Fatalf("roster after restart = %+v, want 2 agents", list.Agents)
	}

	got, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpWait, As: b, TimeoutMS: 5000})
	if err != nil {
		t.Fatalf("wait after restart: %v", err)
	}
	if got.Message == nil || got.Message.ID != sent.ID || got.Message.Body != "survive this" {
		t.Fatalf("undelivered message did not survive restart: %+v", got.Message)
	}
}

func TestRestartDoesNotRedeliver(t *testing.T) {
	root := workspace(t)
	stop := start(t, root, testFlock)

	a := register(t, root, testFlock, "engineer")
	b := register(t, root, testFlock, "reviewer")
	if _, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpSend, From: a, To: b, Body: "once"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpWait, As: b, TimeoutMS: 5000}); err != nil {
		t.Fatalf("wait: %v", err)
	}

	stop()
	defer start(t, root, testFlock)()

	if _, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpWait, As: b, TimeoutMS: 200}); err == nil {
		t.Fatal("an already-delivered message was redelivered after restart")
	}
}

func TestSecondDaemonRefusesLiveSocket(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	if _, err := daemon.New(root, testFlock); err == nil {
		t.Fatal("a second daemon bound a live socket")
	}
}

func TestReplayToleratesTornFinalLine(t *testing.T) {
	root := workspace(t)
	stop := start(t, root, testFlock)
	register(t, root, testFlock, "engineer")
	stop()

	// A daemon killed mid-append leaves a torn final line; everything before
	// it must still replay.
	jp := filepath.Join(root, ".fledge", "flocks", testFlock, "journal.jsonl")
	f, err := os.OpenFile(jp, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"event":"msg.sent","id":"abc`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	defer start(t, root, testFlock)()

	list, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpList})
	if err != nil {
		t.Fatalf("list after torn-line restart: %v", err)
	}
	if len(list.Agents) != 1 || list.Agents[0].Name != "engineer-emperor" {
		t.Fatalf("roster after torn-line restart = %+v", list.Agents)
	}
}

func TestAbandonedWaiterDoesNotSwallowMessages(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	a := register(t, root, testFlock, "engineer")
	b := register(t, root, testFlock, "reviewer")

	// Park a wait, then kill its connection the way a dying agent would.
	conn, err := net.Dial("unix", daemon.SocketPath(root, testFlock))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(conn).Encode(protocol.Request{Op: protocol.OpWait, As: b}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	conn.Close()
	time.Sleep(150 * time.Millisecond)

	// The message must not be handed to the dead waiter; a live wait gets it.
	sent, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpSend, From: a, To: b, Body: "for the living"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	got, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpWait, As: b, TimeoutMS: 2000})
	if err != nil {
		t.Fatalf("wait after abandoned waiter: %v", err)
	}
	if got.Message == nil || got.Message.ID != sent.ID {
		t.Fatalf("delivered %+v, want %s", got.Message, sent.ID)
	}
}
