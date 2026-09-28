package spawn

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
)

type call = herdrscript.Call

func fake(t *testing.T, calls ...call) *spawner {
	t.Helper()
	return &spawner{Client: herdrscript.Client(t, calls...), Now: func() time.Time { return time.Unix(0, 0) }, NewID: func() string { return "m-0a1b2c" }}
}
func snapshot() herdr.SnapshotResult {
	return herdr.SnapshotResult{Type: "session_snapshot", Snapshot: &herdr.Snapshot{Workspaces: []herdr.Workspace{{ID: "w1", Label: "main"}}, Tabs: []herdr.Tab{{ID: "w1:t1", WorkspaceID: "w1", Label: "build"}}, Panes: []herdr.Pane{herdrscript.Pane("w1:p1", "w1", "w1:t1")}, Layouts: []herdr.Layout{{TabID: "w1:t1", WorkspaceID: "w1", FocusedPaneID: "w1:p1"}}, Agents: []herdr.Pane{}}}
}
func started(p herdr.Pane) herdr.AgentResult {
	p.AgentStatus = "idle"
	h := "claude"
	p.Agent = &h
	return herdr.AgentResult{Type: "agent_started", Agent: herdr.AgentDetails{Pane: p, TerminalID: "term_x"}, Argv: []string{"claude"}}
}

// labeled is spawn's default pane.rename of p to the agent's name.
func labeled(p herdr.Pane) call {
	return call{Method: "pane.rename", Params: map[string]any{"pane_id": p.PaneID, "label": "worker"}, Result: herdr.PaneResult{Type: "pane_info", Pane: p}}
}

// namedTab is spawn's default tab.rename of p's new tab to the agent's name.
func namedTab(p herdr.Pane) call {
	return call{Method: "tab.rename", Params: map[string]any{"tab_id": p.TabID, "label": "worker"}, Result: herdr.TabResult{Type: "tab_info", Tab: herdr.Tab{ID: p.TabID, WorkspaceID: p.WorkspaceID, Label: "worker"}}}
}

func waitCall(target string, p herdr.Pane, status string) call {
	return call{Method: "agent.wait", Params: map[string]any{"target": target, "timeout_ms": 30000}, Result: settled(p, status)}
}

// settled is the agent.wait result for worker in p once Herdr admits prompts.
func settled(p herdr.Pane, status string) herdr.AgentResult {
	r := herdrscript.Waited(p, status)
	name, ready := "worker", true
	r.Agent.Name, r.Agent.InteractiveReady = &name, &ready
	return r
}
func TestDefaultSpawnUsesResolvedCallerAndPolicy(t *testing.T) {
	p := herdrscript.Pane("w1:p2", "w1", "w1:t2")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.current", Params: map[string]any{"caller_pane_id": "old:p1"}, Result: herdr.PaneResult{Type: "pane_current", Pane: herdrscript.Pane("w1:p1", "w1", "w1:t1")}}, call{Method: "tab.create", Params: map[string]any{"label": "worker", "workspace_id": "w1", "focus": false}, Result: herdr.CreatedResult{Type: "tab_created", Tab: herdr.Tab{ID: "w1:t2", WorkspaceID: "w1"}, RootPane: p}}, labeled(p), call{Method: "agent.start", Params: map[string]any{"name": "worker", "kind": "claude", "pane_id": "w1:p2", "args": []string{}, "timeout_ms": 30000}, Result: started(p)}, waitCall("worker", p, "idle"))
	out := s.run(context.Background(), validOptions(), nil)
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
}
func TestExistingTabSplitsItsOwnFocusedPane(t *testing.T) {
	p := herdrscript.Pane("w1:p2", "w1", "w1:t1")
	o := validOptions()
	o.Workspace = "main"
	o.Tab = "build"
	o.Label = "worker pane"
	o.Focus = true
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.split", Params: map[string]any{"workspace_id": "w1", "target_pane_id": "w1:p1", "direction": "right", "focus": false}, Result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{Method: "pane.rename", Params: map[string]any{"pane_id": "w1:p2", "label": "worker pane"}, Result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{Method: "pane.focus", Params: map[string]any{"pane_id": "w1:p2"}, Result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	out := s.run(context.Background(), o, nil)
	if out.Status != "success" || !out.Result.(*Result).Split {
		t.Fatalf("%+v", out)
	}
}
func TestDuplicateNameDoesNotMutate(t *testing.T) {
	snap := snapshot()
	name := "worker"
	snap.Snapshot.Agents = []herdr.Pane{{Name: &name}}
	s := fake(t, call{Method: "session.snapshot", Result: snap})
	out := s.run(context.Background(), validOptions(), nil)
	if out.Status != "rejected" {
		t.Fatal(out)
	}
}
func TestExistingPaneStartupOutcomes(t *testing.T) {
	for _, tc := range []struct {
		code, status string
		uncertain    bool
	}{{"agent_not_ready", "partial", false}, {"timeout", "partial", false}, {"transport_error", "unknown", true}, {"agent_pane_not_found", "partial", false}} {
		t.Run(tc.code, func(t *testing.T) {
			o := validOptions()
			o.Pane = "w1:p1"
			s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(herdrscript.Pane("w1:p1", "w1", "w1:t1")), call{Method: "agent.start", Err: &herdr.Error{Code: tc.code, Message: "failure", Uncertain: tc.uncertain}})
			out := s.run(context.Background(), o, nil)
			if out.Status != tc.status {
				t.Fatalf("%+v", out)
			}
		})
	}
}

