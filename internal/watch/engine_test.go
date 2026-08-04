package watch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/statedir"
)

const testSession = "fledge-test-0a1b2c3d"

func TestCycleWakesOnABlockedStatusLine(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "blocked: needs a decision on the schema")

	h.runCycle()

	body := h.onlyDelivery()
	if !strings.Contains(body, "reviewer blocked: needs a decision on the schema") {
		t.Errorf("delivered body = %q, want the blocked reason", body)
	}
}

func TestCycleLogsEveryWakeAsAnAuditTrail(t *testing.T) {
	t.Parallel()

	// The ledger is a work queue that Compact empties, so the decision log is
	// the durable record: every wake needs a queued line naming its ledger ID
	// and a delivered line naming the message it went out as.
	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "blocked: needs a decision")

	h.runCycle()

	if line := h.loggedLine("queued", "w-1", "reviewer"); line == "" {
		t.Errorf("decision log = %q, want a queued line naming ledger ID w-1", h.logs)
	}
	if line := h.loggedLine("delivered", "m-1", "w-1"); line == "" {
		t.Errorf("decision log = %q, want a delivered line pairing message ID m-1 with wake ID w-1", h.logs)
	}
}

func TestCycleDoesNotTreatAnEmptySnapshotAsAMassDeparture(t *testing.T) {
	t.Parallel()

	// A live session always contains the orchestrator, so a snapshot with no
	// agents at all is an untrustworthy read — a hiccup or a mid-teardown
	// glimpse — not every worker leaving at once.
	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"), worker("migrator", "p2", "working"))
	h.runCycle()

	h.herdr.snapshot = herdr.Snapshot{}
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds+1) * time.Second)
	h.runCycle()

	h.wantNoDelivery()

	// The workers are still known once the session reads correctly again.
	h.snapshot(worker("reviewer", "p1", "working"))
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds+1) * time.Second)
	h.runCycle()
	if body := h.onlyDelivery(); !strings.Contains(body, "migrator") {
		t.Errorf("delivered body = %q, want migrator reported gone once the snapshot was trustworthy", body)
	}
}

func TestCycleDoesNotTreatASnapshotMissingTheOrchestratorAsADeparture(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"), worker("migrator", "p2", "working"))
	h.runCycle()

	h.herdr.snapshot = herdr.Snapshot{Agents: []herdr.Agent{worker("reviewer", "p1", "working")}}
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds+1) * time.Second)
	h.runCycle()

	h.wantNoDelivery()
	if got := strings.Join(h.ledger.markers.KnownAgents, ","); got != "migrator,reviewer" {
		t.Errorf("KnownAgents = %q, want the last trustworthy snapshot retained", got)
	}
	if h.loggedLine("suspect", "orchestrator") == "" {
		t.Errorf("decision log = %q, want the suspect snapshot recorded", h.logs)
	}
}

func TestCycleCountsAStrikeWhenTheSubscriptionNeverAcks(t *testing.T) {
	t.Parallel()

	// A stream ctx that expires before the ack also surfaces as
	// DeadlineExceeded. Treating that as a healthy cycle lets a Herdr that
	// accepts connections but never acks look fine forever.
	h := newHarness(t)
	h.engine.Config.EventStream = true
	h.snapshot(worker("reviewer", "p1", "working"))
	h.subscriber.silent = true

	for range 3 {
		h.runCycle()
	}
	if h.subscriber.calls != 3 {
		t.Fatalf("engine subscribed %d times, want 3 attempts", h.subscriber.calls)
	}

	h.runCycle()
	if h.subscriber.calls != 3 {
		t.Errorf("engine subscribed %d times, want the never-acking stream to count strikes and be disabled", h.subscriber.calls)
	}
}

func TestCycleKeepsMarkersWhenTheLedgerRefusedTheWake(t *testing.T) {
	t.Parallel()

	// A transient append failure is the one case where the queue-before-marker
	// invariant could break: the wake only exists in memory, so advancing the
	// status offset would lose it outright if the process died next.
	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "blocked: needs a decision")
	h.ledger.appendErr = errors.New("disk full")
	h.waker.err = errors.New("orchestrator is gone")

	h.runCycle()
	if len(h.ledger.markers.StatusSeen) != 0 {
		t.Errorf("markers advanced to %+v, want the unqueued status line left to be re-read", h.ledger.markers.StatusSeen)
	}

	h.ledger.appendErr = nil
	h.waker.err = nil
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds+1) * time.Second)
	h.runCycle()

	body := h.onlyDelivery()
	if got := strings.Count(body, "blocked: needs a decision"); got != 1 {
		t.Errorf("delivered body = %q, want the recovered wake exactly once", body)
	}
}

func TestCycleDoesNotDeliverAnOrdinaryAppendFailureFromMemory(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "blocked: needs a decision")
	h.ledger.appendErr = errors.New("disk full")

	h.runCycle()
	h.wantNoDelivery()
	if len(h.ledger.markers.StatusSeen) != 0 {
		t.Errorf("markers advanced to %+v, want the failed observation retried", h.ledger.markers.StatusSeen)
	}
}

func TestCycleRollsBackABatchWhenOneLedgerAppendFails(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"), worker("migrator", "p2", "working"))
	h.appendStatus("reviewer", "blocked: review needed")
	h.appendStatus("migrator", "failed: build broke")
	h.ledger.failAppendCall = 2

	h.runCycle()
	h.wantNoDelivery()
	if len(h.ledger.records) != 1 {
		t.Fatalf("durable records = %+v, want the first successful append retained", h.ledger.records)
	}
	if len(h.ledger.markers.StatusSeen) != 0 {
		t.Errorf("markers advanced to %+v, want the whole observation batch retried", h.ledger.markers.StatusSeen)
	}

	h.ledger.failAppendCall = 0
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds+1) * time.Second)
	h.runCycle()
	body := h.onlyDelivery()
	if !strings.Contains(body, "reviewer blocked") || !strings.Contains(body, "migrator failed") {
		t.Errorf("delivered body = %q, want both retried observations", body)
	}
}

