package messaging

import (
	"errors"
	"testing"
)

// registerAll is the common two-agent setup: a coordinator that may delegate
// and one worker holding a single assignment.
func registerAll(t *testing.T, store *Store, params ...RegisterParams) {
	t.Helper()
	for _, p := range params {
		if _, _, err := store.RegisterAgent(p); err != nil {
			t.Fatalf("RegisterAgent(%s): %v", p.Name, err)
		}
	}
}

// A worker owns its own outcome. Losing the agent that assigned the work is a
// reason to skip the wake, never a reason to refuse the durable transition:
// otherwise a worker whose coordinator crashed can no longer record that it
// finished, failed, or is blocked, and the task is stuck active forever.
func TestTransitionSurvivesTheLossOfTheWakeRecipient(t *testing.T) {
	for _, target := range []TaskStatus{TaskCompleted, TaskFailed, TaskBlocked} {
		t.Run(string(target), func(t *testing.T) {
			store := coordinationTestStore(t)
			registerAll(t, store,
				RegisterParams{Name: OrchestratorIdentity, PaneID: "p1", Harness: "codex", Caller: UserIdentity, CanDelegate: true},
				RegisterParams{Name: "worker", PaneID: "p2", Harness: "codex", Caller: OrchestratorIdentity})
			task, err := store.AssignTask(OrchestratorIdentity, "worker", "", "do the thing")
			if err != nil {
				t.Fatal(err)
			}
			if err := store.StopAgent(OrchestratorIdentity, "p1"); err != nil {
				t.Fatal(err)
			}
			updated, err := store.TransitionTask("worker", task.ID, target, "detail")
			if err != nil {
				t.Fatalf("TransitionTask(%s) after assigner loss: %v", target, err)
			}
			if updated.Status != target {
				t.Fatalf("status = %s, want %s", updated.Status, target)
			}
			// The departed coordinator must not be left an undeliverable wake for
			// the dispatcher to replay against a dead pane.
			wakes, err := store.PendingWakes()
			if err != nil {
				t.Fatal(err)
			}
			for _, wake := range wakes {
				if wake.Recipient == OrchestratorIdentity {
					t.Fatalf("queued a wake for the stopped coordinator: %#v", wake)
				}
			}
		})
	}
}

// Stopping a worker must orphan its work and deactivate its pane even when the
// agent that assigned that work is already gone. A failure here leaves the
// registry claiming a closed pane is live, which keeps the dispatcher
// subscribed to it.
func TestStopAgentOrphansWorkWhenTheAssignerIsGone(t *testing.T) {
	store := coordinationTestStore(t)
	registerAll(t, store,
		RegisterParams{Name: OrchestratorIdentity, PaneID: "p1", Harness: "codex", Caller: UserIdentity, CanDelegate: true},
		RegisterParams{Name: "worker", PaneID: "p2", Harness: "codex", Caller: OrchestratorIdentity})
	task, err := store.AssignTask(OrchestratorIdentity, "worker", "", "do the thing")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StopAgent(OrchestratorIdentity, "p1"); err != nil {
		t.Fatal(err)
	}
	if err := store.StopAgent("worker", "p2"); err != nil {
		t.Fatalf("StopAgent(worker) with a departed assigner: %v", err)
	}
	orphaned := taskByID(t, store, task.ID)
	if orphaned.Status != TaskOrphaned {
		t.Fatalf("task = %#v", orphaned)
	}
	if _, err := store.Agent("worker"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("worker still active: %v", err)
	}
}