func TestEmptyWorkspaceLabelDoesNotSelectDestination(t *testing.T) {
	snap := snapshot()
	snap.Snapshot.Workspaces = append(snap.Snapshot.Workspaces, herdr.Workspace{ID: "unrelated", Label: ""})
	p := herdrscript.Pane("w1:p2", "w1", "w1:t2")
	s := fake(t, call{Method: "session.snapshot", Result: snap}, call{Method: "pane.current", Result: herdr.PaneResult{Type: "pane_current", Pane: herdrscript.Pane("w1:p1", "w1", "w1:t1")}}, call{Method: "tab.create", Params: map[string]any{"label": "worker", "workspace_id": "w1", "focus": false}, Result: herdr.CreatedResult{Type: "tab_created", Tab: herdr.Tab{ID: "w1:t2", WorkspaceID: "w1"}, RootPane: p}}, labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	if out := s.run(context.Background(), validOptions(), nil); out.Status != "success" {
		t.Fatal(out)
	}
}
func TestStartWrongPaneIsUnknown(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(herdrscript.Pane("w1:p1", "w1", "w1:t1")), call{Method: "agent.start", Result: started(herdrscript.Pane("w1:p9", "w1", "w1:t1"))})
	out := s.run(context.Background(), o, nil)
	if out.Status != "unknown" {
		t.Fatal(out)
	}
}
func TestCallerFailureRetainsPhase(t *testing.T) {
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.current", Err: &herdr.Error{Code: "pane_not_found", Message: "gone"}})
	out := s.run(context.Background(), validOptions(), nil)
	if out.Error.Phase != "pane.current" {
		t.Fatal(out)
	}
}
func TestWorktreeListFailureRetainsPhase(t *testing.T) {
	o := validOptions()
	o.Worktree = "new"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "worktree.list", Err: &herdr.Error{Code: "not_git_repository", Message: "not a repo"}})
	out := s.run(context.Background(), o, nil)
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
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.current", Result: herdr.PaneResult{Type: "pane_current", Pane: herdrscript.Pane("w1:p1", "w1", "w1:t1")}}, call{Method: "workspace.create", Params: map[string]any{"focus": false, "label": "new workspace", "cwd": "/chosen", "env": map[string]string{"K": "a=b"}, "source_workspace_id": "w1"}, Result: herdr.CreatedResult{Type: "workspace_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "1"}, RootPane: p}}, call{Method: "tab.rename", Params: map[string]any{"tab_id": "w2:t1", "label": "tasks"}, Result: herdr.TabResult{Type: "tab_info", Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "tasks"}}}, labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	out := s.run(context.Background(), o, nil)
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
}
func TestRelativeCwdResolvesAgainstCallerDirectory(t *testing.T) {
	callerCwd := t.TempDir()
	resolved := filepath.Join(callerCwd, "relative/sub")
	o := validOptions()
	o.Workspace = "new workspace"
	o.Cwd = "relative/sub"
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.current", Result: herdr.PaneResult{Type: "pane_current", Pane: herdrscript.Pane("w1:p1", "w1", "w1:t1")}}, call{Method: "workspace.create", Params: map[string]any{"focus": false, "label": "new workspace", "cwd": resolved, "source_workspace_id": "w1"}, Result: herdr.CreatedResult{Type: "workspace_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p}}, namedTab(p), labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	s.Cwd = callerCwd
	out := s.run(context.Background(), o, nil)
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
			s := fake(t, call{Method: "session.snapshot", Result: snap})
			out := s.run(context.Background(), o, nil)
			if out.Status != "rejected" || len(out.Effects) > 0 {
				t.Fatal(out)
			}
		})
	}
}
func TestMalformedMutationResultIsUnknown(t *testing.T) {
	o := validOptions()
	o.WorkspaceID = "w1"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "tab.create", Result: herdr.CreatedResult{Type: "tab_created", Tab: herdr.Tab{ID: "w1:t2", WorkspaceID: "w1"}}})
	out := s.run(context.Background(), o, nil)
	if out.Status != "unknown" || len(out.Effects) != 1 {
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
			s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: method, Result: map[string]any{"type": "wrong"}})
			out := s.run(context.Background(), o, nil)
			if out.Status != "rejected" || out.Error.Phase != method {
				t.Fatal(out)
			}
		})
	}
}
func TestNewWorkspaceDoesNotRequireResolvableCaller(t *testing.T) {
	o := validOptions()
	o.Workspace = "new workspace"
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.current", Err: &herdr.Error{Code: "pane_not_found", Message: "stale caller"}}, call{Method: "workspace.create", Params: map[string]any{"label": "new workspace", "focus": false}, Result: herdr.CreatedResult{Type: "workspace_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p}}, namedTab(p), labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	if out := s.run(context.Background(), o, nil); out.Status != "success" {
		t.Fatal(out)
	}
}
func TestExplicitPaneCustomizationOrder(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.Label = "reviewer"
	o.Focus = true
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.rename", Params: map[string]any{"pane_id": "w1:p1", "label": "reviewer"}, Result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{Method: "pane.focus", Params: map[string]any{"pane_id": "w1:p1"}, Result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	out := s.run(context.Background(), o, nil)
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
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.current", Err: context.Canceled})
	out := s.run(ctx, o, nil)
	if out.Status != "rejected" || out.Error == nil {
		t.Fatal(out)
	}
}

