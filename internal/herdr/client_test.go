package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/fswatch"
)

const (
	helperModeEnv    = "FLEDGE_HERDR_TEST_HELPER"
	helperCaptureEnv = "FLEDGE_HERDR_TEST_CAPTURE"
	helperStdoutEnv  = "FLEDGE_HERDR_TEST_STDOUT"
	helperStderrEnv  = "FLEDGE_HERDR_TEST_STDERR"
	helperExitEnv    = "FLEDGE_HERDR_TEST_EXIT"
	helperBlockEnv   = "FLEDGE_HERDR_TEST_BLOCK"
	helperReadyEnv   = "FLEDGE_HERDR_TEST_READY"
	// helperSequenceEnv names a directory holding one response file per planned
	// invocation, letting a test script a sequence of differing Herdr replies.
	helperSequenceEnv = "FLEDGE_HERDR_TEST_SEQUENCE"
)

type helperInvocation struct {
	Args  []string `json:"args"`
	Dir   string   `json:"dir"`
	Stdin string   `json:"stdin"`
	Env   string   `json:"env,omitempty"`
}

func TestMain(m *testing.M) {
	if os.Getenv(helperModeEnv) == "1" {
		runHerdrHelper()
	}

	os.Exit(m.Run())
}

func runHerdrHelper() {
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if capture := os.Getenv(helperCaptureEnv); capture != "" {
		invocation := helperInvocation{Args: os.Args[1:], Dir: dir, Stdin: string(stdin), Env: os.Getenv("FLEDGE_REPLACED_ENV")}
		contents, err := json.Marshal(invocation)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := os.WriteFile(capture, contents, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}

	if sequence := os.Getenv(helperSequenceEnv); sequence != "" {
		respondFromSequence(sequence)
	}

	_, _ = io.WriteString(os.Stdout, os.Getenv(helperStdoutEnv))
	_, _ = io.WriteString(os.Stderr, os.Getenv(helperStderrEnv))
	if os.Getenv(helperBlockEnv) == "1" {
		if ready := os.Getenv(helperReadyEnv); ready != "" {
			if err := os.WriteFile(ready, nil, 0o600); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
		}
		// Keep the child alive until CommandContext terminates it. A timer keeps
		// the helper from being diagnosed as a deadlocked standalone process if a
		// test fails before cancellation reaches the child.
		<-time.After(time.Hour)
	}

	if value := os.Getenv(helperExitEnv); value != "" {
		code, err := strconv.Atoi(value)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(code)
	}

	os.Exit(0)
}

// respondFromSequence consumes the next queued response and exits. An empty
// response file makes the invocation fail, standing in for a server that is not
// answering yet. Client operations are serialized, so consuming files needs no
// locking.
func respondFromSequence(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "scripted response sequence is exhausted")
		os.Exit(1)
	}
	path := filepath.Join(dir, entries[0].Name())
	response, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := os.Remove(path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if len(response) == 0 {
		fmt.Fprintln(os.Stderr, "herdr is unavailable")
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(response)
	os.Exit(0)
}

func TestClientCheck(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		client := NewClient(helperBinary(t), nil, nil, nil)
		if err := client.Check(); err != nil {
			t.Fatalf("Check() error = %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		binary := filepath.Join(t.TempDir(), "missing-herdr")
		client := NewClient(binary, nil, nil, nil)

		err := client.Check()
		if err == nil {
			t.Fatal("Check() error = nil, want error")
		}
		prefix := fmt.Sprintf("find %q on PATH:", binary)
		if !strings.HasPrefix(err.Error(), prefix) {
			t.Fatalf("Check() error = %q, want prefix %q", err, prefix)
		}
	})
}

func TestClientAttach(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(t.TempDir(), "invocation.json")
	configureHelper(t, capture)
	t.Setenv(helperStdoutEnv, "attached output")
	t.Setenv(helperStderrEnv, "attached warning")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	client := NewClient(helperBinary(t), strings.NewReader("terminal input"), stdout, stderr)

	if err := client.Attach(context.Background(), "session-name", dir); err != nil {
		t.Fatalf("Attach() error = %v", err)
	}

	if got, want := stdout.String(), "attached output"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "attached warning"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}

	invocation := readInvocation(t, capture)
	assertStrings(t, "args", invocation.Args, []string{"--session", "session-name"})
	if invocation.Dir != dir {
		t.Errorf("dir = %q, want %q", invocation.Dir, dir)
	}
	if got, want := invocation.Stdin, "terminal input"; got != want {
		t.Errorf("stdin = %q, want %q", got, want)
	}
}

func TestClientAttachError(t *testing.T) {
	configureHelper(t, "")
	t.Setenv(helperExitEnv, "23")

	client := NewClient(helperBinary(t), nil, io.Discard, io.Discard)
	err := client.Attach(context.Background(), "broken", t.TempDir())
	if err == nil {
		t.Fatal("Attach() error = nil, want error")
	}
	if got, want := err.Error(), `attach to Herdr session "broken": exit status 23`; got != want {
		t.Fatalf("Attach() error = %q, want %q", got, want)
	}
}