func TestCycleSuppressesTheDeadWakeWhileTheDoneGraceIsOpen(t *testing.T) {
	t.Parallel()

	// Reporting done and exiting before the grace resolves is the normal
	// finish; the grace decides that case, so departure must not pre-empt it.
	h := newHarness(t)
	h.snapshot(worker("migrator", "p1", "working"))
	h.appendStatus("migrator", "done: shipped")
	h.runCycle()
	h.wantNoDelivery()

	h.snapshot(worker("reviewer", "p2", "working"))
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds+1) * time.Second)
	h.runCycle()

	for _, body := range h.waker.bodies {
		if strings.Contains(body, "vanished") {
			t.Errorf("delivered body = %q, want the done grace to decide instead of a dead wake", body)
		}
	}
}

func TestCycleWaitsOutTheSignalGraceBeforeReadingAStatusFile(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "blocked: needs a decision")

	h.runCycle()

	grace := time.Duration(h.engine.Config.SignalGraceSeconds) * time.Second
	if !h.slept(grace) {
		t.Errorf("cycle slept %v, want the %s signal grace before classifying", h.clock.slept, grace)
	}
}

func TestCycleSurvivesHerdrFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		apply func(*harness)
	}{
		{name: "list fails", apply: func(h *harness) { h.herdr.listErr = errors.New("herdr is unreachable") }},
		{name: "snapshot fails", apply: func(h *harness) { h.herdr.snapshotErr = errors.New("herdr is unreachable") }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.snapshot(worker("reviewer", "p1", "working"))
			test.apply(h)

			alive, err := h.engine.cycle(t.Context())
			if err != nil {
				t.Fatalf("cycle() error = %v, want a transient Herdr failure to be survivable", err)
			}
			if !alive {
				t.Error("cycle() stopped the watcher over a transient Herdr failure")
			}
			if got := h.clock.lastSleep(); got < time.Second {
				t.Errorf("cycle slept %s after the failure, want a full interval rather than a spin", got)
			}
			h.wantNoDelivery()
		})
	}
}

func TestCycleAbsorbsProgressStatusLines(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("migrator", "p1", "working"))
	h.appendStatus("migrator", "working: refactoring pass 2", "paused: waiting on the build")

	h.runCycle()

	h.wantNoDelivery()
}

func TestCycleReadsOnlyNewStatusLines(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "blocked: first")
	h.runCycle()
	h.waker.bodies = nil
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds) * time.Second)

	h.runCycle()
	h.wantNoDelivery()

	h.appendStatus("reviewer", "blocked: second")
	h.runCycle()
	if body := h.onlyDelivery(); !strings.Contains(body, "blocked: second") || strings.Contains(body, "blocked: first") {
		t.Errorf("delivered body = %q, want only the new line", body)
	}
}

func TestCycleIgnoresAPartialStatusLine(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.writeStatusRaw("reviewer", "blocked: half a li")

	h.runCycle()
	h.wantNoDelivery()

	h.writeStatusRaw("reviewer", "blocked: half a line arrived\n")
	h.runCycle()
	if body := h.onlyDelivery(); !strings.Contains(body, "blocked: half a line arrived") {
		t.Errorf("delivered body = %q, want the completed line", body)
	}
}

func TestCycleStripsOneBOMAtTheStartOfAStatusFile(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.writeStatusRaw("reviewer", "\ufeffblocked: PowerShell wrote this file\n")

	h.runCycle()
	if body := h.onlyDelivery(); !strings.Contains(body, "PowerShell wrote this file") {
		t.Errorf("delivered body = %q, want the BOM-prefixed first status", body)
	}
}

func TestCycleDoesNotStripMoreThanOneBOMOrABOMOnALaterLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
	}{
		{name: "double BOM", contents: "\ufeff\ufeffblocked: ignored\n"},
		{name: "later line", contents: "working: first\n\ufeffblocked: ignored\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.snapshot(worker("reviewer", "p1", "working"))
			h.writeStatusRaw("reviewer", test.contents)
			h.runCycle()
			h.wantNoDelivery()
		})
	}
}

func TestCycleAbsorbsDoneWhenTheWorkerMessagedTheOrchestrator(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "done: shipped")
	h.completions.completed["reviewer"] = true

	h.runCycle()
	h.wantNoDelivery()

	h.clock.advance(time.Duration(h.engine.Config.DoneMessageGraceSeconds+1) * time.Second)
	h.runCycle()
	h.wantNoDelivery()
}

func TestCycleWakesWhenDoneNeverReachedTheOrchestrator(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "done: shipped")

	h.runCycle()
	h.wantNoDelivery()

	h.clock.advance(time.Duration(h.engine.Config.DoneMessageGraceSeconds+1) * time.Second)
	h.runCycle()

	body := h.onlyDelivery()
	if !strings.Contains(body, "reviewer") || !strings.Contains(body, "completion message") {
		t.Errorf("delivered body = %q, want a swallowed-completion reason", body)
	}

	h.waker.bodies = nil
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds+1) * time.Second)
	h.runCycle()
	h.wantNoDelivery()
}

func TestCycleLooksBackForACompletionSentBeforeTheDoneLine(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "done: shipped")
	h.runCycle()
	doneAt := time.Unix(h.ledger.markers.DoneGrace["reviewer"], 0).UTC()

	h.clock.advance(time.Duration(h.engine.Config.DoneMessageGraceSeconds+1) * time.Second)
	h.runCycle()

	if len(h.completions.calls) == 0 {
		t.Fatal("engine never consulted the completion log")
	}
	call := h.completions.calls[len(h.completions.calls)-1]
	grace := time.Duration(h.engine.Config.DoneMessageGraceSeconds) * time.Second
	want := doneAt.Add(-grace)
	if !call.since.Equal(want) {
		t.Errorf("CompletionSince(since=%s), want exact done-grace boundary %s", call.since, want)
	}
}

