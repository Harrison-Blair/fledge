package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

type call struct {
	method string
	params map[string]any
	result any
	err    error
	// before, if set, runs as this call is served, so a test can simulate a
	// side effect (e.g. time passing) tied to the RPC itself rather than to
	// how many times something else was called.
	before func()
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
	if want.before != nil {
		want.before()
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
	return &Service{API: s, CallerPane: "old:p1", Cwd: t.TempDir(), Now: func() time.Time { return time.Unix(0, 0) }}
}
func pane(id, ws, tab string) herdr.Pane { return herdr.Pane{PaneID: id, WorkspaceID: ws, TabID: tab} }
func snapshot() herdr.SnapshotResult {
	return herdr.SnapshotResult{Type: "session_snapshot", Snapshot: &herdr.Snapshot{Workspaces: []herdr.Workspace{{ID: "w1", Label: "main"}}, Tabs: []herdr.Tab{{ID: "w1:t1", WorkspaceID: "w1", Label: "build"}}, Panes: []herdr.Pane{pane("w1:p1", "w1", "w1:t1")}, Layouts: []herdr.Layout{{TabID: "w1:t1", WorkspaceID: "w1", FocusedPaneID: "w1:p1"}}, Agents: []herdr.Pane{}}}
}
func started(p herdr.Pane) herdr.AgentResult {
	p.AgentStatus = "idle"
	h := "claude"
	p.Agent = &h
	return herdr.AgentResult{Type: "agent_started", Agent: herdr.AgentDetails{Pane: p}, Argv: []string{"claude"}}
}

// waited builds the agent.wait settled-state result for a pane already hosting an agent.
func waited(p herdr.Pane, status string) herdr.AgentResult {
	p.AgentStatus = status
	return info(p)
}
func waitCall(target string, p herdr.Pane, status string) call {
	return call{method: "agent.wait", params: map[string]any{"target": target, "timeout_ms": 30000}, result: waited(p, status)}
}
func TestDefaultSpawnUsesResolvedCallerAndPolicy(t *testing.T) {
	p := pane("w1:p2", "w1", "w1:t2")
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.current", params: map[string]any{"caller_pane_id": "old:p1"}, result: herdr.PaneResult{Type: "pane_current", Pane: pane("w1:p1", "w1", "w1:t1")}}, call{method: "tab.create", params: map[string]any{"workspace_id": "w1", "focus": false}, result: herdr.CreatedResult{Type: "tab_created", Tab: herdr.Tab{ID: "w1:t2", WorkspaceID: "w1"}, RootPane: p}}, call{method: "agent.start", params: map[string]any{"name": "worker", "kind": "claude", "pane_id": "w1:p2", "args": []string{}, "timeout_ms": 30000}, result: started(p)}, waitCall("worker", p, "idle"))
	out := s.Spawn(context.Background(), validOptions(), nil)
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
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.split", params: map[string]any{"workspace_id": "w1", "target_pane_id": "w1:p1", "direction": "right", "focus": false}, result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{method: "pane.rename", params: map[string]any{"pane_id": "w1:p2", "label": "worker pane"}, result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{method: "pane.focus", params: map[string]any{"pane_id": "w1:p2"}, result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"))
	out := s.Spawn(context.Background(), o, nil)
	if out.Status != "success" || !out.Result.(*SpawnResult).Split {
		t.Fatalf("%+v", out)
	}
}
func TestDuplicateNameDoesNotMutate(t *testing.T) {
	snap := snapshot()
	name := "worker"
	snap.Snapshot.Agents = []herdr.Pane{{Name: &name}}
	s := fake(t, call{method: "session.snapshot", result: snap})
	out := s.Spawn(context.Background(), validOptions(), nil)
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
			out := s.Spawn(context.Background(), o, nil)
			if out.Status != tc.status {
				t.Fatalf("%+v", out)
			}
		})
	}
}
func TestMessagePreservesContentAndDoesNotWait(t *testing.T) {
	lookupCwd, promptCwd := "/before-prompt", "/after-prompt"
	lookupPane := pane("w1:p1", "w1", "w1:t1")
	lookupPane.AgentStatus, lookupPane.Cwd = "working", &lookupCwd
	promptPane := pane("w1:p1", "w1", "w1:t1")
	promptPane.AgentStatus, promptPane.Cwd = "working", &promptCwd
	s := fake(t, call{method: "agent.get", params: map[string]any{"target": "worker"}, result: info(lookupPane)}, call{method: "agent.prompt", params: map[string]any{"target": "worker", "text": "hello\nworld\n"}, result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: promptPane}}})
	out := s.Message(context.Background(), MessageOptions{Name: "worker", File: "-", FileSet: true}, strings.NewReader("hello\nworld\n"))
	result, ok := out.Result.(MessageResult)
	if out.Status != "success" || !ok || !result.Submitted || result.Cwd == nil || *result.Cwd != promptCwd {
		t.Fatalf("%+v", out)
	}
}
func TestInvalidMessageBeforeAPI(t *testing.T) {
	s := fake(t)
	out := s.Message(context.Background(), MessageOptions{Name: "a", BodySet: true}, strings.NewReader(""))
	if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Message != "message must be nonempty UTF-8" {
		t.Fatal(out)
	}
}
func TestMessageRequiresExactlyOneOfBodyOrFile(t *testing.T) {
	for _, o := range []MessageOptions{
		{Name: "a"},
		{Name: "a", Body: "hi", BodySet: true, File: "-", FileSet: true},
	} {
		s := fake(t)
		out := s.Message(context.Background(), o, strings.NewReader(""))
		if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Message != "exactly one of --body or --file is required" {
			t.Fatalf("%+v", out)
		}
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
	s := fake(t, call{method: "session.snapshot", result: snap}, call{method: "pane.current", result: herdr.PaneResult{Type: "pane_current", Pane: pane("w1:p1", "w1", "w1:t1")}}, call{method: "tab.create", params: map[string]any{"workspace_id": "w1", "focus": false}, result: herdr.CreatedResult{Type: "tab_created", Tab: herdr.Tab{ID: "w1:t2", WorkspaceID: "w1"}, RootPane: p}}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"))
	if out := s.Spawn(context.Background(), validOptions(), nil); out.Status != "success" {
		t.Fatal(out)
	}
}
func TestUnreadableMessageIsRuntimeFailure(t *testing.T) {
	s := fake(t)
	out := s.Message(context.Background(), MessageOptions{Name: "worker", File: "/does/not/exist", FileSet: true}, strings.NewReader(""))
	if out.ExitCode() != 1 || !strings.HasPrefix(out.Error.Message, "read message: ") {
		t.Fatalf("%+v", out)
	}
}
func TestStartWrongPaneIsUnknown(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", result: started(pane("w1:p9", "w1", "w1:t1"))})
	out := s.Spawn(context.Background(), o, nil)
	if out.Status != "unknown" {
		t.Fatal(out)
	}
}
func TestCallerFailureRetainsPhase(t *testing.T) {
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.current", err: &herdr.Error{Code: "pane_not_found", Message: "gone"}})
	out := s.Spawn(context.Background(), validOptions(), nil)
	if out.Error.Phase != "pane.current" {
		t.Fatal(out)
	}
}
func TestWorktreeListFailureRetainsPhase(t *testing.T) {
	o := validOptions()
	o.Worktree = "new"
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "worktree.list", err: &herdr.Error{Code: "not_git_repository", Message: "not a repo"}})
	out := s.Spawn(context.Background(), o, nil)
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
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.current", result: herdr.PaneResult{Type: "pane_current", Pane: pane("w1:p1", "w1", "w1:t1")}}, call{method: "workspace.create", params: map[string]any{"focus": false, "label": "new workspace", "cwd": "/chosen", "env": map[string]string{"K": "a=b"}, "source_workspace_id": "w1"}, result: herdr.CreatedResult{Type: "workspace_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "1"}, RootPane: p}}, call{method: "tab.rename", params: map[string]any{"tab_id": "w2:t1", "label": "tasks"}, result: herdr.TabResult{Type: "tab_info", Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "tasks"}}}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"))
	out := s.Spawn(context.Background(), o, nil)
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
			out := s.Spawn(context.Background(), o, nil)
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
	out := s.Spawn(context.Background(), o, nil)
	if out.Status != "unknown" || len(out.Effects) != 1 {
		t.Fatal(out)
	}
}
func TestMessageBlockedIsRejectedWithoutMutation(t *testing.T) {
	p := pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "blocked"
	s := fake(t, call{method: "agent.get", result: info(p)}, call{method: "agent.prompt", err: &herdr.Error{Code: "agent_blocked", Message: "approval"}})
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
			out := s.Spawn(context.Background(), o, nil)
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
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.current", err: &herdr.Error{Code: "pane_not_found", Message: "stale caller"}}, call{method: "workspace.create", params: map[string]any{"label": "new workspace", "focus": false}, result: herdr.CreatedResult{Type: "workspace_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p}}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"))
	if out := s.Spawn(context.Background(), o, nil); out.Status != "success" {
		t.Fatal(out)
	}
}
func TestExplicitPaneCustomizationOrder(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.Label = "reviewer"
	o.Focus = true
	p := pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "pane.rename", params: map[string]any{"pane_id": "w1:p1", "label": "reviewer"}, result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{method: "pane.focus", params: map[string]any{"pane_id": "w1:p1"}, result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"))
	out := s.Spawn(context.Background(), o, nil)
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
	out := s.Spawn(ctx, o, nil)
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
	out := s.Spawn(context.Background(), o, nil)
	if out.Status != "unknown" || out.Error.Phase != "pane.focus" {
		t.Fatal(out)
	}
}

type recordedWaits struct {
	delays []time.Duration
	cancel context.CancelFunc
}

func (w *recordedWaits) wait(ctx context.Context, d time.Duration) error {
	w.delays = append(w.delays, d)
	if w.cancel != nil {
		w.cancel()
		return ctx.Err()
	}
	return nil
}
func busy() error { return &herdr.Error{Code: "agent_pane_busy", Message: "shell not ready"} }
func splitCalls(t *testing.T, starts ...call) (*Service, *recordedWaits) {
	t.Helper()
	p := pane("w1:p2", "w1", "w1:t1")
	calls := []call{{method: "session.snapshot", result: snapshot()}, {method: "pane.split", result: herdr.PaneResult{Type: "pane_info", Pane: p}}}
	s := fake(t, append(calls, starts...)...)
	w := &recordedWaits{}
	s.Wait = w.wait
	return s, w
}
func splitOptions() SpawnOptions {
	o := validOptions()
	o.Workspace = "main"
	o.Tab = "build"
	return o
}
func TestSpawnRetriesBusyPaneOnce(t *testing.T) {
	p := pane("w1:p2", "w1", "w1:t1")
	s, w := splitCalls(t, call{method: "agent.start", params: map[string]any{"name": "worker", "kind": "claude", "pane_id": "w1:p2", "args": []string{}, "timeout_ms": 30000}, err: busy()}, call{method: "agent.start", params: map[string]any{"name": "worker", "kind": "claude", "pane_id": "w1:p2", "args": []string{}, "timeout_ms": 30000}, result: started(p)}, waitCall("worker", p, "idle"))
	out := s.Spawn(context.Background(), splitOptions(), nil)
	if out.Status != "success" || out.Error != nil {
		t.Fatalf("%+v", out)
	}
	if !reflect.DeepEqual(w.delays, []time.Duration{50 * time.Millisecond}) {
		t.Fatalf("waits %v", w.delays)
	}
	if last := out.Effects[len(out.Effects)-1]; last.Action != "started" || last.ID != "w1:p2" {
		t.Fatalf("%+v", out.Effects)
	}
}
func TestSpawnBusyExhaustionIsPartial(t *testing.T) {
	var starts []call
	for i := 0; i < 7; i++ {
		starts = append(starts, call{method: "agent.start", err: busy()})
	}
	s, w := splitCalls(t, starts...)
	out := s.Spawn(context.Background(), splitOptions(), nil)
	if out.Status != "partial" || out.Error == nil || out.Error.Phase != "agent.start" || out.Error.Code != "agent_pane_busy" {
		t.Fatalf("%+v", out)
	}
	want := []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, 800 * time.Millisecond}
	if !reflect.DeepEqual(w.delays, want) {
		t.Fatalf("waits %v want %v", w.delays, want)
	}
	if !reflect.DeepEqual(out.Effects, []Effect{{Action: "created", Kind: "pane", ID: "w1:p2"}}) || *out.Result.(*SpawnResult).PaneID != "w1:p2" {
		t.Fatalf("resources changed: %+v", out)
	}
}
func TestSpawnDoesNotRetryOtherErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		call call
	}{
		{"agent_not_ready", call{method: "agent.start", err: &herdr.Error{Code: "agent_not_ready", Message: "slow"}}},
		{"timeout", call{method: "agent.start", err: &herdr.Error{Code: "timeout", Message: "slow"}}},
		{"transport_error", call{method: "agent.start", err: &herdr.Error{Code: "transport_error", Message: "lost", Uncertain: true}}},
		{"protocol", call{method: "agent.start", result: herdr.AgentResult{Type: "wrong"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, w := splitCalls(t, tc.call)
			out := s.Spawn(context.Background(), splitOptions(), nil)
			if out.Error == nil || out.Error.Phase != "agent.start" || len(w.delays) != 0 {
				t.Fatalf("%+v waits %v", out, w.delays)
			}
		})
	}
}
func TestSpawnBusyRetryHonorsCancellation(t *testing.T) {
	s, w := splitCalls(t, call{method: "agent.start", err: busy()})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.cancel = cancel
	out := s.Spawn(ctx, splitOptions(), nil)
	if out.Status != "partial" || out.Error == nil || out.Error.Phase != "agent.start" || out.Error.Code != "agent_pane_busy" {
		t.Fatalf("%+v", out)
	}
	if !reflect.DeepEqual(w.delays, []time.Duration{50 * time.Millisecond}) {
		t.Fatalf("waits %v", w.delays)
	}
}
func TestSpawnWaitDefaultsToRealTimer(t *testing.T) {
	s := &Service{}
	if err := s.wait(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- s.wait(ctx, time.Hour) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait ignored cancellation")
	}
}