func TestClientList(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "invocation.json")
	configureHelper(t, capture)
	t.Setenv(helperStdoutEnv, `{"sessions":[{"name":"one","running":true,"socket_path":"/home/user/.config/herdr/sessions/one/herdr.sock"},{"name":"two","running":false}]}`)

	client := NewClient(helperBinary(t), nil, nil, nil)
	sessions, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []Session{{Name: "one", Running: true, SocketPath: "/home/user/.config/herdr/sessions/one/herdr.sock"}, {Name: "two", Running: false}}
	if len(sessions) != len(want) {
		t.Fatalf("List() = %#v, want %#v", sessions, want)
	}
	for i := range want {
		if sessions[i] != want[i] {
			t.Errorf("List()[%d] = %#v, want %#v", i, sessions[i], want[i])
		}
	}

	invocation := readInvocation(t, capture)
	assertStrings(t, "args", invocation.Args, []string{"session", "list", "--json"})
}

func TestClientListResponses(t *testing.T) {
	tests := []struct {
		name       string
		stdout     string
		wantErr    string
		wantPrefix string
	}{
		{name: "empty", wantErr: "list Herdr sessions: decode JSON response: empty response"},
		{name: "malformed", stdout: "{", wantPrefix: "list Herdr sessions: decode JSON response:"},
		{name: "trailing JSON", stdout: "{} {}", wantPrefix: "list Herdr sessions: decode JSON response:"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configureHelper(t, "")
			t.Setenv(helperStdoutEnv, test.stdout)

			sessions, err := NewClient(helperBinary(t), nil, nil, nil).List(context.Background())
			if test.wantErr != "" && (err == nil || err.Error() != test.wantErr) {
				t.Fatalf("List() error = %q, want %q", err, test.wantErr)
			}
			if test.wantPrefix != "" && (err == nil || !strings.HasPrefix(err.Error(), test.wantPrefix)) {
				t.Fatalf("List() error = %q, want prefix %q", err, test.wantPrefix)
			}
			if test.wantErr == "" && test.wantPrefix == "" && err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if sessions != nil {
				t.Fatalf("List() sessions = %#v, want nil", sessions)
			}
		})
	}
}

func TestClientStopAndDelete(t *testing.T) {
	tests := []struct {
		name      string
		operation func(*Client) error
		wantArgs  []string
	}{
		{
			name: "stop",
			operation: func(client *Client) error {
				return client.Stop(context.Background(), "session-name")
			},
			wantArgs: []string{"session", "stop", "session-name", "--json"},
		},
		{
			name: "delete",
			operation: func(client *Client) error {
				return client.Delete(context.Background(), "session-name")
			},
			wantArgs: []string{"session", "delete", "session-name", "--json"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capture := filepath.Join(t.TempDir(), "invocation.json")
			configureHelper(t, capture)
			t.Setenv(helperStdoutEnv, "ignored non-JSON response")

			client := NewClient(helperBinary(t), nil, nil, nil)
			if err := test.operation(client); err != nil {
				t.Fatalf("operation error = %v", err)
			}

			invocation := readInvocation(t, capture)
			assertStrings(t, "args", invocation.Args, test.wantArgs)
		})
	}
}

func TestClientDestructiveCommandErrors(t *testing.T) {
	tests := []struct {
		name      string
		operation func(*Client) error
		want      string
	}{
		{
			name: "stop",
			operation: func(client *Client) error {
				return client.Stop(context.Background(), "broken")
			},
			want: `stop Herdr session "broken": exit status 17: failed`,
		},
		{
			name: "delete",
			operation: func(client *Client) error {
				return client.Delete(context.Background(), "broken")
			},
			want: `delete Herdr session "broken": exit status 17: failed`,
		},
		{
			name: "close pane",
			operation: func(client *Client) error {
				return client.ClosePane(context.Background(), "broken", "w1:p2")
			},
			want: `close Herdr pane "w1:p2": exit status 17: failed`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configureHelper(t, "")
			t.Setenv(helperStderrEnv, "failed")
			t.Setenv(helperExitEnv, "17")

			client := NewClient(helperBinary(t), nil, nil, nil)
			if err := test.operation(client); err == nil || err.Error() != test.want {
				t.Fatalf("operation error = %q, want %q", err, test.want)
			}
		})
	}
}

func TestClientSnapshot(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "invocation.json")
	configureHelper(t, capture)
	t.Setenv(helperStdoutEnv, `{"id":"1","result":{"type":"session_snapshot","snapshot":{"focused_workspace_id":"w1","focused_tab_id":"t1","focused_pane_id":"w1:p1","workspaces":[{"workspace_id":"w1","label":"project"}],"tabs":[{"tab_id":"t1","workspace_id":"w1","label":"orchestrator"}],"panes":[{"pane_id":"w1:p1","tab_id":"t1","workspace_id":"w1","label":"orchestrator","agent_status":"blocked"}],"agents":[{"name":"orchestrator","agent":"codex","pane_id":"w1:p1","tab_id":"t1","workspace_id":"w1","revision":664,"agent_status":"working","agent_session":{"agent":"codex","kind":"id","source":"herdr:codex","value":"019fcf2f-6113"}}]}}}`)

	snapshot, err := NewClient(helperBinary(t), nil, nil, nil).Snapshot(context.Background(), "session-name")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.FocusedPaneID != "w1:p1" || len(snapshot.Agents) != 1 || snapshot.Agents[0].Name == nil || *snapshot.Agents[0].Name != "orchestrator" {
		t.Fatalf("Snapshot() = %#v", snapshot)
	}
	if len(snapshot.Panes) != 1 || snapshot.Panes[0].AgentStatus != "blocked" {
		t.Fatalf("Snapshot() panes = %#v, want agent_status blocked", snapshot.Panes)
	}
	agent := snapshot.Agents[0]
	if agent.Revision != 664 {
		t.Errorf("agent revision = %d, want 664", agent.Revision)
	}
	if agent.AgentStatus != "working" {
		t.Errorf("agent agent_status = %q, want working", agent.AgentStatus)
	}
	if agent.AgentSession == nil || agent.AgentSession.Kind != "id" || agent.AgentSession.Value != "019fcf2f-6113" {
		t.Errorf("agent_session = %#v, want id correlation preserved", agent.AgentSession)
	}
	assertStrings(t, "args", readInvocation(t, capture).Args, []string{"--session", "session-name", "api", "snapshot"})
}

