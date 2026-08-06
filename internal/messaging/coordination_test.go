package messaging

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentAuthorityHashIsDurableAndValidated(t *testing.T) {
	store := coordinationTestStore(t)
	want := strings.Repeat("ab", 32)
	agent, _, err := store.RegisterAgent(RegisterParams{
		Name: "worker", PaneID: "p1", Harness: "codex", AuthorityHash: want, Caller: UserIdentity,
	})
	if err != nil || agent.AuthorityHash != want {
		t.Fatalf("RegisterAgent() = %#v, %v", agent, err)
	}
	resolved, err := store.AgentByAuthorityHashAny(want)
	if err != nil || resolved.Name != "worker" || resolved.PaneID != "p1" {
		t.Fatalf("AgentByAuthorityHashAny() = %#v, %v", resolved, err)
	}
	if _, _, err := store.RegisterAgent(RegisterParams{
		Name: "bad", PaneID: "p2", Harness: "codex", AuthorityHash: "not-a-sha256", Caller: UserIdentity,
	}); err == nil {
		t.Fatal("RegisterAgent() accepted an invalid authority hash")
	}
}

func TestAtomicAgentRegistrationInitialTaskAndWake(t *testing.T) {
	store := coordinationTestStore(t)
	if _, _, err := store.RegisterAgent(RegisterParams{Name: "orchestrator", PaneID: "p1", Harness: "codex", Caller: "user", CanDelegate: true}); err != nil {
		t.Fatal(err)
	}
	agent, task, err := store.RegisterAgent(RegisterParams{Name: "worker", PaneID: "p2", Harness: "claude", Caller: "orchestrator", Task: "review", CanDelegate: true})
	if err != nil {
		t.Fatal(err)
	}
	if agent.PaneID != "p2" || task == nil || task.Assignee != "worker" || task.Status != TaskActive {
		t.Fatalf("agent/task = %#v / %#v", agent, task)
	}
	wakes, err := store.PendingWakes()
	if err != nil || len(wakes) != 1 || wakes[0].ReferenceID != task.ID || wakes[0].RecipientPane != "p2" {
		t.Fatalf("wakes = %#v, %v", wakes, err)
	}
	if filepath.Base(store.LogPath()) != "events.jsonl" {
		t.Fatalf("ledger = %s", store.LogPath())
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(store.LogPath()), "messages.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second ledger exists: %v", err)
	}
}

func TestDelegationLineageCapacityTransitionsCascadeAndOrphan(t *testing.T) {
	store := coordinationTestStore(t)
	for _, p := range []RegisterParams{
		{Name: "orchestrator", PaneID: "p1", Harness: "codex", Caller: "user", CanDelegate: true},
		{Name: "lead", PaneID: "p2", Harness: "codex", Caller: "orchestrator", CanDelegate: true, Task: "lead"},
		{Name: "child", PaneID: "p3", Harness: "codex", Caller: "orchestrator"},
		{Name: "other", PaneID: "p4", Harness: "codex", Caller: "orchestrator"},
	} {
		if _, _, err := store.RegisterAgent(p); err != nil {
			t.Fatal(err)
		}
	}
	tasks, _ := store.Tasks()
	parent := tasks[0]
	child, err := store.AssignTask("lead", "child", parent.ID, "child work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignTask("lead", "child", parent.ID, "overflow"); !errors.Is(err, ErrCapacity) {
		t.Fatalf("capacity error = %v", err)
	}
	if _, err := store.AssignTask("child", "other", child.ID, "forbidden"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("authorization error = %v", err)
	}
	if _, err := store.TransitionTask("child", child.ID, TaskBlocked, "need input"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionTask("lead", child.ID, TaskActive, "answered"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionTask("lead", parent.ID, TaskCompleted, ""); err == nil {
		t.Fatal("completed parent with active child")
	}
	if _, err := store.TransitionTask("lead", parent.ID, TaskFailed, "failed"); err != nil {
		t.Fatal(err)
	}
	child, _ = store.Task(child.ID)
	if child.Status != TaskCanceled {
		t.Fatalf("child status = %s", child.Status)
	}
	assigned, err := store.AssignTask("orchestrator", "other", "", "orphan me")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StopAgent("other", "p4"); err != nil {
		t.Fatal(err)
	}
	assigned, _ = store.Task(assigned.ID)
	if assigned.Status != TaskOrphaned {
		t.Fatalf("orphan status = %s", assigned.Status)
	}
}

func coordinationTestStore(t *testing.T) *Store {
	t.Helper()
	store := New(t.TempDir(), "fledge-test-1234abcd")
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	return store
}
