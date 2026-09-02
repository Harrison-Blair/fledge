package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// recordingHerdr installs a fake herdr that records its argument vector and
// prints output. The returned function reads back the recorded arguments.
func recordingHerdr(t *testing.T, output string) func() []string {
	t.Helper()
	argvPath := filepath.Join(t.TempDir(), "argv")
	fakeHerdr(t, `
for arg in "$@"; do printf '%s\n' "$arg" >>"$HERDR_FAKE_ARGV"; done
printf '%s' "$HERDR_FAKE_OUTPUT"
`)
	t.Setenv("HERDR_FAKE_ARGV", argvPath)
	t.Setenv("HERDR_FAKE_OUTPUT", output)

	return func() []string {
		t.Helper()
		contents, err := os.ReadFile(argvPath)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSuffix(string(contents), "\n"), "\n")
	}
}

func envelope(result string) string {
	return `{"id":"cli:fake","result":` + result + `}`
}

const (
	workspaceJSON = `{"workspace_id":"w1","label":"fledge","number":1,"focused":true,"active_tab_id":"w1:t1","pane_count":2,"tab_count":1,"agent_status":"working"}`
	tabJSON       = `{"tab_id":"w1:t1","workspace_id":"w1","label":"1","number":1,"focused":true,"pane_count":2,"agent_status":"working"}`
	paneJSON      = `{"pane_id":"w1:p1","workspace_id":"w1","tab_id":"w1:t1","focused":false,"cwd":"/x","agent":"claude","agent_status":"working","revision":4}`
	agentJSON     = `{"name":"reviewer","agent":"claude","agent_status":"working","workspace_id":"w1","tab_id":"w1:t1","pane_id":"w1:p1","revision":4}`
)

var (
	wantWorkspace = Workspace{ID: "w1", Label: "fledge", Number: 1, Focused: true, ActiveTabID: "w1:t1"}
	wantTab       = Tab{ID: "w1:t1", WorkspaceID: "w1", Label: "1", Number: 1, Focused: true}
	wantPane      = Pane{ID: "w1:p1", WorkspaceID: "w1", TabID: "w1:t1", CWD: "/x", AgentKind: "claude", AgentStatus: "working"}
	wantAgent     = Agent{Name: "reviewer", Kind: "claude", Status: "working", WorkspaceID: "w1", TabID: "w1:t1", PaneID: "w1:p1"}
)

