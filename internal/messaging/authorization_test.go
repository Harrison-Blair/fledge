package messaging

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// mustTransition drives one task transition, failing the test on any error. It
// keeps the state-machine setup helpers below terse.
func mustTransition(t *testing.T, store *Store, caller, id string, target TaskStatus, detail string) {
	t.Helper()
	if _, err := store.TransitionTask(caller, id, target, detail); err != nil {
		t.Fatalf("TransitionTask(%s -> %s): %v", caller, target, err)
	}
}

// workerTaskInStatus registers the standard orchestrator(p1)+worker(p2) pair,
// assigns the worker one task, drives it into the requested status, then settles
// every wake so callers observe only the transitions they add next.
func workerTaskInStatus(t *testing.T, store *Store, status TaskStatus) string {
	t.Helper()
	task := assignedWorker(t, store)
	switch status {
	case TaskActive:
		// The freshly assigned task is already active.
	case TaskBlocked:
		mustTransition(t, store, "worker", task.ID, TaskBlocked, "stuck")
	case TaskNeedsDecision:
		mustTransition(t, store, "worker", task.ID, TaskNeedsDecision, "which?")
	case TaskCompleted:
		mustTransition(t, store, "worker", task.ID, TaskCompleted, "")
	case TaskFailed:
		mustTransition(t, store, "worker", task.ID, TaskFailed, "broke")
	case TaskCanceled:
		mustTransition(t, store, OrchestratorIdentity, task.ID, TaskCanceled, "")
	default:
		t.Fatalf("workerTaskInStatus: unsupported status %s", status)
	}
	settleWakes(t, store)
	return task.ID
}

// Task 1: a body at the 64 KiB maximum survives the header-wrapped wake envelope
// the drain hands to Herdr, and the message it carries projects to delivered.
func TestMaxBodyFlowsThroughDrainEnvelopeIntact(t *testing.T) {
	store := initializedStore(t)
	body := strings.Repeat("x", MaxBodyBytes)
	message := mustCreate(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: body, RecipientPane: "%1"})

	deliveries := drainWakes(t, store)
	if len(deliveries) != 1 || deliveries[0].kind != "message" || deliveries[0].recipient != "alice" {
		t.Fatalf("deliveries = %#v, want one message wake to alice", deliveries)
	}
	// The full 64 KiB body is present, uncut, inside the wrapped envelope.
	if !strings.Contains(deliveries[0].envelope, body) {
		t.Fatalf("64 KiB body did not survive the envelope intact (envelope %d bytes, body %d bytes)",
			len(deliveries[0].envelope), len(body))
	}
	if len(deliveries[0].envelope) <= MaxBodyBytes {
		t.Fatalf("envelope %d bytes did not exceed the body it wraps", len(deliveries[0].envelope))
	}
	if strings.Count(deliveries[0].envelope, body) != 1 {
		t.Fatalf("body appears %d times in the envelope, want exactly once", strings.Count(deliveries[0].envelope, body))
	}
	delivered, err := store.Get(message.ID)
	if err != nil {
		t.Fatal(err)
	}
	if delivered.Status != StatusDelivered {
		t.Fatalf("message after drain = %s, want delivered", delivered.Status)
	}
}

// Task 2: a Herdr push of 'blocked' wakes exactly the task's assigner, and a
// repeat of the same status is a projection no-op that appends nothing.
func TestRecordAgentStatusBlockedWakesAssignerAndIsIdempotent(t *testing.T) {
	store := coordinationTestStore(t)
	task := assignedWorker(t, store)
	settleWakes(t, store) // clear the assignment wake so only the status wake is left

	if err := store.RecordAgentStatus("p2", "blocked"); err != nil {
		t.Fatal(err)
	}
	wakes, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	if len(wakes) != 1 || wakes[0].Kind != "agent-blocked" || wakes[0].Recipient != OrchestratorIdentity || wakes[0].ReferenceID != task.ID {
		t.Fatalf("wakes = %#v, want one agent-blocked to %s referencing %s", wakes, OrchestratorIdentity, task.ID)
	}
	if agent, err := store.Agent("worker"); err != nil || agent.Status != "blocked" {
		t.Fatalf("worker status = %#v, %v; want blocked", agent, err)
	}

	// A second push of the unchanged status short-circuits: no event, no wake.
	afterFirst := lineCount(t, store.logPath())
	if err := store.RecordAgentStatus("p2", "blocked"); err != nil {
		t.Fatal(err)
	}
	if got := lineCount(t, store.logPath()); got != afterFirst {
		t.Fatalf("repeat status appended %d events, want 0", got-afterFirst)
	}
	if again, _ := store.PendingWakes(); len(again) != 1 {
		t.Fatalf("pending wakes after repeat = %#v, want the single unchanged wake", again)
	}

	// An unknown pane is silently ignored: nil error, nothing appended.
	if err := store.RecordAgentStatus("p-unknown", "blocked"); err != nil {
		t.Fatalf("RecordAgentStatus(unknown pane) = %v, want nil", err)
	}
	if got := lineCount(t, store.logPath()); got != afterFirst {
		t.Fatalf("unknown-pane status appended %d events, want 0", got-afterFirst)
	}
}