func TestCycleAcceptsAVanishedWorkersCompletionFromAnyPointInTheSession(t *testing.T) {
	t.Parallel()

	// The departure question is whether the worker ever messaged the
	// orchestrator this session, not whether it did so inside a window the
	// watcher's own start time defines: a window would re-report every worker
	// that finished before the watcher restarted.
	h := newHarness(t)
	h.ledger.markers.KnownAgents = []string{"migrator"}
	h.snapshot(worker("reviewer", "p2", "working"))
	h.completions.at["migrator"] = h.start.Add(-24 * time.Hour)

	h.runCycle()
	h.wantNoDelivery()

	if len(h.completions.calls) != 1 {
		t.Fatalf("CompletionSince calls = %+v, want one dead-worker lookup", h.completions.calls)
	}
	if got := h.completions.calls[0].since; !got.IsZero() {
		t.Errorf("CompletionSince(since=%s), want the whole session searched", got)
	}
}

func TestCycleRemembersATerminalVerbAcrossAWatcherRestart(t *testing.T) {
	t.Parallel()

	// Terminal state that lives only in the process is lost when the watcher
	// restarts, and the worker's later departure then reads as a vanishing —
	// a spurious wake for a failure the orchestrator was already told about.
	h := newHarness(t)
	h.snapshot(worker("migrator", "p1", "working"))
	h.appendStatus("migrator", "failed: build broke")
	h.runCycle()
	if body := h.onlyDelivery(); !strings.Contains(body, "migrator failed: build broke") {
		t.Fatalf("delivered body = %q, want the failure wake", body)
	}

	h.waker.bodies = nil
	h.restart()
	h.snapshot(worker("reviewer", "p2", "working"))
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds+1) * time.Second)
	h.runCycle()

	h.wantNoDelivery()
}

func TestCycleWakesForAWorkerThatDepartedAfterAnUnqueuedFailure(t *testing.T) {
	t.Parallel()

	// The rollback that keeps an unqueued observation replayable has to undo
	// the terminal mark that observation made too. The worker leaves before the
	// retry, so nothing ever re-reads its status file, and a mark left standing
	// suppresses the departure wake as well: the failure reaches nobody, ever.
	h := newHarness(t)
	h.snapshot(worker("migrator", "p1", "working"), worker("reviewer", "p2", "working"))
	h.runCycle()
	h.wantNoDelivery()

	h.appendStatus("migrator", "failed: build broke")
	h.ledger.appendErr = errors.New("disk full")
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds+1) * time.Second)
	h.runCycle()
	h.wantNoDelivery()

	h.ledger.appendErr = nil
	h.snapshot(worker("reviewer", "p2", "working"))
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds+1) * time.Second)
	h.runCycle()

	if body := h.onlyDelivery(); !strings.Contains(body, "migrator") {
		t.Errorf("delivered body = %q, want the departed worker escalated", body)
	}
}

func TestCycleWakesOnceForAVanishedWorker(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("migrator", "p1", "working"))
	h.runCycle()
	h.wantNoDelivery()

	h.snapshot(worker("reviewer", "p2", "working"))
	h.runCycle()

	if body := h.onlyDelivery(); !strings.Contains(body, "migrator") {
		t.Errorf("delivered body = %q, want a dead-worker reason for migrator", body)
	}

	h.waker.bodies = nil
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds+1) * time.Second)
	h.runCycle()
	h.wantNoDelivery()
}

func TestCycleSkipsTheDeadWakeAfterACompletionMessage(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("migrator", "p1", "working"))
	h.runCycle()

	h.completions.completed["migrator"] = true
	h.snapshot(worker("reviewer", "p2", "working"))
	h.runCycle()

	h.wantNoDelivery()
}

func TestCycleSkipsTheDeadWakeAfterATerminalVerb(t *testing.T) {
	t.Parallel()

	// A worker that reported a terminal verb has already been accounted for:
	// its failure woke the orchestrator, and its disappearance must not wake
	// it a second time even though no completion message was ever sent.
	h := newHarness(t)
	h.snapshot(worker("migrator", "p1", "working"))
	h.appendStatus("migrator", "failed: build broke")
	h.runCycle()
	if body := h.onlyDelivery(); !strings.Contains(body, "migrator failed: build broke") {
		t.Fatalf("delivered body = %q, want the failure wake", body)
	}

	h.waker.bodies = nil
	h.snapshot(worker("reviewer", "p2", "working"))
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds+1) * time.Second)
	h.runCycle()

	h.wantNoDelivery()
}

func TestCycleGoesDormantWithoutWorkers(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.engine.Config.EventStream = true
	h.snapshot()

	h.runCycle()

	if h.subscriber.calls != 0 {
		t.Errorf("engine subscribed %d times, want no subscription while dormant", h.subscriber.calls)
	}
	want := time.Duration(h.engine.Config.IdlePollIntervalSeconds) * time.Second
	if got := h.clock.lastSleep(); got != want {
		t.Errorf("dormant cycle slept %s, want the idle interval %s", got, want)
	}
}

func TestCycleBatchesEveryReasonIntoOneMessage(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"), worker("migrator", "p2", "working"))
	h.appendStatus("reviewer", "blocked: needs a decision")
	h.appendStatus("migrator", "failed: build broke")

	h.runCycle()

	body := h.onlyDelivery()
	if !strings.Contains(body, "reviewer blocked: needs a decision") || !strings.Contains(body, "migrator failed: build broke") {
		t.Errorf("delivered body = %q, want both reasons in one message", body)
	}
}

func TestCycleHoldsWakesInsideTheRateWindow(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"), worker("migrator", "p2", "working"))
	h.appendStatus("reviewer", "blocked: first")
	h.runCycle()
	h.onlyDelivery()

	h.waker.bodies = nil
	h.appendStatus("migrator", "blocked: second")
	h.runCycle()
	h.wantNoDelivery()

	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds) * time.Second)
	h.runCycle()
	if body := h.onlyDelivery(); !strings.Contains(body, "migrator blocked: second") {
		t.Errorf("delivered body = %q, want the held wake once the window reopened", body)
	}
}

