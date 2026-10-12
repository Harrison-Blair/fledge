package run

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/supervise"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/sockettest"
)

func TestDecodePushes(t *testing.T) {
	for line, want := range map[string]event{
		`{"event":"pane.agent_status_changed","data":{"pane_id":"w1:p3","workspace_id":"w1","agent_status":"blocked"}}`: {kind: "status", pane: "w1:p3", status: "blocked"},
		`{"event":"pane_exited","data":{"type":"pane_exited","pane_id":"w1:p3","workspace_id":"w1"}}`:                   {kind: "exited", pane: "w1:p3"},
		`{"event":"pane_closed","data":{"type":"pane_closed","pane_id":"w1:p4","workspace_id":"w1"}}`:                   {kind: "exited", pane: "w1:p4"},
		`{"event":"pane_moved","data":{"type":"pane_moved","previous_pane_id":"w1:p3","pane":{"pane_id":"w2:p1"}}}`:     {kind: "moved", previous: "w1:p3", pane: "w2:p1"},
		`{"event":"pane_agent_detected","data":{"type":"pane_agent_detected","pane_id":"w1:p5"}}`:                       {kind: "detected", pane: "w1:p5"},
	} {
		if got, ok := decode([]byte(line)); !ok || got != want {
			t.Errorf("%s: got %+v %v", line, got, ok)
		}
	}
	for _, line := range []string{`{"event":"tab_created","data":{}}`, `not json`, `{"event":"pane.agent_status_changed","data":{"pane_id":"w1:p3","agent_status":"bogus"}}`} {
		if _, ok := decode([]byte(line)); ok {
			t.Errorf("accepted %s", line)
		}
	}
}

// serve answers one events.subscribe connection with reply, then writes
// pushes, and reports the request it read.
func serve(t *testing.T, reply string, pushes ...string) (string, <-chan map[string]any) {
	l, path := sockettest.Listen(t)
	got := make(chan map[string]any, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		line, _ := bufio.NewReader(conn).ReadBytes('\n')
		var req map[string]any
		json.Unmarshal(line, &req)
		got <- req
		conn.Write([]byte(reply + "\n"))
		for _, p := range pushes {
			conn.Write([]byte(p + "\n"))
		}
	}()
	return path, got
}

func TestSocketSubscriberStreamsPushes(t *testing.T) {
	path, req := serve(t, `{"id":"fledge-supervise","result":{"type":"subscription_started"}}`,
		`{"event":"pane.agent_status_changed","data":{"pane_id":"w1:p3","workspace_id":"w1","agent_status":"idle"}}`)
	out := make(chan event, 4)
	err := socketSubscriber(path)(context.Background(), paneSubscriptions("w1:p3"), out)
	if err == nil {
		t.Fatal("stream ended without an error")
	}
	r := <-req
	if r["method"] != "events.subscribe" {
		t.Fatal(r)
	}
	subs := r["params"].(map[string]any)["subscriptions"].([]any)
	if s := subs[0].(map[string]any); s["type"] != "pane.agent_status_changed" || s["pane_id"] != "w1:p3" {
		t.Fatal(subs)
	}
	if e := <-out; e.kind != "subscribed" {
		t.Fatal(e)
	}
	if e := <-out; e != (event{kind: "status", pane: "w1:p3", status: "idle"}) {
		t.Fatal(e)
	}
}

func TestSocketSubscriberReturnsServerError(t *testing.T) {
	path, _ := serve(t, `{"id":"fledge-supervise","error":{"code":"pane_not_found","message":"pane w9:p9 not found"}}`)
	err := socketSubscriber(path)(context.Background(), paneSubscriptions("w9:p9"), make(chan event, 1))
	var remote *herdr.Error
	if !errors.As(err, &remote) || remote.Code != "pane_not_found" {
		t.Fatal(err)
	}
}

func TestKeepDropsMissingPane(t *testing.T) {
	out := make(chan event, 1)
	sub := func(context.Context, []map[string]any, chan<- event) error {
		return &herdr.Error{Code: "pane_not_found", Message: "gone"}
	}
	keep(context.Background(), sub, paneSubscriptions("w1:p3"), out, "w1:p3", log.New(&bytes.Buffer{}, "", 0))
	if e := <-out; e != (event{kind: "drop", pane: "w1:p3"}) {
		t.Fatal(e)
	}
}

