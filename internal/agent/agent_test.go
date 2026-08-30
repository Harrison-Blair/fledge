package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"fledge/internal/herdr"
	"fledge/internal/project"
	"fledge/internal/session"
)

type fakeHerder struct {
	workspaces   []herdr.Workspace
	panes        []herdr.Pane
	newWorkspace herdr.WorkspaceCreated
	newTab       herdr.TabCreated
	newPane      herdr.Pane
	started      herdr.Agent
	prompted     json.RawMessage
	agents       []herdr.Agent
	found        herdr.Agent
	errs         map[string]error
	calls        []string
	startArgs    []string
	promptOpts   herdr.PromptOptions
	onStartAgent func(context.Context) error
	onClosePane  func(context.Context) error
}

type connectPaneResolver struct {
	pane herdr.Pane
	err  error
}

func (f connectPaneResolver) CurrentPane(context.Context) (herdr.Pane, error) {
	return f.pane, f.err
}

func newFakeHerder() *fakeHerder {
	return &fakeHerder{
		workspaces:   []herdr.Workspace{{ID: "wsA"}, {ID: "wsF", Focused: true}},
		newWorkspace: herdr.WorkspaceCreated{Workspace: herdr.Workspace{ID: "ws2"}, Tab: herdr.Tab{ID: "ws2:tab1", WorkspaceID: "ws2"}, RootPane: herdr.Pane{ID: "ws2:tab1:pane1", WorkspaceID: "ws2", TabID: "ws2:tab1"}},
		newTab:       herdr.TabCreated{Tab: herdr.Tab{ID: "ws1:tab9", WorkspaceID: "ws1"}, RootPane: herdr.Pane{ID: "ws1:tab9:pane1", WorkspaceID: "ws1", TabID: "ws1:tab9"}},
		newPane:      herdr.Pane{ID: "ws1:tab3:pane7", WorkspaceID: "ws1", TabID: "ws1:tab3"},
		errs:         map[string]error{},
	}
}

func (f *fakeHerder) record(call string) { f.calls = append(f.calls, call) }

func (f *fakeHerder) Workspaces(context.Context) ([]herdr.Workspace, error) {
	f.record("Workspaces()")
	return f.workspaces, f.errs["Workspaces"]
}

func (f *fakeHerder) CreateWorkspace(_ context.Context, label string) (herdr.WorkspaceCreated, error) {
	f.record(fmt.Sprintf("CreateWorkspace(%s)", label))
	return f.newWorkspace, f.errs["CreateWorkspace"]
}

func (f *fakeHerder) CreateTab(_ context.Context, workspaceID, label string) (herdr.TabCreated, error) {
	f.record(fmt.Sprintf("CreateTab(%s,%s)", workspaceID, label))
	return f.newTab, f.errs["CreateTab"]
}

func (f *fakeHerder) Panes(_ context.Context, workspaceID string) ([]herdr.Pane, error) {
	f.record(fmt.Sprintf("Panes(%s)", workspaceID))
	return f.panes, f.errs["Panes"]
}

func (f *fakeHerder) SplitPane(_ context.Context, options herdr.SplitOptions) (herdr.Pane, error) {
	ratio := "-"
	if options.Ratio != nil {
		ratio = strconv.FormatFloat(*options.Ratio, 'f', -1, 64)
	}
	f.record(fmt.Sprintf("SplitPane(%s,%s,%s)", options.PaneID, options.Direction, ratio))
	return f.newPane, f.errs["SplitPane"]
}

func (f *fakeHerder) ClosePane(ctx context.Context, id string) error {
	f.record(fmt.Sprintf("ClosePane(%s)", id))
	if f.onClosePane != nil {
		return f.onClosePane(ctx)
	}
	return f.errs["ClosePane"]
}

func (f *fakeHerder) StartAgent(ctx context.Context, options herdr.StartAgentOptions) (herdr.Agent, error) {
	f.record(fmt.Sprintf("StartAgent(%s,%s,%s)", options.Name, options.Kind, options.PaneID))
	f.startArgs = options.Args
	if f.onStartAgent != nil {
		return f.started, f.onStartAgent(ctx)
	}
	return f.started, f.errs["StartAgent"]
}

func (f *fakeHerder) PromptAgent(_ context.Context, options herdr.PromptOptions) (json.RawMessage, error) {
	f.record(fmt.Sprintf("PromptAgent(%s)", options.Target))
	f.promptOpts = options
	return f.prompted, f.errs["PromptAgent"]
}

