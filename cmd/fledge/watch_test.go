package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/client"
	"github.com/Harrison-Blair/fledge/internal/daemon"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/protocol"
)

func TestWatchSelectsExplicitFlockThenEnvironment(t *testing.T) {
	root, sub := scaffoldedWorkspace(t)
	t.Chdir(sub)
	t.Setenv(flock.Env, "flock2")

	original := runLogWatcher
	var got []string
	runLogWatcher = func(_ context.Context, watcherRoot, name string, _ io.Writer) error {
		if watcherRoot != canonical(t, root) {
			t.Errorf("watch root = %q, want %q", watcherRoot, canonical(t, root))
		}
		got = append(got, name)
		return nil
	}
	t.Cleanup(func() { runLogWatcher = original })

	if err := run([]string{"watch", "flock1"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"watch"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"flock1", "flock2"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("watched flocks = %v, want %v", got, want)
	}
}

func TestWatchValidationAndSyntax(t *testing.T) {
	tests := [][]string{
		{"watch", "BAD"},
		{"watch", "flock1", "extra"},
		{"watch", "--json"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			err := run(args)
			if err == nil {
				t.Fatal("invalid watch command succeeded")
			}
			if !strings.Contains(err.Error(), helpPages["watch"]) {
				t.Fatalf("syntax error missing watch help:\n%v", err)
			}
		})
	}

	t.Setenv(flock.Env, "")
	err := run([]string{"watch"})
	if err == nil || !strings.Contains(err.Error(), flock.Env) {
		t.Fatalf("watch without a flock = %v", err)
	}
}

func TestWatchRequiresRunningDaemon(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Chdir(root)
	err := run([]string{"watch", "flock1"})
	if !errors.Is(err, client.ErrNotRunning) {
		t.Fatalf("watch down daemon error = %v", err)
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func waitForText(t *testing.T, out *lockedBuffer, text string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), text) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("output never contained %q:\n%s", text, out.String())
}

