package cli

import (
	"path/filepath"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/ledger"
)

// TestEscalateWritesRecord pins AC-3's happy path: an escalation message is
// written and readable back via internal/ledger.
func TestEscalateWritesRecord(t *testing.T) {
	root := setupLedgerRepo(t)
	if code := Run([]string{"escalate", "some-subject", "--message", "blocked on X"}); code != ExitOK {
		t.Fatalf("escalate exit = %d, want %d (ExitOK)", code, ExitOK)
	}
	rec, err := ledger.Read(filepath.Join(root, ".fledge", "ledger"), "some-subject", ledger.KindEscalation)
	if err != nil {
		t.Fatalf("ledger.Read: %v", err)
	}
	var payload ledger.EscalationRecord
	if err := rec.Decode(&payload); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if payload.Message != "blocked on X" {
		t.Errorf("payload.Message = %q, want %q", payload.Message, "blocked on X")
	}
}