func (f *fakeHerder) Agents(context.Context) ([]herdr.Agent, error) {
	f.record("Agents()")
	return f.agents, f.errs["Agents"]
}

func (f *fakeHerder) GetAgent(_ context.Context, target string) (herdr.Agent, error) {
	f.record(fmt.Sprintf("GetAgent(%s)", target))
	return f.found, f.errs["GetAgent"]
}

func TestConnectInsideHerderPaneValidatesProjectSession(t *testing.T) {
	root := agentProject(t, "fledge-demo-00000001")
	env := map[string]string{
		"HERDR_ENV":     "1",
		"HERDR_SESSION": "fledge-demo-00000001",
		"HERDR_PANE_ID": "wsE:tab1:pane4",
	}
	caller, client, err := Connect(context.Background(), root, func(name string) string { return env[name] }, func(context.Context) ([]herdr.Session, error) {
		return []herdr.Session{{Name: "fledge-demo-00000001", Running: true}}, nil
	}, func(name string) session.PaneResolver {
		if name != "fledge-demo-00000001" {
			t.Fatalf("scoped session = %q", name)
		}
		return connectPaneResolver{pane: herdr.Pane{ID: "wsE:tab1:pane4", WorkspaceID: "wsMoved", TabID: "wsMoved:tab2"}}
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	want := Caller{
		Session:     "fledge-demo-00000001",
		RecordPath:  filepath.Join(root, ".fledge", "sessions", "fledge-demo-00000001"),
		WorkspaceID: "wsMoved",
		PaneID:      "wsE:tab1:pane4",
	}
	if caller != want {
		t.Fatalf("caller = %#v, want %#v", caller, want)
	}
	if !reflect.DeepEqual(client, herdr.New(nil, nil, nil).WithSession("fledge-demo-00000001")) {
		t.Fatalf("client = %#v, want a project-scoped client", client)
	}
}

func TestConnectOutsideHerderResolvesProjectSession(t *testing.T) {
	for _, test := range []struct {
		name     string
		sessions []herdr.Session
		want     string
		wantErr  string
	}{
		{
			name:     "sole running session",
			sessions: []herdr.Session{{Name: "fledge-demo-00000001", Running: true}, {Name: "fledge-demo-00000002", Running: false}},
			want:     "fledge-demo-00000001",
		},
		{
			name:     "no running session",
			sessions: []herdr.Session{{Name: "fledge-demo-00000001", Running: false}},
			wantErr:  "no running Fledge session",
		},
		{
			name:     "multiple running sessions",
			sessions: []herdr.Session{{Name: "fledge-demo-00000001", Running: true}, {Name: "fledge-demo-00000002", Running: true}},
			wantErr:  "multiple registered sessions are running: fledge-demo-00000001, fledge-demo-00000002",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := agentProject(t, "fledge-demo-00000001", "fledge-demo-00000002")
			caller, client, err := Connect(context.Background(), root, emptyEnv, func(context.Context) ([]herdr.Session, error) {
				return test.sessions, nil
			}, nil)

			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("Connect() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Connect() error = %v", err)
			}
			if caller != (Caller{Session: test.want, RecordPath: filepath.Join(root, ".fledge", "sessions", test.want)}) {
				t.Fatalf("caller = %#v, want session %q", caller, test.want)
			}
			if !reflect.DeepEqual(client, herdr.New(nil, nil, nil).WithSession(test.want)) {
				t.Fatalf("client is not scoped to %q", test.want)
			}
		})
	}
}