// The live Herdr capture returned pane_info, including before any agent was launched.
func TestFocusWrongDestinationStopsLaunch(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.Focus = true
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(herdrscript.Pane("w1:p1", "w1", "w1:t1")), call{Method: "pane.focus", Params: map[string]any{"pane_id": "w1:p1"}, Result: herdr.PaneResult{Type: "pane_info", Pane: herdrscript.Pane("w1:p9", "w1", "w1:t1")}})
	out := s.run(context.Background(), o, nil)
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
func splitCalls(t *testing.T, starts ...call) (*spawner, *recordedWaits) {
	t.Helper()
	p := herdrscript.Pane("w1:p2", "w1", "w1:t1")
	calls := []call{{Method: "session.snapshot", Result: snapshot()}, {Method: "pane.split", Result: herdr.PaneResult{Type: "pane_info", Pane: p}}, labeled(p)}
	s := fake(t, append(calls, starts...)...)
	w := &recordedWaits{}
	s.Wait = w.wait
	return s, w
}
func splitOptions() Options {
	o := validOptions()
	o.Workspace = "main"
	o.Tab = "build"
	return o
}
func TestSpawnRetriesBusyPaneOnce(t *testing.T) {
	p := herdrscript.Pane("w1:p2", "w1", "w1:t1")
	s, w := splitCalls(t, call{Method: "agent.start", Params: map[string]any{"name": "worker", "kind": "claude", "pane_id": "w1:p2", "args": []string{}, "timeout_ms": 30000}, Err: busy()}, call{Method: "agent.start", Params: map[string]any{"name": "worker", "kind": "claude", "pane_id": "w1:p2", "args": []string{}, "timeout_ms": 30000}, Result: started(p)}, waitCall("worker", p, "idle"))
	out := s.run(context.Background(), splitOptions(), nil)
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
		starts = append(starts, call{Method: "agent.start", Err: busy()})
	}
	s, w := splitCalls(t, starts...)
	out := s.run(context.Background(), splitOptions(), nil)
	if out.Status != "partial" || out.Error == nil || out.Error.Phase != "agent.start" || out.Error.Code != "agent_pane_busy" {
		t.Fatalf("%+v", out)
	}
	want := []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, 800 * time.Millisecond}
	if !reflect.DeepEqual(w.delays, want) {
		t.Fatalf("waits %v want %v", w.delays, want)
	}
	if !reflect.DeepEqual(out.Effects, []libagent.Effect{{Action: "created", Kind: "pane", ID: "w1:p2"}, {Action: "updated", Kind: "pane_label", ID: "w1:p2"}}) || *out.Result.(*Result).PaneID != "w1:p2" {
		t.Fatalf("resources changed: %+v", out)
	}
}
func TestSpawnDoesNotRetryOtherErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		call call
	}{
		{"agent_not_ready", call{Method: "agent.start", Err: &herdr.Error{Code: "agent_not_ready", Message: "slow"}}},
		{"timeout", call{Method: "agent.start", Err: &herdr.Error{Code: "timeout", Message: "slow"}}},
		{"transport_error", call{Method: "agent.start", Err: &herdr.Error{Code: "transport_error", Message: "lost", Uncertain: true}}},
		{"protocol", call{Method: "agent.start", Result: herdr.AgentResult{Type: "wrong"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, w := splitCalls(t, tc.call)
			out := s.run(context.Background(), splitOptions(), nil)
			if out.Error == nil || out.Error.Phase != "agent.start" || len(w.delays) != 0 {
				t.Fatalf("%+v waits %v", out, w.delays)
			}
		})
	}
}
func TestSpawnBusyRetryHonorsCancellation(t *testing.T) {
	s, w := splitCalls(t, call{Method: "agent.start", Err: busy()})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.cancel = cancel
	out := s.run(ctx, splitOptions(), nil)
	if out.Status != "partial" || out.Error == nil || out.Error.Phase != "agent.start" || out.Error.Code != "agent_pane_busy" {
		t.Fatalf("%+v", out)
	}
	if !reflect.DeepEqual(w.delays, []time.Duration{50 * time.Millisecond}) {
		t.Fatalf("waits %v", w.delays)
	}
}
func TestSpawnWaitDefaultsToRealTimer(t *testing.T) {
	s := &spawner{}
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

func TestSpawnStatusAndPlacementComeFromWait(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	sp := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	sp.AgentStatus = "unknown"
	startHarness := "unknown-detected"
	sp.Agent = &startHarness
	startResult := herdr.AgentResult{Type: "agent_started", Agent: herdr.AgentDetails{Pane: sp, TerminalID: "term_x"}, Argv: []string{"claude"}}
	wp := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	cwd := "/repo"
	wp.Cwd = &cwd
	waitHarness := "claude"
	wp.Agent = &waitHarness
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(herdrscript.Pane("w1:p1", "w1", "w1:t1")), call{Method: "agent.start", Result: startResult}, waitCall("worker", wp, "idle"))
	out := s.run(context.Background(), o, nil)
	r, ok := out.Result.(*Result)
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
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)})
	out := s.run(context.Background(), o, nil)
	r, ok := out.Result.(*Result)
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := validOptions()
			o.Pane = "w1:p1"
			p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
			clock := &movableClock{t: t0}
			s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p), Before: func() { clock.advance(tc.elapsed) }}, call{Method: "agent.wait", Params: map[string]any{"target": "worker", "timeout_ms": tc.wantMs}, Result: settled(p, "idle")})
			s.Now = clock.now
			out := s.run(context.Background(), o, nil)
			if out.Status != "success" {
				t.Fatalf("%+v", out)
			}
		})
	}
}
func TestSpawnBlockedAfterWaitIsPartialWithoutClosingPane(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "blocked"))
	out := s.run(context.Background(), o, nil)
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
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)}, call{Method: "agent.wait", Err: &herdr.Error{Code: "timeout", Message: "no settled state"}})
	out := s.run(context.Background(), o, nil)
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
			r := settled(herdrscript.Pane("w1:p1", "w1", "w1:t1"), "idle")
			r.Type = "wrong"
			return r
		}},
		{"incomplete agent info", func() herdr.AgentResult {
			r := settled(herdrscript.Pane("w1:p1", "w1", "w1:t1"), "idle")
			r.Agent.TerminalID = ""
			return r
		}},
		{"different pane", func() herdr.AgentResult {
			return settled(herdrscript.Pane("w1:p9", "w1", "w1:t1"), "idle")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := validOptions()
			o.Pane = "w1:p1"
			p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
			s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)}, call{Method: "agent.wait", Result: tc.result()})
			out := s.run(context.Background(), o, nil)
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
			p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
			p.AgentStatus = "idle"
			s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), senderCall(), call{Method: "agent.prompt", Result: tc.result(p)})
			out := s.run(context.Background(), o, nil)
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
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": header + "hello\nworld\n"}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	out := s.run(context.Background(), o, nil)
	r, ok := out.Result.(*Result)
	if out.Status != "success" || !ok || !r.Prompted {
		t.Fatalf("%+v", out)
	}
	if r.MessageID == nil || *r.MessageID != "m-0a1b2c" || r.Sender == nil || r.Sender.Kind != "named" || *r.Sender.Name != "orchestrator" {
		t.Fatalf("%+v", r)
	}
	if last := out.Effects[len(out.Effects)-1]; last.Action != "submitted" || last.Kind != "message" || last.ID != "w1:p1" {
		t.Fatalf("%+v", out.Effects)
	}
}

