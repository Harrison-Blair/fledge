package pirpc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMain lets the test binary re-exec itself as a fake pi agent, so the
// runner is exercised against a real subprocess without needing pi installed.
func TestMain(m *testing.M) {
	if os.Getenv("PIRPC_TEST_CHILD") == "1" {
		stub(os.Getenv("PIRPC_TEST_MODE"))
		return
	}
	os.Exit(m.Run())
}

// stub is the fake pi agent. "events" emits canned frames and exits on stdin
// EOF; "echo" replies to every frame it reads; "stubborn" ignores EOF entirely.
func stub(mode string) {
	switch mode {
	case "events":
		fmt.Println(`{"type":"agent_start"}`)
		fmt.Println("not json at all")
		fmt.Println(`{"id":"m1","type":"agent_settled","detail":{"turns":2}}`)
	case "many":
		// Long, distinct frames, so a Raw that aliases the scanner's reused
		// buffer instead of copying shows up as another frame's bytes.
		for i := range 50 {
			fmt.Printf("{\"id\":\"m%d\",\"type\":\"chunk\",\"pad\":\"%s\"}\n", i, strings.Repeat("x", i*64))
		}
	case "stubborn":
		time.Sleep(30 * time.Second)
		return
	case "orphan":
		// Leave a descendant holding the stdout write end, then exit: the
		// pipe never reaches EOF even though the direct child is gone.
		helper := exec.Command("sh", "-c", "sleep 5 &")
		helper.Stdout = os.Stdout
		helper.Run()
		return
	case "huge":
		fmt.Println(strings.Repeat("x", maxFrameBytes+1))
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		if mode == "echo" {
			reply, _ := json.Marshal(struct {
				Type     string `json:"type"`
				Received string `json:"received"`
			}{Type: "echo", Received: scanner.Text()})
			fmt.Println(string(reply))
		}
	}
	if mode == "crash" {
		os.Exit(2)
	}
}

// startStub launches the test binary in the given stub mode.
func startStub(t *testing.T, mode string, onEvent func(Event)) *Runner {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if onEvent == nil {
		onEvent = func(Event) {}
	}
	r, err := Start(
		[]string{exe, "pi-stub"},
		t.TempDir(),
		[]string{"PIRPC_TEST_CHILD=1", "PIRPC_TEST_MODE=" + mode},
		nil,
		onEvent,
	)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// collector accumulates events from the reader goroutine.
type collector struct {
	mu     sync.Mutex
	events []Event
}

func (c *collector) add(e Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *collector) snapshot() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Event(nil), c.events...)
}

func TestEventsDeliveredInOrder(t *testing.T) {
	var c collector
	r := startStub(t, "events", c.add)
	defer r.Stop()

	if err := r.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	events := c.snapshot()
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2 (unparseable lines skipped): %+v", len(events), events)
	}
	if events[0].Type != "agent_start" || events[0].ID != "" {
		t.Errorf("event 0 = %+v", events[0])
	}
	if events[1].Type != "agent_settled" || events[1].ID != "m1" {
		t.Errorf("event 1 = %+v", events[1])
	}
	if want := `{"id":"m1","type":"agent_settled","detail":{"turns":2}}`; string(events[1].Raw) != want {
		t.Errorf("event 1 Raw = %s, want %s", events[1].Raw, want)
	}
}

func TestRawSurvivesLaterFrames(t *testing.T) {
	var c collector
	r := startStub(t, "many", c.add)
	defer r.Stop()

	if err := r.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	events := c.snapshot()
	if len(events) != 50 {
		t.Fatalf("got %d events, want 50", len(events))
	}
	for i, e := range events {
		var frame struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(e.Raw, &frame); err != nil {
			t.Fatalf("event %d: Raw is not valid JSON after later frames arrived: %v", i, err)
		}
		if want := fmt.Sprintf("m%d", i); frame.ID != want {
			t.Fatalf("event %d: Raw holds id %q, want %q (Raw aliases the scanner buffer)", i, frame.ID, want)
		}
	}
}