// Orphaning is the assigner's news: they are the one still waiting on work that
// nobody is doing any more.
func TestOrphaningWakesTheAssigner(t *testing.T) {
	store := coordinationTestStore(t)
	registerAll(t, store,
		RegisterParams{Name: OrchestratorIdentity, PaneID: "p1", Harness: "codex", Caller: UserIdentity, CanDelegate: true},
		RegisterParams{Name: "lead", PaneID: "p2", Harness: "codex", Caller: OrchestratorIdentity, CanDelegate: true, Task: "lead work"},
		RegisterParams{Name: "child", PaneID: "p3", Harness: "codex", Caller: OrchestratorIdentity})
	tasks, err := store.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.AssignTask("lead", "child", tasks[0].ID, "child work")
	if err != nil {
		t.Fatal(err)
	}
	settleWakes(t, store)
	if err := store.StopAgent("child", "p3"); err != nil {
		t.Fatal(err)
	}
	orphaned := taskByID(t, store, child.ID)
	if orphaned.Status != TaskOrphaned {
		t.Fatalf("task = %#v", orphaned)
	}
	wakes, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	if len(wakes) != 1 || wakes[0].Recipient != "lead" || wakes[0].Kind != "task-orphaned" {
		t.Fatalf("wakes = %#v, want one task-orphaned for lead", wakes)
	}
}

// The registry is the only authority record commands have, so a stopped agent's
// pane must remain resolvable to that agent rather than looking unknown.
func TestAgentByPaneAnyKeepsStoppedAndReusedPanesDistinguishable(t *testing.T) {
	store := coordinationTestStore(t)
	registerAll(t, store,
		RegisterParams{Name: OrchestratorIdentity, PaneID: "p1", Harness: "codex", Caller: UserIdentity, CanDelegate: true},
		RegisterParams{Name: "worker", PaneID: "p2", Harness: "codex", Caller: OrchestratorIdentity})
	if err := store.StopAgent("worker", "p2"); err != nil {
		t.Fatal(err)
	}
	stopped, err := store.AgentByPaneAny("p2")
	if err != nil || stopped.Name != "worker" || stopped.Active {
		t.Fatalf("AgentByPaneAny(stopped) = %#v, %v", stopped, err)
	}
	if _, err := store.AgentByPane("p2"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("AgentByPane() still resolves a stopped pane: %v", err)
	}
	// A pane recycled by Herdr for a new agent must resolve to the live one.
	registerAll(t, store, RegisterParams{Name: "successor", PaneID: "p2", Harness: "codex", Caller: OrchestratorIdentity})
	reused, err := store.AgentByPaneAny("p2")
	if err != nil || reused.Name != "successor" || !reused.Active {
		t.Fatalf("AgentByPaneAny(reused) = %#v, %v", reused, err)
	}
	if _, err := store.AgentByPaneAny("p-unknown"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("AgentByPaneAny(unknown) = %v", err)
	}
}

