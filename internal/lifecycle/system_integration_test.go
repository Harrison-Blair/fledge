package lifecycle

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/watchproc"
)

// TestSystemLifecycleStartSpawnDeliverComplete is an end-to-end lifecycle system
// test. It runs the REAL manager (Start -> Spawn --task -> TransitionTask) against
// a REAL watchproc dispatcher (watchproc.Run, not the nil watchRunner stub the
// other manager tests use) and a fake Herdr that models pane lifecycle. It asserts
// the two seams that carry coordination end to end:
//
//  1. after Spawn(--task), the running dispatcher delivers the task envelope to the
//     worker's pane via PromptAgent, and
//  2. after the worker completes the task, the dispatcher delivers the completion
//     wake — carrying the worker's summary — to the orchestrator's pane.
//
// The dispatcher runs in its own goroutine with the real native ledger watcher
// (WatchFile left to its production default), so an appended coordination event is
// carried to delivery by inotify, not by elapsed time. Synchronization is entirely
// event/deadline driven: the test waits on watchproc.WaitReady and then polls the
// fake's recorded prompts against a bounded deadline, never a bare sleep. The
// Herdr push-event stream is injected (a sanctioned Options seam) because a fake
// Herdr exposes no real event socket; the wake-delivery path under test — ledger
// append -> drain -> PromptAgent — is exercised for real.
func TestSystemLifecycleStartSpawnDeliverComplete(t *testing.T) {
	t.Parallel()

	const (
		taskText = "Review the diff on branch dev"
		summary  = "worker finished reviewing the diff and it looks good"
	)

	root := t.TempDir()
	initTestProject(t, root)

	fake := newSystemFakeHerdr()
	var output bytes.Buffer
	manager := NewManager(fake, &fakeConfirmer{}, nil, &output)
	manager.random = bytes.NewReader(make([]byte, 16))
	// Two distinct 32-byte pane-authority tokens: the orchestrator's is minted
	// first during Start, the worker's second during Spawn. Knowing them lets the
	// test act as each identity by setting FLEDGE_PANE_AUTHORITY.
	orchestratorAuthority := hex.EncodeToString(bytes.Repeat([]byte{0x01}, paneAuthorityBytes))
	workerAuthority := hex.EncodeToString(bytes.Repeat([]byte{0x02}, paneAuthorityBytes))
	manager.authorityRandom = bytes.NewReader(append(
		bytes.Repeat([]byte{0x01}, paneAuthorityBytes),
		bytes.Repeat([]byte{0x02}, paneAuthorityBytes)...,
	))
	manager.lookPath = installedTestHarness
	// No detached watcher daemon: the test owns the one real dispatcher below.
	manager.watchLauncher = func(string) error { return nil }
	manager.watchStopper = func(string, string) error { return nil }
	env := map[string]string{}
	manager.getenv = func(key string) string { return env[key] }

	// Start: create the session and register the orchestrator on pane w1:p1.
	if err := manager.Start(context.Background(), root, StartOptions{
		Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true,
	}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	rec, found, err := readRecord(root)
	if err != nil || !found {
		t.Fatalf("readRecord() = %#v, found %v, err %v", rec, found, err)
	}
	session := rec.SessionName
	store := messaging.New(root, session)

	// Spawn --task as the orchestrator, so the task's assigner is the orchestrator
	// and the completion wake will target it.
	env[paneAuthorityEnvironment] = orchestratorAuthority
	if err := manager.Spawn(context.Background(), root, SpawnOptions{
		Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex", Task: taskText,
	}); err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}

	// The worker's task is now durable; find its ID so the worker can complete it.
	taskID := ""
	tasks, err := store.Tasks()
	if err != nil {
		t.Fatalf("Tasks() error = %v", err)
	}
	for _, task := range tasks {
		if task.Assignee == "worker" && task.Assigner == orchestratorIdentity {
			taskID = task.ID
		}
	}
	if taskID == "" {
		t.Fatalf("no task assigned by orchestrator to worker; tasks = %#v", tasks)
	}

	// Bring up the real dispatcher against this session.
	dispatcherCtx, cancelDispatcher := context.WithCancel(context.Background())
	dispatcherDone := make(chan error, 1)
	go func() {
		dispatcherDone <- watchproc.Run(dispatcherCtx, watchproc.Options{
			Root: root, Session: session, Herdr: fake,
			// Native ledger watcher (real fswatch) left to its production default;
			// only the Herdr push-event stream is injected, since a fake Herdr has no
			// real event socket to dial.
			Subscribe: func(streamCtx context.Context, _ []string, onReady func(), _ func(herdr.Event)) error {
				onReady()
				<-streamCtx.Done()
				return streamCtx.Err()
			},
		})
	}()
	t.Cleanup(func() {
		cancelDispatcher()
		select {
		case err := <-dispatcherDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("dispatcher exited with %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("dispatcher did not stop")
		}
	})

	readyCtx, cancelReady := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelReady()
	if err := watchproc.WaitReady(readyCtx, root, session); err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}

	// Assertion 1: the dispatcher delivered the task envelope to the worker's pane.
	workerPrompts := awaitPrompt(t, fake, dispatcherDone, "worker")
	if !strings.Contains(workerPrompts[len(workerPrompts)-1], taskText) {
		t.Fatalf("worker envelope = %q, want it to carry the task text %q",
			workerPrompts[len(workerPrompts)-1], taskText)
	}

	// The worker completes the task. Acting as the worker requires its pane
	// authority; the transition appends a completion wake for the orchestrator,
	// and the real ledger watcher carries that append to the dispatcher's drain.
	env[paneAuthorityEnvironment] = workerAuthority
	if _, err := manager.TaskTransition(context.Background(), root, taskID, messaging.TaskCompleted, summary); err != nil {
		t.Fatalf("TaskTransition(complete) error = %v", err)
	}

	// Assertion 2: the orchestrator was woken with the completion carrying the summary.
	orchestratorPrompts := awaitPrompt(t, fake, dispatcherDone, orchestratorIdentity)
	last := orchestratorPrompts[len(orchestratorPrompts)-1]
	if !strings.Contains(last, summary) {
		t.Fatalf("orchestrator wake = %q, want it to carry the completion summary %q", last, summary)
	}
	if !strings.Contains(last, taskID) {
		t.Fatalf("orchestrator wake = %q, want it to reference task %q", last, taskID)
	}
}