func TestCycleKeepsTheQueueWhenDeliveryFails(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "blocked: needs a decision")
	h.waker.err = errors.New("orchestrator is gone")

	h.runCycle()

	if len(h.ledger.records) != 1 {
		t.Fatalf("ledger holds %d records, want the undelivered wake to stay queued", len(h.ledger.records))
	}
	if len(h.ledger.deliveredIDs) != 0 {
		t.Errorf("ledger marked %v delivered, want nothing marked after a failed delivery", h.ledger.deliveredIDs)
	}

	h.waker.err = nil
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds) * time.Second)
	h.runCycle()

	if body := h.onlyDelivery(); !strings.Contains(body, "blocked: needs a decision") {
		t.Errorf("delivered body = %q, want the retried wake", body)
	}
}

func TestCycleMarksEverySupersededIDDelivered(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "blocked: first")
	h.waker.err = errors.New("orchestrator is busy")
	h.runCycle()

	h.waker.err = nil
	h.appendStatus("reviewer", "blocked: second")
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds) * time.Second)
	h.runCycle()

	// Both appends collapsed into one record; retiring only the survivor
	// would leave the superseded entry queued to resurface with a stale
	// reason on the next drain.
	if len(h.ledger.deliveredIDs) != 2 {
		t.Errorf("MarkDelivered saw %v, want both the superseded and surviving IDs", h.ledger.deliveredIDs)
	}
	if len(h.ledger.records) != 0 {
		t.Errorf("ledger still holds %+v, want an empty queue", h.ledger.records)
	}
}

func TestCycleQueuesBeforeAdvancingMarkers(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "blocked: needs a decision")
	h.ledger.saveErr = errors.New("disk full")
	h.waker.err = errors.New("orchestrator is gone")

	h.runCycle()

	if len(h.ledger.records) != 1 {
		t.Fatalf("ledger holds %d records, want the wake queued before the marker advanced", len(h.ledger.records))
	}
	if len(h.ledger.recordsAtSave) == 0 {
		t.Fatal("engine never saved markers")
	}
	if h.ledger.recordsAtSave[0] == 0 {
		t.Error("markers advanced while the ledger was still empty; a crash there loses the wake")
	}

	// Replay: the markers never advanced, so the same line is read again and
	// must collapse onto the queued wake instead of duplicating it.
	h.ledger.saveErr = nil
	h.waker.err = nil
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds) * time.Second)
	h.runCycle()

	body := h.onlyDelivery()
	if got := strings.Count(body, "blocked: needs a decision"); got != 1 {
		t.Errorf("delivered body = %q, want the replayed wake exactly once", body)
	}
}

func TestCycleWakesOncePerBlockedPaneAndRearmsOnWorking(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.engine.Config.EventStream = true
	h.snapshot(worker("reviewer", "p1", "working"))
	h.subscriber.stream(
		[]Event{{PaneID: "p1", AgentStatus: "blocked", Agent: "claude"}, {PaneID: "p1", AgentStatus: "blocked", Agent: "claude"}},
		[]Event{{PaneID: "p1", AgentStatus: "working", Agent: "claude"}},
		[]Event{{PaneID: "p1", AgentStatus: "blocked", Agent: "claude"}},
	)

	h.runCycle()
	if body := h.onlyDelivery(); !strings.Contains(body, "reviewer") {
		t.Errorf("delivered body = %q, want one wake naming the blocked worker", body)
	}

	h.waker.bodies = nil
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds) * time.Second)
	h.runCycle()
	h.wantNoDelivery()

	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds) * time.Second)
	h.runCycle()
	if body := h.onlyDelivery(); !strings.Contains(body, "reviewer") {
		t.Errorf("delivered body = %q, want the re-armed pane to wake again", body)
	}
}

func TestCycleReconcilesAlreadyBlockedWorkersAfterSubscribing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.engine.Config.EventStream = true
	h.snapshot(worker("reviewer", "p1", "blocked"))

	h.runCycle()

	if body := h.onlyDelivery(); !strings.Contains(body, "reviewer") {
		t.Errorf("delivered body = %q, want the pre-existing blocked worker to wake", body)
	}
	if !h.subscriber.reconciledAfterReady {
		t.Error("engine did not snapshot after the subscription was acknowledged, leaving a gap where a transition is lost")
	}
}

func TestCycleRollsBackEventEscalationWhenTheLedgerAppendFails(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.engine.Config.EventStream = true
	h.snapshot(worker("reviewer", "p1", "blocked"))
	h.ledger.appendErr = errors.New("disk full")

	h.runCycle()
	h.wantNoDelivery()
	if h.ledger.markers.EventEscalated["p1"] {
		t.Error("event escalation marker advanced without a durable wake")
	}
}

func TestCycleFallsBackToPollingAfterThreeEventFailures(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.engine.Config.EventStream = true
	h.snapshot(worker("reviewer", "p1", "working"))
	h.subscriber.err = errors.New("socket refused")

	for range 3 {
		h.runCycle()
	}
	if h.subscriber.calls != 3 {
		t.Fatalf("engine subscribed %d times, want 3 attempts before giving up", h.subscriber.calls)
	}

	h.runCycle()
	if h.subscriber.calls != 3 {
		t.Errorf("engine subscribed %d times, want the event stream disabled after 3 failures", h.subscriber.calls)
	}
	want := time.Duration(h.engine.Config.PollIntervalSeconds) * time.Second
	if got := h.clock.lastSleep(); got != want {
		t.Errorf("poll-only cycle slept %s, want the poll interval %s", got, want)
	}
}