// A lost connection is logged and retried after the backoff; the next ack is
// marked as a retry so the loop reconciles.
func TestKeepReconnectsWithBackoff(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := func(ctx context.Context, _ []map[string]any, out chan<- event) error {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			out <- event{kind: "subscribed"}
			return errors.New("EOF")
		}
		out <- event{kind: "subscribed"}
		<-ctx.Done()
		return ctx.Err()
	}
	out := make(chan event, 4)
	var logs bytes.Buffer
	logger := log.New(&syncWriter{w: &logs}, "", 0)
	go keep(ctx, sub, globalSubscriptions, out, "", logger)
	first, second := <-out, <-out
	if first.retry || !second.retry {
		t.Fatalf("%+v %+v", first, second)
	}
	cancel()
	time.Sleep(10 * time.Millisecond)
	if !strings.Contains(syncString(&logs), "reconnect global in 1s: EOF") {
		t.Fatal(logs.String())
	}
}

var logMu sync.Mutex

type syncWriter struct{ w *bytes.Buffer }

func (s *syncWriter) Write(p []byte) (int, error) {
	logMu.Lock()
	defer logMu.Unlock()
	return s.w.Write(p)
}

func syncString(b *bytes.Buffer) string {
	logMu.Lock()
	defer logMu.Unlock()
	return b.String()
}

// The daemon holds the lock while it runs, reports a heartbeat, and exits
// after 60 seconds with nothing to watch.
func TestRunExitsWhenIdle(t *testing.T) {
	w := newWorld(t)
	t.Setenv("HERDR_SESSION", "")
	start := time.Now()
	var mu sync.Mutex
	ticks := 0
	now := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		ticks++
		return start.Add(time.Duration(ticks) * 40 * time.Second)
	}
	c := libagent.Client{API: emptyHerdr{}}
	idle := func(ctx context.Context, _ []map[string]any, out chan<- event) error { <-ctx.Done(); return ctx.Err() }
	var logs bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), c, Options{Cwd: w.repo, Version: "v9", Log: &syncWriter{w: &logs}, subscribe: idle, now: now})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("daemon did not exit")
	}
	text := syncString(&logs)
	if !strings.Contains(text, "start pid") || !strings.Contains(text, "version v9") || !strings.Contains(text, "exit: nothing to watch") {
		t.Fatal(text)
	}
	if _, live, _ := supervise.Read(supervise.Dir(w.repo, "")); live {
		t.Fatal("lock still held")
	}
}

// emptyHerdr answers every agent.list with no agents.
type emptyHerdr struct{}

func (emptyHerdr) Call(_ context.Context, method string, _, result any) error {
	if method != "agent.list" {
		return errors.New("unexpected " + method)
	}
	*result.(*herdr.AgentListResult) = herdr.AgentListResult{Type: "agent_list", Agents: []herdr.AgentDetails{}}
	return nil
}

func TestRunExitsSilentlyWhenLockHeld(t *testing.T) {
	repo := identitytest.Repository(t)
	t.Setenv("HERDR_SESSION", "s")
	l, err := supervise.Acquire(supervise.Dir(repo, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	var logs bytes.Buffer
	if err := Run(context.Background(), herdrscript.Client(t), Options{Cwd: repo, Log: &logs}); err != nil || logs.Len() != 0 {
		t.Fatal(err, logs.String())
	}
}

// The status right after start already names the watched agents.
func TestRunReportsWatchedSetAtStart(t *testing.T) {
	w := newWorld(t)
	t.Setenv("HERDR_SESSION", "")
	c := libagent.Client{API: listHerdr{agents: []herdr.AgentDetails{w.bossA, w.wrkA}}}
	ctx, cancel := context.WithCancel(context.Background())
	idle := func(ctx context.Context, _ []map[string]any, out chan<- event) error { <-ctx.Done(); return ctx.Err() }
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, c, Options{Cwd: w.repo, Version: "v", Log: &syncWriter{w: &bytes.Buffer{}}, subscribe: idle})
	}()
	defer func() { cancel(); <-done }()
	dir := supervise.Dir(w.repo, "")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if st, live, _ := supervise.Read(dir); live && st.PID != 0 {
			if len(st.Watched) == 1 && st.Watched[0] == w.work.ID {
				return
			}
			if st.HeartbeatAt != "" && time.Now().After(deadline) {
				t.Fatalf("%+v", st)
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("no status")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// listHerdr answers agent.list with agents and nothing else.
type listHerdr struct{ agents []herdr.AgentDetails }

func (h listHerdr) Call(_ context.Context, method string, _, result any) error {
	if method != "agent.list" {
		return errors.New("unexpected " + method)
	}
	*result.(*herdr.AgentListResult) = herdr.AgentListResult{Type: "agent_list", Agents: h.agents}
	return nil
}
