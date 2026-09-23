package wait

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// serveFanOut answers agent.wait for "a" with agent_not_running at once and
// for "b" with an idle agent once release closes, each on its own connection.
func serveFanOut(t *testing.T, release <-chan struct{}) {
	t.Helper()
	dir, err := os.MkdirTemp("", "fw-")
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
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				var req struct {
					ID     string `json:"id"`
					Params struct {
						Target string `json:"target"`
					} `json:"params"`
				}
				json.NewDecoder(conn).Decode(&req)
				reply := map[string]any{"id": req.ID, "error": map[string]any{"code": "agent_not_running", "message": "gone"}}
				if req.Params.Target == "b" {
					<-release
					reply = map[string]any{"id": req.ID, "result": map[string]any{"type": "agent_info", "agent": map[string]any{
						"pane_id": "w1:p2", "workspace_id": "w1", "tab_id": "w1:t1", "name": "b", "agent": "pi", "agent_status": "idle", "terminal_id": "term_b", "focused": false, "revision": 0}}}
				}
				json.NewEncoder(conn).Encode(reply)
			}()
		}
	}()
}

// signalWriter records writes and signals each one.
type signalWriter struct {
	mu    sync.Mutex
	b     bytes.Buffer
	wrote chan struct{}
}

func (w *signalWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	select {
	case w.wrote <- struct{}{}:
	default:
	}
	return w.b.Write(p)
}

func (w *signalWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

func run(args []string, stdout, stderr *signalWriter) <-chan error {
	done := make(chan error, 1)
	cmd := New()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	go func() { done <- cmd.Execute() }()
	return done
}

func TestAnyReportsFailureOnStderrBeforeResult(t *testing.T) {
	release := make(chan struct{})
	serveFanOut(t, release)
	stdout, stderr := &signalWriter{wrote: make(chan struct{}, 1)}, &signalWriter{wrote: make(chan struct{}, 1)}
	done := run([]string{"--name", "a", "--name", "b", "--any"}, stdout, stderr)
	select {
	case <-stderr.wrote:
	case err := <-done:
		t.Fatalf("wait ended before b matched: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("no progress while b was pending")
	}
	if got, want := stderr.String(), "a failed: agent_not_running: gone (still waiting on 1 target).\n"; got != want || stdout.String() != "" {
		t.Fatalf("stderr %q, stdout %q", got, stdout.String())
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "a failed: agent_not_running: gone.\nb is idle (first match).\n"; got != want {
		t.Fatalf("stdout %q", got)
	}
}

func TestAnyJSONWritesNoProgress(t *testing.T) {
	release := make(chan struct{})
	serveFanOut(t, release)
	time.AfterFunc(200*time.Millisecond, func() { close(release) })
	stdout, stderr := &signalWriter{wrote: make(chan struct{}, 1)}, &signalWriter{wrote: make(chan struct{}, 1)}
	if err := <-run([]string{"--name", "a", "--name", "b", "--any", "--json"}, stdout, stderr); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Result struct {
			Winner string `json:"winner"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &out); err != nil || out.Result.Winner != "b" || stderr.String() != "" {
		t.Fatalf("%v: stdout %q, stderr %q", err, stdout.String(), stderr.String())
	}
}
