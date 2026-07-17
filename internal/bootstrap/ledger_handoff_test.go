package bootstrap

import (
	"fmt"
	"strings"
	"testing"
)

// ledgerHandoffFiles is the seven agent-neutral orchestration files FTHR-075
// rewrites so every state-bearing handoff goes through the PLM-030 ledger
// (fledge heartbeat/await/verdict/escalate/pulse, fledge ledger read) instead
// of message-peer content.
var ledgerHandoffFiles = []string{
	"worker-protocols.md",
	"incubator.md",
	"brooder.md",
	"skua.md",
	"foraging.md",
	"implementation.md",
	"planning.md",
}

// readOrchestrateDoc reads one of the seven files from the embedded core
// skills tree.
func readOrchestrateDoc(t *testing.T, name string) string {
	t.Helper()
	data, err := FS.ReadFile("core/skills/fledge-orchestrate/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// findAwaitExistsViolations scans doc line by line for a `fledge await`
// invocation naming the write-once `--kind verdict` or `--kind escalation`
// without `--exists` on the same line. Per PLM-034 FC-1/FC-2, verdict and
// escalation records are written once: a change-wait (no --exists) on them
// deadlocks whenever the record lands before the waiter asks.
func findAwaitExistsViolations(doc string) []string {
	var violations []string
	for i, line := range strings.Split(doc, "\n") {
		if !strings.Contains(line, "fledge await") {
			continue
		}
		for _, kind := range []string{"--kind verdict", "--kind escalation"} {
			if strings.Contains(line, kind) && !strings.Contains(line, "--exists") {
				violations = append(violations, fmt.Sprintf("line %d: %s (%q without --exists)", i+1, strings.TrimSpace(line), kind))
			}
		}
	}
	return violations
}

// findAwaitStatusExistsViolations scans doc for a `fledge await ... --kind
// status` invocation that incorrectly carries `--exists`. status is
// repeatedly written (FC-1), so an existence-wait on it returns on the
// subject's very first heartbeat, long before the terminal value the waiter
// actually wants.
func findAwaitStatusExistsViolations(doc string) []string {
	var violations []string
	for i, line := range strings.Split(doc, "\n") {
		if strings.Contains(line, "fledge await") && strings.Contains(line, "--kind status") && strings.Contains(line, "--exists") {
			violations = append(violations, fmt.Sprintf("line %d: %s (--kind status must never carry --exists)", i+1, strings.TrimSpace(line)))
		}
	}
	return violations
}

// findAwaitMissingTimeout scans doc for a `fledge await` invocation with no
// `--timeout` on the same line. --timeout is mandatory in the shipped binary
// (PLM-034 FC-3): a prose example omitting it is a usage error against the
// real CLI, not merely a style gap.
func findAwaitMissingTimeout(doc string) []string {
	var violations []string
	for i, line := range strings.Split(doc, "\n") {
		if strings.Contains(line, "fledge await") && !strings.Contains(line, "--timeout") {
			violations = append(violations, fmt.Sprintf("line %d: %s (no --timeout)", i+1, strings.TrimSpace(line)))
		}
	}
	return violations
}

// TestAwaitGuardHelpersDetectDeadlockingForm is a self-test of the three
// checker functions above, proving each flags the deadlocking/deadlock-
// adjacent form it exists to catch, and does not flag the corrected form.
// Written and run before any prose rewrite so the checkers themselves are
// test-first, independent of whether the current prose happens to already
// contain the bad shape.
func TestAwaitGuardHelpersDetectDeadlockingForm(t *testing.T) {
	if v := findAwaitExistsViolations("run `fledge await FTHR-1 --kind verdict --timeout 5m`"); len(v) == 0 {
		t.Error("findAwaitExistsViolations: must flag a bare --kind verdict await without --exists")
	}
	if v := findAwaitExistsViolations("run `fledge await FTHR-1 --kind verdict --exists --timeout 5m`"); len(v) != 0 {
		t.Errorf("findAwaitExistsViolations: false positive on correct --kind verdict --exists form: %v", v)
	}
	if v := findAwaitExistsViolations("run `fledge await peer --kind escalation --timeout 5m`"); len(v) == 0 {
		t.Error("findAwaitExistsViolations: must flag a bare --kind escalation await without --exists")
	}

	if v := findAwaitStatusExistsViolations("run `fledge await peer --kind status --exists --timeout 5m`"); len(v) == 0 {
		t.Error("findAwaitStatusExistsViolations: must flag --kind status carrying --exists")
	}
	if v := findAwaitStatusExistsViolations("run `fledge await peer --kind status --timeout 5m`"); len(v) != 0 {
		t.Errorf("findAwaitStatusExistsViolations: false positive on correct --kind status form: %v", v)
	}

	if v := findAwaitMissingTimeout("run `fledge await peer --kind status`"); len(v) == 0 {
		t.Error("findAwaitMissingTimeout: must flag a fledge await with no --timeout")
	}
	if v := findAwaitMissingTimeout("run `fledge await peer --kind status --timeout 5m`"); len(v) != 0 {
		t.Errorf("findAwaitMissingTimeout: false positive on correct form: %v", v)
	}
}

// TestNoAwaitKindVerdictOrEscalationWithoutExists is the AC-2 negative guard:
// no file under core/skills/fledge-orchestrate describes a bare
// `fledge await X --kind verdict` or `--kind escalation` — the write-once
// kinds must always be existence-waits.
func TestNoAwaitKindVerdictOrEscalationWithoutExists(t *testing.T) {
	for _, name := range ledgerHandoffFiles {
		doc := readOrchestrateDoc(t, name)
		for _, v := range findAwaitExistsViolations(doc) {
			t.Errorf("%s: %s", name, v)
		}
	}
}

// TestNoAwaitKindStatusWithExists is the other AC-2 direction: --kind status
// must never carry --exists (it would return on the first heartbeat).
func TestNoAwaitKindStatusWithExists(t *testing.T) {
	for _, name := range ledgerHandoffFiles {
		doc := readOrchestrateDoc(t, name)
		for _, v := range findAwaitStatusExistsViolations(doc) {
			t.Errorf("%s: %s", name, v)
		}
	}
}

// TestNoAwaitWithoutTimeout is the AC-3 negative guard: every fledge await
// invocation in the prose carries a concrete --timeout.
func TestNoAwaitWithoutTimeout(t *testing.T) {
	for _, name := range ledgerHandoffFiles {
		doc := readOrchestrateDoc(t, name)
		for _, v := range findAwaitMissingTimeout(doc) {
			t.Errorf("%s: %s", name, v)
		}
	}
}

// TestNoIndefiniteWaitLanguage is the other half of AC-3: the string "block
// indefinitely" and equivalent unbounded-wait phrasing appear nowhere in the
// seven files — PLM-034 made --timeout mandatory, so no prose may describe
// an unbounded wait.
func TestNoIndefiniteWaitLanguage(t *testing.T) {
	phrases := []string{"block indefinitely", "wait indefinitely", "indefinitely", "no timeout"}
	for _, name := range ledgerHandoffFiles {
		doc := strings.ToLower(readOrchestrateDoc(t, name))
		for _, phrase := range phrases {
			if strings.Contains(doc, phrase) {
				t.Errorf("%s: contains unbounded-wait phrasing %q", name, phrase)
			}
		}
	}
}

// TestWorkerProtocolsDescribesHeartbeatDiscipline is the AC-5 positive guard:
// worker-protocols.md instructs heartbeat before AND periodically during long
// operations (never before-only), and declares --expect for a single
// blocking call with no seam to heartbeat at.
func TestWorkerProtocolsDescribesHeartbeatDiscipline(t *testing.T) {
	doc := readOrchestrateDoc(t, "worker-protocols.md")

	if !strings.Contains(doc, "fledge heartbeat") {
		t.Error("worker-protocols.md must name fledge heartbeat")
	}
	if !strings.Contains(doc, "periodically during") {
		t.Error("worker-protocols.md must instruct heartbeat periodically during long operations, not just before")
	}
	beforeIdx := strings.Index(doc, "before")
	duringIdx := strings.Index(doc, "during")
	if beforeIdx == -1 || duringIdx == -1 || !(beforeIdx < duringIdx) {
		t.Error("worker-protocols.md must state the before-and-during heartbeat instruction in that order")
	}
	if !strings.Contains(doc, "--expect") {
		t.Error("worker-protocols.md must declare --expect for a single blocking call with no seam")
	}
}

// TestWorkerProtocolsDescribesPulseRecovery is the AC-4 guard for the shared
// recovery pattern: worker-protocols.md states the exit-4 (ExitTimeout)
// recovery in terms of fledge pulse, covering all three classifications
// (not stalled / stalled / no record), and never instructs comparing
// timestamps by hand.
func TestWorkerProtocolsDescribesPulseRecovery(t *testing.T) {
	doc := readOrchestrateDoc(t, "worker-protocols.md")

	for _, want := range []string{"fledge pulse", "Not stalled", "Stalled", "No status record"} {
		if !strings.Contains(doc, want) {
			t.Errorf("worker-protocols.md must contain %q as part of the pulse recovery pattern", want)
		}
	}
	if !strings.Contains(doc, "Never compare timestamps by hand") && !strings.Contains(doc, "never compare timestamps by hand") {
		t.Error("worker-protocols.md must forbid comparing timestamps by hand")
	}
}

// TestWorkerProtocolsRescopesMessagePeer is the AC-7 guard: message-peer is
// described as a stateless wake-up nudge only, never the carrier of
// verdict/status/escalation content.
func TestWorkerProtocolsRescopesMessagePeer(t *testing.T) {
	doc := readOrchestrateDoc(t, "worker-protocols.md")

	if !strings.Contains(doc, "message-peer") {
		t.Fatal("worker-protocols.md must still mention message-peer")
	}
	if !strings.Contains(doc, "stateless") {
		t.Error("worker-protocols.md must describe message-peer as stateless")
	}
	if !strings.Contains(doc, "nudge") {
		t.Error("worker-protocols.md must describe message-peer as a nudge")
	}
}

// TestSkuaDescribesVerdictCommand is a positive guard: skua.md describes
// fledge verdict, not a bare pass/fail message, as how a verdict reaches its
// reader.
func TestSkuaDescribesVerdictCommand(t *testing.T) {
	doc := readOrchestrateDoc(t, "skua.md")
	if !strings.Contains(doc, "fledge verdict") {
		t.Error("skua.md must describe fledge verdict as the verdict mechanism")
	}
}

// TestBrooderDescribesEscalateCommand is a positive guard: brooder.md
// describes fledge escalate for blockers.
func TestBrooderDescribesEscalateCommand(t *testing.T) {
	doc := readOrchestrateDoc(t, "brooder.md")
	if !strings.Contains(doc, "fledge escalate") {
		t.Error("brooder.md must describe fledge escalate for blockers")
	}
}

// TestForagingDescribesAwaitStatusDoneSignal is a positive guard: foraging.md's
// Commissioner section describes fledge await ... --kind status --timeout
// and a status terminal value as the done-signal.
func TestForagingDescribesAwaitStatusDoneSignal(t *testing.T) {
	doc := readOrchestrateDoc(t, "foraging.md")

	commissionerIdx := strings.Index(doc, "## Commissioner")
	if commissionerIdx == -1 {
		t.Fatal("foraging.md must contain a \"## Commissioner\" section")
	}
	forageIdx := strings.Index(doc, "## Forager")
	section := doc[commissionerIdx:]
	if forageIdx != -1 {
		section = doc[commissionerIdx:forageIdx]
	}

	for _, want := range []string{"fledge await", "--kind status", "--timeout"} {
		if !strings.Contains(section, want) {
			t.Errorf("foraging.md Commissioner section must contain %q", want)
		}
	}
	if !strings.Contains(section, "done") {
		t.Error("foraging.md Commissioner section must describe a done terminal value")
	}
}

// TestForagingStatesAwaitIsNotPolling is the AC-8 guard: foraging.md states
// explicitly that fledge await is a deterministic replacement for event
// re-invocation, not a return to hand-rolled polling.
func TestForagingStatesAwaitIsNotPolling(t *testing.T) {
	doc := readOrchestrateDoc(t, "foraging.md")

	if !strings.Contains(doc, "never poll") && !strings.Contains(doc, "never `sleep`-poll") {
		t.Error("foraging.md must keep the never-poll prohibition")
	}
	if !strings.Contains(doc, "deterministic replacement") {
		t.Error("foraging.md must state fledge await is a deterministic replacement for event re-invocation, not hand-rolled polling")
	}
}

// TestSevenFilesNamePulseAsRecovery is the AC-4 guard applied to every file:
// each of the seven files names fledge pulse as the exit-4 recovery for any
// wait site it describes.
func TestSevenFilesNamePulseAsRecovery(t *testing.T) {
	for _, name := range ledgerHandoffFiles {
		doc := readOrchestrateDoc(t, name)
		if !strings.Contains(doc, "fledge pulse") {
			t.Errorf("%s: must name fledge pulse as the exit-4 recovery for its wait site(s)", name)
		}
	}
}

// TestSevenFilesUseLedgerVocabulary is the AC-6 guard: all seven files
// describe their state-bearing handoffs in terms of ledger reads/writes.
func TestSevenFilesUseLedgerVocabulary(t *testing.T) {
	verbs := []string{"fledge heartbeat", "fledge await", "fledge verdict", "fledge escalate", "fledge pulse", "ledger read"}
	for _, name := range ledgerHandoffFiles {
		doc := readOrchestrateDoc(t, name)
		found := false
		for _, v := range verbs {
			if strings.Contains(doc, v) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: describes no ledger command (heartbeat/await/verdict/escalate/pulse/ledger read)", name)
		}
	}
}

// TestImplementationAndPlanningReferenceRewrittenSections guards against
// duplicating stale message-based wait/verdict wording in implementation.md
// and planning.md: they must reference the rewritten foraging.md/skua.md
// sections (and the ledger commands directly for their own dispatch/approval
// steps) rather than repeating a bare "its skua messages you a pass" or
// "wait for its final message" description with no ledger command in sight.
func TestImplementationAndPlanningReferenceRewrittenSections(t *testing.T) {
	impl := readOrchestrateDoc(t, "implementation.md")
	if strings.Contains(impl, "its skua messages you a pass") {
		t.Error("implementation.md must not describe merge clearance as a bare skua pass message")
	}
	if !strings.Contains(impl, "fledge verdict") {
		t.Error("implementation.md must describe fledge verdict as how merge clearance is learned")
	}
	if !strings.Contains(impl, "fledge escalate") {
		t.Error("implementation.md must describe fledge escalate for worker escalations")
	}

	planning := readOrchestrateDoc(t, "planning.md")
	if strings.Contains(planning, "on-disk `.fledge/nest/` state is never an input to that decision") {
		t.Error("planning.md must not duplicate foraging.md's full commissioner-wait paragraph verbatim; it should reference foraging.md's Commissioner section instead")
	}
	if !strings.Contains(planning, "foraging.md") {
		t.Error("planning.md must reference foraging.md's Commissioner section for the forager wait")
	}
}
