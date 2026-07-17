package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/ledger"
)

// setupLedgerRepo creates a minimal git+.fledge repo in a temp dir and
// chdirs the test into it, mirroring TestLockRollsBackOnStatusWriteFailure's
// setup pattern. Returns the repo root.
func setupLedgerRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(root, ".fledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	return root
}

// TestVerdictRejectsInvalidResult pins AC-2: an invalid --result value is a
// usage error and writes no record.
func TestVerdictRejectsInvalidResult(t *testing.T) {
	root := setupLedgerRepo(t)
	if code := Run([]string{"verdict", "some-subject", "--result", "maybe"}); code != ExitUsage {
		t.Fatalf("verdict exit = %d, want %d (ExitUsage)", code, ExitUsage)
	}
	if _, err := os.Stat(filepath.Join(root, ".fledge", "ledger", "some-subject.verdict.json")); !os.IsNotExist(err) {
		t.Error("verdict record was written despite invalid --result")
	}
}

// TestVerdictWritesRecord pins AC-2's happy path: a valid pass/fail verdict
// is written and readable back via internal/ledger.
func TestVerdictWritesRecord(t *testing.T) {
	root := setupLedgerRepo(t)
	if code := Run([]string{"verdict", "some-subject", "--result", "pass", "--note", "looks good"}); code != ExitOK {
		t.Fatalf("verdict exit = %d, want %d (ExitOK)", code, ExitOK)
	}
	rec, err := ledger.Read(filepath.Join(root, ".fledge", "ledger"), "some-subject", ledger.KindVerdict)
	if err != nil {
		t.Fatalf("ledger.Read: %v", err)
	}
	var payload ledger.VerdictRecord
	if err := rec.Decode(&payload); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if payload.Result != "pass" || payload.Note != "looks good" {
		t.Errorf("payload = %+v, want Result=pass Note=%q", payload, "looks good")
	}
}
