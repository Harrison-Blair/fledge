package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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
	t.Setenv(helperStdoutEnv, `{"sessions":[{"name":"one","running":true},{"name":"two","running":false}]}`)

	client := NewClient(helperBinary(t), nil, nil, nil)
	sessions, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []Session{{Name: "one", Running: true}, {Name: "two", Running: false}}
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
	t.Setenv(helperStdoutEnv, `{"id":"1","result":{"type":"session_snapshot","snapshot":{"focused_workspace_id":"w1","focused_tab_id":"t1","focused_pane_id":"w1:p1","workspaces":[{"workspace_id":"w1","label":"project"}],"tabs":[{"tab_id":"t1","workspace_id":"w1","label":"orchestrator"}],"panes":[{"pane_id":"w1:p1","tab_id":"t1","workspace_id":"w1","label":"orchestrator"}],"agents":[{"name":"orchestrator","agent":"codex","pane_id":"w1:p1","tab_id":"t1","workspace_id":"w1","revision":664,"agent_session":{"agent":"codex","kind":"id","source":"herdr:codex","value":"019fcf2f-6113"}}]}}}`)

	snapshot, err := NewClient(helperBinary(t), nil, nil, nil).Snapshot(context.Background(), "session-name")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.FocusedPaneID != "w1:p1" || len(snapshot.Agents) != 1 || snapshot.Agents[0].Name == nil || *snapshot.Agents[0].Name != "orchestrator" {
		t.Fatalf("Snapshot() = %#v", snapshot)
	}
	agent := snapshot.Agents[0]
	if agent.Revision != 664 {
		t.Errorf("agent revision = %d, want 664", agent.Revision)
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