// awaitPrompt polls the fake's recorded prompts for one addressed to recipient,
// bounded by a deadline, failing fast if the dispatcher exits first. Only the test
// needs this bound; the dispatcher itself is driven purely by events.
func awaitPrompt(t *testing.T, fake *systemFakeHerdr, done <-chan error, recipient string) []string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if prompts := fake.promptsFor(recipient); len(prompts) > 0 {
			return prompts
		}
		select {
		case err := <-done:
			t.Fatalf("dispatcher exited before delivering to %q: %v", recipient, err)
		case <-deadline:
			t.Fatalf("no prompt delivered to %q before deadline", recipient)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// systemFakeHerdr models just enough Herdr pane lifecycle for one Start + Spawn to
// register the orchestrator and worker, and serves the dispatcher a live-pane
// snapshot so its reconciliation retires nothing. It is fully mutex-guarded: the
// manager (main goroutine) and the dispatcher (its own goroutines) both call it.
type systemFakeHerdr struct {
	mu          sync.Mutex
	sessionName string
	running     bool
	livePanes   map[string]bool
	promptCalls []promptCall
}

func newSystemFakeHerdr() *systemFakeHerdr {
	return &systemFakeHerdr{livePanes: map[string]bool{}}
}

func (f *systemFakeHerdr) promptsFor(recipient string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, call := range f.promptCalls {
		if call.recipient == recipient {
			out = append(out, call.prompt)
		}
	}
	return out
}

func (f *systemFakeHerdr) snapshot() herdr.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, 0, len(f.livePanes))
	for id := range f.livePanes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	panes := make([]herdr.Pane, 0, len(ids))
	for _, id := range ids {
		panes = append(panes, herdr.Pane{PaneID: id, TabID: "t1", WorkspaceID: "w1"})
	}
	return herdr.Snapshot{
		FocusedWorkspaceID: "w1", FocusedTabID: "t1", FocusedPaneID: "w1:p1",
		Workspaces: []herdr.Workspace{{WorkspaceID: "w1"}},
		Tabs:       []herdr.Tab{{TabID: "t1", WorkspaceID: "w1"}},
		Panes:      panes,
	}
}