func TestConnectRejectsStaleAndCrossProjectAmbientSessions(t *testing.T) {
	root := agentProject(t, "session-a")
	tests := []struct {
		name    string
		session string
		pane    herdr.Pane
		want    string
	}{
		{name: "cross project", session: "session-b", want: "does not belong"},
		{name: "stale pane", session: "session-a", pane: herdr.Pane{ID: "live", WorkspaceID: "ws", TabID: "tab"}, want: "is stale"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := map[string]string{"HERDR_ENV": "1", "HERDR_SESSION": test.session, "HERDR_PANE_ID": "stale"}
			_, _, err := Connect(context.Background(), root, func(name string) string { return env[name] }, func(context.Context) ([]herdr.Session, error) {
				return []herdr.Session{{Name: "session-a", Running: true}, {Name: "session-b", Running: true}}, nil
			}, func(string) session.PaneResolver { return connectPaneResolver{pane: test.pane} })
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Connect() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestConnectKeepsSimultaneousProjectSessionsIsolated(t *testing.T) {
	rootA := agentProject(t, "session-a")
	rootB := agentProject(t, "session-b")
	listed := []herdr.Session{{Name: "session-a", Running: true}, {Name: "session-b", Running: true}}
	list := func(context.Context) ([]herdr.Session, error) { return listed, nil }
	for _, test := range []struct {
		root string
		want string
	}{{root: rootA, want: "session-a"}, {root: rootB, want: "session-b"}} {
		caller, client, err := Connect(context.Background(), test.root, emptyEnv, list, nil)
		if err != nil {
			t.Fatalf("Connect(%q) error = %v", test.root, err)
		}
		if caller.Session != test.want || caller.RecordPath != filepath.Join(test.root, ".fledge", "sessions", test.want) || !reflect.DeepEqual(client, herdr.New(nil, nil, nil).WithSession(test.want)) {
			t.Fatalf("Connect(%q) = %#v, %#v; want session %q", test.root, caller, client, test.want)
		}
	}
}

func TestMessagePassesOptionsThrough(t *testing.T) {
	client := newFakeHerder()
	client.prompted = json.RawMessage(`{"type":"agent_prompted"}`)

	result, err := Message(context.Background(), client, MessageOptions{
		Target:    "reviewer",
		Text:      "status?",
		Wait:      true,
		Until:     []string{"idle", "waiting"},
		TimeoutMS: 2500,
	})
	if err != nil {
		t.Fatalf("Message() error = %v", err)
	}
	if string(result) != `{"type":"agent_prompted"}` {
		t.Fatalf("result = %s, want the raw Herder result", result)
	}
	want := herdr.PromptOptions{Target: "reviewer", Text: "status?", Wait: true, Until: []string{"idle", "waiting"}, TimeoutMS: 2500}
	if !reflect.DeepEqual(client.promptOpts, want) {
		t.Fatalf("prompt options = %#v, want %#v", client.promptOpts, want)
	}
}

func TestMessagePropagatesFailure(t *testing.T) {
	client := newFakeHerder()
	want := errors.New("prompt failed")
	client.errs["PromptAgent"] = want

	if _, err := Message(context.Background(), client, MessageOptions{Target: "reviewer"}); !errors.Is(err, want) {
		t.Fatalf("Message() error = %v, want %v", err, want)
	}
}

func TestListReturnsAgents(t *testing.T) {
	client := newFakeHerder()
	client.agents = []herdr.Agent{{Name: "reviewer", Kind: "claude"}}

	agents, err := List(context.Background(), client)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !reflect.DeepEqual(agents, client.agents) {
		t.Fatalf("agents = %#v, want %#v", agents, client.agents)
	}
}

func TestStopResolvesAgentThenClosesItsPane(t *testing.T) {
	client := newFakeHerder()
	client.found = herdr.Agent{Name: "reviewer", PaneID: "ws1:tab2:pane5"}

	pane, err := Stop(context.Background(), client, "reviewer")
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if pane != "ws1:tab2:pane5" {
		t.Fatalf("pane = %q, want ws1:tab2:pane5", pane)
	}
	want := []string{"GetAgent(reviewer)", "ClosePane(ws1:tab2:pane5)"}
	if !reflect.DeepEqual(client.calls, want) {
		t.Fatalf("calls = %#v, want %#v", client.calls, want)
	}
}

func TestStopDoesNotCloseWhenLookupFails(t *testing.T) {
	client := newFakeHerder()
	want := errors.New("unknown agent")
	client.errs["GetAgent"] = want

	_, err := Stop(context.Background(), client, "ghost")
	if !errors.Is(err, want) {
		t.Fatalf("Stop() error = %v, want %v", err, want)
	}
	if !reflect.DeepEqual(client.calls, []string{"GetAgent(ghost)"}) {
		t.Fatalf("calls = %#v, want only the lookup", client.calls)
	}
}

func agentProject(t *testing.T, names ...string) string {
	t.Helper()
	root, err := project.Init(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		recordDir := filepath.Join(root, ".fledge", "sessions", name)
		if err := os.MkdirAll(recordDir, 0o755); err != nil {
			t.Fatal(err)
		}
		config := `{"schema_version":1,"herdr_session_name":"` + name + `","created_at":"2026-08-24T14:15:16Z"}` + "\n"
		if err := os.WriteFile(filepath.Join(recordDir, "config.json"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func emptyEnv(string) string { return "" }
