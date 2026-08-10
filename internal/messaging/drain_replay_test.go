package messaging

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"
)

// wakeEnvelope reproduces, byte for byte, the prompt text watchproc.drain wraps
// a pending wake in before handing it to Herdr. Building it here lets the store
// tests assert on the exact wire envelope a replayed delivery carries.
func wakeEnvelope(w Wake) string {
	return fmt.Sprintf("[Fledge wake]\nDelivery-ID: %s\nKind: %s\n\n%s", w.ID, w.Kind, w.Body)
}

// delivery records one prompt the drain would have sent.
type delivery struct {
	id, kind, recipient, envelope string
}

// drainWakes mirrors watchproc.drain at the store layer: it walks PendingWakes
// in order, marks each attempt, records a successful outcome, and returns the
// envelopes it would have prompted. It takes a single PendingWakes snapshot so
// the returned slice preserves the store's delivery order, and its envelope
// format matches the dispatcher's exactly.
func drainWakes(t *testing.T, store *Store) []delivery {
	t.Helper()
	wakes, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	out := make([]delivery, 0, len(wakes))
	for _, w := range wakes {
		attempted, err := store.RecordWakeAttempt(w.ID)
		if err != nil {
			t.Fatalf("RecordWakeAttempt(%s): %v", w.ID, err)
		}
		out = append(out, delivery{id: attempted.ID, kind: attempted.Kind,
			recipient: attempted.Recipient, envelope: wakeEnvelope(attempted)})
		if _, err := store.RecordWakeOutcome(w.ID, true, ""); err != nil {
			t.Fatalf("RecordWakeOutcome(%s): %v", w.ID, err)
		}
	}
	return out
}