func TestResolveTarget(t *testing.T) {
	for _, c := range []struct {
		name, pane, want string
		ok               bool
	}{{"worker", "", "worker", true}, {"", "w1:p3", "w1:p3", true}, {"", "", "", false}, {"worker", "w1:p3", "", false}} {
		got, err := resolveTarget(c.name, c.pane)
		if got != c.want || (err == nil) != c.ok {
			t.Fatalf("%+v: got %q, %v", c, got, err)
		}
		if !c.ok && err.Error() != "exactly one of --name or --pane is required" {
			t.Fatalf("%+v: %v", c, err)
		}
	}
}

func TestLookupReturnsValidatedAgentInfo(t *testing.T) {
	focused := true
	var revision uint64
	details := herdr.AgentDetails{Pane: liveAgent("working"), TerminalID: "term_x", Focused: &focused, Revision: &revision}
	out := Outcome{Status: "success", Effects: []Effect{}}
	got, err := fake(t, call{method: "agent.get", params: map[string]any{"target": "worker"}, result: herdr.AgentResult{Type: "agent_info", Agent: details}}).lookup(context.Background(), "worker", &out)
	if err != nil || !reflect.DeepEqual(got, details) || out.Status != "success" || out.Error != nil {
		t.Fatalf("%+v %v %+v", got, err, out)
	}
}

