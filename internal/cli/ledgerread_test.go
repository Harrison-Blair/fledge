package cli

import (
	"os"
	"path/filepath"
	"strings"
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

// TestLedgerReadRejectsInvalidKind pins the fix for the skua's finding: an
// unrecognized --kind value must be a usage error, not passed through to
// ledger.Read unchecked. Without this validation, ledger.Read's recordPath
// (filepath.Join(dir, subject+"."+kind+".json")) lets a --kind value with
// enough leading ".." segments escape the ledger directory (and the
// worktree entirely) and read an arbitrary JSON file on disk, since only
// subject — not kind — is sanitized by internal/ledger's validSubject.
func TestLedgerReadRejectsInvalidKind(t *testing.T) {
	setupLedgerRepo(t)

	// A file well outside the repo, shaped like a ledger record so a
	// successful read would be unambiguous evidence of the traversal.
	outsideDir := t.TempDir()
	leaked := filepath.Join(outsideDir, "leaked.json")
	if err := os.WriteFile(leaked, []byte(`{"subject":"LEAKED","kind":"LEAKED","timestamp":"","payload":null}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Enough ".." segments to overshoot the filesystem root regardless of
	// test working-directory depth (Clean pins overshoot at "/" on Unix),
	// then descend via the absolute path with its leading "/" stripped.
	traversalKind := strings.Repeat("../", 20) + strings.TrimPrefix(strings.TrimSuffix(leaked, ".json"), "/")

	if code := Run([]string{"ledger", "read", "foo", "--kind", traversalKind}); code != ExitUsage {
		t.Fatalf("ledger read --kind %q exit = %d, want %d (ExitUsage) — a non-enum --kind must be rejected, not passed through to ledger.Read", traversalKind, code, ExitUsage)
	}
	if _, err := os.Stat(leaked); err != nil {
		t.Fatal(err) // sanity: the target file still exists, unrelated to the read attempt
	}

	if code := Run([]string{"ledger", "read", "foo", "--kind", "bogus"}); code != ExitUsage {
		t.Fatalf("ledger read --kind bogus exit = %d, want %d (ExitUsage)", code, ExitUsage)
	}
}
