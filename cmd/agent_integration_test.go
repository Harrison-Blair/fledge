package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// newSocket starts a fake Herdr Unix socket listener and points the
// environment at it, so ExecuteWithArgs/execute talk to it. The test runs
// outside any Git repository so agent records never reach a real checkout.
func newSocket(t *testing.T) net.Listener {
	t.Helper()
	dir, err := os.MkdirTemp("", "fc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "s")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_SOCKET_PATH", path)
	t.Chdir(t.TempDir())
	return l
}

type rpcCall struct {
	Method string
	Params json.RawMessage
}

// serveRPCs replies to exactly len(results) requests on l in order, then
// closes l so an unexpected extra request fails fast (connection refused)
// instead of hanging the test.
func serveRPCs(l net.Listener, results ...any) <-chan []rpcCall {
	done := make(chan []rpcCall, 1)
	go func() {
		var calls []rpcCall
		for _, result := range results {
			conn, err := l.Accept()
			if err != nil {
				done <- calls
				return
			}
			var req struct {
				ID     string          `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			json.NewDecoder(conn).Decode(&req)
			calls = append(calls, rpcCall{Method: req.Method, Params: req.Params})
			json.NewEncoder(conn).Encode(map[string]any{"id": req.ID, "result": result})
			conn.Close()
		}
		l.Close()
		done <- calls
	}()
	return done
}

// waitCalls closes l first, so a serveRPCs goroutine blocked in Accept on an
// RPC the command under test never made unblocks immediately instead of
// waiting out the full bound, then reads from done with a bound as a
// backstop, failing fast with a clear message.
func waitCalls(t *testing.T, l net.Listener, done <-chan []rpcCall, want int) []rpcCall {
	t.Helper()
	l.Close()
	select {
	case calls := <-done:
		if len(calls) != want {
			t.Fatalf("got %d calls, want %d: %+v", len(calls), want, calls)
		}
		return calls
	case <-time.After(time.Second):
		t.Fatal("fake server: timed out waiting for RPCs")
		return nil
	}
}
func paramsField(t *testing.T, c rpcCall, field string) any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(c.Params, &m); err != nil {
		t.Fatal(err)
	}
	return m[field]
}

// headered reports whether an agent.prompt text is body behind the
// unknown-sender header used when HERDR_PANE_ID is unset.
func headered(t *testing.T, c rpcCall, body string) bool {
	t.Helper()
	text, _ := paramsField(t, c, "text").(string)
	return regexp.MustCompile(`^ᛉ fledge message from unknown sender · id m-[0-9a-f]{6}\n` + regexp.QuoteMeta(body) + `$`).MatchString(text)
}

func snapshotResult() any {
	return map[string]any{"type": "session_snapshot", "snapshot": map[string]any{"protocol": 999, "version": "future", "workspaces": []any{}, "tabs": []any{}, "layouts": []any{}, "agents": []any{}, "panes": []any{map[string]any{"pane_id": "w1:p1", "workspace_id": "w1", "tab_id": "w1:t1"}}}}
}
func startedResult(argv ...string) any {
	return map[string]any{"type": "agent_started", "agent": map[string]any{"pane_id": "w1:p1", "workspace_id": "w1", "tab_id": "w1:t1", "name": "worker", "agent": "claude", "agent_status": "unknown", "terminal_id": "term_x", "launch_pending": true}, "argv": argv}
}
func waitedResult() any {
	return map[string]any{"type": "agent_info", "agent": map[string]any{"pane_id": "w1:p1", "workspace_id": "w1", "tab_id": "w1:t1", "agent": "claude", "agent_status": "idle", "terminal_id": "term_x", "focused": false, "revision": 0}}
}

// readyAs is the spawned worker's settled, prompt-ready agent.wait result as harness.
func readyAs(harness string) any {
	r := waitedResult().(map[string]any)
	a := r["agent"].(map[string]any)
	a["name"], a["agent"], a["interactive_ready"] = "worker", harness, true
	return r
}
func promptedResult() any {
	return map[string]any{"type": "agent_prompted", "agent": map[string]any{"pane_id": "w1:p1", "workspace_id": "w1", "tab_id": "w1:t1", "agent": "claude", "agent_status": "working"}}
}

func TestSpawnForwardsExactNativeTokens(t *testing.T) {
	l := newSocket(t)
	done := serveRPCs(l, snapshotResult(), startedResult("--setting=a,b", "two words", "--native", "x,y"), readyAs("claude"))
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--harness", "claude", "--pane", "w1:p1", "--args=--setting=a,b", "--args", "two words", "--json", "--", "--native", "x,y"}, &out)
	if err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 3)
	var args struct {
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(calls[1].Params, &args); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args.Args, []string{"--setting=a,b", "two words", "--native", "x,y"}) {
		t.Fatalf("%q", args.Args)
	}
	var envelope map[string]any
	if err = json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err, out.String())
	}
	if envelope["status"] != "success" {
		t.Fatal(out.String())
	}
}

func TestSpawnPromptFlagReachesAgentPromptWithExactText(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	l := newSocket(t)
	done := serveRPCs(l, snapshotResult(), startedResult("claude"), readyAs("claude"), promptedResult())
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--harness", "claude", "--pane", "w1:p1", "--prompt", "review this", "--json"}, &out)
	if err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 4)
	if calls[3].Method != "agent.prompt" || !headered(t, calls[3], "review this") {
		t.Fatalf("%+v", calls)
	}
	var envelope map[string]any
	if err = json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err, out.String())
	}
	result, _ := envelope["result"].(map[string]any)
	if envelope["status"] != "success" || result == nil || result["prompted"] != true {
		t.Fatal(out.String())
	}
}

// TestSpawnNoWaitFlagSkipsAgentWaitOnSocket proves --no-wait reaches the
// service: the fake server only answers snapshot+start, so if the flag were
// unwired (Spawn still waiting) the third dial would fail fast (listener
// closed) instead of the test hanging.
func TestSpawnNoWaitFlagSkipsAgentWaitOnSocket(t *testing.T) {
	l := newSocket(t)
	done := serveRPCs(l, snapshotResult(), startedResult("claude"))
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--harness", "claude", "--pane", "w1:p1", "--no-wait", "--json"}, &out)
	if err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 2)
	if calls[1].Method != "agent.start" {
		t.Fatalf("%+v", calls)
	}
}

// TestSpawnShortTimeoutKeepsStartReservation proves a short --timeout still
// reserves Herdr's 30s startup, so the name outlives a slow launch.
func TestSpawnShortTimeoutKeepsStartReservation(t *testing.T) {
	l := newSocket(t)
	done := serveRPCs(l, snapshotResult(), startedResult("claude"))
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--harness", "claude", "--pane", "w1:p1", "--timeout", "3001ms", "--no-wait", "--json"}, &out)
	if err != nil {
		t.Fatal(err, out.String())
	}
	if calls := waitCalls(t, l, done, 2); paramsField(t, calls[1], "timeout_ms") != float64(30000) {
		t.Fatalf("%s", calls[1].Params)
	}
}

// TestSpawnFileFlagPathReachesAgentPrompt proves --file <path> is wired
// (FileSet) end to end: the file's exact text reaches agent.prompt.
func TestSpawnFileFlagPathReachesAgentPrompt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(path, []byte("from a file on disk"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PANE_ID", "")
	l := newSocket(t)
	done := serveRPCs(l, snapshotResult(), startedResult("claude"), readyAs("claude"), promptedResult())
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--harness", "claude", "--pane", "w1:p1", "--file", path, "--json"}, &out)
	if err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 4)
	if calls[3].Method != "agent.prompt" || !headered(t, calls[3], "from a file on disk") {
		t.Fatalf("%+v", calls)
	}
}

// TestSpawnFileDashReadsCommandStdin proves --file - reads the command's own
// injected stdin (cmd.InOrStdin()), not a bare nil reader.
func TestSpawnFileDashReadsCommandStdin(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	l := newSocket(t)
	done := serveRPCs(l, snapshotResult(), startedResult("claude"), readyAs("claude"), promptedResult())
	var out bytes.Buffer
	err := execute([]string{"agent", "spawn", "--name", "worker", "--harness", "claude", "--pane", "w1:p1", "--file", "-", "--json"}, strings.NewReader("from stdin"), &out, &out)
	if err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 4)
	if calls[3].Method != "agent.prompt" || !headered(t, calls[3], "from stdin") {
		t.Fatalf("%+v", calls)
	}
}

// TestSpawnBlockedWaitWithPromptIsPartialWithFledgeHints serves only
// snapshot, start, and a blocked wait, so an agent.prompt dial would fail
// fast; the human output must name the unsent prompt without echoing it.
func TestSpawnBlockedWaitWithPromptIsPartialWithFledgeHints(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	l := newSocket(t)
	blocked := readyAs("claude").(map[string]any)
	blocked["agent"].(map[string]any)["agent_status"] = "blocked"
	done := serveRPCs(l, snapshotResult(), startedResult("claude"), blocked)
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--harness", "claude", "--pane", "w1:p1", "--prompt", "secret brief"}, &out)
	if ExitCode(err) != 1 {
		t.Fatalf("exit code = %d (%v): %s", ExitCode(err), err, out.String())
	}
	if calls := waitCalls(t, l, done, 3); calls[2].Method != "agent.wait" {
		t.Fatalf("%+v", calls)
	}
	s := out.String()
	for _, want := range []string{"partial:", "The first prompt was not submitted", "fledge agent read --pane w1:p1", "fledge agent send --pane w1:p1 --key <key>", "fledge agent message --pane w1:p1 --file <brief>"} {
		if !strings.Contains(s, want) {
			t.Fatalf("%q missing %q", s, want)
		}
	}
	if strings.Contains(s, "secret brief") || strings.Contains(s, "herdr ") {
		t.Fatalf("%q", s)
	}
}

func TestGetForwardsTargetAndDecodesDetails(t *testing.T) {
	for _, flag := range []string{"--name", "--pane"} {
		t.Run(flag, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "fg-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(dir) })
			path := filepath.Join(dir, "s")
			l, err := net.Listen("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			t.Setenv("HERDR_ENV", "1")
			t.Setenv("HERDR_SOCKET_PATH", path)
			t.Chdir(t.TempDir())
			target := "reviewer"
			if flag == "--pane" {
				target = "w2:p3"
			}
			done := make(chan string, 1)
			go func() {
				conn, err := l.Accept()
				if err != nil {
					done <- err.Error()
					return
				}
				defer conn.Close()
				var req struct {
					ID     string         `json:"id"`
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				if err := json.NewDecoder(conn).Decode(&req); err != nil {
					done <- err.Error()
					return
				}
				if req.Method != "agent.get" || !reflect.DeepEqual(req.Params, map[string]any{"target": target}) {
					done <- "incorrect request"
					return
				}
				result := json.RawMessage(`{"type":"agent_info","agent":{"pane_id":"w2:p3","workspace_id":"w2","tab_id":"w2:t1","name":"reviewer","agent":"codex","agent_status":"working","cwd":"/repo","terminal_id":"term_x","foreground_cwd":"/repo/sub","interactive_ready":false,"launch_pending":true,"focused":false,"revision":0,"title":"Review","agent_session":{"source":"herdr:codex","agent":"codex","kind":"path","value":"/sessions/123"}}}`)
				if err := json.NewEncoder(conn).Encode(map[string]any{"id": req.ID, "result": result}); err != nil {
					done <- err.Error()
					return
				}
				done <- ""
			}()
			var out bytes.Buffer
			err = ExecuteWithArgs([]string{"agent", "get", flag, target, "--json"}, &out)
			if err != nil {
				t.Fatal(err, out.String())
			}
			if problem := <-done; problem != "" {
				t.Fatal(problem)
			}
			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"operation": "agent.get", "status": "success", "effects": []any{}, "error": nil, "result": map[string]any{"pane_id": "w2:p3", "workspace_id": "w2", "tab_id": "w2:t1", "name": "reviewer", "harness": "codex", "agent_status": "working", "cwd": "/repo", "foreground_cwd": "/repo/sub", "interactive_ready": false, "launch_pending": true, "focused": false, "title": "Review", "agent_session": map[string]any{"source": "herdr:codex", "harness": "codex", "kind": "path", "value": "/sessions/123"}, "record": nil}}
			if !reflect.DeepEqual(envelope, want) {
				t.Fatalf("got %s", out.String())
			}
		})
	}
}

// gitRepo makes the current directory a fresh Git repository.
func gitRepo(t *testing.T) {
	t.Helper()
	if b, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, b)
	}
}

func TestListParentFlagFiltersAgents(t *testing.T) {
	l := newSocket(t)
	gitRepo(t)
	done := serveRPCs(l, map[string]any{"type": "agent_list", "agents": []any{map[string]any{"pane_id": "w1:p1", "workspace_id": "w1", "tab_id": "w1:t1", "agent_status": "idle", "terminal_id": "term_x", "focused": false, "revision": 0}}})
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "list", "--parent", "0000beef", "--json"}, &out); err != nil {
		t.Fatal(err, out.String())
	}
	waitCalls(t, l, done, 1)
	if !strings.Contains(out.String(), `"agents":[]`) {
		t.Fatal(out.String())
	}
}

func TestMineAndCurrentRequireRegisteredCaller(t *testing.T) {
	for _, args := range [][]string{{"agent", "list", "--mine", "--json"}, {"agent", "current", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			l := newSocket(t)
			gitRepo(t)
			t.Setenv("HERDR_PANE_ID", "")
			done := serveRPCs(l)
			var out bytes.Buffer
			err := ExecuteWithArgs(args, &out)
			waitCalls(t, l, done, 0)
			var status interface{ ExitCode() int }
			if !errors.As(err, &status) || status.ExitCode() != 1 || !strings.Contains(out.String(), `"code":"caller_unregistered"`) {
				t.Fatalf("%v %s", err, out.String())
			}
		})
	}
}