func TestWatchEmitsHistoryAndAppendsInOrder(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	startDaemon(t, root, "flock1")
	logPath := filepath.Join(flock.Dir(root, "flock1"), protocol.LogName)
	if err := os.WriteFile(logPath, []byte("old one\nold two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	originalPoll := watchPollInterval
	watchPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { watchPollInterval = originalPoll })
	ctx, cancel := context.WithCancel(context.Background())
	out := &lockedBuffer{}
	done := make(chan error, 1)
	go func() { done <- watchDaemonLog(ctx, root, "flock1", out) }()
	waitForText(t, out, "old two")

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(f, "new one\nnew two\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	waitForText(t, out, "new two")
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "old one\nold two\nnew one\nnew two\n"; got != want {
		t.Fatalf("watch output = %q, want %q", got, want)
	}
}

func TestWatchReportsDaemonShutdown(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	logPath := filepath.Join(flock.Dir(root, "flock1"), protocol.LogName)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("history\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := daemon.New(root, "flock1")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan struct{})
	go func() { _ = d.Serve(); close(served) }()

	originalPoll := watchPollInterval
	watchPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { watchPollInterval = originalPoll })
	out := &lockedBuffer{}
	done := make(chan error, 1)
	go func() { done <- watchDaemonLog(context.Background(), root, "flock1", out) }()
	waitForText(t, out, "history")
	d.Close()
	<-served
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "history\n") || !strings.HasSuffix(got, "fledge watch: daemon stopped\n") {
		t.Fatalf("shutdown output = %q", got)
	}
}

func (r *wireRecorder) methodParamsAt(method string, occurrence int) (map[string]any, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, env := range r.got {
		if string(env["method"]) != `"`+method+`"` {
			continue
		}
		if occurrence > 0 {
			occurrence--
			continue
		}
		var params map[string]any
		if json.Unmarshal(env["params"], &params) != nil {
			return nil, false
		}
		return params, true
	}
	return nil, false
}

func TestInteractiveStartBuildsThreePaneLayoutAfterNativeLaunchReadiness(t *testing.T) {
	rec, _, _, out, err := interactiveStart(t, map[string]agentcfg.Config{
		"orchestrator-profile": {Integration: "claude", Model: "claude-opus-4"},
	}, "1\n")
	if err != nil {
		t.Fatalf("interactive start: %v\n%s", err, out)
	}

	split, ok := rec.methodParamsAt("pane.split", 0)
	if !ok {
		t.Fatal("interactive start never split the right CLI pane")
	}
	if split["target_pane_id"] != "w1:p1" || split["direction"] != "down" ||
		split["ratio"] != 0.5 || split["focus"] != false {
		t.Fatalf("right-side split params = %+v", split)
	}

	input, ok := rec.methodParamsAt("pane.send_input", 0)
	if !ok {
		t.Fatal("interactive start never started the watcher in the upper-right pane")
	}
	text, _ := input["text"].(string)
	if input["pane_id"] != "w1:p1" || !strings.HasPrefix(text, "exec ") ||
		!strings.HasSuffix(text, " watch 'flock1'") || input["keys"] == nil {
		t.Fatalf("watcher command params = %+v", input)
	}
	focus, ok := rec.methodParamsAt("pane.focus", 1)
	if !ok || focus["pane_id"] != "w1:p2" {
		t.Fatalf("watcher final focus = %+v", focus)
	}

	methods := rec.methods()
	agentAt, swapAt, splitAt, watcherAt, finalFocusAt := -1, -1, -1, -1, -1
	for i, method := range methods {
		if method == "agent.start" {
			agentAt = i
		}
		if method == "pane.swap" {
			swapAt = i
		}
		if method == "pane.split" {
			splitAt = i
		}
		if method == "pane.send_input" {
			watcherAt = i
		}
		if method == "pane.focus" && i > watcherAt {
			finalFocusAt = i
		}
	}
	if agentAt < 0 || swapAt < agentAt || splitAt < swapAt || watcherAt < splitAt || finalFocusAt < watcherAt {
		t.Fatalf("startup methods = %v; want spawn/place, split, watch, refocus ordering", methods)
	}
	if strings.Count(strings.Join(methods, ","), "agent.start") != 1 {
		t.Fatalf("startup methods = %v; want one orchestrator agent", methods)
	}
	if strings.Count(strings.Join(methods, ","), "workspace.create") != 1 {
		t.Fatalf("startup methods = %v; want one workspace", methods)
	}
	for _, forbidden := range []string{"pane.close", "workspace.close"} {
		if slices.Contains(methods, forbidden) {
			t.Fatalf("startup methods = %v; watcher must not invoke %s", methods, forbidden)
		}
	}
	for _, call := range rec.got {
		var params map[string]any
		if json.Unmarshal(call["params"], &params) == nil && params["pane_id"] == "w1:p3" {
			t.Fatalf("new lower shell was modified by %s: %+v", call["method"], params)
		}
	}
}

type layoutCall struct {
	method string
	params map[string]any
}

type layoutRecorder struct {
	mu      sync.Mutex
	calls   []layoutCall
	replies map[string][]string
	counts  map[string]int
}

func startLayoutServer(t *testing.T, replies map[string][]string) (string, *layoutRecorder) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	rec := &layoutRecorder{replies: replies, counts: map[string]int{}}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				line, err := bufio.NewReader(conn).ReadBytes('\n')
				if err != nil {
					return
				}
				var env struct {
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				if json.Unmarshal(line, &env) != nil {
					return
				}
				rec.mu.Lock()
				rec.calls = append(rec.calls, layoutCall{env.Method, env.Params})
				idx := rec.counts[env.Method]
				rec.counts[env.Method]++
				choices := rec.replies[env.Method]
				reply := `{"id":"1","result":{}}`
				if env.Method == "pane.split" {
					reply = splitPaneReply
				}
				if len(choices) > 0 {
					if idx >= len(choices) {
						idx = len(choices) - 1
					}
					reply = choices[idx]
				}
				rec.mu.Unlock()
				_, _ = io.WriteString(conn, reply+"\n")
			}()
		}
	}()
	return sock, rec
}

func (r *layoutRecorder) snapshot() []layoutCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]layoutCall(nil), r.calls...)
}

