package daemon_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/client"
	"github.com/Harrison-Blair/fledge/internal/daemon"
	"github.com/Harrison-Blair/fledge/internal/filebridge"
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

// A torn final line must be truncated on replay, not merely skipped. If it is
// left in place, the next O_APPEND write concatenates onto it, the torn bytes
// stop being the final line, and every later replay hard-fails on the fused
// malformed line. This exercises the append-after-torn-tail path that a plain
// tolerate test does not.
func TestTornTailTruncatedAcrossRestart(t *testing.T) {
	root := workspace(t)
	stop := start(t, root, testFlock)
	register(t, root, testFlock, "engineer")
	stop()

	jp := filepath.Join(root, ".fledge", "flocks", testFlock, "journal.jsonl")
	f, err := os.OpenFile(jp, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"event":"msg.sent","id":"abc`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// First restart tolerates the torn tail, then appends (daemon.started plus a
	// second registration).
	stop = start(t, root, testFlock)
	register(t, root, testFlock, "reviewer")
	stop()

	// Second restart replays a journal that, without truncation, holds the torn
	// bytes fused to a later line in a non-final position: a hard failure.
	defer start(t, root, testFlock)()

	list, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpList})
	if err != nil {
		t.Fatalf("list after torn-tail append and restart: %v", err)
	}
	if len(list.Agents) != 2 {
		t.Fatalf("roster after torn-tail append and restart = %+v, want 2 agents", list.Agents)
	}
}

// A final line that is a complete, valid event but lacks its trailing newline
// replays fine, so truncation never touches it — yet the next O_APPEND write
// still fuses onto it, causing the same permanent corruption. Replay must
// re-terminate it (append the missing newline), not discard the event.
func TestUnterminatedFinalLineReterminated(t *testing.T) {
	root := workspace(t)
	stop := start(t, root, testFlock)
	register(t, root, testFlock, "engineer")
	stop()

	jp := filepath.Join(root, ".fledge", "flocks", testFlock, "journal.jsonl")
	f, err := os.OpenFile(jp, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	// A complete, authoritative event, but with no trailing newline.
	if _, err := f.WriteString(`{"event":"agent.registered","name":"reviewer-king","type":"reviewer","species":"king","pid":123}`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// First restart replays the unterminated line and must re-terminate it, then
	// appends (daemon.started plus a third registration).
	stop = start(t, root, testFlock)
	register(t, root, testFlock, "auditor")
	stop()

	// Second restart replays a journal that, without re-termination, holds the
	// once-final event fused to daemon.started in a non-final position: a hard
	// failure.
	defer start(t, root, testFlock)()

	list, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpList})
	if err != nil {
		t.Fatalf("list after unterminated-line append and restart: %v", err)
	}
	if len(list.Agents) != 3 {
		t.Fatalf("roster after unterminated-line restart = %+v, want 3 agents", list.Agents)
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

// TestAbandonedBridgeWaiterDoesNotSwallowMessages mirrors the socket case for
// the file-bridge path: a sandboxed `agent msg wait` (no timeout) that is
// abandoned must have its waiter dropped, not swallow the next message.
func TestAbandonedBridgeWaiterDoesNotSwallowMessages(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	a := register(t, root, testFlock, "engineer")
	b := register(t, root, testFlock, "reviewer")

	// Park a wait over the file bridge the way a sandboxed client does: submit
	// an OpWait with no timeout and let the daemon take and park it.
	id, err := filebridge.Submit(root, testFlock, protocol.Request{Op: protocol.OpWait, As: b})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !filebridge.Awaiting(root, testFlock, id) {
		if time.Now().After(deadline) {
			t.Fatal("daemon never accepted the bridge wait")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Let the daemon register the waiter, then abandon: a client that gives up
	// removes its exchange files via Cleanup.
	time.Sleep(100 * time.Millisecond)
	filebridge.Cleanup(root, testFlock, id)
	// Wait past a liveness-poll tick (250ms) so the waiter is dropped.
	time.Sleep(600 * time.Millisecond)

	// The message must not be handed to the dead bridge waiter; a live wait gets it.
	sent, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpSend, From: a, To: b, Body: "for the living"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	got, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpWait, As: b, TimeoutMS: 2000})
	if err != nil {
		t.Fatalf("wait after abandoned bridge waiter: %v", err)
	}
	if got.Message == nil || got.Message.ID != sent.ID {
		t.Fatalf("delivered %+v, want %s", got.Message, sent.ID)
	}
}

// TestKilledBridgeWaiterDoesNotSwallowMessages is the non-cooperative case: a
// real client process parks a bridge wait and is then killed outright, so it
// runs no deferred Cleanup — exactly like SIGKILL or a closed pane. The daemon
// must still detect the abandonment (via the pid stamped in the marker) and
// drop the waiter instead of swallowing the next message.
func TestKilledBridgeWaiterDoesNotSwallowMessages(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	a := register(t, root, testFlock, "engineer")
	b := register(t, root, testFlock, "reviewer")

	idFile := filepath.Join(t.TempDir(), "helper-id")
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperBridgeWait$")
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		"FLEDGE_HELPER_ROOT="+root,
		"FLEDGE_HELPER_FLOCK="+testFlock,
		"FLEDGE_HELPER_AS="+b,
		"FLEDGE_HELPER_IDFILE="+idFile,
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	killed := false
	defer func() {
		if !killed {
			cmd.Process.Kill()
		}
		cmd.Wait()
	}()

	// Wait for the child to publish its exchange id.
	var id string
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(idFile)
		if err == nil && len(data) > 0 {
			id = strings.TrimSpace(string(data))
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper never published its exchange id")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Wait until the daemon has taken and parked the child's wait.
	deadline = time.Now().Add(2 * time.Second)
	for !filebridge.Awaiting(root, testFlock, id) {
		if time.Now().After(deadline) {
			t.Fatal("daemon never accepted the bridge wait")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Kill the client outright and reap it: no Cleanup runs, so only a liveness
	// probe of the stamped pid can now detect the abandonment.
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill helper: %v", err)
	}
	cmd.Wait()
	killed = true
	// Wait past a liveness-poll tick (250ms) so the dead waiter is dropped.
	time.Sleep(1 * time.Second)

	// The message must not be handed to the dead bridge waiter; a live wait gets it.
	sent, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpSend, From: a, To: b, Body: "for the living"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	got, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpWait, As: b, TimeoutMS: 2000})
	if err != nil {
		t.Fatalf("wait after killed bridge waiter: %v", err)
	}
	if got.Message == nil || got.Message.ID != sent.ID {
		t.Fatalf("delivered %+v, want %s", got.Message, sent.ID)
	}
}

// TestHelperBridgeWait is not a standalone test: it is the child process
// spawned by TestKilledBridgeWaiterDoesNotSwallowMessages. It publishes a
// bridge wait, records its exchange id, and blocks so the parent can kill it.
func TestHelperBridgeWait(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	id, err := filebridge.Submit(
		os.Getenv("FLEDGE_HELPER_ROOT"),
		os.Getenv("FLEDGE_HELPER_FLOCK"),
		protocol.Request{Op: protocol.OpWait, As: os.Getenv("FLEDGE_HELPER_AS")},
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper submit:", err)
		os.Exit(2)
	}
	if err := os.WriteFile(os.Getenv("FLEDGE_HELPER_IDFILE"), []byte(id), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "helper write id:", err)
		os.Exit(2)
	}
	time.Sleep(60 * time.Second)
}
