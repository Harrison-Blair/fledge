package cli

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/ledger"
)

// TestClassifyPulseNotStalled pins AC-3/AC-4: a fresh lease reports not
// stalled, mirroring ledger.ClassifyLiveness's reason exactly, alongside the
// declared expect and computed elapsed.
func TestClassifyPulseNotStalled(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 5, 0, 0, time.UTC)
	updatedAt := now.Add(-1 * time.Minute)
	rec := statusRecordFixture(t, "watcher", updatedAt, "5m")

	report, err := classifyPulse("watcher", rec, now)
	if err != nil {
		t.Fatal(err)
	}
	wantStalled, wantReason := ledger.ClassifyLiveness(updatedAt, 5*time.Minute, now)
	if report.Stalled != wantStalled {
		t.Errorf("Stalled = %v, want %v", report.Stalled, wantStalled)
	}
	if report.Reason != wantReason {
		t.Errorf("Reason = %q, want %q (must mirror ClassifyLiveness exactly)", report.Reason, wantReason)
	}
	if report.Expect != "5m" {
		t.Errorf("Expect = %q, want %q", report.Expect, "5m")
	}
	if report.Elapsed != (1 * time.Minute).String() {
		t.Errorf("Elapsed = %q, want %q", report.Elapsed, (1 * time.Minute).String())
	}
}

// TestClassifyPulseStalled pins AC-6: a lease past its declared period
// reports stalled, mirroring ClassifyLiveness's reason.
func TestClassifyPulseStalled(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 30, 0, 0, time.UTC)
	updatedAt := now.Add(-10 * time.Minute)
	rec := statusRecordFixture(t, "gone-quiet", updatedAt, "5m")

	report, err := classifyPulse("gone-quiet", rec, now)
	if err != nil {
		t.Fatal(err)
	}
	wantStalled, wantReason := ledger.ClassifyLiveness(updatedAt, 5*time.Minute, now)
	if !report.Stalled || !wantStalled {
		t.Fatalf("Stalled = %v, want true", report.Stalled)
	}
	if report.Reason != wantReason {
		t.Errorf("Reason = %q, want %q", report.Reason, wantReason)
	}
}

// TestClassifyPulseNotStalledPastOldThreshold pins AC-7: a lease declaring
// 30m, aged 10m — past the old hardcoded 5-minute threshold but within its
// own declared period — reports not stalled. This is the behavior PLM-035
// makes possible. now is passed explicitly (the awaitClock convention) so
// this is exact and doesn't touch the wall clock or sleep.
func TestClassifyPulseNotStalledPastOldThreshold(t *testing.T) {
	now := time.Date(2026, 7, 17, 13, 0, 0, 0, time.UTC)
	updatedAt := now.Add(-10 * time.Minute)
	rec := statusRecordFixture(t, "long-runner", updatedAt, "30m")

	report, err := classifyPulse("long-runner", rec, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Stalled {
		t.Errorf("Stalled = true, want false (10m aged, 30m declared, past the old 5m threshold)")
	}
}

// TestClassifyPulseNoRecord pins AC-5: no status record is a distinct third
// state — not stalled, with a reason naming the absence — never
// stalled:true. This path has no ClassifyLiveness coverage by construction,
// since it never reaches it.
func TestClassifyPulseNoRecord(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	report, err := classifyPulse("nobody-yet", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Stalled {
		t.Error("Stalled = true, want false for a worker with no status record")
	}
	if report.Reason == "" {
		t.Error("Reason is empty, want a reason naming the absence of a status record")
	}
	if report.Expect != "" || report.Elapsed != "" {
		t.Errorf("Expect/Elapsed = %q/%q, want both empty when there is no record", report.Expect, report.Elapsed)
	}
}

// TestRunPulseNoRecordExitsOK pins AC-5/AC-9 end to end through Run: a
// worker with no status record exits ExitOK, never encoding "stalled" via
// exit code, and --json includes the distinguishing reason.
func TestRunPulseNoRecordExitsOK(t *testing.T) {
	setupLedgerRepo(t)
	if code := Run([]string{"pulse", "nobody-yet", "--json"}); code != ExitOK {
		t.Fatalf("pulse exit = %d, want %d (ExitOK)", code, ExitOK)
	}
}

// TestRunPulseStalledExitsOK pins AC-6 end to end: a stalled worker still
// exits ExitOK — the classification lives in the output, not the exit code.
func TestRunPulseStalledExitsOK(t *testing.T) {
	root := setupLedgerRepo(t)
	dir := filepath.Join(root, ".fledge", "ledger")
	writeStatusFixture(t, dir, "gone-quiet", "2024-01-01T00:00:00Z", "5m")

	if code := Run([]string{"pulse", "gone-quiet", "--json"}); code != ExitOK {
		t.Fatalf("pulse exit = %d, want %d (ExitOK) even when stalled", code, ExitOK)
	}
}

// TestRunPulseMissingNameIsUsageError pins AC-10.
func TestRunPulseMissingNameIsUsageError(t *testing.T) {
	setupLedgerRepo(t)
	if code := Run([]string{"pulse"}); code != ExitUsage {
		t.Fatalf("pulse exit = %d, want %d (ExitUsage)", code, ExitUsage)
	}
}

// TestRunPulseRejectsEscapingSubject pins AC-10: a subject that would
// escape the ledger directory is rejected, not sanitized.
func TestRunPulseRejectsEscapingSubject(t *testing.T) {
	setupLedgerRepo(t)
	for _, subject := range []string{"../escape", "a/b"} {
		if code := Run([]string{"pulse", subject}); code != ExitUsage {
			t.Errorf("pulse %q exit = %d, want %d (ExitUsage)", subject, code, ExitUsage)
		}
	}
}

// statusRecordFixture builds a *ledger.Record wrapping a StatusRecord
// payload, without going through ledger.Write, for classifyPulse's unit
// tests.
func statusRecordFixture(t *testing.T, subject string, updatedAt time.Time, expect string) *ledger.Record {
	t.Helper()
	rec, err := ledger.Write(t.TempDir(), subject, ledger.KindStatus, ledger.StatusRecord{
		Note:      "test",
		Expect:    expect,
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

// writeStatusFixture writes a status record directly to dir with an
// explicit updated_at, so a lease can be aged deterministically (back-dated)
// rather than by sleeping.
func writeStatusFixture(t *testing.T, dir, subject, updatedAt, expect string) {
	t.Helper()
	if _, err := ledger.Write(dir, subject, ledger.KindStatus, ledger.StatusRecord{
		Note:      "test",
		Expect:    expect,
		UpdatedAt: updatedAt,
	}); err != nil {
		t.Fatal(err)
	}
}