func TestClientWaitReadyAcceptsSettledEmptyServer(t *testing.T) {
	configureHelper(t, "")
	t.Setenv(helperSequenceEnv, writeSequence(t, []string{
		`{"sessions":[{"name":"session-name","running":true,"socket_path":"/herdr/session.sock"}]}`,
		`{"id":"1","result":{"type":"session_snapshot","snapshot":{}}}`,
	}))
	client := NewClient(helperBinary(t), nil, nil, nil)
	client.watch = func(string) (fswatch.Watcher, error) { return newFakeWatcher(), nil }

	snapshot, err := client.WaitReady(context.Background(), "session-name", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tabs) != 0 || len(snapshot.Panes) != 0 {
		t.Fatalf("WaitReady() = %#v, want empty snapshot", snapshot)
	}
}

func TestClientWaitReadyReturnsInitialLayout(t *testing.T) {
	configureHelper(t, "")
	t.Setenv(helperSequenceEnv, writeSequence(t, []string{
		`{"sessions":[{"name":"session-name","running":true,"socket_path":"/herdr/session.sock"}]}`,
		`{"id":"1","result":{"type":"session_snapshot","snapshot":{"focused_tab_id":"t1","focused_pane_id":"w1:p1","tabs":[{"tab_id":"t1","workspace_id":"w1"}],"panes":[{"pane_id":"w1:p1","tab_id":"t1","workspace_id":"w1"}]}}}`,
	}))
	client := NewClient(helperBinary(t), nil, nil, nil)
	client.watch = func(string) (fswatch.Watcher, error) { return newFakeWatcher(), nil }

	snapshot, err := client.WaitReady(context.Background(), "session-name", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tabs) != 1 || snapshot.Tabs[0].TabID != "t1" || len(snapshot.Panes) != 1 || snapshot.Panes[0].PaneID != "w1:p1" {
		t.Fatalf("WaitReady() = %#v, want initial layout", snapshot)
	}
}

func TestClientWaitReadyRetriesOnlyAfterSocketChange(t *testing.T) {
	const initialLayout = `{"id":"1","result":{"type":"session_snapshot","snapshot":{"tabs":[{"tab_id":"t1","workspace_id":"w1"}],"panes":[{"pane_id":"w1:p1","tab_id":"t1","workspace_id":"w1"}]}}}`
	configureHelper(t, "")
	t.Setenv(helperSequenceEnv, writeSequence(t, []string{
		`{"sessions":[{"name":"session-name","running":true,"socket_path":"/herdr/session.sock"}]}`,
		"",
		initialLayout,
	}))
	watcher := newFakeWatcher()
	client := NewClient(helperBinary(t), nil, nil, nil)
	client.watch = func(path string) (fswatch.Watcher, error) {
		if path != "/herdr/session.sock" {
			t.Fatalf("watched path = %q", path)
		}
		watcher.events <- struct{}{}
		return watcher, nil
	}

	snapshot, err := client.WaitReady(context.Background(), "session-name", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tabs) != 1 || len(snapshot.Panes) != 1 {
		t.Fatalf("WaitReady() = %#v, want the initial layout", snapshot)
	}
}