const layoutFailureReply = `{"id":"1","error":{"code":"failed","message":"can't do that"}}`

func TestInstallWatcherPaneSplitsRightAndLeavesLowerShell(t *testing.T) {
	sock, rec := startLayoutServer(t, nil)
	if err := installWatcherPane(sock, "/work/project", "flock7", "/opt/fledge's bin/fledge", "w1:p1", "w1:p2"); err != nil {
		t.Fatal(err)
	}
	calls := rec.snapshot()
	if len(calls) != 3 {
		t.Fatalf("layout calls = %+v", calls)
	}
	if calls[0].method != "pane.split" || calls[1].method != "pane.send_input" ||
		calls[2].method != "pane.focus" || calls[2].params["pane_id"] != "w1:p2" {
		t.Fatalf("layout ordering = %+v", calls)
	}
	split := calls[0].params
	if split["target_pane_id"] != "w1:p1" || split["direction"] != "down" ||
		split["ratio"] != 0.5 || split["cwd"] != "/work/project" || split["focus"] != false {
		t.Fatalf("split params = %+v", split)
	}
	input := calls[1].params
	if input["pane_id"] != "w1:p1" || input["text"] != `exec '/opt/fledge'"'"'s bin/fledge' watch 'flock7'` || input["keys"] == nil {
		t.Fatalf("watcher command params = %+v", input)
	}
	for _, call := range calls {
		if call.params["pane_id"] == "w1:p3" {
			t.Fatalf("lower shell was modified: %+v", calls)
		}
		switch call.method {
		case "agent.start", "pane.close", "workspace.create", "workspace.close", "tab.rename":
			t.Fatalf("watcher setup invoked %s: %+v", call.method, calls)
		}
	}
}

func TestInstallWatcherPaneFailuresRecoverAndRefocus(t *testing.T) {
	tests := []struct {
		name        string
		replies     map[string][]string
		wantWarning bool
		wantClose   bool
	}{
		{"split", map[string][]string{"pane.split": {layoutFailureReply}}, true, false},
		{"command delivery", map[string][]string{"pane.send_input": {layoutFailureReply, `{"id":"1","result":{}}`}}, true, true},
		{"cleanup", map[string][]string{
			"pane.send_input": {layoutFailureReply, `{"id":"1","result":{}}`},
			"pane.close":      {layoutFailureReply},
		}, true, true},
		{"final focus", map[string][]string{"pane.focus": {layoutFailureReply}}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sock, rec := startLayoutServer(t, tt.replies)
			if err := installWatcherPane(sock, "/work/project", "flock1", "/bin/fledge", "w1:p1", "w1:p2"); err == nil {
				t.Fatal("layout failure returned nil")
			}
			calls := rec.snapshot()
			var warning map[string]any
			var close, finalFocus map[string]any
			for _, call := range calls {
				if call.method == "pane.send_input" && call.params["pane_id"] == "w1:p1" &&
					strings.Contains(fmt.Sprint(call.params["text"]), "automatic log watcher unavailable") {
					warning = call.params
				}
				if call.method == "pane.close" {
					close = call.params
				}
				if call.method == "pane.focus" {
					finalFocus = call.params
				}
				switch call.method {
				case "agent.start", "workspace.create", "workspace.close", "tab.rename":
					t.Fatalf("failure created unrelated layout with %s: %+v", call.method, calls)
				}
			}
			if tt.wantWarning && (warning == nil || warning["keys"] == nil) {
				t.Fatalf("manual warning = %+v", warning)
			}
			if !tt.wantWarning && warning != nil {
				t.Fatalf("unexpected warning after watcher started: %+v", warning)
			}
			if tt.wantClose && (close == nil || close["pane_id"] != "w1:p3") {
				t.Fatalf("lower-pane cleanup = %+v", close)
			}
			if !tt.wantClose && close != nil {
				t.Fatalf("unexpected lower-pane cleanup = %+v", close)
			}
			if finalFocus == nil || finalFocus["pane_id"] != "w1:p2" {
				t.Fatalf("final orchestrator focus = %+v", finalFocus)
			}
		})
	}
}