func TestCycleTreatsAnExpiredEventBudgetAsACleanCycle(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.engine.Config.EventStream = true
	h.snapshot(worker("reviewer", "p1", "working"))
	h.subscriber.err = errors.New("socket refused")
	h.runCycle()
	h.runCycle()

	h.subscriber.err = nil
	h.runCycle()

	// Two more failures only reach a streak of two, so the stream stays armed
	// and a sixth cycle still subscribes.
	h.subscriber.err = errors.New("socket refused")
	for range 3 {
		h.runCycle()
	}
	if h.subscriber.calls != 6 {
		t.Errorf("engine subscribed %d times, want a clean cycle to reset the failure streak", h.subscriber.calls)
	}
}

func TestCycleForgivesAStreamThatLivedMostOfItsBudget(t *testing.T) {
	t.Parallel()

	// A stream that carried events for most of its budget and then ended is
	// Herdr recycling a connection. Counting those as strikes lets three
	// ordinary early EOFs, hours apart, retire the push stream for good.
	h := newHarness(t)
	h.engine.Config.EventStream = true
	h.snapshot(worker("reviewer", "p1", "working"))
	h.subscriber.liveFor = h.engine.pollInterval()
	h.subscriber.err = errors.New("herdr closed the stream")

	for range 4 {
		h.runCycle()
	}

	if h.subscriber.calls != 4 {
		t.Errorf("engine subscribed %d times, want a healthy stretch to clear the failure streak", h.subscriber.calls)
	}
}

func TestCycleRetriesTheEventStreamAfterStandingItDown(t *testing.T) {
	t.Parallel()

	// Giving up on the push stream for the life of the process turns a Herdr
	// restart into permanently slow supervision, so the stand-down expires.
	h := newHarness(t)
	h.engine.Config.EventStream = true
	h.snapshot(worker("reviewer", "p1", "working"))
	h.subscriber.err = errors.New("socket refused")

	for range 4 {
		h.runCycle()
	}
	if h.subscriber.calls != 3 {
		t.Fatalf("engine subscribed %d times, want the stream stood down after 3 failures", h.subscriber.calls)
	}

	h.clock.advance(eventsDisabledFor * h.engine.pollInterval())
	h.subscriber.err = nil
	h.runCycle()

	if h.subscriber.calls != 4 {
		t.Errorf("engine subscribed %d times, want the stand-down to expire", h.subscriber.calls)
	}
}

func TestCycleBoundsEveryCallOutOfTheProcess(t *testing.T) {
	t.Parallel()

	// A wedged Herdr subprocess or a stalled delivery answers neither with
	// data nor an error, so only a deadline the engine imposes ends the wait.
	h := newHarness(t)
	h.engine.Timeout = context.WithTimeout
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "blocked: needs a decision")

	h.runCycle()

	budgets := map[string]struct {
		got  time.Duration
		want time.Duration
	}{
		"List":     {got: h.herdr.listBudget, want: herdrCallTimeout},
		"Snapshot": {got: h.herdr.snapshotBudget, want: herdrCallTimeout},
		"Deliver":  {got: h.waker.deliverBudget, want: deliverTimeout},
	}
	for name, budget := range budgets {
		if budget.got <= 0 || budget.got > budget.want {
			t.Errorf("%s ran with a %s deadline, want one of at most %s", name, budget.got, budget.want)
		}
	}
}

func TestCycleDoesNotHotSpinOnEventFailures(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.engine.Config.EventStream = true
	h.snapshot(worker("reviewer", "p1", "working"))
	h.subscriber.err = errors.New("socket refused")

	h.runCycle()

	if got := h.clock.lastSleep(); got <= 0 {
		t.Errorf("failed event cycle slept %s, want it to wait out the poll budget", got)
	}
}

func TestHeartbeatBacksOffAndResetsOnARealWake(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))

	// The first cycle is what starts the quiet clock.
	h.runCycle()
	h.wantNoDelivery()

	h.clock.advance(time.Duration(h.engine.Config.HeartbeatSeconds+1) * time.Second)
	h.runCycle()
	if body := h.onlyDelivery(); !strings.Contains(strings.ToLower(body), "heartbeat") {
		t.Errorf("delivered body = %q, want a heartbeat", body)
	}

	// The next beat is twice as far away: the previous interval alone is not
	// enough to earn another one.
	h.waker.bodies = nil
	h.clock.advance(time.Duration(h.engine.Config.HeartbeatSeconds+1) * time.Second)
	h.runCycle()
	h.wantNoDelivery()

	h.clock.advance(time.Duration(h.engine.Config.HeartbeatSeconds) * time.Second)
	h.runCycle()
	if body := h.onlyDelivery(); !strings.Contains(strings.ToLower(body), "heartbeat") {
		t.Errorf("delivered body = %q, want the backed-off heartbeat", body)
	}

	// A real wake resets the streak, so one plain interval earns a beat again.
	h.waker.bodies = nil
	h.appendStatus("reviewer", "blocked: needs a decision")
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds) * time.Second)
	h.runCycle()
	h.onlyDelivery()

	h.waker.bodies = nil
	h.clock.advance(time.Duration(h.engine.Config.HeartbeatSeconds+1) * time.Second)
	h.runCycle()
	if body := h.onlyDelivery(); !strings.Contains(strings.ToLower(body), "heartbeat") {
		t.Errorf("delivered body = %q, want the streak reset by the real wake", body)
	}
}

func TestHeartbeatIntervalIsCappedByTheConfiguredMaximum(t *testing.T) {
	t.Parallel()

	config := defaultConfig()
	tests := []struct {
		streak int
		want   time.Duration
	}{
		{streak: 0, want: 600 * time.Second},
		{streak: 1, want: 1200 * time.Second},
		{streak: 2, want: 2400 * time.Second},
		{streak: 4, want: 7200 * time.Second},
		{streak: 50, want: 7200 * time.Second},
	}

	for _, test := range tests {
		t.Run(fmt.Sprint(test.streak), func(t *testing.T) {
			t.Parallel()

			if got := heartbeatInterval(config, test.streak); got != test.want {
				t.Errorf("heartbeatInterval(streak=%d) = %s, want %s", test.streak, got, test.want)
			}
		})
	}
}

