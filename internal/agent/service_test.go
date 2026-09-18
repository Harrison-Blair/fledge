package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

type call struct {
	method string
	params map[string]any
	result any
	err    error
}
type scripted struct {
	t     *testing.T
	calls []call
	index int
}

func (s *scripted) Call(_ context.Context, method string, params any, result any) error {
	s.t.Helper()
	if s.index >= len(s.calls) {
		s.t.Fatalf("unexpected call %s %#v", method, params)
	}
	want := s.calls[s.index]
	s.index++
	if method != want.method {
		s.t.Fatalf("call %d: got %s want %s", s.index, method, want.method)
	}
	data, _ := json.Marshal(params)
	var got map[string]any
	json.Unmarshal(data, &got)
	if want.params != nil {
		b, _ := json.Marshal(want.params)
		var normalized map[string]any
		json.Unmarshal(b, &normalized)
		if !reflect.DeepEqual(got, normalized) {
			s.t.Fatalf("%s params %#v want %#v", method, got, normalized)
		}
	}
	if want.err != nil {
		return want.err
	}
	b, _ := json.Marshal(want.result)
	return json.Unmarshal(b, result)
}
func fake(t *testing.T, calls ...call) *Service {
	t.Helper()
	s := &scripted{t: t, calls: calls}
	t.Cleanup(func() {
		if s.index != len(s.calls) {
			t.Errorf("used %d of %d calls", s.index, len(s.calls))
		}
	})
	return &Service{API: s, CallerPane: "old:p1", Cwd: t.TempDir()}
}
func pane(id, ws, tab string) herdr.Pane { return herdr.Pane{PaneID: id, WorkspaceID: ws, TabID: tab} }
func snapshot() herdr.SnapshotResult {
	return herdr.SnapshotResult{Type: "session_snapshot", Snapshot: &herdr.Snapshot{Workspaces: []herdr.Workspace{{ID: "w1", Label: "main"}}, Tabs: []herdr.Tab{{ID: "w1:t1", WorkspaceID: "w1", Label: "build"}}, Panes: []herdr.Pane{pane("w1:p1", "w1", "w1:t1")}, Layouts: []herdr.Layout{{TabID: "w1:t1", WorkspaceID: "w1", FocusedPaneID: "w1:p1"}}, Agents: []herdr.Pane{}}}
}
func started(p herdr.Pane) herdr.AgentResult {
	p.AgentStatus = "idle"
	h := "claude"
	p.Agent = &h
	return herdr.AgentResult{Type: "agent_started", Agent: p, Argv: []string{"claude"}}
}
func TestDefaultSpawnUsesResolvedCallerAndPolicy(t *testing.T) {
	p := pane("w1:p2", "w1", "w1:t2")
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.current", params: map[string]any{"caller_pane_id": "old:p1"}, result: herdr.PaneResult{Type: "pane_current", Pane: pane("w1:p1", "w1", "w1:t1")}}, call{method: "tab.create", params: map[string]any{"workspace_id": "w1", "focus": false}, result: herdr.CreatedResult{Type: "tab_created", Tab: herdr.Tab{ID: "w1:t2", WorkspaceID: "w1"}, RootPane: p}}, call{method: "agent.start", params: map[string]any{"name": "worker", "kind": "claude", "pane_id": "w1:p2", "args": []string{}, "timeout_ms": 30000}, result: started(p)})
	out := s.Spawn(context.Background(), validOptions())
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
}
func TestExistingTabSplitsItsOwnFocusedPane(t *testing.T) {
	p := pane("w1:p2", "w1", "w1:t1")
	o := validOptions()
	o.Workspace = "main"
	o.Tab = "build"
	o.Label = "worker pane"
	o.Focus = true
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.split", params: map[string]any{"workspace_id": "w1", "target_pane_id": "w1:p1", "direction": "right", "focus": false}, result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{method: "pane.rename", params: map[string]any{"pane_id": "w1:p2", "label": "worker pane"}, result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{method: "pane.focus", params: map[string]any{"pane_id": "w1:p2"}, result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{method: "agent.start", result: started(p)})
	out := s.Spawn(context.Background(), o)
	if out.Status != "success" || !out.Result.(*SpawnResult).Split {
		t.Fatalf("%+v", out)
	}
}
func TestDuplicateNameDoesNotMutate(t *testing.T) {
	snap := snapshot()
	name := "worker"
	snap.Snapshot.Agents = []herdr.Pane{{Name: &name}}
	s := fake(t, call{method: "session.snapshot", result: snap})
	out := s.Spawn(context.Background(), validOptions())
	if out.Status != "rejected" {
		t.Fatal(out)
	}
}
func TestExistingPaneStartupOutcomes(t *testing.T) {
	for _, tc := range []struct {
		code, status string
		uncertain    bool
	}{{"agent_not_ready", "partial", false}, {"timeout", "partial", false}, {"transport_error", "unknown", true}, {"agent_pane_not_found", "rejected", false}} {
		t.Run(tc.code, func(t *testing.T) {
			o := validOptions()
			o.Pane = "w1:p1"
			s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", err: &herdr.Error{Code: tc.code, Message: "failure", Uncertain: tc.uncertain}})
			out := s.Spawn(context.Background(), o)
			if out.Status != tc.status {
				t.Fatalf("%+v", out)
			}
		})
	}
}
func TestMessagePreservesContentAndDoesNotWait(t *testing.T) {
	p := pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "working"
	s := fake(t, call{method: "agent.get", params: map[string]any{"target": "worker"}, result: herdr.AgentResult{Type: "agent_info", Agent: p}}, call{method: "agent.prompt", params: map[string]any{"target": "worker", "text": "hello\nworld\n"}, result: herdr.AgentResult{Type: "agent_prompted", Agent: p}})
	out := s.Message(context.Background(), MessageOptions{Name: "worker", File: "-", FileSet: true}, strings.NewReader("hello\nworld\n"))
	if out.Status != "success" {
		t.Fatal(out)
	}
}
func TestInvalidMessageBeforeAPI(t *testing.T) {
	s := fake(t)
	out := s.Message(context.Background(), MessageOptions{Name: "a", BodySet: true}, strings.NewReader(""))
	if out.Status != "rejected" || out.ExitCode() != 2 {
		t.Fatal(out)
	}
}
func TestListIncludesUnnamed(t *testing.T) {
	p := pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	s := fake(t, call{method: "agent.list", result: herdr.AgentListResult{Type: "agent_list", Agents: []herdr.Pane{p}}})
	out := s.List(context.Background())
	if out.Status != "success" || len(out.Result.(ListResult).Agents) != 1 {
		t.Fatal(out)
	}
}
func TestRuntimeErrorExit(t *testing.T) {
	s := fake(t, call{method: "agent.list", err: errors.New("offline")})
	out := s.List(context.Background())
	if out.ExitCode() != 1 {
		t.Fatal(out)
	}
}