const header = "ᛉ fledge message from orchestrator (old:p1) · id m-0a1b2c · reply: fledge agent message --name orchestrator\n"

// senderCall resolves the scripted caller pane old:p1 to the named agent orchestrator.
func senderCall() call {
	p := herdrscript.Pane("old:p1", "old", "old:t1")
	p.AgentStatus = "working"
	name := "orchestrator"
	p.Name = &name
	return call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: herdrscript.Info(p)}
}

func TestSpawnPromptFromFileStdin(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.File = "-"
	o.FileSet = true
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": header + "from file\n"}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	out := s.run(context.Background(), o, strings.NewReader("from file\n"))
	if out.Status != "success" || !out.Result.(*Result).Prompted {
		t.Fatalf("%+v", out)
	}
}
func TestSpawnNoPromptFlagsDoesNotCallAgentPrompt(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	out := s.run(context.Background(), o, nil)
	if r := out.Result.(*Result); out.Status != "success" || r.Prompted || r.MessageID != nil || r.Sender != nil {
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
	out := s.run(context.Background(), o, nil)
	if out.Status != "rejected" || out.ExitCode() != 2 || len(out.Effects) != 0 || out.Error.Message != "at most one of --prompt or --file is allowed" {
		t.Fatalf("%+v", out)
	}
}
func TestSpawnUnreadablePromptFileFailsBeforeMutation(t *testing.T) {
	o := validOptions()
	o.File = "/does/not/exist"
	o.FileSet = true
	s := fake(t)
	out := s.run(context.Background(), o, nil)
	if out.ExitCode() != 1 || out.Error.Phase != "validation" || len(out.Effects) != 0 || !strings.HasPrefix(out.Error.Message, "read prompt: ") {
		t.Fatalf("%+v", out)
	}
}
func TestSpawnEmptyPromptRejected(t *testing.T) {
	o := validOptions()
	o.Prompt = ""
	o.PromptSet = true
	s := fake(t)
	out := s.run(context.Background(), o, nil)
	if out.Status != "rejected" || out.ExitCode() != 2 || len(out.Effects) != 0 || out.Error.Message != "prompt must be nonempty UTF-8" {
		t.Fatalf("%+v", out)
	}
}
func TestSpawnPromptFailureRetainsEarlierEffects(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.Prompt = "hi"
	o.PromptSet = true
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), senderCall(), call{Method: "agent.prompt", Err: &herdr.Error{Code: "agent_blocked", Message: "approval"}})
	out := s.run(context.Background(), o, nil)
	if out.Status != "partial" || out.Error == nil || out.Error.Phase != "agent.prompt" || out.ExitCode() != 1 {
		t.Fatalf("%+v", out)
	}
	if len(out.Effects) == 0 {
		t.Fatalf("expected retained effects: %+v", out)
	}
}