// Task 2 (continued): 'working' while a task is active is recorded on the
// registry but wakes nobody — only blocked/idle/failed/stopped raise a wake.
func TestRecordAgentStatusWorkingRecordsWithoutWaking(t *testing.T) {
	store := coordinationTestStore(t)
	workerTaskInStatus(t, store, TaskActive)
	// Move to blocked first so the following 'working' is a real status change.
	if err := store.RecordAgentStatus("p2", "blocked"); err != nil {
		t.Fatal(err)
	}
	settleWakes(t, store)

	before := lineCount(t, store.logPath())
	if err := store.RecordAgentStatus("p2", "working"); err != nil {
		t.Fatal(err)
	}
	// Exactly one event (agent_status_changed) and no wake.
	if got := lineCount(t, store.logPath()); got != before+1 {
		t.Fatalf("working transition appended %d events, want 1 (status, no wake)", got-before)
	}
	if agent, err := store.Agent("worker"); err != nil || agent.Status != "working" {
		t.Fatalf("worker status = %#v, %v; want working", agent, err)
	}
	if wakes, _ := store.PendingWakes(); len(wakes) != 0 {
		t.Fatalf("pending wakes after working = %#v, want none", wakes)
	}
}

// Task 3: Reply refuses a message whose delivery is not (yet) replyable and a
// bogus original, appending nothing in every rejected case.
func TestReplyRejectsNonReplyableStatusAndMissingOriginal(t *testing.T) {
	store := initializedStore(t)
	pending := mustCreate(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: "question", RecipientPane: "%1"})
	failed := mustCreate(t, store, CreateParams{Sender: "user", Recipient: "bob", Body: "question", RecipientPane: "%2"})
	if _, err := store.RecordWakeAttempt("w-" + failed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordWakeOutcome("w-"+failed.ID, false, "Herdr refused"); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(failed.ID); err != nil || got.Status != StatusFailed {
		t.Fatalf("failed message = %#v, %v; want status failed", got, err)
	}

	cases := []struct {
		name         string
		originalID   string
		replier      string
		replierPane  string
		wantErrIs    error
		wantContains string
	}{
		{name: "pending original", originalID: pending.ID, replier: "alice", replierPane: "%1", wantContains: "in status"},
		{name: "failed original", originalID: failed.ID, replier: "bob", replierPane: "%2", wantContains: "in status"},
		{name: "missing original", originalID: "does-not-exist", replier: "alice", replierPane: "%1", wantErrIs: ErrNotFound},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			before := lineCount(t, store.logPath())
			_, err := store.Reply(test.originalID, test.replier, test.replierPane, "answer", "")
			if err == nil {
				t.Fatalf("Reply(%s) succeeded, want rejection", test.name)
			}
			if test.wantErrIs != nil && !errors.Is(err, test.wantErrIs) {
				t.Fatalf("Reply error = %v, want %v", err, test.wantErrIs)
			}
			if test.wantContains != "" && !strings.Contains(err.Error(), test.wantContains) {
				t.Fatalf("Reply error = %q, want containing %q", err.Error(), test.wantContains)
			}
			if got := lineCount(t, store.logPath()); got != before {
				t.Fatalf("rejected reply appended %d events, want 0", got-before)
			}
		})
	}
}