func TestClientWaitReadyReturnsSocketResolutionErrorsWithoutRetrying(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{name: "missing", response: `{"sessions":[]}`, want: "was not found"},
		{name: "stopped", response: `{"sessions":[{"name":"session-name"}]}`, want: "is not running"},
		{name: "missing path", response: `{"sessions":[{"name":"session-name","running":true}]}`, want: "has no socket path"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configureHelper(t, "")
			t.Setenv(helperStdoutEnv, test.response)
			_, err := NewClient(helperBinary(t), nil, nil, nil).WaitReady(context.Background(), "session-name", 30*time.Second)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("WaitReady() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestClientWaitReadyDoesNotRetryWithoutAChangeEvent(t *testing.T) {
	configureHelper(t, "")
	sequence := writeSequence(t, []string{
		`{"sessions":[{"name":"session-name","running":true,"socket_path":"/herdr/session.sock"}]}`,
		"",
		`{"id":"1","result":{"type":"session_snapshot","snapshot":{}}}`,
	})
	t.Setenv(helperSequenceEnv, sequence)
	watcher := newFakeWatcher()
	want := errors.New("watch stopped")
	watcher.errors <- want
	client := NewClient(helperBinary(t), nil, nil, nil)
	client.watch = func(string) (fswatch.Watcher, error) { return watcher, nil }

	if _, err := client.WaitReady(context.Background(), "session-name", 30*time.Second); !errors.Is(err, want) {
		t.Fatalf("WaitReady() error = %v, want %v", err, want)
	}
	remaining, err := os.ReadDir(sequence)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining scripted responses = %d, want one unconsumed snapshot", len(remaining))
	}
}

func TestClientWaitReadyReportsWatchChannelClosure(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*fakeWatcher)
	}{
		{name: "events channel closed", setup: func(w *fakeWatcher) { close(w.events) }},
		{name: "errors channel closed", setup: func(w *fakeWatcher) { close(w.errors) }},
		{name: "nil error delivered", setup: func(w *fakeWatcher) { w.errors <- nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configureHelper(t, "")
			// The first snapshot fails (empty response) so WaitReady enters its
			// watch loop, where the closed/nil channel is the only signal.
			t.Setenv(helperSequenceEnv, writeSequence(t, []string{
				`{"sessions":[{"name":"session-name","running":true,"socket_path":"/herdr/session.sock"}]}`,
				"",
			}))
			watcher := newFakeWatcher()
			test.setup(watcher)
			client := NewClient(helperBinary(t), nil, nil, nil)
			client.watch = func(string) (fswatch.Watcher, error) { return watcher, nil }

			_, err := client.WaitReady(context.Background(), "session-name", 30*time.Second)
			if err == nil || !strings.Contains(err.Error(), "filesystem watch ended") {
				t.Fatalf("WaitReady() error = %v, want 'filesystem watch ended'", err)
			}
		})
	}
}

type fakeWatcher struct {
	events chan struct{}
	errors chan error
}

func newFakeWatcher() *fakeWatcher {
	return &fakeWatcher{events: make(chan struct{}, 1), errors: make(chan error, 1)}
}

func (w *fakeWatcher) Events() <-chan struct{} { return w.events }
func (w *fakeWatcher) Errors() <-chan error    { return w.errors }
func (w *fakeWatcher) Close() error            { return nil }

func TestClientLayoutAndAgentCommands(t *testing.T) {
	tests := []struct {
		name      string
		stdout    string
		operation func(*Client) error
		wantArgs  []string
	}{
		{
			name: "rename tab",
			operation: func(client *Client) error {
				return client.RenameTab(context.Background(), "s", "t1", "orchestrator")
			},
			wantArgs: []string{"--session", "s", "tab", "rename", "t1", "orchestrator"},
		},
		{
			name: "rename pane",
			operation: func(client *Client) error {
				return client.RenamePane(context.Background(), "s", "w1:p1", "worker")
			},
			wantArgs: []string{"--session", "s", "pane", "rename", "w1:p1", "worker"},
		},
		{
			name: "close tab",
			operation: func(client *Client) error {
				return client.CloseTab(context.Background(), "s", "t2")
			},
			wantArgs: []string{"--session", "s", "tab", "close", "t2"},
		},
		{
			name: "close pane",
			operation: func(client *Client) error {
				return client.ClosePane(context.Background(), "s", "w1:p2")
			},
			wantArgs: []string{"--session", "s", "pane", "close", "w1:p2"},
		},
		{
			name: "focus agent",
			operation: func(client *Client) error {
				return client.FocusAgent(context.Background(), "s", "worker")
			},
			wantArgs: []string{"--session", "s", "agent", "focus", "worker"},
		},
		{
			name: "start agent",
			operation: func(client *Client) error {
				return client.StartAgent(context.Background(), "s", "worker", "codex", "w1:p2", 45*time.Second, []string{"--model", "gpt-custom", "--foo"})
			},
			wantArgs: []string{"--session", "s", "agent", "start", "worker", "--kind", "codex", "--pane", "w1:p2", "--timeout", "45000", "--", "--model", "gpt-custom", "--foo"},
		},
		{
			name: "start agent without native args omits the trailing dash",
			operation: func(client *Client) error {
				return client.StartAgent(context.Background(), "s", "worker", "codex", "w1:p2", 45*time.Second, nil)
			},
			wantArgs: []string{"--session", "s", "agent", "start", "worker", "--kind", "codex", "--pane", "w1:p2", "--timeout", "45000"},
		},
		{
			name: "prompt agent",
			operation: func(client *Client) error {
				return client.PromptAgent(context.Background(), "s", "worker", "Review this")
			},
			wantArgs: []string{"--session", "s", "agent", "prompt", "worker", "Review this"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capture := filepath.Join(t.TempDir(), "invocation.json")
			configureHelper(t, capture)
			t.Setenv(helperStdoutEnv, test.stdout)
			if err := test.operation(NewClient(helperBinary(t), nil, nil, nil)); err != nil {
				t.Fatal(err)
			}
			assertStrings(t, "args", readInvocation(t, capture).Args, test.wantArgs)
		})
	}
}

