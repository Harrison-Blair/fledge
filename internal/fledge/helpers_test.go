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
		st.Agents["worker"] = state.Agent{Name: "worker", PaneID: "stale-agent-pane"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// assertDisposableStateCleared is the counterpart of seedDisposableState: every
// disposable field is empty and only the stop generation survived.
func assertDisposableStateCleared(t *testing.T, st state.Session, wantGeneration uint64) {
	t.Helper()
	if st.StopGeneration != wantGeneration || st.Socket != "" || st.WorkspaceID != "" ||
		st.OrchestratorTabID != "" || st.OrchestratorPaneID != "" ||
		st.OrchestratorInitialized || st.ActiveRunID != "" || len(st.Agents) != 0 {
		t.Fatalf("disposable state was not cleared at generation %d: %#v", wantGeneration, st)
	}
}

// mustStartAgent spawns name into its own tab, the arrangement almost every
// agent-facing test needs before exercising anything else.
func mustStartAgent(t *testing.T, service *Service, name string) AgentStartResult {
	t.Helper()
	result, err := service.SpawnAgent(t.Context(), AgentStartOptions{
		Name: name, Kind: "codex", NewTab: true, Timeout: 30 * time.Second,
	})
	if err != nil {
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