func TestWorkspaceRenameFailureRetainsResources(t *testing.T) {
	for _, tc := range []struct {
		name, status, code string
		result             herdr.TabResult
		err                error
	}{
		{name: "server rejection", status: "partial", code: "rename_failed", err: &herdr.Error{Code: "rename_failed", Message: "failed"}},
		{name: "wrong type", status: "unknown", code: "protocol_error", result: herdr.TabResult{Type: "wrong", Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}}},
		{name: "wrong tab", status: "unknown", code: "protocol_error", result: herdr.TabResult{Type: "tab_info", Tab: herdr.Tab{ID: "w2:t2", WorkspaceID: "w2"}}},
		{name: "wrong workspace", status: "unknown", code: "protocol_error", result: herdr.TabResult{Type: "tab_info", Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w3"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := validOptions()
			o.Workspace, o.Tab = "new workspace", "tasks"
			p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
			s := fake(t,
				call{Method: "session.snapshot", Result: snapshot()},
				call{Method: "pane.current", Result: herdr.PaneResult{Type: "pane_current", Pane: herdrscript.Pane("w1:p1", "w1", "w1:t1")}},
				call{Method: "workspace.create", Result: herdr.CreatedResult{Type: "workspace_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "1"}, RootPane: p}},
				call{Method: "tab.rename", Params: map[string]any{"tab_id": "w2:t1", "label": "tasks"}, Result: tc.result, Err: tc.err},
			)
			out := s.run(context.Background(), o, nil)
			if out.Status != tc.status || out.Error == nil || out.Error.Code != tc.code || out.Error.Phase != "tab.rename" {
				t.Fatalf("%+v", out)
			}
			want := []libagent.Effect{{Action: "created", Kind: "workspace", ID: "w2"}, {Action: "created", Kind: "tab", ID: "w2:t1"}, {Action: "created", Kind: "pane", ID: "w2:p1"}}
			if !reflect.DeepEqual(out.Effects, want) {
				t.Fatalf("effects = %+v, want %+v", out.Effects, want)
			}
			result := out.Result.(*Result)
			if result.PaneID == nil || *result.PaneID != p.PaneID || result.TabID == nil || *result.TabID != p.TabID || result.WorkspaceID == nil || *result.WorkspaceID != p.WorkspaceID {
				t.Fatalf("lost placement: %+v", result)
			}
		})
	}
}

// A blocked startup wait never reaches agent.prompt (the script has no such
// call, so one would fail the test), keeps the agent and its registration
// attempt, and reports a requested but unsubmitted first prompt.
func TestSpawnBlockedWaitReportsRequestedPromptUnsubmitted(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Options)
		in   string
		want bool
	}{
		{"inline prompt", func(o *Options) { o.Prompt, o.PromptSet = "secret brief", true }, "", true},
		{"file prompt", func(o *Options) { o.File, o.FileSet = "-", true }, "secret brief\n", true},
		{"no prompt", func(*Options) {}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := validOptions()
			o.Pane = "w1:p1"
			tc.set(&o)
			p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
			s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "blocked"))
			out := s.run(context.Background(), o, strings.NewReader(tc.in))
			r := out.Result.(*Result)
			if out.Status != "partial" || out.ExitCode() != 1 || out.Error == nil || out.Error.Code != "agent_blocked" || out.Error.Phase != "agent.wait" {
				t.Fatalf("%+v", out)
			}
			if r.Prompted || r.PromptRequested != tc.want || r.MessageID != nil {
				t.Fatalf("prompted=%v prompt_requested=%v message_id=%v", r.Prompted, r.PromptRequested, r.MessageID)
			}
			if !r.Registered && r.RegistrationError == nil {
				t.Fatalf("registration not attempted: %+v", r)
			}
		})
	}
}