// The wake matrix: who is woken for which transition, and which transitions
// deliberately wake nobody.
func TestWakeMatrixRoutesEachTransitionToOneParticipant(t *testing.T) {
	type step struct {
		name      string
		run       func(*testing.T, *Store, string) error
		wantKind  string
		wantWoken string
	}
	steps := []step{
		{name: "assign wakes the assignee", wantKind: "task-assigned", wantWoken: "worker"},
		{
			name: "progress wakes nobody",
			run: func(_ *testing.T, s *Store, id string) error {
				_, err := s.RecordProgress("worker", id, "halfway")
				return err
			},
		},
		{
			name: "blocked wakes the assigner",
			run: func(_ *testing.T, s *Store, id string) error {
				_, err := s.TransitionTask("worker", id, TaskBlocked, "stuck")
				return err
			},
			wantKind:  "task-blocked",
			wantWoken: OrchestratorIdentity,
		},
		{
			name: "needs-decision wakes the assigner",
			run: func(_ *testing.T, s *Store, id string) error {
				_, err := s.TransitionTask("worker", id, TaskNeedsDecision, "which?")
				return err
			},
			wantKind:  "task-needs-decision",
			wantWoken: OrchestratorIdentity,
		},
		{
			name: "resume wakes the assignee",
			run: func(t *testing.T, s *Store, id string) error {
				if _, err := s.TransitionTask("worker", id, TaskBlocked, "stuck"); err != nil {
					return err
				}
				settleWakes(t, s)
				_, err := s.TransitionTask(OrchestratorIdentity, id, TaskActive, "go")
				return err
			},
			wantKind:  "task-resumed",
			wantWoken: "worker",
		},
		{
			name: "complete wakes the assigner",
			run: func(_ *testing.T, s *Store, id string) error {
				_, err := s.TransitionTask("worker", id, TaskCompleted, "")
				return err
			},
			wantKind:  "task-completed",
			wantWoken: OrchestratorIdentity,
		},
		{
			name: "cancel wakes the assignee",
			run: func(_ *testing.T, s *Store, id string) error {
				_, err := s.TransitionTask(OrchestratorIdentity, id, TaskCanceled, "")
				return err
			},
			wantKind:  "task-canceled",
			wantWoken: "worker",
		},
		{
			name: "fail wakes the assigner",
			run: func(_ *testing.T, s *Store, id string) error {
				_, err := s.TransitionTask("worker", id, TaskFailed, "broke")
				return err
			},
			wantKind:  "task-failed",
			wantWoken: OrchestratorIdentity,
		},
	}
	for _, current := range steps {
		t.Run(current.name, func(t *testing.T) {
			store := coordinationTestStore(t)
			registerAll(t, store,
				RegisterParams{Name: OrchestratorIdentity, PaneID: "p1", Harness: "codex", Caller: UserIdentity, CanDelegate: true},
				RegisterParams{Name: "worker", PaneID: "p2", Harness: "codex", Caller: OrchestratorIdentity})
			task, err := store.AssignTask(OrchestratorIdentity, "worker", "", "do the thing")
			if err != nil {
				t.Fatal(err)
			}
			if current.run != nil {
				// Settle the assignment wake so only the transition under test is left.
				settleWakes(t, store)
				if err := current.run(t, store, task.ID); err != nil {
					t.Fatal(err)
				}
			}
			wakes, err := store.PendingWakes()
			if err != nil {
				t.Fatal(err)
			}
			if current.wantWoken == "" {
				if len(wakes) != 0 {
					t.Fatalf("pending wakes = %#v, want none", wakes)
				}
				return
			}
			if len(wakes) != 1 {
				t.Fatalf("pending wakes = %#v, want exactly one", wakes)
			}
			if wakes[0].Recipient != current.wantWoken || wakes[0].Kind != current.wantKind {
				t.Fatalf("wake = %s/%s, want %s/%s", wakes[0].Kind, wakes[0].Recipient, current.wantKind, current.wantWoken)
			}
			if wakes[0].ReferenceID != task.ID {
				t.Fatalf("wake reference = %s, want %s", wakes[0].ReferenceID, task.ID)
			}
		})
	}
}

// Cascade cancellation must reach every descendant and wake each one's own
// assignee, since they are the agents that have to stop working.
func TestCascadeCancelWakesEveryDescendantAssignee(t *testing.T) {
	store := coordinationTestStore(t)
	registerAll(t, store,
		RegisterParams{Name: OrchestratorIdentity, PaneID: "p1", Harness: "codex", Caller: UserIdentity, CanDelegate: true},
		RegisterParams{Name: "lead", PaneID: "p2", Harness: "codex", Caller: OrchestratorIdentity, CanDelegate: true, Task: "lead work"},
		RegisterParams{Name: "child", PaneID: "p3", Harness: "codex", Caller: OrchestratorIdentity})
	tasks, err := store.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	parent := tasks[0]
	child, err := store.AssignTask("lead", "child", parent.ID, "child work")
	if err != nil {
		t.Fatal(err)
	}
	settleWakes(t, store)
	if _, err := store.TransitionTask(OrchestratorIdentity, parent.ID, TaskCanceled, ""); err != nil {
		t.Fatal(err)
	}
	canceled := taskByID(t, store, child.ID)
	if canceled.Status != TaskCanceled {
		t.Fatalf("child = %#v", canceled)
	}
	woken := map[string]string{}
	wakes, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	for _, wake := range wakes {
		woken[wake.Recipient] = wake.ReferenceID
	}
	if woken["lead"] != parent.ID || woken["child"] != child.ID || len(woken) != 2 {
		t.Fatalf("cascade wakes = %#v", woken)
	}
}

func settleWakes(t *testing.T, store *Store) {
	t.Helper()
	wakes, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	for _, wake := range wakes {
		if _, err := store.RecordWakeAttempt(wake.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := store.RecordWakeOutcome(wake.ID, true, ""); err != nil {
			t.Fatal(err)
		}
	}
}