func TestAPIRequests(t *testing.T) {
	ratio := 0.25
	promptResult := `{"type":"agent_prompted","agent":` + agentJSON + `}`

	tests := []struct {
		name     string
		output   string
		call     func(context.Context, *Client) (any, error)
		wantArgv []string
		want     any
	}{
		{
			name:   "status",
			output: `{"client":{"version":"0.8.2"},"server":{"status":"running","running":true},"update":{}}` + "\n",
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Status(ctx)
			},
			wantArgv: []string{"status", "--json"},
			want:     Status{Running: true},
		},
		{
			name:   "workspaces",
			output: envelope(`{"type":"workspace_list","workspaces":[`+workspaceJSON+`]}`) + "\n",
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Workspaces(ctx)
			},
			wantArgv: []string{"workspace", "list"},
			want:     []Workspace{wantWorkspace},
		},
		{
			name:   "create workspace",
			output: envelope(`{"type":"workspace_created","workspace":` + workspaceJSON + `,"tab":` + tabJSON + `,"root_pane":` + paneJSON + `}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.CreateWorkspace(ctx, "docs ws")
			},
			wantArgv: []string{"workspace", "create", "--label", "docs ws", "--no-focus"},
			want:     WorkspaceCreated{Workspace: wantWorkspace, Tab: wantTab, RootPane: wantPane},
		},
		{
			name:   "rename workspace",
			output: envelope(`{"type":"workspace_info","workspace":` + workspaceJSON + `}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.RenameWorkspace(ctx, "w1", "new label")
			},
			wantArgv: []string{"workspace", "rename", "w1", "new label"},
		},
		{
			name:   "tabs",
			output: envelope(`{"type":"tab_list","tabs":[` + tabJSON + `]}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Tabs(ctx, "w1")
			},
			wantArgv: []string{"tab", "list", "--workspace", "w1"},
			want:     []Tab{wantTab},
		},
		{
			name:   "create tab",
			output: envelope(`{"type":"tab_created","tab":` + tabJSON + `,"root_pane":` + paneJSON + `}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.CreateTab(ctx, "w1", "second")
			},
			wantArgv: []string{"tab", "create", "--workspace", "w1", "--label", "second", "--no-focus"},
			want:     TabCreated{Tab: wantTab, RootPane: wantPane},
		},
		{
			name:   "rename tab",
			output: envelope(`{"type":"tab_info","tab":` + tabJSON + `}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.RenameTab(ctx, "w1:t1", "renamed")
			},
			wantArgv: []string{"tab", "rename", "w1:t1", "renamed"},
		},
		{
			name:   "tabs everywhere",
			output: envelope(`{"type":"tab_list","tabs":[` + tabJSON + `]}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Tabs(ctx, "")
			},
			wantArgv: []string{"tab", "list"},
			want:     []Tab{wantTab},
		},
		{
			name:   "panes in workspace",
			output: envelope(`{"type":"pane_list","panes":[` + paneJSON + `]}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Panes(ctx, "w1")
			},
			wantArgv: []string{"pane", "list", "--workspace", "w1"},
			want:     []Pane{wantPane},
		},
		{
			name:   "panes everywhere",
			output: envelope(`{"type":"pane_list","panes":[` + paneJSON + `]}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Panes(ctx, "")
			},
			wantArgv: []string{"pane", "list"},
			want:     []Pane{wantPane},
		},
		{
			name:   "current pane",
			output: envelope(`{"type":"pane_current","pane":` + paneJSON + `}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.CurrentPane(ctx)
			},
			wantArgv: []string{"pane", "current", "--current"},
			want:     wantPane,
		},
		{
			name:   "split pane with ratio",
			output: envelope(`{"type":"pane_info","pane":` + paneJSON + `}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.SplitPane(ctx, SplitOptions{PaneID: "w1:p1", Direction: "right", Ratio: &ratio})
			},
			wantArgv: []string{"pane", "split", "--pane", "w1:p1", "--direction", "right", "--ratio", "0.25", "--no-focus"},
			want:     wantPane,
		},
		{
			name:   "split pane without ratio",
			output: envelope(`{"type":"pane_info","pane":` + paneJSON + `}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.SplitPane(ctx, SplitOptions{PaneID: "w1:p1", Direction: "down"})
			},
			wantArgv: []string{"pane", "split", "--pane", "w1:p1", "--direction", "down", "--no-focus"},
			want:     wantPane,
		},
		{
			name:   "close pane",
			output: envelope(`{"type":"ok"}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.ClosePane(ctx, "w1:p2")
			},
			wantArgv: []string{"pane", "close", "w1:p2"},
		},
		{
			name:   "process info",
			output: envelope(`{"type":"pane_process_info","process_info":{"pane_id":"w1:p1","shell_pid":129736,"tty":"/dev/pts/4","foreground_process_group_id":130012,"foreground_processes":[{"pid":130012,"name":"claude","argv":["claude","--secret"],"cmdline":"claude --secret"}]}}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.ProcessInfo(ctx, "w1:p1")
			},
			wantArgv: []string{"pane", "process-info", "--pane", "w1:p1"},
			want:     ProcessInfo{PaneID: "w1:p1", ShellPID: ptr[uint32](129736), TTY: ptr("/dev/pts/4"), ForegroundPGID: ptr[uint32](130012)},
		},
		{
			name:   "process info with nulls",
			output: envelope(`{"type":"pane_process_info","process_info":{"pane_id":"w1:p1","shell_pid":null,"tty":null,"foreground_process_group_id":null,"foreground_processes":[]}}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.ProcessInfo(ctx, "w1:p1")
			},
			wantArgv: []string{"pane", "process-info", "--pane", "w1:p1"},
			want:     ProcessInfo{PaneID: "w1:p1"},
		},
		{
			name:   "close workspace",
			output: envelope(`{"type":"ok"}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.CloseWorkspace(ctx, "w2")
			},
			wantArgv: []string{"workspace", "close", "w2"},
		},
		{
			name:   "prompt agent waiting",
			output: envelope(promptResult),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.PromptAgent(ctx, PromptOptions{
					Target:    "reviewer",
					Text:      "review the diff",
					Wait:      true,
					Until:     []string{"idle", "done"},
					TimeoutMS: 120000,
				})
			},
			wantArgv: []string{"agent", "prompt", "reviewer", "review the diff", "--wait", "--until", "idle", "--until", "done", "--timeout", "120000"},
			want:     json.RawMessage(promptResult),
		},
		{
			name:   "prompt agent without waiting",
			output: envelope(promptResult),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.PromptAgent(ctx, PromptOptions{Target: "w1:p1", Text: "hello"})
			},
			wantArgv: []string{"agent", "prompt", "w1:p1", "hello"},
			want:     json.RawMessage(promptResult),
		},
		{
			name:   "agents",
			output: envelope(`{"type":"agent_list","agents":[` + agentJSON + `]}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Agents(ctx)
			},
			wantArgv: []string{"agent", "list"},
			want:     []Agent{wantAgent},
		},
		{
			name:   "get agent",
			output: envelope(`{"type":"agent_info","agent":` + agentJSON + `}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.GetAgent(ctx, "reviewer")
			},
			wantArgv: []string{"agent", "get", "reviewer"},
			want:     wantAgent,
		},
		{
			name:   "invoke",
			output: envelope(`{"type":"pane_read","read":{"text":"hi"}}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Invoke(ctx, "pane", "read", "w1:p1", "--lines", "5")
			},
			wantArgv: []string{"pane", "read", "w1:p1", "--lines", "5"},
			want:     json.RawMessage(`{"type":"pane_read","read":{"text":"hi"}}`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			argv := recordingHerdr(t, tc.output)

			got, err := tc.call(context.Background(), New(nil, nil, nil))
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if recorded := argv(); !reflect.DeepEqual(recorded, tc.wantArgv) {
				t.Errorf("argv = %q, want %q", recorded, tc.wantArgv)
			}
			if tc.want != nil && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("result = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

func TestProcessInfoRejectsMalformedResult(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "wrong type", output: envelope(`{"type":"ok"}`), want: `result type "ok", want "pane_process_info"`},
		{name: "other pane", output: envelope(`{"type":"pane_process_info","process_info":{"pane_id":"w1:p2","shell_pid":1}}`), want: "result describes pane w1:p2, want w1:p1"},
		{name: "missing pane", output: envelope(`{"type":"pane_process_info","process_info":{"shell_pid":1}}`), want: `result describes pane "", want w1:p1`},
		{name: "negative shell pid", output: envelope(`{"type":"pane_process_info","process_info":{"pane_id":"w1:p1","shell_pid":-1}}`), want: "decode result"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recordingHerdr(t, tc.output)
			_, err := New(nil, nil, nil).ProcessInfo(context.Background(), "w1:p1")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ProcessInfo error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestWithSessionPrefixesArguments(t *testing.T) {
	tests := []struct {
		name     string
		session  string
		wantArgv []string
	}{
		{name: "named session", session: "fledge-demo", wantArgv: []string{"--session", "fledge-demo", "agent", "list"}},
		{name: "ambient session", session: "", wantArgv: []string{"agent", "list"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			argv := recordingHerdr(t, envelope(`{"type":"agent_list","agents":[]}`))

			if _, err := New(nil, nil, nil).WithSession(tc.session).Agents(context.Background()); err != nil {
				t.Fatalf("Agents: %v", err)
			}
			if recorded := argv(); !reflect.DeepEqual(recorded, tc.wantArgv) {
				t.Fatalf("argv = %q, want %q", recorded, tc.wantArgv)
			}
		})
	}
}

func TestWithSessionLeavesReceiverUnchanged(t *testing.T) {
	client := New(nil, nil, nil)
	if session := client.WithSession("fledge-demo"); session == client || session.session != "fledge-demo" {
		t.Fatalf("WithSession returned %#v", session)
	}
	if client.session != "" {
		t.Fatalf("receiver session = %q, want empty", client.session)
	}
}

func TestAgentNameDefaultsToEmpty(t *testing.T) {
	tests := []struct {
		name   string
		agents string
	}{
		{name: "null name", agents: `[{"name":null,"agent":"claude","agent_status":"idle","workspace_id":"w1","tab_id":"w1:t1","pane_id":"w1:p1"}]`},
		{name: "absent name", agents: `[{"agent":"claude","agent_status":"idle","workspace_id":"w1","tab_id":"w1:t1","pane_id":"w1:p1"}]`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recordingHerdr(t, envelope(`{"type":"agent_list","agents":`+tc.agents+`}`))

			got, err := New(nil, nil, nil).Agents(context.Background())
			if err != nil {
				t.Fatalf("Agents: %v", err)
			}
			want := []Agent{{Kind: "claude", Status: "idle", WorkspaceID: "w1", TabID: "w1:t1", PaneID: "w1:p1"}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Agents = %#v, want %#v", got, want)
			}
		})
	}
}

func TestAPIRejectsInvalidOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "invalid JSON", output: `{`, want: "decode output"},
		{name: "trailing JSON", output: envelope(`{"type":"agent_list","agents":[]}`) + ` {}`, want: "trailing JSON"},
		{name: "missing result", output: `{"id":"cli:fake"}`, want: "missing result object"},
		{name: "null result", output: `{"id":"cli:fake","result":null}`, want: "missing result object"},
		{name: "wrong type", output: envelope(`{"type":"pane_list","agents":[]}`), want: `result type "pane_list", want "agent_list"`},
		{name: "missing type", output: envelope(`{"agents":[]}`), want: `result type "", want "agent_list"`},
		{name: "wrong agents type", output: envelope(`{"type":"agent_list","agents":{}}`), want: "decode result"},
		{name: "missing pane id", output: envelope(`{"type":"agent_list","agents":[{"workspace_id":"w1","tab_id":"w1:t1"}]}`), want: "missing pane_id"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recordingHerdr(t, tc.output)

			_, err := New(nil, nil, nil).Agents(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Agents error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestAPIRejectsMissingIdentifiers(t *testing.T) {
	tests := []struct {
		name   string
		output string
		call   func(context.Context, *Client) error
		want   string
	}{
		{
			name:   "workspace without workspace_id",
			output: envelope(`{"type":"workspace_list","workspaces":[{"label":"fledge","number":1}]}`),
			call: func(ctx context.Context, c *Client) error {
				_, err := c.Workspaces(ctx)
				return err
			},
			want: "missing workspace_id",
		},
		{
			name:   "tab without workspace_id",
			output: envelope(`{"type":"tab_list","tabs":[{"tab_id":"w1:t1","label":"1"}]}`),
			call: func(ctx context.Context, c *Client) error {
				_, err := c.Tabs(ctx, "w1")
				return err
			},
			want: "missing tab_id or workspace_id",
		},
		{
			name:   "pane without tab_id",
			output: envelope(`{"type":"pane_list","panes":[{"pane_id":"w1:p1","workspace_id":"w1"}]}`),
			call: func(ctx context.Context, c *Client) error {
				_, err := c.Panes(ctx, "w1")
				return err
			},
			want: "missing pane_id, workspace_id, or tab_id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recordingHerdr(t, tc.output)

			err := tc.call(context.Background(), New(nil, nil, nil))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestCurrentPaneRejectsMalformedResult(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "wrong type", output: envelope(`{"type":"pane_info","pane":` + paneJSON + `}`), want: `result type "pane_info", want "pane_current"`},
		{name: "missing pane", output: envelope(`{"type":"pane_current"}`), want: "missing pane_id"},
		{name: "malformed pane", output: envelope(`{"type":"pane_current","pane":{"pane_id":"p1"}}`), want: "missing pane_id, workspace_id, or tab_id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recordingHerdr(t, test.output)
			_, err := New(nil, nil, nil).CurrentPane(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CurrentPane() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestCurrentPaneReportsStructuredServerError(t *testing.T) {
	fakeHerdr(t, `printf '%s' '{"error":{"code":"pane_not_found","message":"current pane is gone"}}' >&2; exit 1`)
	_, err := New(nil, nil, nil).WithSession("managed").CurrentPane(context.Background())
	var reported *Error
	if !errors.As(err, &reported) {
		t.Fatalf("CurrentPane() error = %v, want *Error", err)
	}
	if reported.Operation != "herdr --session managed pane current --current" || reported.Code != "pane_not_found" {
		t.Fatalf("CurrentPane() structured error = %#v", reported)
	}
}

func TestAPIReportsServerError(t *testing.T) {
	fakeHerdr(t, `printf '%s' '{"error":{"code":"agent_pane_not_found","message":"agent target pane w1:p99 not found"},"id":"cli:agent:get"}' >&2; exit 1`)

	_, err := New(nil, nil, nil).WithSession("fledge-demo").GetAgent(context.Background(), "w1:p99")
	var reported *Error
	if !errors.As(err, &reported) {
		t.Fatalf("GetAgent error = %v, want *Error", err)
	}
	want := Error{
		Operation: "herdr --session fledge-demo agent get w1:p99",
		Code:      "agent_pane_not_found",
		Message:   "agent target pane w1:p99 not found",
	}
	if *reported != want {
		t.Fatalf("error = %#v, want %#v", *reported, want)
	}
	if got := reported.Error(); got != want.Operation+": agent_pane_not_found: agent target pane w1:p99 not found" {
		t.Fatalf("Error() = %q", got)
	}
}

func TestAPIReportsPlainCommandFailure(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{name: "usage error", script: `printf 'unknown option: --nope' >&2; exit 2`},
		{name: "error field is not an object", script: `printf '{"error":"refused"}' >&2; exit 1`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeHerdr(t, tc.script)

			_, err := New(nil, nil, nil).Agents(context.Background())
			var reported *Error
			if err == nil || errors.As(err, &reported) {
				t.Fatalf("Agents error = %v, want a plain command error", err)
			}
			if !strings.Contains(err.Error(), "herdr agent list") {
				t.Fatalf("Agents error = %v", err)
			}
		})
	}
}

func TestStatusReportsCommandFailure(t *testing.T) {
	fakeHerdr(t, `printf 'no server' >&2; exit 1`)

	_, err := New(nil, nil, nil).Status(context.Background())
	if err == nil || !strings.Contains(err.Error(), "herdr status --json") || !strings.Contains(err.Error(), "no server") {
		t.Fatalf("Status error = %v", err)
	}
}

func TestStatusRejectsInvalidOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "invalid JSON", output: `{`},
		{name: "trailing JSON", output: `{"server":{"running":true}} {}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recordingHerdr(t, tc.output)

			_, err := New(nil, nil, nil).Status(context.Background())
			if err == nil || !strings.Contains(err.Error(), "herdr status --json: decode output") {
				t.Fatalf("Status error = %v", err)
			}
		})
	}
}

func TestRunUsesInjectedBinaryOnlyForAmbientClient(t *testing.T) {
	for _, test := range []struct {
		name       string
		herdrEnv   string
		injected   bool
		named      bool
		wantBinary string
	}{
		{name: "ambient pane", herdrEnv: "1", injected: true, wantBinary: "injected"},
		{name: "outside pane", herdrEnv: "0", injected: true, wantBinary: "path"},
		{name: "named client", herdrEnv: "1", injected: true, named: true, wantBinary: "path"},
		{name: "missing injected path", herdrEnv: "1", wantBinary: "path"},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection := filepath.Join(t.TempDir(), "selection")
			output := envelope(`{"type":"agent_list","agents":[]}`)
			fakeHerdr(t, `printf 'path\n' >>"$HERDR_FAKE_SELECTION"; printf '%s' "$HERDR_FAKE_OUTPUT"`)
			injectedPath := filepath.Join(t.TempDir(), " injected herdr ")
			fakeHerdrExecutable(t, injectedPath, `printf 'injected\n' >>"$HERDR_FAKE_SELECTION"; printf '%s' "$HERDR_FAKE_OUTPUT"`)
			t.Setenv("HERDR_FAKE_SELECTION", selection)
			t.Setenv("HERDR_FAKE_OUTPUT", output)
			t.Setenv("HERDR_ENV", test.herdrEnv)
			if test.injected {
				t.Setenv("HERDR_BIN_PATH", injectedPath)
			} else {
				t.Setenv("HERDR_BIN_PATH", "")
			}

			client := New(nil, nil, nil)
			if test.named {
				client = client.WithSession("managed")
			}
			if _, err := client.Agents(context.Background()); err != nil {
				t.Fatalf("Agents: %v", err)
			}
			contents, err := os.ReadFile(selection)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(string(contents)); got != test.wantBinary {
				t.Fatalf("binary = %q, want %q", got, test.wantBinary)
			}
		})
	}
}

func TestRunDoesNotFallBackAfterInjectedBinaryFailure(t *testing.T) {
	selection := filepath.Join(t.TempDir(), "selection")
	fakeHerdr(t, `printf 'path\n' >>"$HERDR_FAKE_SELECTION"; printf '%s' '{"id":"cli:fake","result":{"type":"agent_list","agents":[]}}'`)
	t.Setenv("HERDR_FAKE_SELECTION", selection)
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_BIN_PATH", filepath.Join(t.TempDir(), "missing-herdr"))

	_, err := New(nil, nil, nil).Agents(context.Background())
	if err == nil || !strings.HasPrefix(err.Error(), "herdr agent list:") {
		t.Fatalf("Agents error = %v, want logical herdr operation", err)
	}
	if _, statErr := os.Stat(selection); !os.IsNotExist(statErr) {
		t.Fatalf("PATH binary ran after injected failure; stat error = %v", statErr)
	}
}

func TestRunRecordsCancellationCauseAtSubprocessFailure(t *testing.T) {
	started := filepath.Join(t.TempDir(), "started")
	fakeHerdr(t, `: >"$HERDR_FAKE_STARTED"
exec sleep 600`)
	t.Setenv("HERDR_FAKE_STARTED", started)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := New(nil, nil, nil).Agents(ctx)
		done <- err
	}()
	startupDeadline := time.NewTimer(2 * time.Second)
	defer startupDeadline.Stop()
	poll := time.NewTicker(time.Millisecond)
	defer poll.Stop()
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		select {
		case <-startupDeadline.C:
			t.Fatal("fake Herdr did not start")
		case <-poll.C:
		}
	}
	cancel()
	completionDeadline := time.NewTimer(2 * time.Second)
	defer completionDeadline.Stop()
	var err error
	select {
	case err = <-done:
	case <-completionDeadline.C:
		t.Fatal("cancelled fake Herdr did not exit")
	}
	if !errors.Is(ContextCause(err), context.Canceled) {
		t.Fatalf("ContextCause(%v) = %v, want context.Canceled", err, ContextCause(err))
	}
}

func TestGlobalMethodsIgnoreInjectedBinary(t *testing.T) {
	for _, test := range []struct {
		name   string
		output string
		call   func(context.Context, *Client) error
	}{
		{
			name:   "list",
			output: `{"sessions":[]}`,
			call: func(ctx context.Context, client *Client) error {
				_, err := client.List(ctx)
				return err
			},
		},
		{
			name: "launch",
			call: func(ctx context.Context, client *Client) error {
				return client.Launch(ctx, t.TempDir(), "managed")
			},
		},
		{
			name: "stop",
			call: func(ctx context.Context, client *Client) error {
				return client.Stop(ctx, "managed")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fakeHerdr(t, `printf '%s' "$HERDR_FAKE_OUTPUT"`)
			t.Setenv("HERDR_FAKE_OUTPUT", test.output)
			t.Setenv("HERDR_ENV", "1")
			t.Setenv("HERDR_BIN_PATH", filepath.Join(t.TempDir(), "missing-herdr"))

			if err := test.call(context.Background(), New(nil, nil, nil)); err != nil {
				t.Fatalf("global method used injected binary: %v", err)
			}
		})
	}
}