func TestEmptyWorkspaceLabelDoesNotSelectDestination(t *testing.T) {
	snap := snapshot()
	snap.Snapshot.Workspaces = append(snap.Snapshot.Workspaces, herdr.Workspace{ID: "unrelated", Label: ""})
	p := pane("w1:p2", "w1", "w1:t2")
	s := fake(t, call{method: "session.snapshot", result: snap}, call{method: "pane.current", result: herdr.PaneResult{Type: "pane_current", Pane: pane("w1:p1", "w1", "w1:t1")}}, call{method: "tab.create", params: map[string]any{"workspace_id": "w1", "focus": false}, result: herdr.CreatedResult{Type: "tab_created", Tab: herdr.Tab{ID: "w1:t2", WorkspaceID: "w1"}, RootPane: p}}, call{method: "agent.start", result: started(p)})
	if out := s.Spawn(context.Background(), validOptions()); out.Status != "success" {
		t.Fatal(out)
	}
}
func TestUnreadableMessageIsRuntimeFailure(t *testing.T) {
	s := fake(t)
	out := s.Message(context.Background(), MessageOptions{Name: "worker", File: "/does/not/exist", FileSet: true}, strings.NewReader(""))
	if out.ExitCode() != 1 {
		t.Fatalf("%+v", out)
	}
}
func TestStartWrongPaneIsUnknown(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", result: started(pane("w1:p9", "w1", "w1:t1"))})
	out := s.Spawn(context.Background(), o)
	if out.Status != "unknown" {
		t.Fatal(out)
	}
}
func TestCallerFailureRetainsPhase(t *testing.T) {
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.current", err: &herdr.Error{Code: "pane_not_found", Message: "gone"}})
	out := s.Spawn(context.Background(), validOptions())
	if out.Error.Phase != "pane.current" {
		t.Fatal(out)
	}
}
func TestWorktreeListFailureRetainsPhase(t *testing.T) {
	o := validOptions()
	o.Worktree = "new"
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "worktree.list", err: &herdr.Error{Code: "not_git_repository", Message: "not a repo"}})
	out := s.Spawn(context.Background(), o)
	if out.Error.Phase != "worktree.list" {
		t.Fatal(out)
	}
}
func TestNewWorkspaceReusesInitialTabAndPane(t *testing.T) {
	o := validOptions()
	o.Workspace = "new workspace"
	o.Tab = "tasks"
	o.Cwd = "/chosen"
	o.Env = []string{"K=a=b"}
	p := pane("w2:p1", "w2", "w2:t1")
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.current", result: herdr.PaneResult{Type: "pane_current", Pane: pane("w1:p1", "w1", "w1:t1")}}, call{method: "workspace.create", params: map[string]any{"focus": false, "label": "new workspace", "cwd": "/chosen", "env": map[string]string{"K": "a=b"}, "source_workspace_id": "w1"}, result: herdr.CreatedResult{Type: "workspace_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "1"}, RootPane: p}}, call{method: "tab.rename", params: map[string]any{"tab_id": "w2:t1", "label": "tasks"}, result: herdr.TabResult{Type: "tab_info", Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "tasks"}}}, call{method: "agent.start", result: started(p)})
	out := s.Spawn(context.Background(), o)
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
}
func TestSelectorFailuresDoNotMutate(t *testing.T) {
	for _, kind := range []string{"ambiguous workspace", "ambiguous tab", "wrong owner", "missing source", "missing anchor"} {
		t.Run(kind, func(t *testing.T) {
			snap := snapshot()
			o := validOptions()
			o.Workspace = "main"
			switch kind {
			case "ambiguous workspace":
				snap.Snapshot.Workspaces = append(snap.Snapshot.Workspaces, herdr.Workspace{ID: "w2", Label: "main"})
			case "ambiguous tab":
				o.Tab = "build"
				snap.Snapshot.Tabs = append(snap.Snapshot.Tabs, herdr.Tab{ID: "w1:t2", WorkspaceID: "w1", Label: "build"})
			case "wrong owner":
				o.TabID = "w2:t1"
				snap.Snapshot.Tabs = append(snap.Snapshot.Tabs, herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "build"})
			case "missing source":
				o.Workspace = "missing"
				o.Worktree = "new"
			case "missing anchor":
				o.Tab = "build"
				snap.Snapshot.Layouts = []herdr.Layout{}
			}
			s := fake(t, call{method: "session.snapshot", result: snap})
			out := s.Spawn(context.Background(), o)
			if out.Status != "rejected" || len(out.Effects) > 0 {
				t.Fatal(out)
			}
		})
	}
}
func TestMalformedMutationResultIsUnknown(t *testing.T) {
	o := validOptions()
	o.WorkspaceID = "w1"
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "tab.create", result: herdr.CreatedResult{Type: "tab_created", Tab: herdr.Tab{ID: "w1:t2", WorkspaceID: "w1"}}})
	out := s.Spawn(context.Background(), o)
	if out.Status != "unknown" || len(out.Effects) != 1 {
		t.Fatal(out)
	}
}
func TestMessageBlockedIsRejectedWithoutMutation(t *testing.T) {
	p := pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "blocked"
	s := fake(t, call{method: "agent.get", result: herdr.AgentResult{Type: "agent_info", Agent: p}}, call{method: "agent.prompt", err: &herdr.Error{Code: "agent_blocked", Message: "approval"}})
	out := s.Message(context.Background(), MessageOptions{Name: "worker", Body: "hello", BodySet: true}, strings.NewReader(""))
	if out.Status != "rejected" || out.Error.Code != "agent_blocked" {
		t.Fatal(out)
	}
}
func TestMalformedReadResponseRetainsPhase(t *testing.T) {
	for _, method := range []string{"pane.current", "worktree.list"} {
		t.Run(method, func(t *testing.T) {
			o := validOptions()
			if method == "worktree.list" {
				o.Worktree = "new"
			}
			s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: method, result: map[string]any{"type": "wrong"}})
			out := s.Spawn(context.Background(), o)
			if out.Status != "rejected" || out.Error.Phase != method {
				t.Fatal(out)
			}
		})
	}
}
func TestNewWorkspaceDoesNotRequireResolvableCaller(t *testing.T) {
	o := validOptions()
	o.Workspace = "new workspace"
	p := pane("w2:p1", "w2", "w2:t1")
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.current", err: &herdr.Error{Code: "pane_not_found", Message: "stale caller"}}, call{method: "workspace.create", params: map[string]any{"label": "new workspace", "focus": false}, result: herdr.CreatedResult{Type: "workspace_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p}}, call{method: "agent.start", result: started(p)})
	if out := s.Spawn(context.Background(), o); out.Status != "success" {
		t.Fatal(out)
	}
}
func TestExplicitPaneCustomizationOrder(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.Label = "reviewer"
	o.Focus = true
	p := pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.rename", params: map[string]any{"pane_id": "w1:p1", "label": "reviewer"}, result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{method: "pane.focus", params: map[string]any{"pane_id": "w1:p1"}, result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{method: "agent.start", result: started(p)})
	out := s.Spawn(context.Background(), o)
	if out.Status != "success" {
		t.Fatal(out)
	}
	for _, effect := range out.Effects {
		if effect.Action == "created" {
			t.Fatalf("created unnecessary resource: %+v", effect)
		}
	}
}
func TestOptionalCallerResolutionHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o := validOptions()
	o.Workspace = "new workspace"
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.current", err: context.Canceled})
	out := s.Spawn(ctx, o)
	if out.Status != "rejected" || out.Error == nil {
		t.Fatal(out)
	}
}

// The live Herdr capture returned pane_info, including before any agent was launched.
func TestFocusWrongDestinationStopsLaunch(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.Focus = true
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.focus", params: map[string]any{"pane_id": "w1:p1"}, result: herdr.PaneResult{Type: "pane_info", Pane: pane("w1:p9", "w1", "w1:t1")}})
	out := s.Spawn(context.Background(), o)
	if out.Status != "unknown" || out.Error.Phase != "pane.focus" {
		t.Fatal(out)
	}
}