func TestPromptWritesFrame(t *testing.T) {
	var c collector
	r := startStub(t, "echo", c.add)
	defer r.Stop()

	if err := r.Prompt("p1", "hello agent"); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if err := r.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	events := c.snapshot()
	if len(events) == 0 {
		t.Fatal("no echo frames received")
	}

	var echoed struct {
		Received string `json:"received"`
	}
	if err := json.Unmarshal(events[0].Raw, &echoed); err != nil {
		t.Fatalf("unmarshal echo: %v", err)
	}
	if want := `{"id":"p1","type":"prompt","message":"hello agent"}`; echoed.Received != want {
		t.Errorf("child received %s, want %s", echoed.Received, want)
	}
}

func TestPromptAfterStop(t *testing.T) {
	r := startStub(t, "echo", nil)
	r.Stop()

	if err := r.Prompt("p1", "too late"); err == nil {
		t.Error("Prompt after Stop succeeded, want error")
	}
}

func TestStopCooperative(t *testing.T) {
	r := startStub(t, "echo", nil)

	start := time.Now()
	if err := r.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= stopGrace {
		t.Errorf("Stop took %v, want well under the %v grace period", elapsed, stopGrace)
	}

	select {
	case <-r.Done():
	default:
		t.Error("Done not closed after Stop returned")
	}
}

func TestStopStubbornKills(t *testing.T) {
	defer withGrace(t, 200*time.Millisecond)()

	r := startStub(t, "stubborn", nil)
	pid := r.PID()

	start := time.Now()
	if err := r.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed < stopGrace {
		t.Errorf("Stop returned after %v, want at least the %v grace period", elapsed, stopGrace)
	}
	if elapsed > 5*time.Second {
		t.Errorf("Stop took %v, want the child killed promptly after the grace period", elapsed)
	}
	select {
	case <-r.Done():
	default:
		t.Error("Stop returned before the killed process was reaped")
	}
	if alive(pid) {
		t.Errorf("process %d still alive after Stop", pid)
	}
}

func TestStopWithOrphanHoldingStdout(t *testing.T) {
	defer withGrace(t, 200*time.Millisecond)()

	r := startStub(t, "orphan", nil)

	stopped := make(chan error, 1)
	go func() { stopped <- r.Stop() }()

	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(stopGrace + 2*time.Second):
		t.Fatal("Stop blocked: a descendant still holds the stdout write end")
	}

	select {
	case <-r.Done():
	default:
		t.Error("Done not closed after Stop returned")
	}
}

func TestStopReportsOversizedFrame(t *testing.T) {
	r := startStub(t, "huge", nil)

	err := r.Stop()
	if err == nil {
		t.Fatal("Stop returned nil, want the oversized-frame read error")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("Stop error = %v, want it to mention the too-long token", err)
	}
}

func TestStopReportsExitStatus(t *testing.T) {
	r := startStub(t, "crash", nil)

	err := r.Stop()
	if err == nil {
		t.Fatal("Stop returned nil, want the child's abnormal exit status")
	}
	if !strings.Contains(err.Error(), "exit status 2") {
		t.Errorf("Stop error = %v, want it to mention exit status 2", err)
	}
}

func TestDoubleStop(t *testing.T) {
	r := startStub(t, "echo", nil)

	if err := r.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := r.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestPID(t *testing.T) {
	defer withGrace(t, 200*time.Millisecond)()

	r := startStub(t, "stubborn", nil)
	defer r.Stop()

	pid := r.PID()
	if pid <= 0 {
		t.Fatalf("PID = %d, want a real pid", pid)
	}
	if !alive(pid) {
		t.Errorf("process %d is not running", pid)
	}
}

// alive reports whether pid names a live process, by reading its /proc status;
// a killed-but-reaped child leaves no entry.
func alive(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return false
	}
	return !strings.Contains(string(data), "State:\tZ")
}

// withGrace lowers the stop grace period for one test and restores it.
func withGrace(t *testing.T, d time.Duration) func() {
	t.Helper()
	prev := stopGrace
	stopGrace = d
	return func() { stopGrace = prev }
}