func TestClientCreationResponses(t *testing.T) {
	t.Run("create workspace", func(t *testing.T) {
		capture := filepath.Join(t.TempDir(), "invocation.json")
		configureHelper(t, capture)
		t.Setenv(helperStdoutEnv, `{"id":"1","result":{"type":"workspace_created","workspace":{"workspace_id":"w1"},"tab":{"tab_id":"w1:t1","workspace_id":"w1"},"root_pane":{"pane_id":"w1:p1","tab_id":"w1:t1","workspace_id":"w1"}}}`)
		workspace, tab, pane, err := NewClient(helperBinary(t), nil, nil, nil).CreateWorkspace(context.Background(), "s", "/project", "orchestrator")
		if err != nil || workspace.WorkspaceID != "w1" || tab.TabID != "w1:t1" || pane.PaneID != "w1:p1" {
			t.Fatalf("CreateWorkspace() = %#v, %#v, %#v, %v", workspace, tab, pane, err)
		}
		want := []string{"--session", "s", "workspace", "create", "--cwd", "/project", "--label", "orchestrator", "--no-focus"}
		assertStrings(t, "args", readInvocation(t, capture).Args, want)
	})

	t.Run("split pane", func(t *testing.T) {
		capture := filepath.Join(t.TempDir(), "invocation.json")
		configureHelper(t, capture)
		t.Setenv(helperStdoutEnv, `{"id":"1","result":{"type":"pane_created","pane":{"pane_id":"w1:p2","tab_id":"t1","workspace_id":"w1"}}}`)
		pane, err := NewClient(helperBinary(t), nil, nil, nil).SplitPane(context.Background(), "s", "w1:p1", "/project", map[string]string{"Z_VAR": "last", "A_VAR": "first"})
		if err != nil || pane.PaneID != "w1:p2" {
			t.Fatalf("SplitPane() = %#v, %v", pane, err)
		}
		want := []string{"--session", "s", "pane", "split", "w1:p1", "--direction", "right", "--ratio", "0.5", "--cwd", "/project", "--env", "A_VAR=first", "--env", "Z_VAR=last", "--no-focus"}
		assertStrings(t, "args", readInvocation(t, capture).Args, want)
	})

	t.Run("create tab", func(t *testing.T) {
		capture := filepath.Join(t.TempDir(), "invocation.json")
		configureHelper(t, capture)
		t.Setenv(helperStdoutEnv, `{"id":"1","result":{"type":"tab_created","tab":{"tab_id":"t2","workspace_id":"w1","label":"worker"},"root_pane":{"pane_id":"w1:p2","tab_id":"t2","workspace_id":"w1"}}}`)
		tab, pane, err := NewClient(helperBinary(t), nil, nil, nil).CreateTab(context.Background(), "s", "w1", "/project", "worker", map[string]string{"OPENCODE_CONFIG_CONTENT": "original"})
		if err != nil || tab.TabID != "t2" || pane.PaneID != "w1:p2" {
			t.Fatalf("CreateTab() = %#v, %#v, %v", tab, pane, err)
		}
		want := []string{"--session", "s", "tab", "create", "--workspace", "w1", "--cwd", "/project", "--label", "worker", "--env", "OPENCODE_CONFIG_CONTENT=original", "--no-focus"}
		assertStrings(t, "args", readInvocation(t, capture).Args, want)
	})
}