// Task 4(a): the task state machine rejects every illegal (from, target) pair
// with 'invalid task transition', and blocked/needs-decision/failed each demand
// a non-blank detail.
func TestTransitionTaskStateMachineRejectsIllegalMovesAndRequiresDetail(t *testing.T) {
	invalid := []struct {
		name   string
		from   TaskStatus
		target TaskStatus
	}{
		{name: "active to active", from: TaskActive, target: TaskActive},
		{name: "blocked to needs-decision", from: TaskBlocked, target: TaskNeedsDecision},
		{name: "needs-decision to blocked", from: TaskNeedsDecision, target: TaskBlocked},
		{name: "completed to blocked", from: TaskCompleted, target: TaskBlocked},
		{name: "completed to active", from: TaskCompleted, target: TaskActive},
		{name: "canceled to completed", from: TaskCanceled, target: TaskCompleted},
		{name: "failed to active", from: TaskFailed, target: TaskActive},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			store := coordinationTestStore(t)
			id := workerTaskInStatus(t, store, test.from)
			before := lineCount(t, store.logPath())
			// A privileged caller bypasses authorization so only the state machine
			// can be what rejects the move.
			_, err := store.TransitionTask(OrchestratorIdentity, id, test.target, "detail")
			if err == nil || !strings.Contains(err.Error(), "invalid task transition") {
				t.Fatalf("TransitionTask(%s->%s) error = %v, want invalid task transition", test.from, test.target, err)
			}
			if got := lineCount(t, store.logPath()); got != before {
				t.Fatalf("rejected transition appended %d events, want 0", got-before)
			}
		})
	}

	requiresDetail := []TaskStatus{TaskBlocked, TaskNeedsDecision, TaskFailed}
	for _, target := range requiresDetail {
		t.Run("blank detail "+string(target), func(t *testing.T) {
			store := coordinationTestStore(t)
			id := workerTaskInStatus(t, store, TaskActive)
			before := lineCount(t, store.logPath())
			_, err := store.TransitionTask("worker", id, target, "   ")
			if err == nil || !strings.Contains(err.Error(), "detail must not be blank") {
				t.Fatalf("TransitionTask(%s, blank detail) error = %v, want blank-detail rejection", target, err)
			}
			if got := lineCount(t, store.logPath()); got != before {
				t.Fatalf("rejected transition appended %d events, want 0", got-before)
			}
		})
	}
}

// Task 4(b): RecordProgress rejects an unauthorized caller, a non-active task, a
// blank detail, and an unknown task, leaving the log untouched every time.
func TestRecordProgressErrorPathsAppendNothing(t *testing.T) {
	activeStore := func(t *testing.T) (*Store, string) {
		t.Helper()
		store := coordinationTestStore(t)
		task := assignedWorker(t, store)
		settleWakes(t, store)
		return store, task.ID
	}

	t.Run("unauthorized third agent", func(t *testing.T) {
		store, id := activeStore(t)
		before := lineCount(t, store.logPath())
		if _, err := store.RecordProgress("stranger", id, "halfway"); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("RecordProgress error = %v, want ErrUnauthorized", err)
		}
		if got := lineCount(t, store.logPath()); got != before {
			t.Fatalf("unauthorized progress appended %d events, want 0", got-before)
		}
	})

	t.Run("non-active task", func(t *testing.T) {
		store, id := activeStore(t)
		mustTransition(t, store, "worker", id, TaskBlocked, "stuck")
		settleWakes(t, store)
		before := lineCount(t, store.logPath())
		_, err := store.RecordProgress("worker", id, "halfway")
		if err == nil || !strings.Contains(err.Error(), "is not active") {
			t.Fatalf("RecordProgress on blocked task error = %v, want not-active rejection", err)
		}
		if got := lineCount(t, store.logPath()); got != before {
			t.Fatalf("progress on blocked task appended %d events, want 0", got-before)
		}
	})

	t.Run("blank detail", func(t *testing.T) {
		store, id := activeStore(t)
		before := lineCount(t, store.logPath())
		_, err := store.RecordProgress("worker", id, "   ")
		if err == nil || !strings.Contains(err.Error(), "detail must not be blank") {
			t.Fatalf("RecordProgress(blank detail) error = %v, want blank-detail rejection", err)
		}
		if got := lineCount(t, store.logPath()); got != before {
			t.Fatalf("blank progress appended %d events, want 0", got-before)
		}
	})

	t.Run("unknown task", func(t *testing.T) {
		store, _ := activeStore(t)
		before := lineCount(t, store.logPath())
		if _, err := store.RecordProgress("worker", "t-nope", "halfway"); !errors.Is(err, ErrTaskNotFound) {
			t.Fatalf("RecordProgress(unknown task) error = %v, want ErrTaskNotFound", err)
		}
		if got := lineCount(t, store.logPath()); got != before {
			t.Fatalf("unknown-task progress appended %d events, want 0", got-before)
		}
	})
}