func TestCycleDegradesWhenTheLedgerIsCorrupt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "blocked: needs a decision")
	h.ledger.appendErr = fmt.Errorf("read wake ledger: %w", ErrCorruptLog)

	h.runCycle()

	if len(h.waker.bodies) == 0 {
		t.Fatal("engine delivered nothing, want a corruption notice")
	}
	notice := h.waker.bodies[0]
	if !strings.Contains(notice, "corrupt") {
		t.Errorf("first delivery = %q, want a corruption notice", notice)
	}

	// Degraded, but still supervising: the next wake is delivered from memory
	// and the notice is not repeated.
	h.waker.bodies = nil
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds) * time.Second)
	h.appendStatus("reviewer", "blocked: still stuck")
	h.runCycle()

	body := h.onlyDelivery()
	if strings.Contains(body, "corrupt") {
		t.Errorf("delivered body = %q, want the corruption notice sent only once", body)
	}
	if !strings.Contains(body, "blocked: still stuck") {
		t.Errorf("delivered body = %q, want supervision to continue in memory", body)
	}
}

func TestDegradedModeReplaysItsObservationsAfterTheWatcherDies(t *testing.T) {
	t.Parallel()

	// A degraded watcher holds its wakes in memory, so a process that dies
	// takes them with it. Advancing the stored markers anyway suppresses the
	// observations behind those wakes forever; leaving the stored markers
	// where they were is what keeps the guarantee at least-once.
	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "blocked: needs a decision")
	h.ledger.appendErr = fmt.Errorf("read wake ledger: %w", ErrCorruptLog)
	h.waker.err = errors.New("orchestrator is gone")

	h.runCycle()
	h.wantNoDelivery()

	// A fresh watcher over the same stored markers and status file: the wake
	// the dead process held has to be observed a second time.
	h.ledger.appendErr = nil
	h.waker.err = nil
	h.restart()
	h.clock.advance(time.Duration(h.engine.Config.WakeMinIntervalSeconds+1) * time.Second)
	h.runCycle()

	if body := h.onlyDelivery(); !strings.Contains(body, "blocked: needs a decision") {
		t.Errorf("delivered body = %q, want the lost wake replayed", body)
	}
}

func TestCycleFallsBackToTheDefaultDoneGraceWhenTheConfiguredOneIsNegative(t *testing.T) {
	t.Parallel()

	// A negative grace makes the completion look-back reach into the future,
	// so every clean finish is reported as a swallowed one.
	h := newHarness(t)
	h.writeConfig(`{"done_message_grace_seconds": -5, "event_stream": false}`)
	h.engine.Config = LoadConfig(h.root)
	h.snapshot(worker("reviewer", "p1", "working"))
	h.appendStatus("reviewer", "done: shipped")
	h.completions.at["reviewer"] = h.clock.Now()

	h.runCycle()
	h.wantNoDelivery()

	h.clock.advance(time.Duration(h.engine.Config.DoneMessageGraceSeconds+1) * time.Second)
	h.runCycle()

	h.wantNoDelivery()
}

func TestCycleClampsIntervalsToAvoidAHotSpin(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.engine.Config.PollIntervalSeconds = 0
	h.engine.Config.IdlePollIntervalSeconds = 0
	h.snapshot(worker("reviewer", "p1", "working"))

	h.runCycle()
	if got := h.clock.lastSleep(); got < time.Second {
		t.Errorf("poll cycle slept %s, want at least one second", got)
	}

	h.snapshot()
	h.runCycle()
	if got := h.clock.lastSleep(); got < time.Second {
		t.Errorf("dormant cycle slept %s, want at least one second", got)
	}
}

func TestRunStopsWhenTheSessionIsGone(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.herdr.sessions = nil

	if err := h.runUntilExit(); err != nil {
		t.Errorf("Run() error = %v, want a clean exit once the session is gone", err)
	}
	if h.herdr.snapshots != 0 {
		t.Errorf("engine took %d snapshots, want none once the session is gone", h.herdr.snapshots)
	}
}

func TestRunStopsWhenTheSessionIsNotRunning(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.herdr.sessions = []herdr.Session{{Name: testSession, Running: false}}

	if err := h.runUntilExit(); err != nil {
		t.Errorf("Run() error = %v, want a clean exit once the session stopped", err)
	}
}

func TestRunStopsWhenTheContextIsCanceled(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.snapshot(worker("reviewer", "p1", "working"))

	ctx, cancel := context.WithCancel(t.Context())
	h.clock.onSleep = func() { cancel() }

	returned := make(chan error, 1)
	go func() { returned <- h.engine.Run(ctx) }()

	select {
	case err := <-returned:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after the context was canceled")
	}
}

// --- harness ------------------------------------------------------------

type harness struct {
	t           *testing.T
	engine      *Engine
	herdr       *fakeHerdrAPI
	ledger      *fakeLedger
	waker       *fakeWaker
	completions *fakeCompletions
	clock       *stepClock
	subscriber  *fakeSubscriber
	root        string
	start       time.Time
	logs        []string
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(statedir.StatusDir(root, testSession), 0o700); err != nil {
		t.Fatalf("create status directory: %v", err)
	}

	start := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	clock := &stepClock{now: start}
	fakes := &harness{
		t:           t,
		herdr:       &fakeHerdrAPI{sessions: []herdr.Session{{Name: testSession, Running: true}}},
		ledger:      newFakeLedger(),
		waker:       &fakeWaker{messageID: "m-1"},
		completions: &fakeCompletions{completed: map[string]bool{}, at: map[string]time.Time{}},
		clock:       clock,
		subscriber:  &fakeSubscriber{},
		root:        root,
		start:       start,
	}

	fakes.subscriber.herdr = fakes.herdr
	fakes.subscriber.t = t
	fakes.subscriber.now = clock.Now
	fakes.subscriber.advance = clock.advance

	config := defaultConfig()
	config.EventStream = false
	fakes.engine = fakes.newEngine(config)
	fakes.subscriber.pollInterval = fakes.engine.pollInterval()

	return fakes
}