func TestClientRequiresResponsePayloadsAndIDs(t *testing.T) {
	tests := []struct {
		name      string
		stdout    string
		operation func(*Client) error
		want      string
	}{
		{
			name: "empty snapshot", operation: func(client *Client) error {
				_, err := client.Snapshot(context.Background(), "s")
				return err
			}, want: "empty response",
		},
		{
			name: "snapshot missing type", stdout: `{}`, operation: func(client *Client) error {
				_, err := client.Snapshot(context.Background(), "s")
				return err
			}, want: "unexpected response type",
		},
		{
			name: "workspace missing IDs", stdout: `{}`, operation: func(client *Client) error {
				_, _, _, err := client.CreateWorkspace(context.Background(), "s", "/project", "orchestrator")
				return err
			}, want: "missing workspace_id",
		},
		{
			name: "split missing pane", stdout: `{}`, operation: func(client *Client) error {
				_, err := client.SplitPane(context.Background(), "s", "p1", "/project", nil)
				return err
			}, want: "missing pane_id",
		},
		{
			name: "tab missing IDs", stdout: `{}`, operation: func(client *Client) error {
				_, _, err := client.CreateTab(context.Background(), "s", "w1", "/project", "worker", nil)
				return err
			}, want: "missing tab_id",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configureHelper(t, "")
			t.Setenv(helperStdoutEnv, test.stdout)
			err := test.operation(NewClient(helperBinary(t), nil, nil, nil))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("operation error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestClientStartServerDetached(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(t.TempDir(), "invocation.json")
	configureHelper(t, capture)
	client := NewClient(helperBinary(t), nil, nil, nil)
	t.Setenv("FLEDGE_REPLACED_ENV", "old")
	if err := client.StartServer("session-name", dir, map[string]string{"FLEDGE_REPLACED_ENV": "new"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(capture); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("detached helper did not record its invocation")
		}
		time.Sleep(5 * time.Millisecond)
	}
	invocation := readInvocation(t, capture)
	assertStrings(t, "args", invocation.Args, []string{"--session", "session-name", "server"})
	if invocation.Dir != dir {
		t.Errorf("dir = %q, want %q", invocation.Dir, dir)
	}
	if invocation.Env != "new" {
		t.Errorf("replaced environment = %q, want new", invocation.Env)
	}
}

func TestEnvironmentConstructionIsDeterministic(t *testing.T) {
	gotArgs := appendEnvironmentArgs([]string{"command"}, map[string]string{"Z": "last", "A": "first"})
	wantArgs := []string{"command", "--env", "A=first", "--env", "Z=last"}
	assertStrings(t, "args", gotArgs, wantArgs)

	gotEnvironment := replaceEnvironment(
		[]string{"KEEP=value", "Z=old", "A=old", "A=duplicate"},
		map[string]string{"Z": "last", "A": "first"},
	)
	wantEnvironment := []string{"KEEP=value", "A=first", "Z=last"}
	assertStrings(t, "environment", gotEnvironment, wantEnvironment)
}

func TestClientListCommandErrors(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   string
	}{
		{name: "empty stderr", want: "list Herdr sessions: exit status 19"},
		{name: "whitespace stderr", stderr: " \n\t ", want: "list Herdr sessions: exit status 19"},
		{name: "padded stderr", stderr: " \n  herdr exploded \t\n", want: "list Herdr sessions: exit status 19: herdr exploded"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configureHelper(t, "")
			t.Setenv(helperStderrEnv, test.stderr)
			t.Setenv(helperExitEnv, "19")

			_, err := NewClient(helperBinary(t), nil, nil, nil).List(context.Background())
			if err == nil || err.Error() != test.want {
				t.Fatalf("List() error = %q, want %q", err, test.want)
			}
		})
	}
}

func TestClientDecodesStructuredCommandErrors(t *testing.T) {
	configureHelper(t, "")
	t.Setenv(helperStderrEnv, `{"id":"req-7","error":{"code":"agent_pane_busy","message":"pane shell still owns foreground"}}`)
	t.Setenv(helperExitEnv, "19")

	err := NewClient(helperBinary(t), nil, nil, nil).StartAgent(
		context.Background(), "s", "worker", "codex", "w1:p2", 45*time.Second, nil,
	)
	if err == nil {
		t.Fatal("StartAgent() error = nil")
	}
	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("StartAgent() error = %T %v, want *CommandError in chain", err, err)
	}
	if commandErr.Code != "agent_pane_busy" || commandErr.Message != "pane shell still owns foreground" || commandErr.RequestID != "req-7" {
		t.Fatalf("CommandError = %#v", commandErr)
	}
	if !IsErrorCode(err, "agent_pane_busy") || IsErrorCode(err, "agent_name_taken") {
		t.Fatalf("structured classification was not exact: %v", err)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 19 {
		t.Fatalf("underlying exit error = %#v, want exit code 19", exitErr)
	}
	want := `start Herdr agent "worker": exit status 19: agent_pane_busy: pane shell still owns foreground (request req-7)`
	if err.Error() != want {
		t.Fatalf("StartAgent() error = %q, want %q", err, want)
	}
}

func TestIsErrorCodeSearchesJoinedErrorsWithExactEquality(t *testing.T) {
	busyErr := &CommandError{Code: "agent_pane_busy", Message: "busy", RequestID: "req-busy"}
	fatalErr := &CommandError{Code: "input_failed", Message: "fatal", RequestID: "req-fatal"}
	for _, err := range []error{errors.Join(busyErr, fatalErr), errors.Join(fatalErr, busyErr)} {
		if !IsErrorCode(err, "agent_pane_busy") || !IsErrorCode(err, "input_failed") {
			t.Fatalf("IsErrorCode() did not search every joined branch: %v", err)
		}
		if IsErrorCode(err, "Agent_Pane_Busy") || IsErrorCode(err, "agent_pane") {
			t.Fatalf("IsErrorCode() used non-exact matching: %v", err)
		}
	}
}

func TestClientCommandErrorClassificationRejectsAmbiguousStderr(t *testing.T) {
	tests := []struct {
		name       string
		stderr     string
		classified string
	}{
		{name: "plain text", stderr: "agent_pane_busy: try again"},
		{name: "malformed JSON", stderr: `{"id":"req-1","error":{"code":"agent_pane_busy","message":"try again"}`},
		{name: "trailing output", stderr: `{"id":"req-1","error":{"code":"agent_pane_busy","message":"try again"}} warning`},
		{name: "trailing JSON", stderr: `{"id":"req-1","error":{"code":"agent_pane_busy","message":"try again"}} {}`},
		{name: "missing request id", stderr: `{"error":{"code":"agent_pane_busy","message":"try again"}}`},
		{name: "null request id", stderr: `{"id":null,"error":{"code":"agent_pane_busy","message":"try again"}}`},
		{name: "empty request id", stderr: `{"id":"","error":{"code":"agent_pane_busy","message":"try again"}}`},
		{name: "blank request id", stderr: `{"id":" \t","error":{"code":"agent_pane_busy","message":"try again"}}`},
		{name: "wrong type request id", stderr: `{"id":7,"error":{"code":"agent_pane_busy","message":"try again"}}`},
		{name: "missing error", stderr: `{"id":"req-1"}`},
		{name: "null error", stderr: `{"id":"req-1","error":null}`},
		{name: "wrong type error", stderr: `{"id":"req-1","error":"agent_pane_busy"}`},
		{name: "missing code", stderr: `{"id":"req-1","error":{"message":"try again"}}`},
		{name: "null code", stderr: `{"id":"req-1","error":{"code":null,"message":"try again"}}`},
		{name: "empty code", stderr: `{"id":"req-1","error":{"code":"","message":"try again"}}`},
		{name: "blank code", stderr: `{"id":"req-1","error":{"code":" \t","message":"try again"}}`},
		{name: "wrong type code", stderr: `{"id":"req-1","error":{"code":7,"message":"try again"}}`},
		{name: "missing message", stderr: `{"id":"req-1","error":{"code":"agent_pane_busy"}}`},
		{name: "null message", stderr: `{"id":"req-1","error":{"code":"agent_pane_busy","message":null}}`},
		{name: "empty message", stderr: `{"id":"req-1","error":{"code":"agent_pane_busy","message":""}}`},
		{name: "blank message", stderr: `{"id":"req-1","error":{"code":"agent_pane_busy","message":" \t"}}`},
		{name: "wrong type message", stderr: `{"id":"req-1","error":{"code":"agent_pane_busy","message":7}}`},
		{name: "unrelated JSON", stderr: `{"code":"agent_pane_busy","message":"try again"}`},
		{name: "other structured code", stderr: `{"id":"req-2","error":{"code":"agent_name_taken","message":"worker exists"}}`, classified: "agent_name_taken"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configureHelper(t, "")
			t.Setenv(helperStderrEnv, test.stderr)
			t.Setenv(helperExitEnv, "1")

			_, err := NewClient(helperBinary(t), nil, nil, nil).List(context.Background())
			if err == nil {
				t.Fatal("List() error = nil")
			}
			if IsErrorCode(err, "agent_pane_busy") {
				t.Fatalf("List() error ambiguously classified as busy: %v", err)
			}
			var commandErr *CommandError
			if test.classified == "" {
				if errors.As(err, &commandErr) {
					t.Fatalf("List() error = %#v, want generic command failure", commandErr)
				}
				if !IsUnstructuredCommandError(err) {
					t.Fatalf("List() error = %T %v, want unstructured command failure", err, err)
				}
				return
			}
			if IsUnstructuredCommandError(err) {
				t.Fatalf("List() error = %T %v, want structured command failure", err, err)
			}
			if !errors.As(err, &commandErr) || !IsErrorCode(err, test.classified) {
				t.Fatalf("List() error = %T %v, want structured code %q", err, err, test.classified)
			}
		})
	}
}