// Task 4(c): malformed CreateParams are rejected before any message_created
// event reaches the log.
func TestValidateCreateRejectsMalformedParamsWithoutAppending(t *testing.T) {
	cases := []struct {
		name   string
		params CreateParams
	}{
		{name: "blank sender", params: CreateParams{Sender: "  ", Recipient: "alice", Body: "hi", RecipientPane: "%1"}},
		{name: "blank recipient", params: CreateParams{Sender: "user", Recipient: "  ", Body: "hi", RecipientPane: "%1"}},
		{name: "sender equals recipient", params: CreateParams{Sender: "alice", Recipient: "alice", Body: "hi", RecipientPane: "%1"}},
		{name: "user with pane", params: CreateParams{Sender: "agent", Recipient: "user", Body: "hi", RecipientPane: "%1"}},
		{name: "non-user empty pane", params: CreateParams{Sender: "user", Recipient: "alice", Body: "hi", RecipientPane: ""}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			store := initializedStore(t)
			before := lineCount(t, store.logPath())
			if _, err := store.Create(test.params); err == nil {
				t.Fatalf("Create(%s) succeeded, want validation error", test.name)
			}
			if got := lineCount(t, store.logPath()); got != before {
				t.Fatalf("rejected Create appended %d events, want 0", got-before)
			}
		})
	}
}

// Task 4(d): the transition authorization matrix — third parties are denied, the
// assigner may cancel or resume but not complete, and the assignee and the two
// privileged identities are allowed.
func TestAuthorizeTransitionDenialMatrix(t *testing.T) {
	t.Parallel()
	task := Task{ID: "t-1", Assigner: "lead", Assignee: "child"}
	cases := []struct {
		name    string
		caller  string
		target  TaskStatus
		wantErr bool
	}{
		{name: "third party blocked", caller: "stranger", target: TaskBlocked, wantErr: true},
		{name: "third party cancel", caller: "stranger", target: TaskCanceled, wantErr: true},
		{name: "assigner denied complete", caller: "lead", target: TaskCompleted, wantErr: true},
		{name: "assigner denied fail", caller: "lead", target: TaskFailed, wantErr: true},
		{name: "assigner allowed cancel", caller: "lead", target: TaskCanceled},
		{name: "assigner allowed resume", caller: "lead", target: TaskActive},
		{name: "assignee allowed complete", caller: "child", target: TaskCompleted},
		{name: "assignee allowed block", caller: "child", target: TaskBlocked},
		{name: "user privileged", caller: UserIdentity, target: TaskCompleted},
		{name: "orchestrator privileged", caller: OrchestratorIdentity, target: TaskCompleted},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := authorizeTransition(test.caller, task, test.target)
			if (err != nil) != test.wantErr {
				t.Fatalf("authorizeTransition(%s, %s) error = %v, want error %v", test.caller, test.target, err, test.wantErr)
			}
			if err != nil && !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("authorizeTransition(%s, %s) error = %v, want ErrUnauthorized", test.caller, test.target, err)
			}
		})
	}
}

// Task 4(d, path): the same denial and allowance play out through the public
// TransitionTask, which threads authorizeTransition ahead of the state machine.
func TestTransitionTaskAuthorizationThroughPublicPath(t *testing.T) {
	store := coordinationTestStore(t)
	parentID := orchestratorLeadChild(t, store)
	child, err := store.AssignTask("lead", "child", parentID, "child work")
	if err != nil {
		t.Fatal(err)
	}
	settleWakes(t, store)

	// A stranger and the assigner attempting to complete are both denied, and the
	// log is untouched.
	before := lineCount(t, store.logPath())
	if _, err := store.TransitionTask("stranger", child.ID, TaskBlocked, "x"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("stranger transition error = %v, want ErrUnauthorized", err)
	}
	if _, err := store.TransitionTask("lead", child.ID, TaskCompleted, "done"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("assigner complete error = %v, want ErrUnauthorized", err)
	}
	if got := lineCount(t, store.logPath()); got != before {
		t.Fatalf("denied transitions appended %d events, want 0", got-before)
	}

	// The assigner is allowed to cancel its own delegated work.
	canceled, err := store.TransitionTask("lead", child.ID, TaskCanceled, "stop")
	if err != nil || canceled.Status != TaskCanceled {
		t.Fatalf("assigner cancel = %#v, %v; want canceled", canceled, err)
	}
}

