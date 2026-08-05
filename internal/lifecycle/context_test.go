package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcontext"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/statedir"
)

const (
	claudeMiniTranscript = `{"type":"assistant","message":{"model":"claude-opus-4-8","usage":{"input_tokens":2,"cache_creation_input_tokens":1000,"cache_read_input_tokens":20000,"output_tokens":8888}},"timestamp":"2026-08-04T23:00:00Z"}` + "\n"
	codexMiniTranscript  = `{"type":"event_msg","timestamp":"2026-08-04T23:30:00Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":18194,"output_tokens":92},"model_context_window":258400}}}` + "\n"
)

// injectContextDeps wires the Manager's context collection to an in-memory
// filesystem so the test never touches a real harness store. The clock is
// fixed and the home is a stable stub, mirroring how ProductionDeps is shaped.
func injectContextDeps(manager *Manager) {
	home := "/home/u"
	files := map[string]string{
		filepath.Join(home, ".claude", "projects", "proj", "claude-id.jsonl"):                                       claudeMiniTranscript,
		filepath.Join(home, ".codex", "sessions", "2026", "08", "04", "rollout-2026-08-04T19-50-16-codex-id.jsonl"): codexMiniTranscript,
	}
	manager.homeDir = func() (string, error) { return home, nil }
	manager.contextDeps = func(_ context.Context, h string) agentcontext.Deps {
		return agentcontext.Deps{
			Home: h,
			Now:  func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) },
			ReadFile: func(path string) ([]byte, error) {
				contents, ok := files[path]
				if !ok {
					return nil, os.ErrNotExist
				}
				return []byte(contents), nil
			},
			Glob: func(pattern string) ([]string, error) {
				var matches []string
				for path := range files {
					if ok, err := filepath.Match(pattern, path); err == nil && ok {
						matches = append(matches, path)
					}
				}
				sort.Strings(matches)
				return matches, nil
			},
		}
	}
}

func contextSnapshot() herdr.Snapshot {
	orchestrator, worker := "orchestrator", "worker"
	claude, codex := "claude", "codex"
	return herdr.Snapshot{
		Workspaces: []herdr.Workspace{{WorkspaceID: "w1"}},
		Tabs:       []herdr.Tab{{TabID: "t1", WorkspaceID: "w1"}},
		Panes:      []herdr.Pane{{PaneID: "w1:p1", TabID: "t1", WorkspaceID: "w1"}},
		Agents: []herdr.Agent{
			{Name: &worker, Agent: &codex, Revision: 7, AgentSession: &herdr.AgentSession{Agent: "codex", Kind: "id", Value: "codex-id"}},
			{Name: &orchestrator, Agent: &claude, Revision: 3, AgentSession: &herdr.AgentSession{Agent: "claude", Kind: "id", Value: "claude-id"}},
			// An anonymous split pane must be ignored: it is not a Fledge agent.
			{Name: nil, Agent: &codex, Revision: 1},
		},
	}
}

func newContextManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	writeTestRecord(t, root)
	manager, _ := newTestManager(&fakeHerdr{
		sessions: []herdr.Session{{Name: testSessionName, Running: true}},
		snapshot: contextSnapshot(),
	}, &fakeConfirmer{})
	injectContextDeps(manager)
	return manager, root
}

func TestManagerContextBuildsSortedReportAndPersists(t *testing.T) {
	t.Parallel()
	manager, root := newContextManager(t)

	report, err := manager.Context(context.Background(), root, "")
	if err != nil {
		t.Fatalf("Context() error = %v", err)
	}
	if len(report.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2 (anonymous pane excluded)", len(report.Agents))
	}
	// Sorted by name, orchestrator retained, correlation revision preserved.
	if report.Agents[0].Name != "orchestrator" || report.Agents[0].Revision != 3 {
		t.Errorf("first agent = %+v, want orchestrator rev 3", report.Agents[0])
	}
	if report.Agents[0].Used == nil || *report.Agents[0].Used != 21002 {
		t.Errorf("orchestrator used = %v, want 21002", report.Agents[0].Used)
	}
	if report.Agents[1].Name != "worker" || report.Agents[1].Window == nil || *report.Agents[1].Window != 258400 {
		t.Errorf("worker = %+v, want window 258400", report.Agents[1])
	}

	// The full report is persisted at 0600 under the session context dir.
	persisted, ok, err := agentcontext.Load(statedir.Context(root, testSessionName))
	if err != nil || !ok {
		t.Fatalf("Load persisted report: ok %v err %v", ok, err)
	}
	if len(persisted.Agents) != 2 {
		t.Errorf("persisted agents = %d, want 2", len(persisted.Agents))
	}
}

func TestManagerContextNarrowsByName(t *testing.T) {
	t.Parallel()
	manager, root := newContextManager(t)

	report, err := manager.Context(context.Background(), root, "worker")
	if err != nil {
		t.Fatalf("Context() error = %v", err)
	}
	if len(report.Agents) != 1 || report.Agents[0].Name != "worker" {
		t.Fatalf("narrowed report = %+v, want only worker", report.Agents)
	}
	// Even a narrowed query persists the full snapshot.
	persisted, _, err := agentcontext.Load(statedir.Context(root, testSessionName))
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.Agents) != 2 {
		t.Errorf("persisted agents = %d, want the full 2", len(persisted.Agents))
	}
}

func TestManagerContextUnknownNameErrors(t *testing.T) {
	t.Parallel()
	manager, root := newContextManager(t)

	if _, err := manager.Context(context.Background(), root, "ghost"); err == nil {
		t.Error("Context() error = nil, want an unknown-agent error")
	}
}

func TestSpawnRefreshesPersistedContext(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestRecord(t, root)
	client := &fakeHerdr{
		sessions:    []herdr.Session{{Name: testSessionName, Running: true}},
		snapshot:    contextSnapshot(),
		createdTab:  herdr.Tab{TabID: "t2", WorkspaceID: "w1", Label: "worker"},
		createdPane: herdr.Pane{PaneID: "w1:p2", TabID: "t2", WorkspaceID: "w1"},
	}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness
	manager.getenv = func(string) string { return "" }
	injectContextDeps(manager)

	if err := manager.Spawn(context.Background(), root, SpawnOptions{Name: "newbie", Harness: "codex", Timeout: 60 * time.Second}); err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	// The best-effort refresh must leave a persisted report behind.
	if _, ok, err := agentcontext.Load(statedir.Context(root, testSessionName)); err != nil || !ok {
		t.Fatalf("expected a persisted report after Spawn: ok %v err %v", ok, err)
	}
}

func TestManagerContextRequiresRunningSession(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestRecord(t, root)
	manager, _ := newTestManager(&fakeHerdr{
		sessions: []herdr.Session{{Name: testSessionName, Running: false}},
		snapshot: contextSnapshot(),
	}, &fakeConfirmer{})
	injectContextDeps(manager)

	if _, err := manager.Context(context.Background(), root, ""); err == nil {
		t.Error("Context() error = nil, want a not-running error")
	}
}
