package fledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

var testSpawnSelection = state.SpawnSelection{Harness: "codex", Model: "gpt-5.6"}

// The paste-settle before delivery's submitting enter only slows tests down.
func init() { promptSubmitSettle = time.Millisecond }

// seedDisposableState fills every field a coordinated stop is expected to
// clear, so assertDisposableStateCleared has something to prove. The active
// run is a real one: a fabricated ID would make the run close a no-op and
// leave the ActiveRunID assertion unproven.
func seedDisposableState(t *testing.T, service *Service, generation uint64) {
	t.Helper()
	startTestMessageRun(t, service)
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.StopGeneration = generation
		st.Socket = "/stale/socket"
		st.WorkspaceID = "stale-workspace"
		st.OrchestratorTabID = "stale-tab"
		st.OrchestratorPaneID = "stale-pane"
		st.OrchestratorInitialized = true
		selection := testSpawnSelection
		st.LastSpawnSelection = &selection
		st.Agents["worker"] = state.Agent{Name: "worker", PaneID: "stale-agent-pane"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// assertDisposableStateCleared is the counterpart of seedDisposableState: every
// disposable field is empty while durable picker history survives.
func assertDisposableStateCleared(
	t *testing.T,
	st state.Session,
	wantGeneration uint64,
	wantSelection *state.SpawnSelection,
) {
	t.Helper()
	if st.StopGeneration != wantGeneration || st.Socket != "" || st.WorkspaceID != "" ||
		st.OrchestratorTabID != "" || st.OrchestratorPaneID != "" ||
		st.OrchestratorInitialized || st.ActiveRunID != "" || len(st.Agents) != 0 {
		t.Fatalf("disposable state was not cleared at generation %d: %#v", wantGeneration, st)
	}
	if (st.LastSpawnSelection == nil) != (wantSelection == nil) ||
		st.LastSpawnSelection != nil && *st.LastSpawnSelection != *wantSelection {
		t.Fatalf("spawn selection after cleanup = %#v, want %#v", st.LastSpawnSelection, wantSelection)
	}
}

// mustStartAgent spawns name into its own tab, the arrangement almost every
// agent-facing test needs before exercising anything else. After the parent
// spawn returns it stands in for the in-pane child the injected bootstrap
// command starts: a current-pane SpawnAgent that claims the pane, activates
// messaging, and hands the pane to the (stubbed) harness exec.
func mustStartAgent(t *testing.T, service *Service, name string) AgentStartResult {
	t.Helper()
	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: name, Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	previousExec := service.ExecAgent
	service.ExecAgent = func(string, []string, []string) error { return nil }
	defer func() { service.ExecAgent = previousExec }()
	if _, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: name, Kind: "codex", CurrentPaneID: result.Agent.PaneID,
		Executable: "/usr/bin/codex", Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

// launchTestCleanupWorker returns a LaunchStopCleanup that runs the detached
// finalizer in-process and reports its result on done. A non-zero timeout
// overrides the one the request carries.
func launchTestCleanupWorker(done chan<- error, timeout time.Duration) func(StopCleanupRequest) error {
	return func(request StopCleanupRequest) error {
		workerStore, err := state.New(request.StateDir)
		if err != nil {
			return err
		}
		worker := &Service{
			Project: project.Info{Root: request.ProjectRoot, Session: request.Session},
			Binary:  herdr.Binary{Path: request.HerdrBinary},
			Store:   workerStore,
		}
		workerTimeout := request.Timeout
		if timeout != 0 {
			workerTimeout = timeout
		}
		go func() {
			done <- worker.FinalizeStop(context.Background(), request.BaseGeneration, workerTimeout)
		}()
		return nil
	}
}

// corruptMessageRun appends an unparseable record to a run log so the next
// read fails with messaging.ErrCorrupt.
func corruptMessageRun(t *testing.T, service *Service, runID string) {
	t.Helper()
	path := filepath.Join(service.messageStore().Dir, runID+".jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("{not json\n"); err != nil {
		t.Fatal(err)
	}
}