// Task 4(e): StopAgentByPane orphans a stopped agent's active work, and is a
// no-op on an already-stopped or unknown pane.
func TestStopAgentByPaneIdempotentAndOrphansBoundWork(t *testing.T) {
	store := coordinationTestStore(t)
	task := assignedWorker(t, store)
	settleWakes(t, store)

	// The first close deactivates the pane and orphans its bound task.
	if err := store.StopAgentByPane("p2"); err != nil {
		t.Fatal(err)
	}
	if orphaned := taskByID(t, store, task.ID); orphaned.Status != TaskOrphaned {
		t.Fatalf("task after pane close = %#v, want orphaned", orphaned)
	}
	if _, err := store.Agent("worker"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("worker still active after pane close: %v", err)
	}

	// A repeat close of the now-stopped pane appends nothing.
	settled := lineCount(t, store.logPath())
	if err := store.StopAgentByPane("p2"); err != nil {
		t.Fatalf("StopAgentByPane(already stopped) = %v, want nil", err)
	}
	if got := lineCount(t, store.logPath()); got != settled {
		t.Fatalf("repeat pane close appended %d events, want 0", got-settled)
	}

	// An unknown pane is equally harmless.
	if err := store.StopAgentByPane("p-unknown"); err != nil {
		t.Fatalf("StopAgentByPane(unknown) = %v, want nil", err)
	}
	if got := lineCount(t, store.logPath()); got != settled {
		t.Fatalf("unknown pane close appended %d events, want 0", got-settled)
	}
}

// Task 4(f): SessionID returns the durable ID for the matching session, rejects
// a log that belongs to a different session, and never initializes an absent
// log.
func TestSessionIDReturnsDurableIDOrMismatch(t *testing.T) {
	root := t.TempDir()
	store := New(root, testSession)
	id, err := store.Initialize()
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.SessionID()
	if err != nil || got != id {
		t.Fatalf("SessionID() = %q, %v; want durable %q", got, err, id)
	}

	// A log carried under a different session name is a mismatch.
	moved := New(root, otherSession)
	if err := os.Rename(store.statePath(), moved.statePath()); err != nil {
		t.Fatal(err)
	}
	if _, err := moved.SessionID(); !errors.Is(err, ErrSessionMismatch) {
		t.Fatalf("SessionID() on renamed log = %v, want ErrSessionMismatch", err)
	}

	// The original session name no longer has a log, and SessionID must not
	// silently create one.
	if _, err := store.SessionID(); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("SessionID() on absent log = %v, want ErrNotInitialized", err)
	}
	if _, statErr := os.Stat(store.logPath()); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("SessionID() initialized an absent log: %v", statErr)
	}
}

// Task 4(g): Agents() returns every registered agent, active and stopped alike,
// sorted by name.
func TestAgentsReturnsActiveAndStoppedNameSorted(t *testing.T) {
	store := coordinationTestStore(t)
	registerAll(t, store,
		RegisterParams{Name: OrchestratorIdentity, PaneID: "p1", Harness: "codex", Caller: UserIdentity, CanDelegate: true},
		RegisterParams{Name: "zeta", PaneID: "p2", Harness: "codex", Caller: OrchestratorIdentity},
		RegisterParams{Name: "alpha", PaneID: "p3", Harness: "codex", Caller: OrchestratorIdentity})
	if err := store.StopAgent("zeta", "p2"); err != nil {
		t.Fatal(err)
	}

	agents, err := store.Agents()
	if err != nil {
		t.Fatal(err)
	}
	gotNames := make([]string, len(agents))
	for i, agent := range agents {
		gotNames[i] = agent.Name
	}
	wantNames := []string{"alpha", OrchestratorIdentity, "zeta"}
	if strings.Join(gotNames, ",") != strings.Join(wantNames, ",") {
		t.Fatalf("Agents() names = %v, want %v (active and stopped, name-sorted)", gotNames, wantNames)
	}
	active := map[string]bool{}
	for _, agent := range agents {
		active[agent.Name] = agent.Active
	}
	if active["zeta"] {
		t.Fatalf("stopped agent zeta reported active")
	}
	if !active["alpha"] || !active[OrchestratorIdentity] {
		t.Fatalf("live agents reported inactive: %#v", active)
	}
}