func (h *harness) newEngine(config Config) *Engine {
	return &Engine{
		Root:        h.root,
		Session:     testSession,
		Config:      config,
		Herdr:       h.herdr,
		Ledger:      h.ledger,
		Waker:       h.waker,
		Completions: h.completions,
		Subscriber:  h.subscriber.subscribe,
		Now:         h.clock.Now,
		Sleep:       h.clock.SleepFor,
		Log:         func(message string) { h.logs = append(h.logs, message) },
		Timeout:     immediateTimeout(h.clock.Now),
	}
}

// restart replaces the engine with a fresh one over the same status files,
// ledger and stored markers: what survives a watcher restart is whatever the
// markers hold.
func (h *harness) restart() {
	h.t.Helper()

	h.engine = h.newEngine(h.engine.Config)
}

// writeConfig installs a project-local watch.json so a test can exercise the
// real loader rather than a hand-built Config.
func (h *harness) writeConfig(contents string) {
	h.t.Helper()

	if err := os.MkdirAll(statedir.Root(h.root), 0o755); err != nil {
		h.t.Fatalf("create state directory: %v", err)
	}
	path := filepath.Join(statedir.Root(h.root), configFilename)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		h.t.Fatalf("write watch config: %v", err)
	}
}

func (h *harness) runCycle() {
	h.t.Helper()

	if _, err := h.engine.cycle(h.t.Context()); err != nil {
		h.t.Fatalf("cycle() error = %v", err)
	}
}

// runUntilExit runs the loop expecting it to exit on its own. Any wait means
// it decided to keep supervising, so the context is canceled to end the run
// and the returned error reports what happened instead of hanging the test.
func (h *harness) runUntilExit() error {
	h.t.Helper()

	ctx, cancel := context.WithCancel(h.t.Context())
	defer cancel()
	h.clock.onSleep = cancel

	returned := make(chan error, 1)
	go func() { returned <- h.engine.Run(ctx) }()

	select {
	case err := <-returned:
		return err
	case <-time.After(5 * time.Second):
		h.t.Fatal("Run() never returned")
		return nil
	}
}

func (h *harness) snapshot(agents ...herdr.Agent) {
	h.t.Helper()

	orchestrator := worker(orchestratorAgent, "p0", "idle")
	h.herdr.snapshot = herdr.Snapshot{Agents: append([]herdr.Agent{orchestrator}, agents...)}
}

func (h *harness) appendStatus(agent string, lines ...string) {
	h.t.Helper()

	h.writeStatusRaw(agent, strings.Join(lines, "\n")+"\n")
}

func (h *harness) writeStatusRaw(agent, contents string) {
	h.t.Helper()

	path := statedir.StatusFile(h.root, testSession, agent)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		h.t.Fatalf("open status file: %v", err)
	}
	defer file.Close()

	if _, err := file.WriteString(contents); err != nil {
		h.t.Fatalf("write status file: %v", err)
	}
	// The engine compares mtime to decide a file changed; temp filesystems are
	// coarse enough that same-second writes would otherwise look identical.
	stamp := h.clock.Now().Add(time.Duration(len(contents)) * time.Second)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		h.t.Fatalf("stamp status file: %v", err)
	}
}

func (h *harness) slept(d time.Duration) bool {
	h.t.Helper()

	for _, slept := range h.clock.slept {
		if slept == d {
			return true
		}
	}
	return false
}

func (h *harness) onlyDelivery() string {
	h.t.Helper()

	if len(h.waker.bodies) != 1 {
		h.t.Fatalf("engine delivered %d messages (%q), want exactly 1", len(h.waker.bodies), h.waker.bodies)
	}
	return h.waker.bodies[0]
}

func (h *harness) wantNoDelivery() {
	h.t.Helper()

	if len(h.waker.bodies) != 0 {
		h.t.Fatalf("engine delivered %q, want nothing", h.waker.bodies)
	}
}

// loggedLine returns the first decision-log line containing every fragment.
func (h *harness) loggedLine(fragments ...string) string {
	h.t.Helper()

	for _, line := range h.logs {
		matched := true
		for _, fragment := range fragments {
			if !strings.Contains(line, fragment) {
				matched = false
				break
			}
		}
		if matched {
			return line
		}
	}
	return ""
}

func worker(name, paneID, status string) herdr.Agent {
	return herdr.Agent{Name: &name, PaneID: paneID, AgentStatus: status}
}

// --- fakes --------------------------------------------------------------

type stepClock struct {
	now     time.Time
	slept   []time.Duration
	onSleep func()
}

func (c *stepClock) Now() time.Time { return c.now }

func (c *stepClock) SleepFor(ctx context.Context, d time.Duration) {
	c.slept = append(c.slept, d)
	c.now = c.now.Add(d)
	if c.onSleep != nil {
		c.onSleep()
	}
}

func (c *stepClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func (c *stepClock) lastSleep() time.Duration {
	if len(c.slept) == 0 {
		return 0
	}
	return c.slept[len(c.slept)-1]
}

type fakeHerdrAPI struct {
	sessions       []herdr.Session
	snapshot       herdr.Snapshot
	snapshots      int
	listBudget     time.Duration
	snapshotBudget time.Duration
	listErr        error
	snapshotErr    error
}

func (f *fakeHerdrAPI) List(ctx context.Context) ([]herdr.Session, error) {
	f.listBudget = budgetOf(ctx)
	return f.sessions, f.listErr
}

func (f *fakeHerdrAPI) Snapshot(ctx context.Context, _ string) (herdr.Snapshot, error) {
	f.snapshots++
	f.snapshotBudget = budgetOf(ctx)
	return f.snapshot, f.snapshotErr
}

// budgetOf reports how long a call has left to run, which is what shows the
// engine bounded it. It reads the real clock because context deadlines do.
func budgetOf(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0
	}
	return time.Until(deadline)
}

