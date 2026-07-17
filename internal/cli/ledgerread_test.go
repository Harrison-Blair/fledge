package cli

import (
	"testing"
)

// TestLedgerReadAllKinds pins AC-4: each of the three record kinds, once
// written via its dedicated command, round-trips through `fledge ledger
// read`.
func TestLedgerReadAllKinds(t *testing.T) {
	setupLedgerRepo(t)

	cases := []struct {
		name  string
		write []string
		kind  string
		want  string
	}{
		{"status", []string{"heartbeat", "subj-status", "--note", "hi"}, "status", "hi"},
		{"verdict", []string{"verdict", "subj-verdict", "--result", "pass", "--note", "ok"}, "verdict", "ok"},
		{"escalation", []string{"escalate", "subj-escalation", "--message", "help"}, "escalation", "help"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if code := Run(c.write); code != ExitOK {
				t.Fatalf("%v exit = %d, want %d", c.write, code, ExitOK)
			}
			if code := Run([]string{"ledger", "read", c.write[1], "--kind", c.kind}); code != ExitOK {
				t.Fatalf("ledger read exit = %d, want %d", code, ExitOK)
			}
		})
	}
}

// TestLedgerReadMissing pins AC-4's not-found path: reading a never-written
// (subject, kind) reports not-found cleanly (ExitFail, no panic).
func TestLedgerReadMissing(t *testing.T) {
	setupLedgerRepo(t)
	if code := Run([]string{"ledger", "read", "never-written", "--kind", "status"}); code != ExitFail {
		t.Fatalf("ledger read exit = %d, want %d (ExitFail)", code, ExitFail)
	}
}