// ledgerEntries decodes every durable line in the ledger through the exported
// diagnostic reader, so a test can count events by type without reimplementing
// the record framing.
func ledgerEntries(t *testing.T, path string) []LedgerEntry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entries []LedgerEntry
	for _, line := range bytes.SplitAfter(data, []byte{'\n'}) {
		if entry, ok := DecodeLedgerLine(line); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func countType(entries []LedgerEntry, eventType string) int {
	n := 0
	for _, entry := range entries {
		if entry.Type == eventType {
			n++
		}
	}
	return n
}

// Test 1: a message travels the full drain both ways. The worker sends the
// orchestrator a message; draining delivers it and projects StatusDelivered onto
// the message. The orchestrator replies; draining delivers the correlated reply
// back to the worker and projects StatusDelivered onto it.
func TestMessageRoundTripThroughDrain(t *testing.T) {
	store := coordinationTestStore(t)
	registerAll(t, store,
		RegisterParams{Name: OrchestratorIdentity, PaneID: "p1", Harness: "codex", Caller: UserIdentity, CanDelegate: true},
		RegisterParams{Name: "worker", PaneID: "p2", Harness: "codex", Caller: OrchestratorIdentity})

	outbound, err := store.Create(CreateParams{Sender: "worker", Recipient: OrchestratorIdentity, RecipientPane: "p1", Body: "status update"})
	if err != nil {
		t.Fatal(err)
	}
	if outbound.Status != StatusPending {
		t.Fatalf("created message status = %s, want pending", outbound.Status)
	}

	deliveries := drainWakes(t, store)
	if len(deliveries) != 1 || deliveries[0].kind != "message" || deliveries[0].recipient != OrchestratorIdentity {
		t.Fatalf("first drain deliveries = %#v", deliveries)
	}
	if deliveries[0].id != "w-"+outbound.ID {
		t.Fatalf("message wake id = %s, want stable w-%s", deliveries[0].id, outbound.ID)
	}
	delivered, err := store.Get(outbound.ID)
	if err != nil {
		t.Fatal(err)
	}
	if delivered.Status != StatusDelivered {
		t.Fatalf("message after drain = %s, want delivered", delivered.Status)
	}

	reply, err := store.Reply(outbound.ID, OrchestratorIdentity, "p1", "ack received", "p2")
	if err != nil {
		t.Fatal(err)
	}
	if reply.ReplyTo != outbound.ID || reply.Recipient != "worker" || reply.Status != StatusPending {
		t.Fatalf("reply = %#v", reply)
	}

	replyDeliveries := drainWakes(t, store)
	if len(replyDeliveries) != 1 || replyDeliveries[0].recipient != "worker" || replyDeliveries[0].id != "w-"+reply.ID {
		t.Fatalf("reply drain deliveries = %#v", replyDeliveries)
	}
	deliveredReply, err := store.Get(reply.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deliveredReply.Status != StatusDelivered || deliveredReply.ReplyTo != outbound.ID {
		t.Fatalf("reply after drain = %#v", deliveredReply)
	}
}

// Test 2: reconstruction of a torn multi-event transaction.
//
// RegisterAgent-with-Task appends agent_registered + task_assigned +
// wake_requested in a single write. A crash mid-write, once readAndRepairLog
// trims the torn tail, can leave the ledger ending cleanly after only
// agent_registered. This pins what reconstruction then does: it does NOT reject
// the partial transaction, and it surfaces a registered agent with neither its
// task nor its wake.
//
// This documents a real durability gap (see notes): the ledger has no
// transaction framing, so a torn append can reconstruct a partial transaction.
func TestTornRegistrationTransactionReconstructsPartialState(t *testing.T) {
	store := coordinationTestStore(t)
	if _, _, err := store.RegisterAgent(RegisterParams{
		Name: "worker", PaneID: "p2", Harness: "codex", Caller: UserIdentity, Task: "do the thing",
	}); err != nil {
		t.Fatal(err)
	}
	// Sanity: the intact transaction wrote all three coordination events.
	entries := ledgerEntries(t, store.logPath())
	if countType(entries, eventAgentRegistered) != 1 || countType(entries, eventTaskAssigned) != 1 || countType(entries, eventWakeRequested) != 1 {
		t.Fatalf("intact ledger = %#v", entries)
	}

	// Simulate a crash whose torn tail was repaired back to the newline after
	// agent_registered, dropping task_assigned and wake_requested.
	truncateAfterType(t, store.logPath(), eventAgentRegistered)

	reopened := New(store.root, store.session)
	agent, err := reopened.Agent("worker")
	if err != nil {
		t.Fatalf("reopened Agent(worker) = %v; reconstruction neither rejected the torn log nor hid the partial agent", err)
	}
	if !agent.Active || agent.PaneID != "p2" {
		t.Fatalf("reopened agent = %#v", agent)
	}
	// The task and its wake vanished with the torn tail: the agent is surfaced
	// without them.
	tasks, err := reopened.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("tasks after torn append = %#v, want none", tasks)
	}
	wakes, err := reopened.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	if len(wakes) != 0 {
		t.Fatalf("pending wakes after torn append = %#v, want none", wakes)
	}
}

// Companion durability check. appendEvents has no fsync, so its only promise is
// OS visibility, not power-loss durability: a Store reopened from an independent
// handle sees a committed append, but the bytes are not proven on stable
// storage. This pins the achievable guarantee and flags the weaker-than-claimed
// durability of the "appends events durably" commit path (see notes).
func TestAppendIsOSVisibleToAReopenedStore(t *testing.T) {
	store := coordinationTestStore(t)
	created, err := store.Create(CreateParams{Sender: "user", Recipient: "alice", Body: "hello", RecipientPane: "%1"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := New(store.root, store.session).Get(created.ID)
	if err != nil {
		t.Fatalf("reopened Get after append: %v", err)
	}
	if got.ID != created.ID || got.Body != "hello" {
		t.Fatalf("reopened message = %#v", got)
	}
}

// Test 3: losing the orchestrator pane. The orchestrator holds a user-assigned
// task and has itself assigned the worker a subtask. A pane.closed for the
// orchestrator (StopAgentByPane) must deactivate it and orphan its own task, but
// queue NO wake for 'user' (the assigner resolves to nil), and leave the worker
// subtree untouched so it can still finish its work.
func TestOrchestratorPaneLossOrphansWithoutUserWake(t *testing.T) {
	store := coordinationTestStore(t)
	if _, _, err := store.RegisterAgent(RegisterParams{
		Name: OrchestratorIdentity, PaneID: "p1", Harness: "codex", Caller: UserIdentity, CanDelegate: true, Task: "coordinate",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RegisterAgent(RegisterParams{Name: "worker", PaneID: "p2", Harness: "codex", Caller: OrchestratorIdentity}); err != nil {
		t.Fatal(err)
	}
	workerTask, err := store.AssignTask(OrchestratorIdentity, "worker", "", "do work")
	if err != nil {
		t.Fatal(err)
	}
	// The orchestrator's own task is the user-assigned one created at registration.
	orchestratorTasks, err := store.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	var orchestratorTaskID string
	for _, task := range orchestratorTasks {
		if task.Assignee == OrchestratorIdentity {
			orchestratorTaskID = task.ID
		}
	}
	if orchestratorTaskID == "" {
		t.Fatalf("no user-assigned orchestrator task in %#v", orchestratorTasks)
	}
	settleWakes(t, store) // clear the two assignment wakes so only the loss matters

	if err := store.StopAgentByPane("p1"); err != nil {
		t.Fatal(err)
	}

	// The orchestrator is deactivated.
	if _, err := store.Agent(OrchestratorIdentity); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("orchestrator still active after pane loss: %v", err)
	}
	if orphaned := taskByID(t, store, orchestratorTaskID); orphaned.Status != TaskOrphaned {
		t.Fatalf("orchestrator task = %#v, want orphaned", orphaned)
	}
	// Orphaning the orchestrator's task woke nobody: its assigner is 'user', which
	// wakeFor resolves to nil. No wake for 'user' means the dispatcher has nothing
	// to deliver and rests idle rather than exiting.
	wakes, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	if len(wakes) != 0 {
		t.Fatalf("pending wakes after orchestrator loss = %#v, want none (no 'user' wake)", wakes)
	}
	for _, wake := range wakes {
		if wake.Recipient == UserIdentity {
			t.Fatalf("queued a wake for 'user': %#v", wake)
		}
	}
	// The worker subtree is untouched: still active work it can finish.
	if _, err := store.Agent("worker"); err != nil {
		t.Fatalf("worker no longer active after orchestrator loss: %v", err)
	}
	if live := taskByID(t, store, workerTask.ID); live.Status != TaskActive {
		t.Fatalf("worker task = %#v, want still active", live)
	}
}

// Test 4: a burst of contradictory wakes for one task, delivered in one drain.
//
// Assigning, then blocking, then canceling a task leaves three pending wakes
// that all reference it. This pins the ordering contract: PendingWakes replays
// them in creation order (wakeOrder), the drain delivers ALL of them (stale
// intermediate wakes are NOT suppressed by the store), and the terminal cancel
// instruction is delivered last.
func TestContradictoryWakesForOneTaskDrainInOrder(t *testing.T) {
	store := coordinationTestStore(t)
	task := assignedWorker(t, store)
	if _, err := store.TransitionTask("worker", task.ID, TaskBlocked, "stuck"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionTask(OrchestratorIdentity, task.ID, TaskCanceled, ""); err != nil {
		t.Fatal(err)
	}

	// The store queues all three, in creation order, every one referencing the task.
	pending, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	wantKinds := []string{"task-assigned", "task-blocked", "task-canceled"}
	wantRecipients := []string{"worker", OrchestratorIdentity, "worker"}
	if len(pending) != len(wantKinds) {
		t.Fatalf("pending wakes = %#v, want %d contradictory wakes", pending, len(wantKinds))
	}
	for i, wake := range pending {
		if wake.Kind != wantKinds[i] || wake.Recipient != wantRecipients[i] {
			t.Fatalf("pending[%d] = %s/%s, want %s/%s", i, wake.Kind, wake.Recipient, wantKinds[i], wantRecipients[i])
		}
		if wake.ReferenceID != task.ID {
			t.Fatalf("pending[%d] references %s, want task %s", i, wake.ReferenceID, task.ID)
		}
	}

	// A single drain delivers all three in that order; none is coalesced away.
	deliveries := drainWakes(t, store)
	if len(deliveries) != len(wantKinds) {
		t.Fatalf("deliveries = %#v, want %d", deliveries, len(wantKinds))
	}
	for i, got := range deliveries {
		if got.kind != wantKinds[i] {
			t.Fatalf("delivery[%d] kind = %s, want %s", i, got.kind, wantKinds[i])
		}
	}
	// The terminal cancel is the last thing the recipient sees.
	if last := deliveries[len(deliveries)-1]; last.kind != "task-canceled" || last.recipient != "worker" {
		t.Fatalf("terminal delivery = %#v, want task-canceled to worker", last)
	}
	if drained, _ := store.PendingWakes(); len(drained) != 0 {
		t.Fatalf("pending after drain = %#v, want empty", drained)
	}
}

// Test 5: idempotent replay after a crash between wake_attempt and wake_outcome.
//
// A wake left uncertain (attempt recorded, no outcome) is replayed by the next
// dispatcher. The replay must rebuild the identical envelope from the stable
// Delivery-ID, must not append a second wake_attempt, and must settle the wake
// with exactly one outcome — bounding the double-delivery window to at most one
// extra prompt.
func TestUncertainWakeReplaysIdempotently(t *testing.T) {
	store := coordinationTestStore(t)
	task := assignedWorker(t, store)
	wakeID := "w-" + task.ID

	// The first dispatcher marks the attempt, then crashes before the outcome.
	before, err := store.RecordWakeAttempt(wakeID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Status != StatusUncertain {
		t.Fatalf("wake after first attempt = %s, want uncertain", before.Status)
	}
	originalEnvelope := wakeEnvelope(before)

	// The next dispatcher reopens the ledger and replays. The uncertain wake is
	// still pending delivery and rebuilds the same envelope from the stable ID.
	reopened := New(store.root, store.session)
	pending, err := reopened.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != wakeID {
		t.Fatalf("replay pending = %#v, want the uncertain wake %s", pending, wakeID)
	}
	if replay := wakeEnvelope(pending[0]); replay != originalEnvelope {
		t.Fatalf("replayed envelope differs from the original:\n got %q\nwant %q", replay, originalEnvelope)
	}

	// The replayed attempt is idempotent: re-marking an uncertain wake appends no
	// second wake_attempt event.
	linesBefore := lineCount(t, reopened.logPath())
	if _, err := reopened.RecordWakeAttempt(wakeID); err != nil {
		t.Fatal(err)
	}
	if got := lineCount(t, reopened.logPath()); got != linesBefore {
		t.Fatalf("redundant attempt appended %d events, want 0", got-linesBefore)
	}

	// Recording the outcome settles the wake with exactly one attempt and one
	// outcome in the durable ledger.
	if _, err := reopened.RecordWakeOutcome(wakeID, true, ""); err != nil {
		t.Fatal(err)
	}
	if drained, _ := reopened.PendingWakes(); len(drained) != 0 {
		t.Fatalf("pending after replay outcome = %#v, want empty", drained)
	}
	entries := ledgerEntries(t, reopened.logPath())
	if got := countType(entries, eventWakeAttempt); got != 1 {
		t.Fatalf("wake_attempt count = %d, want exactly 1", got)
	}
	if got := countType(entries, eventWakeOutcome); got != 1 {
		t.Fatalf("wake_outcome count = %d, want exactly 1", got)
	}
}

// truncateAfterType trims the ledger so it ends cleanly at the newline after the
// first line whose event type matches, modelling a torn append that
// readAndRepairLog has already repaired back to a record boundary.
func truncateAfterType(t *testing.T, path, eventType string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	keep := 0
	found := false
	for _, line := range bytes.SplitAfter(data, []byte{'\n'}) {
		keep += len(line)
		if entry, ok := DecodeLedgerLine(line); ok && entry.Type == eventType {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no %s event in ledger %q", eventType, path)
	}
	if err := os.Truncate(path, int64(keep)); err != nil {
		t.Fatal(err)
	}
}