type fakeLedger struct {
	records        []WakeRecord
	markers        Markers
	deliveredIDs   []string
	messageIDs     []string
	compacts       int
	recordsAtSave  []int
	nextID         int
	appendErr      error
	appendCalls    int
	failAppendCall int
	saveErr        error
}

func newFakeLedger() *fakeLedger {
	return &fakeLedger{markers: Markers{}}
}

func (l *fakeLedger) Append(kind WakeKind, key, reason string) (WakeRecord, error) {
	l.appendCalls++
	if l.failAppendCall > 0 && l.appendCalls == l.failAppendCall {
		return WakeRecord{}, errors.New("injected append failure")
	}
	if l.appendErr != nil {
		return WakeRecord{}, l.appendErr
	}

	l.nextID++
	id := fmt.Sprintf("w-%d", l.nextID)
	for i, record := range l.records {
		if record.WakeKind == kind && record.Key == key {
			record.ID = id
			record.IDs = append(record.IDs, id)
			record.Reason = reason
			l.records[i] = record
			return record, nil
		}
	}

	record := WakeRecord{ID: id, IDs: []string{id}, WakeKind: kind, Key: key, Reason: reason}
	l.records = append(l.records, record)
	return record, nil
}

func (l *fakeLedger) Pending() ([]WakeRecord, error) { return l.records, nil }

func (l *fakeLedger) MarkDelivered(ids []string, messageID string) error {
	l.deliveredIDs = append(l.deliveredIDs, ids...)
	l.messageIDs = append(l.messageIDs, messageID)

	retired := make(map[string]bool, len(ids))
	for _, id := range ids {
		retired[id] = true
	}

	var kept []WakeRecord
	for _, record := range l.records {
		var remaining []string
		for _, id := range record.IDs {
			if !retired[id] {
				remaining = append(remaining, id)
			}
		}
		if len(remaining) == 0 {
			continue
		}
		record.IDs = remaining
		record.ID = remaining[len(remaining)-1]
		kept = append(kept, record)
	}
	l.records = kept

	return nil
}

func (l *fakeLedger) Compact() error {
	l.compacts++
	return nil
}

func (l *fakeLedger) LoadMarkers() (Markers, error) { return l.markers, nil }

func (l *fakeLedger) SaveMarkers(markers Markers) error {
	l.recordsAtSave = append(l.recordsAtSave, len(l.records))
	if l.saveErr != nil {
		return l.saveErr
	}
	l.markers = markers
	return nil
}

type fakeWaker struct {
	bodies        []string
	messageID     string
	deliverBudget time.Duration
	err           error
}

func (w *fakeWaker) Deliver(ctx context.Context, body string) (string, error) {
	w.deliverBudget = budgetOf(ctx)
	if w.err != nil {
		return "", w.err
	}
	w.bodies = append(w.bodies, body)
	return w.messageID, nil
}

type completionCall struct {
	worker string
	since  time.Time
}

// fakeCompletions answers from at when a worker's completion message has a
// timestamp, which is what makes the since boundary observable, and from
// completed when the test only cares that one exists.
type fakeCompletions struct {
	completed map[string]bool
	at        map[string]time.Time
	calls     []completionCall
	err       error
}

func (c *fakeCompletions) CompletionSince(worker string, since time.Time) (bool, error) {
	c.calls = append(c.calls, completionCall{worker: worker, since: since})
	if sent, ok := c.at[worker]; ok {
		return !sent.Before(since), c.err
	}
	return c.completed[worker], c.err
}

type fakeSubscriber struct {
	t                    *testing.T
	silent               bool
	liveFor              time.Duration
	pollInterval         time.Duration
	now                  func() time.Time
	advance              func(time.Duration)
	calls                int
	paneIDs              [][]string
	batches              [][]Event
	err                  error
	reconciledAfterReady bool
	herdr                *fakeHerdrAPI
}

// stream queues one batch of events per subscribe call.
func (s *fakeSubscriber) stream(batches ...[]Event) { s.batches = batches }

func (s *fakeSubscriber) subscribe(ctx context.Context, paneIDs []string, onReady func(), onEvent func(Event)) error {
	s.calls++
	s.paneIDs = append(s.paneIDs, paneIDs)

	// The engine must bound every subscription by the poll budget; an
	// unbounded ctx would hang the cycle on a wedged Herdr forever.
	deadline, ok := ctx.Deadline()
	if !ok {
		s.t.Error("subscriber received an unbounded context")
	} else if got := deadline.Sub(s.now()); got != s.pollInterval {
		s.t.Errorf("subscription budget = %s, want the poll interval %s", got, s.pollInterval)
	}

	// A stream that acknowledges, carries its subscription for a while and then
	// ends: the ordinary early EOF, not a stream that never worked.
	if s.liveFor > 0 {
		onReady()
		s.advance(s.liveFor)
	}
	if s.err != nil {
		return s.err
	}
	if s.silent {
		// Acknowledged never arrives: the stream ctx expires first, which
		// surfaces as DeadlineExceeded without onReady ever firing.
		<-ctx.Done()
		return ctx.Err()
	}

	before := s.herdr.snapshots
	onReady()
	s.reconciledAfterReady = s.herdr.snapshots > before

	if len(s.batches) > 0 {
		batch := s.batches[0]
		s.batches = s.batches[1:]
		for _, event := range batch {
			onEvent(event)
		}
	}

	<-ctx.Done()
	return ctx.Err()
}

type immediateDeadlineContext struct {
	context.Context
	deadline time.Time
	done     <-chan struct{}
}

func (c immediateDeadlineContext) Deadline() (time.Time, bool) { return c.deadline, true }
func (c immediateDeadlineContext) Done() <-chan struct{}       { return c.done }
func (c immediateDeadlineContext) Err() error                  { return context.DeadlineExceeded }

func immediateTimeout(now func() time.Time) func(context.Context, time.Duration) (context.Context, context.CancelFunc) {
	return func(parent context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
		done := make(chan struct{})
		close(done)
		return immediateDeadlineContext{Context: parent, deadline: now().Add(budget), done: done}, func() {}
	}
}
