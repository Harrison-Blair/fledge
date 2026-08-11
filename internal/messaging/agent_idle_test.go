package messaging

import (
	"strings"
	"testing"
	"time"
)

type supervisionFixture struct {
	store *Store
	now   time.Time
}

func newSupervisionFixture(t *testing.T) *supervisionFixture {
	t.Helper()
	fixture := &supervisionFixture{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	fixture.store = New(t.TempDir(), "fledge-test-1234abcd", WithClock(func() time.Time { return fixture.now }))
	if _, err := fixture.store.Initialize(); err != nil {
		t.Fatal(err)
	}
	registerAll(t, fixture.store,
		RegisterParams{Name: OrchestratorIdentity, PaneID: "p1", Harness: "codex", Caller: UserIdentity, CanDelegate: true},
		RegisterParams{Name: "worker", PaneID: "p2", Harness: "codex", Caller: OrchestratorIdentity})
	return fixture
}

func (f *supervisionFixture) assign(t *testing.T, worker string) Task {
	t.Helper()
	task, err := f.store.AssignTask(OrchestratorIdentity, worker, "", "do the thing")
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func wakeForKind(t *testing.T, store *Store, taskID, kind string) Wake {
	t.Helper()
	wakes, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	for _, wake := range wakes {
		if wake.ReferenceID == taskID && wake.Kind == kind {
			return wake
		}
	}
	t.Fatalf("no pending %s wake for %s in %#v", kind, taskID, wakes)
	return Wake{}
}

func settleWake(t *testing.T, store *Store, wake Wake, accepted bool) {
	t.Helper()
	if _, err := store.RecordWakeAttempt(wake.ID); err != nil {
		t.Fatal(err)
	}
	detail := ""
	if !accepted {
		detail = "delivery refused"
	}
	if _, err := store.RecordWakeOutcome(wake.ID, accepted, detail); err != nil {
		t.Fatal(err)
	}
}

func TestAgentIdleGraceStartsAtTerminalDeliveryAndAuditsOnceAtBoundary(t *testing.T) {
	fixture := newSupervisionFixture(t)
	task := fixture.assign(t, "worker")
	assignment := wakeForKind(t, fixture.store, task.ID, "task-assigned")

	// Pre-start idle is persisted but does not produce the old premature wake.
	if err := fixture.store.RecordAgentStatus("p2", "idle"); err != nil {
		t.Fatal(err)
	}
	if wakes, _ := fixture.store.PendingWakes(); len(wakes) != 1 || wakes[0].Kind != "task-assigned" {
		t.Fatalf("pre-delivery wakes = %#v, want only task-assigned", wakes)
	}
	if deadline, err := fixture.store.NextAgentIdleDeadline(); err != nil || !deadline.IsZero() {
		t.Fatalf("pending delivery deadline = %v, %v; want zero", deadline, err)
	}

	if _, err := fixture.store.RecordWakeAttempt(assignment.ID); err != nil {
		t.Fatal(err)
	}
	if deadline, err := fixture.store.NextAgentIdleDeadline(); err != nil || !deadline.IsZero() {
		t.Fatalf("uncertain delivery deadline = %v, %v; want zero", deadline, err)
	}
	fixture.now = fixture.now.Add(2 * time.Second)
	if _, err := fixture.store.RecordWakeOutcome(assignment.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	wantDeadline := fixture.now.Add(agentStartGrace)
	if deadline, err := fixture.store.NextAgentIdleDeadline(); err != nil || !deadline.Equal(wantDeadline) {
		t.Fatalf("deadline = %v, %v; want %v", deadline, err, wantDeadline)
	}

	fixture.now = wantDeadline.Add(-time.Nanosecond)
	if wakes, err := fixture.store.AuditDueAgentIdle(); err != nil || len(wakes) != 0 {
		t.Fatalf("early audit = %#v, %v; want none", wakes, err)
	}
	fixture.now = wantDeadline
	wakes, err := fixture.store.AuditDueAgentIdle()
	if err != nil || len(wakes) != 1 {
		t.Fatalf("boundary audit = %#v, %v; want one", wakes, err)
	}
	if wakes[0].Kind != "agent-idle" || !strings.Contains(wakes[0].Body, "did not start task "+task.ID) || !strings.Contains(wakes[0].Body, "within 5 seconds") {
		t.Fatalf("agent-idle wake = %#v", wakes[0])
	}
	if again, err := fixture.store.AuditDueAgentIdle(); err != nil || len(again) != 0 {
		t.Fatalf("duplicate audit = %#v, %v; want none", again, err)
	}

	// Replay reconstructs the activation-scoped alert bit from the existing
	// wake request; a restart cannot emit a duplicate even before it is drained.
	reopened := New(fixture.store.root, fixture.store.session, WithClock(func() time.Time { return fixture.now }))
	if deadline, err := reopened.NextAgentIdleDeadline(); err != nil || !deadline.IsZero() {
		t.Fatalf("replayed deadline = %v, %v; want zero", deadline, err)
	}
	if duplicate, err := reopened.AuditDueAgentIdle(); err != nil || len(duplicate) != 0 {
		t.Fatalf("replayed duplicate audit = %#v, %v", duplicate, err)
	}
}

func TestAlreadyIdleAssignmentWaitsForGrace(t *testing.T) {
	fixture := newSupervisionFixture(t)
	if err := fixture.store.RecordAgentStatus("p2", "idle"); err != nil {
		t.Fatal(err)
	}
	task := fixture.assign(t, "worker")
	assignment := wakeForKind(t, fixture.store, task.ID, "task-assigned")
	settleWake(t, fixture.store, assignment, true)

	if pending, _ := fixture.store.PendingWakes(); len(pending) != 0 {
		t.Fatalf("assignment to already-idle worker queued premature alert: %#v", pending)
	}
	deadline, err := fixture.store.NextAgentIdleDeadline()
	if err != nil {
		t.Fatal(err)
	}
	fixture.now = deadline
	if wakes, err := fixture.store.AuditDueAgentIdle(); err != nil || len(wakes) != 1 {
		t.Fatalf("grace audit = %#v, %v; want one", wakes, err)
	}
}

func TestPostActivationWorkingObservationAndProgressCancelGrace(t *testing.T) {
	t.Run("working observation despite synthetic working projection", func(t *testing.T) {
		fixture := newSupervisionFixture(t)
		task := fixture.assign(t, "worker")
		settleWake(t, fixture.store, wakeForKind(t, fixture.store, task.ID, "task-assigned"), true)

		before := lineCount(t, fixture.store.logPath())
		if err := fixture.store.RecordAgentStatus("p2", "working"); err != nil {
			t.Fatal(err)
		}
		if got := lineCount(t, fixture.store.logPath()); got != before+1 {
			t.Fatalf("real working observation appended %d events, want 1", got-before)
		}
		if deadline, _ := fixture.store.NextAgentIdleDeadline(); !deadline.IsZero() {
			t.Fatalf("working observation left deadline %v", deadline)
		}

		if err := fixture.store.RecordAgentStatus("p2", "idle"); err != nil {
			t.Fatal(err)
		}
		wake := wakeForKind(t, fixture.store, task.ID, "agent-idle")
		if strings.Contains(wake.Body, "did not start") {
			t.Fatalf("post-start idle used grace body: %q", wake.Body)
		}
	})

	t.Run("task progress", func(t *testing.T) {
		fixture := newSupervisionFixture(t)
		task := fixture.assign(t, "worker")
		settleWake(t, fixture.store, wakeForKind(t, fixture.store, task.ID, "task-assigned"), true)
		if _, err := fixture.store.RecordProgress("worker", task.ID, "started"); err != nil {
			t.Fatal(err)
		}
		if deadline, _ := fixture.store.NextAgentIdleDeadline(); !deadline.IsZero() {
			t.Fatalf("progress left deadline %v", deadline)
		}
	})
}

func TestAgentIdleSupervisionExcludesPausedTerminalOrInactiveWork(t *testing.T) {
	tests := []struct {
		name   string
		change func(*testing.T, *supervisionFixture, Task)
	}{
		{name: "paused", change: func(t *testing.T, f *supervisionFixture, task Task) {
			if _, err := f.store.TransitionTask("worker", task.ID, TaskBlocked, "waiting"); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "terminal", change: func(t *testing.T, f *supervisionFixture, task Task) {
			if _, err := f.store.TransitionTask("worker", task.ID, TaskCompleted, ""); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "orphaned and inactive assignee", change: func(t *testing.T, f *supervisionFixture, _ Task) {
			if err := f.store.StopAgent("worker", "p2"); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "inactive assigner", change: func(t *testing.T, f *supervisionFixture, _ Task) {
			if err := f.store.StopAgent(OrchestratorIdentity, "p1"); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSupervisionFixture(t)
			task := fixture.assign(t, "worker")
			settleWake(t, fixture.store, wakeForKind(t, fixture.store, task.ID, "task-assigned"), true)
			test.change(t, fixture, task)
			if deadline, err := fixture.store.NextAgentIdleDeadline(); err != nil || !deadline.IsZero() {
				t.Fatalf("deadline = %v, %v; want zero", deadline, err)
			}
			fixture.now = fixture.now.Add(time.Minute)
			if wakes, err := fixture.store.AuditDueAgentIdle(); err != nil || len(wakes) != 0 {
				t.Fatalf("inactive audit = %#v, %v; want none", wakes, err)
			}
		})
	}
}

func TestResumeCreatesFreshSupervisionEpisode(t *testing.T) {
	fixture := newSupervisionFixture(t)
	task := fixture.assign(t, "worker")
	settleWake(t, fixture.store, wakeForKind(t, fixture.store, task.ID, "task-assigned"), true)
	firstDeadline, _ := fixture.store.NextAgentIdleDeadline()
	fixture.now = firstDeadline
	if wakes, err := fixture.store.AuditDueAgentIdle(); err != nil || len(wakes) != 1 {
		t.Fatalf("first episode audit = %#v, %v", wakes, err)
	}
	settleWakes(t, fixture.store)
	if _, err := fixture.store.TransitionTask("worker", task.ID, TaskBlocked, "waiting"); err != nil {
		t.Fatal(err)
	}
	settleWakes(t, fixture.store)
	fixture.now = fixture.now.Add(time.Second)
	if _, err := fixture.store.TransitionTask(OrchestratorIdentity, task.ID, TaskActive, "continue"); err != nil {
		t.Fatal(err)
	}
	if deadline, _ := fixture.store.NextAgentIdleDeadline(); !deadline.IsZero() {
		t.Fatalf("pending resume wake armed deadline %v", deadline)
	}
	settleWake(t, fixture.store, wakeForKind(t, fixture.store, task.ID, "task-resumed"), true)
	secondDeadline, _ := fixture.store.NextAgentIdleDeadline()
	if want := fixture.now.Add(agentStartGrace); !secondDeadline.Equal(want) {
		t.Fatalf("resume deadline = %v, want %v", secondDeadline, want)
	}
	fixture.now = secondDeadline
	if wakes, err := fixture.store.AuditDueAgentIdle(); err != nil || len(wakes) != 1 {
		t.Fatalf("second episode audit = %#v, %v", wakes, err)
	}
	alerts := 0
	for _, entry := range ledgerEntries(t, fixture.store.logPath()) {
		if entry.Type == eventWakeRequested && entry.WakeKind == "agent-idle" {
			alerts++
		}
	}
	if alerts != 2 {
		t.Fatalf("agent-idle wake count = %d, want one per activation episode", alerts)
	}
}

func TestMultipleAgentIdleDeadlinesAreAuditedInOrder(t *testing.T) {
	fixture := newSupervisionFixture(t)
	if _, _, err := fixture.store.RegisterAgent(RegisterParams{Name: "worker2", PaneID: "p3", Harness: "codex", Caller: OrchestratorIdentity}); err != nil {
		t.Fatal(err)
	}
	first := fixture.assign(t, "worker")
	settleWake(t, fixture.store, wakeForKind(t, fixture.store, first.ID, "task-assigned"), true)
	firstOutcome := fixture.now

	fixture.now = fixture.now.Add(2 * time.Second)
	second := fixture.assign(t, "worker2")
	settleWake(t, fixture.store, wakeForKind(t, fixture.store, second.ID, "task-assigned"), true)
	if deadline, _ := fixture.store.NextAgentIdleDeadline(); !deadline.Equal(firstOutcome.Add(agentStartGrace)) {
		t.Fatalf("earliest deadline = %v, want %v", deadline, firstOutcome.Add(agentStartGrace))
	}

	fixture.now = firstOutcome.Add(agentStartGrace)
	wakes, err := fixture.store.AuditDueAgentIdle()
	if err != nil || len(wakes) != 1 || wakes[0].ReferenceID != first.ID {
		t.Fatalf("first due set = %#v, %v", wakes, err)
	}
	secondDeadline, _ := fixture.store.NextAgentIdleDeadline()
	if want := firstOutcome.Add(2*time.Second + agentStartGrace); !secondDeadline.Equal(want) {
		t.Fatalf("second deadline = %v, want %v", secondDeadline, want)
	}
	fixture.now = secondDeadline
	wakes, err = fixture.store.AuditDueAgentIdle()
	if err != nil || len(wakes) != 1 || wakes[0].ReferenceID != second.ID {
		t.Fatalf("second due set = %#v, %v", wakes, err)
	}
}

func TestFailedAssignmentDeliveryStartsGraceAtFailureOutcome(t *testing.T) {
	fixture := newSupervisionFixture(t)
	task := fixture.assign(t, "worker")
	wake := wakeForKind(t, fixture.store, task.ID, "task-assigned")
	if _, err := fixture.store.RecordWakeAttempt(wake.ID); err != nil {
		t.Fatal(err)
	}
	fixture.now = fixture.now.Add(3 * time.Second)
	if _, err := fixture.store.RecordWakeOutcome(wake.ID, false, "refused"); err != nil {
		t.Fatal(err)
	}
	deadline, err := fixture.store.NextAgentIdleDeadline()
	if err != nil || !deadline.Equal(fixture.now.Add(agentStartGrace)) {
		t.Fatalf("failed-delivery deadline = %v, %v", deadline, err)
	}
	fixture.now = deadline
	wakes, err := fixture.store.AuditDueAgentIdle()
	if err != nil || len(wakes) != 1 || !strings.Contains(wakes[0].Body, "delivery failed") {
		t.Fatalf("failed-delivery audit = %#v, %v", wakes, err)
	}
}

func TestPreStartBlockedFailedAndStoppedStillWakeImmediately(t *testing.T) {
	for _, status := range []string{"blocked", "failed", "stopped"} {
		t.Run(status, func(t *testing.T) {
			fixture := newSupervisionFixture(t)
			task := fixture.assign(t, "worker")
			if err := fixture.store.RecordAgentStatus("p2", status); err != nil {
				t.Fatal(err)
			}
			wake := wakeForKind(t, fixture.store, task.ID, "agent-"+status)
			if wake.Recipient != OrchestratorIdentity {
				t.Fatalf("wake recipient = %s", wake.Recipient)
			}
		})
	}
}