func TestClientWaitPaneOutput(t *testing.T) {
	t.Run("exact command", func(t *testing.T) {
		capture := filepath.Join(t.TempDir(), "invocation.json")
		configureHelper(t, capture)
		client := NewClient(helperBinary(t), nil, nil, nil)

		if err := client.WaitPaneOutput(context.Background(), "session-name", "w1:p2", 5*time.Second); err != nil {
			t.Fatalf("WaitPaneOutput() error = %v", err)
		}
		want := []string{
			"--session", "session-name", "pane", "wait-output", "w1:p2",
			"--source", "recent", "--regex", ".", "--raw", "--timeout", "5000",
		}
		assertStrings(t, "args", readInvocation(t, capture).Args, want)
	})

	t.Run("caller cancellation", func(t *testing.T) {
		configureHelper(t, "")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := NewClient(helperBinary(t), nil, nil, nil).WaitPaneOutput(ctx, "session-name", "w1:p2", 5*time.Second)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WaitPaneOutput() error = %v, want context.Canceled", err)
		}
	})
}

type triggeredContext struct {
	context.Context
	done  chan struct{}
	cause error
}

func newTriggeredContext(cause error) (*triggeredContext, func()) {
	ctx := &triggeredContext{Context: context.Background(), done: make(chan struct{}), cause: cause}
	return ctx, func() { close(ctx.done) }
}

func (c *triggeredContext) Done() <-chan struct{} { return c.done }

func (c *triggeredContext) Err() error {
	select {
	case <-c.done:
		return c.cause
	default:
		return nil
	}
}