func TestSpawnStatusAndPlacementComeFromWait(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	sp := pane("w1:p1", "w1", "w1:t1")
	sp.AgentStatus = "unknown"
	startHarness := "unknown-detected"
	sp.Agent = &startHarness
	startResult := herdr.AgentResult{Type: "agent_started", Agent: herdr.AgentDetails{Pane: sp}, Argv: []string{"claude"}}
	wp := pane("w1:p1", "w1", "w1:t1")
	cwd := "/repo"
	wp.Cwd = &cwd
	waitHarness := "claude"
	wp.Agent = &waitHarness
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", result: startResult}, waitCall("worker", wp, "idle"))
	out := s.Spawn(context.Background(), o, nil)
	r, ok := out.Result.(*SpawnResult)
	if out.Status != "success" || !ok || r.AgentStatus == nil || *r.AgentStatus != "idle" || r.Cwd == nil || *r.Cwd != "/repo" {
		t.Fatalf("%+v", out)
	}
	if r.DetectedHarness == nil || *r.DetectedHarness != "claude" {
		t.Fatalf("detected_harness not from wait result: %+v", out)
	}
}
func TestSpawnNoWaitSkipsWait(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.NoWait = true
	p := pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", result: started(p)})
	out := s.Spawn(context.Background(), o, nil)
	r, ok := out.Result.(*SpawnResult)
	if out.Status != "success" || !ok || r.AgentStatus == nil || *r.AgentStatus != "idle" {
		t.Fatalf("%+v", out)
	}
}