func TestSpawnWaitTimeoutReportsRequestedPrompt(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.Prompt, o.PromptSet = "hi", true
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)}, call{Method: "agent.wait", Err: &herdr.Error{Code: "timeout", Message: "no settled state"}})
	out := s.run(context.Background(), o, nil)
	if r := out.Result.(*Result); out.Status != "partial" || r.Prompted || !r.PromptRequested {
		t.Fatalf("%+v", out)
	}
}

func TestSpawnUnknownPromptSubmissionKeepsRequest(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.Prompt, o.PromptSet = "hi", true
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), senderCall(), call{Method: "agent.prompt", Err: &herdr.Error{Code: "transport_error", Message: "lost", Uncertain: true}})
	out := s.run(context.Background(), o, nil)
	if r := out.Result.(*Result); out.Status != "unknown" || out.Error.Phase != "agent.prompt" || r.Prompted || !r.PromptRequested {
		t.Fatalf("%+v", out)
	}
}

func TestSpawnSubmittedPromptIsRequested(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.Prompt, o.PromptSet = "hi", true
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), senderCall(), call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	out := s.run(context.Background(), o, nil)
	if r := out.Result.(*Result); out.Status != "success" || !r.Prompted || !r.PromptRequested {
		t.Fatalf("%+v", out)
	}
}