func TestClientWaitPaneOutputPreservesInFlightContextCauses(t *testing.T) {
	tests := []struct {
		name           string
		cause          error
		stderr         string
		structuredCode string
		unstructured   bool
		termination    bool
	}{
		{name: "cancellation with process failure", cause: context.Canceled, termination: true},
		{name: "deadline with process failure", cause: context.DeadlineExceeded, termination: true},
		{
			name:         "deadline with generic failure",
			cause:        context.DeadlineExceeded,
			stderr:       "pane output unavailable",
			unstructured: true,
		},
		{
			name:           "deadline with structured failure",
			cause:          context.DeadlineExceeded,
			stderr:         `{"id":"req-deadline","error":{"code":"input_failed","message":"pane disappeared"}}`,
			structuredCode: "input_failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capture := filepath.Join(t.TempDir(), "invocation.json")
			ready := filepath.Join(t.TempDir(), "ready")
			configureHelper(t, capture)
			t.Setenv(helperBlockEnv, "1")
			t.Setenv(helperReadyEnv, ready)
			t.Setenv(helperStderrEnv, test.stderr)
			ctx, trigger := newTriggeredContext(test.cause)
			binary := helperBinary(t)
			result := make(chan error, 1)
			go func() {
				result <- NewClient(binary, nil, nil, nil).WaitPaneOutput(ctx, "session-name", "w1:p2", 5*time.Second)
			}()

			deadline := time.Now().Add(10 * time.Second)
			for {
				if _, err := os.Stat(ready); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("blocking helper did not start")
				}
				time.Sleep(time.Millisecond)
			}
			trigger()

			var err error
			select {
			case err = <-result:
			case <-time.After(10 * time.Second):
				t.Fatal("WaitPaneOutput() did not return after context completion")
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("WaitPaneOutput() error = %v, want context cause %v", err, test.cause)
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("WaitPaneOutput() error = %T %v, want *exec.ExitError in chain", err, err)
			}
			if test.termination && (exitErr.ProcessState == nil || exitErr.ProcessState.Exited()) {
				t.Fatalf("context-killed process state = %#v, want signal termination with Exited false", exitErr.ProcessState)
			}
			var terminationErr *commandContextTerminationError
			if got := errors.As(err, &terminationErr); got != test.termination {
				t.Fatalf("command cancellation marker present = %v, want %v: %v", got, test.termination, err)
			}
			if test.termination {
				want := fmt.Sprintf("wait for Herdr pane %q output: %s\n%s", "w1:p2", exitErr.Error(), test.cause.Error())
				if err.Error() != want {
					t.Fatalf("WaitPaneOutput() error = %q, want %q", err, want)
				}
			}
			if got := IsUnstructuredCommandError(err); got != test.unstructured {
				t.Fatalf("IsUnstructuredCommandError(%v) = %v, want %v", err, got, test.unstructured)
			}
			var commandErr *CommandError
			if test.structuredCode == "" {
				if errors.As(err, &commandErr) {
					t.Fatalf("WaitPaneOutput() error = %#v, want no structured error", commandErr)
				}
				return
			}
			if !errors.As(err, &commandErr) || !IsErrorCode(err, test.structuredCode) {
				t.Fatalf("WaitPaneOutput() error = %T %v, want structured code %q", err, err, test.structuredCode)
			}
			if commandErr.RequestID != "req-deadline" {
				t.Fatalf("CommandError request id = %q, want req-deadline", commandErr.RequestID)
			}
		})
	}
}

func TestClientDoesNotMarkProcessExitCompletedBeforeCancellation(t *testing.T) {
	configureHelper(t, "")
	t.Setenv(helperExitEnv, "23")
	ctx, trigger := newTriggeredContext(context.DeadlineExceeded)
	waitCompleted := make(chan struct{})
	classify := make(chan struct{})
	client := NewClient(helperBinary(t), nil, nil, nil)
	client.afterCommandWait = func() {
		close(waitCompleted)
		<-classify
	}
	result := make(chan error, 1)
	go func() {
		result <- client.WaitPaneOutput(ctx, "session-name", "w1:p2", 5*time.Second)
	}()

	select {
	case <-waitCompleted:
	case <-time.After(10 * time.Second):
		t.Fatal("WaitPaneOutput() did not reach post-Wait classification seam")
	}
	trigger()
	close(classify)

	var err error
	select {
	case err = <-result:
	case <-time.After(10 * time.Second):
		t.Fatal("WaitPaneOutput() did not return after classification resumed")
	}

	if err == nil {
		t.Fatal("WaitPaneOutput() error = nil")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ProcessState == nil || exitErr.ExitCode() != 23 || !exitErr.ProcessState.Exited() {
		t.Fatalf("WaitPaneOutput() error = %T %v, want completed exit status 23", err, err)
	}
	var terminationErr *commandContextTerminationError
	if errors.As(err, &terminationErr) {
		t.Fatalf("WaitPaneOutput() error = %v, must not mark an exit completed before cancellation", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitPaneOutput() error = %v, want later context deadline retained as an independent cause", err)
	}
}

func helperBinary(t *testing.T) string {
	t.Helper()

	path, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test binary: %v", err)
	}
	return path
}

func configureHelper(t *testing.T, capture string) {
	t.Helper()
	t.Setenv(helperModeEnv, "1")
	t.Setenv(helperCaptureEnv, capture)
	// Clear all behavior controls so each subprocess is determined only by its test.
	t.Setenv(helperStdoutEnv, "")
	t.Setenv(helperStderrEnv, "")
	t.Setenv(helperExitEnv, "")
	t.Setenv(helperBlockEnv, "")
	t.Setenv(helperReadyEnv, "")
	t.Setenv(helperSequenceEnv, "")
}

// writeSequence queues responses for the helper to return in order.
func writeSequence(t *testing.T, responses []string) string {
	t.Helper()

	dir := t.TempDir()
	for i, response := range responses {
		path := filepath.Join(dir, fmt.Sprintf("%02d", i))
		if err := os.WriteFile(path, []byte(response), 0o600); err != nil {
			t.Fatalf("write scripted response: %v", err)
		}
	}
	return dir
}

func readInvocation(t *testing.T, path string) helperInvocation {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read helper invocation: %v", err)
	}

	var invocation helperInvocation
	if err := json.Unmarshal(contents, &invocation); err != nil {
		t.Fatalf("decode helper invocation: %v", err)
	}
	return invocation
}

func assertStrings(t *testing.T, name string, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s = %#v, want %#v", name, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %q, want %q", name, i, got[i], want[i])
		}
	}
}