// movableClock holds steady at t until advance moves it forward. Tying the
// advance to the agent.start call itself (rather than counting Now() calls)
// lets the test tell whether `started` was captured before or after
// agent.start ran.
type movableClock struct{ t time.Time }

func (c *movableClock) now() time.Time          { return c.t }
func (c *movableClock) advance(d time.Duration) { c.t = c.t.Add(d) }
func TestSpawnWaitTimeoutIsRemainingBudget(t *testing.T) {
	t0 := time.Unix(1700000000, 0)
	for _, tc := range []struct {
		name    string
		elapsed time.Duration
		wantMs  int64
	}{
		{"partial elapsed subtracts from budget", 12 * time.Second, 18000},
		{"elapsed past budget clamps to zero", 40 * time.Second, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := validOptions()
			o.Pane = "w1:p1"
			p := pane("w1:p1", "w1", "w1:t1")
			clock := &movableClock{t: t0}
			s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", result: started(p), before: func() { clock.advance(tc.elapsed) }}, call{method: "agent.wait", params: map[string]any{"target": "worker", "timeout_ms": tc.wantMs}, result: waited(p, "idle")})
			s.Now = clock.now
			out := s.Spawn(context.Background(), o, nil)
			if out.Status != "success" {
				t.Fatalf("%+v", out)
			}
		})
	}
}
func TestSpawnBlockedAfterWaitIsPartialWithoutClosingPane(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	p := pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "blocked"))
	out := s.Spawn(context.Background(), o, nil)
	if out.Status != "partial" || out.ExitCode() != 1 || out.Error == nil || out.Error.Code != "agent_blocked" || out.Error.Phase != "agent.wait" {
		t.Fatalf("%+v", out)
	}
	if len(out.Effects) == 0 {
		t.Fatalf("expected retained effects: %+v", out)
	}
	for _, e := range out.Effects {
		if e.Kind == "pane" && e.Action == "closed" {
			t.Fatalf("pane closed on blocked wait: %+v", out.Effects)
		}
	}
}
func TestSpawnWaitTimeoutIsPartial(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	p := pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", result: started(p)}, call{method: "agent.wait", err: &herdr.Error{Code: "timeout", Message: "no settled state"}})
	out := s.Spawn(context.Background(), o, nil)
	if out.Status != "partial" || out.Error == nil || out.Error.Code != "timeout" || out.Error.Phase != "agent.wait" {
		t.Fatalf("%+v", out)
	}
}