func (f *systemFakeHerdr) Check() error { return nil }

func (f *systemFakeHerdr) Attach(context.Context, string, string) error { return nil }

func (f *systemFakeHerdr) StartServer(session, _ string, _ map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessionName = session
	f.running = true
	return nil
}

// WaitReady returns a fixed fresh-session layout with the orchestrator's root pane
// present, so initialLayout selects it during Start regardless of the live-pane set.
func (f *systemFakeHerdr) WaitReady(context.Context, string, time.Duration) (herdr.Snapshot, error) {
	return testSnapshot(), nil
}

func (f *systemFakeHerdr) Snapshot(context.Context, string) (herdr.Snapshot, error) {
	return f.snapshot(), nil
}

func (f *systemFakeHerdr) CreateWorkspace(context.Context, string, string, string) (herdr.Workspace, herdr.Tab, herdr.Pane, error) {
	return herdr.Workspace{WorkspaceID: "w1"},
		herdr.Tab{TabID: "t1", WorkspaceID: "w1"},
		herdr.Pane{PaneID: "w1:p1", TabID: "t1", WorkspaceID: "w1"}, nil
}

func (f *systemFakeHerdr) RenameTab(context.Context, string, string, string) error  { return nil }
func (f *systemFakeHerdr) RenamePane(context.Context, string, string, string) error { return nil }

func (f *systemFakeHerdr) SplitPane(context.Context, string, string, string, map[string]string) (herdr.Pane, error) {
	return herdr.Pane{}, nil
}

func (f *systemFakeHerdr) CreateTab(_ context.Context, _, _, _, _ string, _ map[string]string) (herdr.Tab, herdr.Pane, error) {
	return herdr.Tab{TabID: "t2", WorkspaceID: "w1", Label: "worker"},
		herdr.Pane{PaneID: "w1:p2", TabID: "t2", WorkspaceID: "w1"}, nil
}

func (f *systemFakeHerdr) CloseTab(context.Context, string, string) error  { return nil }
func (f *systemFakeHerdr) ClosePane(context.Context, string, string) error { return nil }
func (f *systemFakeHerdr) FocusAgent(context.Context, string, string) error {
	return nil
}

// StartAgent brings the named agent's pane live, so a later Snapshot advertises it
// and the dispatcher's reconciliation keeps it registered.
func (f *systemFakeHerdr) StartAgent(_ context.Context, _, _, _, pane string, _ time.Duration, _ []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.livePanes[pane] = true
	return nil
}

func (f *systemFakeHerdr) PromptAgent(_ context.Context, session, recipient, prompt string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.promptCalls = append(f.promptCalls, promptCall{session: session, recipient: recipient, prompt: prompt})
	return nil
}

func (f *systemFakeHerdr) List(context.Context) ([]herdr.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.running {
		return nil, nil
	}
	return []herdr.Session{{Name: f.sessionName, Running: true}}, nil
}

func (f *systemFakeHerdr) Protocol(context.Context) (int, error) {
	return watchproc.RequiredHerdrProtocol, nil
}

func (f *systemFakeHerdr) Stop(context.Context, string) error   { return nil }
func (f *systemFakeHerdr) Delete(context.Context, string) error { return nil }