func paneOptions() Options {
	o := validOptions()
	o.Pane = "w1:p1"
	return o
}

// Herdr drops the name when its start reservation expires, so agent.start
// always reserves at least 30s while the local budget stays --timeout.
func TestSpawnStartReservationOutlastsShortTimeout(t *testing.T) {
	for _, tc := range []struct {
		timeout       time.Duration
		start, waitMs int64
		noWait        bool
	}{
		{3001 * time.Millisecond, 30000, 3001, false},
		{3001 * time.Millisecond, 30000, 0, true},
		{30 * time.Second, 30000, 30000, false},
		{45 * time.Second, 45000, 45000, false},
		{300 * time.Second, 300000, 300000, false},
	} {
		t.Run(tc.timeout.String(), func(t *testing.T) {
			p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
			o := paneOptions()
			o.Timeout, o.NoWait = tc.timeout, tc.noWait
			calls := []call{{Method: "session.snapshot", Result: snapshot()}, labeled(p), {Method: "agent.start", Params: map[string]any{"name": "worker", "kind": "claude", "pane_id": "w1:p1", "args": []string{}, "timeout_ms": tc.start}, Result: started(p)}}
			if !tc.noWait {
				calls = append(calls, call{Method: "agent.wait", Params: map[string]any{"target": "worker", "timeout_ms": tc.waitMs}, Result: settled(p, "idle")})
			}
			if out := fake(t, calls...).run(context.Background(), o, nil); out.Status != "success" {
				t.Fatalf("%+v %+v", out, out.Error)
			}
		})
	}
}

func TestSpawnLabelsCreatedTabAndPaneWithName(t *testing.T) {
	p := herdrscript.Pane("w1:p2", "w1", "w1:t2")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.current", Result: herdr.PaneResult{Type: "pane_current", Pane: herdrscript.Pane("w1:p1", "w1", "w1:t1")}}, call{Method: "tab.create", Params: map[string]any{"workspace_id": "w1", "focus": false, "label": "worker"}, Result: herdr.CreatedResult{Type: "tab_created", Tab: herdr.Tab{ID: "w1:t2", WorkspaceID: "w1", Label: "worker"}, RootPane: p}}, call{Method: "pane.rename", Params: map[string]any{"pane_id": "w1:p2", "label": "worker"}, Result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	out := s.run(context.Background(), validOptions(), nil)
	if out.Status != "success" || !slices.Contains(out.Effects, libagent.Effect{Action: "updated", Kind: "pane_label", ID: "w1:p2"}) {
		t.Fatalf("%+v", out)
	}
}
func TestSpawnNamesNewWorkspaceInitialTab(t *testing.T) {
	o := validOptions()
	o.Workspace = "new workspace"
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.current", Result: herdr.PaneResult{Type: "pane_current", Pane: herdrscript.Pane("w1:p1", "w1", "w1:t1")}}, call{Method: "workspace.create", Result: herdr.CreatedResult{Type: "workspace_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "1"}, RootPane: p}}, call{Method: "tab.rename", Params: map[string]any{"tab_id": "w2:t1", "label": "worker"}, Result: herdr.TabResult{Type: "tab_info", Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "worker"}}}, call{Method: "pane.rename", Params: map[string]any{"pane_id": "w2:p1", "label": "worker"}, Result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	if out := s.run(context.Background(), o, nil); out.Status != "success" {
		t.Fatalf("%+v", out)
	}
}
func TestSpawnSplitKeepsExistingTabLabel(t *testing.T) {
	p := herdrscript.Pane("w1:p2", "w1", "w1:t1")
	o := validOptions()
	o.Tab = "build"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.current", Result: herdr.PaneResult{Type: "pane_current", Pane: herdrscript.Pane("w1:p1", "w1", "w1:t1")}}, call{Method: "pane.split", Result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{Method: "pane.rename", Params: map[string]any{"pane_id": "w1:p2", "label": "worker"}, Result: herdr.PaneResult{Type: "pane_info", Pane: p}}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	if out := s.run(context.Background(), o, nil); out.Status != "success" {
		t.Fatalf("%+v", out)
	}
}