// Each case mutates exactly one field of an otherwise-fully-valid agent.wait
// result, so each of the three checks (type, AgentInfo completeness, same
// pane) is proven independently rather than masked by the others.
func TestSpawnMalformedWaitResultIsUnknown(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result func() herdr.AgentResult
	}{
		{"wrong type", func() herdr.AgentResult {
			r := waited(pane("w1:p1", "w1", "w1:t1"), "idle")
			r.Type = "wrong"
			return r
		}},
		{"incomplete agent info", func() herdr.AgentResult {
			r := waited(pane("w1:p1", "w1", "w1:t1"), "idle")
			r.Agent.TerminalID = ""
			return r
		}},
		{"different pane", func() herdr.AgentResult {
			return waited(pane("w1:p9", "w1", "w1:t1"), "idle")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := validOptions()
			o.Pane = "w1:p1"
			p := pane("w1:p1", "w1", "w1:t1")
			s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", result: started(p)}, call{method: "agent.wait", result: tc.result()})
			out := s.Spawn(context.Background(), o, nil)
			if out.Status != "unknown" || out.Error == nil || out.Error.Phase != "agent.wait" {
				t.Fatalf("%+v", out)
			}
		})
	}
}

// Each case mutates exactly one field of an otherwise-fully-valid agent.prompt
// result, so the type check and the pane validity check are proven
// independently, and the failure phase is asserted for a locally-detected
// (not remote-error) malformed result.
func TestSpawnMalformedPromptResultIsUnknown(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result func(p herdr.Pane) herdr.AgentResult
	}{
		{"wrong type", func(p herdr.Pane) herdr.AgentResult {
			return herdr.AgentResult{Type: "wrong", Agent: herdr.AgentDetails{Pane: p}}
		}},
		{"invalid pane", func(p herdr.Pane) herdr.AgentResult {
			p.AgentStatus = ""
			return herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := validOptions()
			o.Pane = "w1:p1"
			o.Prompt = "hi"
			o.PromptSet = true
			p := pane("w1:p1", "w1", "w1:t1")
			p.AgentStatus = "idle"
			s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"), call{method: "agent.prompt", result: tc.result(p)})
			out := s.Spawn(context.Background(), o, nil)
			if out.Status != "unknown" || out.Error == nil || out.Error.Phase != "agent.prompt" {
				t.Fatalf("%+v", out)
			}
		})
	}
}

func TestSpawnSendsPromptAfterWaitWithExactText(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.Prompt = "hello\nworld\n"
	o.PromptSet = true
	p := pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"), call{method: "agent.prompt", params: map[string]any{"target": "worker", "text": "hello\nworld\n"}, result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	out := s.Spawn(context.Background(), o, nil)
	r, ok := out.Result.(*SpawnResult)
	if out.Status != "success" || !ok || !r.Prompted {
		t.Fatalf("%+v", out)
	}
	if last := out.Effects[len(out.Effects)-1]; last.Action != "submitted" || last.Kind != "message" || last.ID != "w1:p1" {
		t.Fatalf("%+v", out.Effects)
	}
}
func TestSpawnPromptFromFileStdin(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.File = "-"
	o.FileSet = true
	p := pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"), call{method: "agent.prompt", params: map[string]any{"target": "worker", "text": "from file\n"}, result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	out := s.Spawn(context.Background(), o, strings.NewReader("from file\n"))
	if out.Status != "success" || !out.Result.(*SpawnResult).Prompted {
		t.Fatalf("%+v", out)
	}
}
func TestSpawnNoPromptFlagsDoesNotCallAgentPrompt(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	p := pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"))
	out := s.Spawn(context.Background(), o, nil)
	if out.Status != "success" || out.Result.(*SpawnResult).Prompted {
		t.Fatalf("%+v", out)
	}
}
func TestSpawnBothPromptAndFileRejectedBeforeMutation(t *testing.T) {
	o := validOptions()
	o.Prompt = "hi"
	o.PromptSet = true
	o.File = "-"
	o.FileSet = true
	s := fake(t)
	out := s.Spawn(context.Background(), o, nil)
	if out.Status != "rejected" || out.ExitCode() != 2 || len(out.Effects) != 0 || out.Error.Message != "at most one of --prompt or --file is allowed" {
		t.Fatalf("%+v", out)
	}
}
func TestSpawnUnreadablePromptFileFailsBeforeMutation(t *testing.T) {
	o := validOptions()
	o.File = "/does/not/exist"
	o.FileSet = true
	s := fake(t)
	out := s.Spawn(context.Background(), o, nil)
	if out.ExitCode() != 1 || out.Error.Phase != "validation" || len(out.Effects) != 0 || !strings.HasPrefix(out.Error.Message, "read prompt: ") {
		t.Fatalf("%+v", out)
	}
}
func TestSpawnEmptyPromptRejected(t *testing.T) {
	o := validOptions()
	o.Prompt = ""
	o.PromptSet = true
	s := fake(t)
	out := s.Spawn(context.Background(), o, nil)
	if out.Status != "rejected" || out.ExitCode() != 2 || len(out.Effects) != 0 || out.Error.Message != "prompt must be nonempty UTF-8" {
		t.Fatalf("%+v", out)
	}
}
func TestSpawnPromptFailureRetainsEarlierEffects(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.Prompt = "hi"
	o.PromptSet = true
	p := pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"), call{method: "agent.prompt", err: &herdr.Error{Code: "agent_blocked", Message: "approval"}})
	out := s.Spawn(context.Background(), o, nil)
	if out.Status != "partial" || out.Error == nil || out.Error.Phase != "agent.prompt" || out.ExitCode() != 1 {
		t.Fatalf("%+v", out)
	}
	if len(out.Effects) == 0 {
		t.Fatalf("expected retained effects: %+v", out)
	}
}
