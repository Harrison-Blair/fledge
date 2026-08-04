package wake

import (
	"errors"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// drain delivers every wake Pending currently reports, the way the watcher
// engine does after it sends one batched orchestrator message.
func drain(t *testing.T, ledger *Ledger, messageID string) {
	t.Helper()
	var ids []string
	for _, record := range mustPending(t, ledger) {
		ids = append(ids, record.IDs...)
	}
	if len(ids) == 0 {
		return
	}
	if err := ledger.MarkDelivered(ids, messageID); err != nil {
		t.Fatal(err)
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) == 0 {
		return nil
	}
	return strings.SplitAfter(strings.TrimSuffix(string(contents), "\n"), "\n")
}

func TestCompactDropsRetiredWakes(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	mustAppend(t, ledger, KindStatus, "alice", "working")
	mustAppend(t, ledger, KindHeartbeat, "600", "quiet")
	mustAppend(t, ledger, KindStatus, "alice", "blocked")
	drain(t, ledger, "m-1")

	// Three queued wakes, and three delivery markers: the collapsed status group
	// names both of its entries.
	before := readLines(t, ledger.logPath())
	if len(before) != 6 {
		t.Fatalf("ledger has %d lines before compaction, want 6", len(before))
	}
	if err := ledger.Compact(); err != nil {
		t.Fatal(err)
	}
	if after := readLines(t, ledger.logPath()); len(after) != 0 {
		t.Fatalf("compacted ledger = %q, want empty", after)
	}
	if got := mustPending(t, New(root, testSession)); len(got) != 0 {
		t.Fatalf("Pending() after compaction = %+v, want none", summarize(got))
	}

	// A compacted ledger is still a working ledger.
	next := mustAppend(t, ledger, KindDead, "bob", "vanished")
	got := mustPending(t, New(root, testSession))
	if len(got) != 1 || got[0].ID != next.ID {
		t.Fatalf("Pending() after appending to a compacted ledger = %+v", summarize(got))
	}
}

func TestCompactPreservesPendingWakesByteForByte(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	mustAppend(t, ledger, KindStatus, "alice", "working")
	mustAppend(t, ledger, KindStatus, "alice", "blocked: needs permission")
	drain(t, ledger, "m-1")
	// Queued after the drain, so these must survive compaction untouched.
	survivor := mustAppend(t, ledger, KindDead, "bob", "vanished from the snapshot")
	beat := mustAppend(t, ledger, KindHeartbeat, "600", "no activity for 600s")

	pendingBefore := mustPending(t, ledger)
	wantLines := make([]string, 0, 2)
	for _, line := range readLines(t, ledger.logPath()) {
		if strings.Contains(line, survivor.ID) || strings.Contains(line, beat.ID) {
			wantLines = append(wantLines, line)
		}
	}
	if len(wantLines) != 2 {
		t.Fatalf("expected 2 pending lines in the ledger, found %d", len(wantLines))
	}

	if err := ledger.Compact(); err != nil {
		t.Fatal(err)
	}

	if got := readLines(t, ledger.logPath()); !reflect.DeepEqual(got, wantLines) {
		t.Fatalf("compacted ledger lines =\n%q\nwant\n%q", got, wantLines)
	}
	if got := mustPending(t, New(root, testSession)); !reflect.DeepEqual(got, pendingBefore) {
		t.Fatalf("Pending() after compaction = %+v, want %+v", summarize(got), summarize(pendingBefore))
	}
}

func TestCompactIsIdempotentAndSkipsAnUnretiredLedger(t *testing.T) {
	t.Run("already compact", func(t *testing.T) {
		root := t.TempDir()
		ledger := testLedger(t, root)
		mustAppend(t, ledger, KindStatus, "alice", "blocked")
		before, err := os.Stat(ledger.logPath())
		if err != nil {
			t.Fatal(err)
		}
		wantLines := readLines(t, ledger.logPath())

		if err := ledger.Compact(); err != nil {
			t.Fatal(err)
		}
		after, err := os.Stat(ledger.logPath())
		if err != nil {
			t.Fatal(err)
		}
		// Nothing was retired, so the ledger is not rewritten at all.
		if !os.SameFile(before, after) {
			t.Fatal("Compact rewrote a ledger that had nothing to drop")
		}
		if got := readLines(t, ledger.logPath()); !reflect.DeepEqual(got, wantLines) {
			t.Fatalf("ledger lines = %q, want %q", got, wantLines)
		}
	})

	t.Run("repeated compaction", func(t *testing.T) {
		root := t.TempDir()
		ledger := testLedger(t, root)
		mustAppend(t, ledger, KindStatus, "alice", "working")
		drain(t, ledger, "m-1")
		mustAppend(t, ledger, KindDead, "bob", "vanished")

		if err := ledger.Compact(); err != nil {
			t.Fatal(err)
		}
		first := readLines(t, ledger.logPath())
		if err := ledger.Compact(); err != nil {
			t.Fatal(err)
		}
		if second := readLines(t, ledger.logPath()); !reflect.DeepEqual(second, first) {
			t.Fatalf("second compaction changed the ledger: %q, want %q", second, first)
		}
	})

	t.Run("ledger that was never written", func(t *testing.T) {
		root := t.TempDir()
		ledger := testLedger(t, root)
		if err := ledger.Compact(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(ledger.logPath()); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Compact created a ledger file: %v", err)
		}
	})
}

func TestCompactReplacesTheLedgerAtomically(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("an open handle blocks rename-over-existing on Windows")
	}
	root := t.TempDir()
	ledger := testLedger(t, root)
	mustAppend(t, ledger, KindStatus, "alice", "working")
	drain(t, ledger, "m-1")
	mustAppend(t, ledger, KindDead, "bob", "vanished")

	before, err := os.Stat(ledger.logPath())
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(ledger.logPath())
	if err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(ledger.logPath())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	if err := ledger.Compact(); err != nil {
		t.Fatal(err)
	}

	after, err := os.Stat(ledger.logPath())
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("Compact rewrote the ledger in place")
	}
	held := make([]byte, len(original))
	if _, err := reader.Read(held); err != nil {
		t.Fatal(err)
	}
	if string(held) != string(original) {
		t.Fatalf("the previously opened ledger changed: %q, want %q", held, original)
	}
	for _, name := range mustReadDir(t, ledger.watchPath()) {
		switch name {
		case logFilename, lockFilename, markersFilename:
		default:
			t.Fatalf("Compact left %q behind", name)
		}
	}
}

func TestCompactLeavesPendingWakesUnchangedAcrossMixedGroups(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	// A fully retired group, a partially retired group, a heartbeat run, and a
	// marker naming an entry no longer in the log.
	mustAppend(t, ledger, KindStatus, "alice", "working")
	drain(t, ledger, "m-1")
	mustAppend(t, ledger, KindStatus, "bob", "working")
	mustAppend(t, ledger, KindStatus, "bob", "blocked")
	mustAppend(t, ledger, KindHeartbeat, "600", "quiet")
	mustAppend(t, ledger, KindHeartbeat, "1200", "still quiet")
	mustAppend(t, ledger, KindDead, "carol", "vanished")
	if err := ledger.MarkDelivered([]string{"w-does-not-exist"}, "m-2"); err != nil {
		t.Fatal(err)
	}

	before := mustPending(t, ledger)
	if err := ledger.Compact(); err != nil {
		t.Fatal(err)
	}
	after := mustPending(t, New(root, testSession))
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("Pending() after compaction = %+v, want %+v", summarize(after), summarize(before))
	}

	contents, err := os.ReadFile(ledger.logPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "w-does-not-exist") {
		t.Fatalf("compaction discarded a marker naming an unknown entry:\n%s", contents)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(ledger.logPath())
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("compacted ledger permission = %04o, want 0600", got)
		}
	}
}

func TestCompactRepairsATornTail(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	mustAppend(t, ledger, KindStatus, "alice", "working")
	drain(t, ledger, "m-1")
	survivor := mustAppend(t, ledger, KindDead, "bob", "vanished")
	partial := `{"kind":"queued","id":"w-00000009","wake_kind":"dead"`
	appendRaw(t, ledger.logPath(), partial)

	if err := ledger.Compact(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(ledger.logPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), partial) {
		t.Fatalf("torn tail survived compaction:\n%s", contents)
	}
	got := mustPending(t, New(root, testSession))
	if len(got) != 1 || got[0].ID != survivor.ID {
		t.Fatalf("Pending() after compacting a torn ledger = %+v", summarize(got))
	}
}

func TestCompactRefusesACorruptLedger(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	mustAppend(t, ledger, KindStatus, "alice", "blocked")
	drain(t, ledger, "m-1")
	appendRaw(t, ledger.logPath(), `{"kind":"queued","id":"w-9","wake_kind":"nap","time":"2026-08-04T12:00:00Z"}`+"\n")
	before := readLines(t, ledger.logPath())

	if err := ledger.Compact(); !errors.Is(err, ErrCorruptLog) {
		t.Fatalf("Compact() error = %v, want ErrCorruptLog", err)
	}
	if after := readLines(t, ledger.logPath()); !reflect.DeepEqual(after, before) {
		t.Fatalf("Compact modified a corrupt ledger:\n%q\nwant\n%q", after, before)
	}
}

func TestCompactRejectsALedgerSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	root := t.TempDir()
	ledger := testLedger(t, root)
	if err := ledger.Ensure(); err != nil {
		t.Fatal(err)
	}
	target := mustWriteFile(t, "unchanged")
	if err := os.Symlink(target, ledger.logPath()); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Compact(); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Compact() error = %v, want symlink rejection", err)
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "unchanged" {
		t.Fatalf("symlink target was modified: %q", contents)
	}
}

func TestRetainPending(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		entries []entry
		want    []string
	}{
		{name: "empty", entries: nil, want: []string{}},
		{
			name:    "nothing retired",
			entries: []entry{queuedEntry("w-1", KindStatus, "alice", "blocked"), queuedEntry("w-2", KindDead, "bob", "gone")},
			want:    []string{"queued/w-1", "queued/w-2"},
		},
		{
			name: "retired queued entries and their markers are dropped together",
			entries: []entry{
				queuedEntry("w-1", KindStatus, "alice", "working"),
				queuedEntry("w-2", KindStatus, "alice", "blocked"),
				deliveredEntry("w-1"),
				deliveredEntry("w-2"),
				queuedEntry("w-3", KindDead, "bob", "gone"),
			},
			want: []string{"queued/w-3"},
		},
		{
			name: "a partially delivered group keeps what is still owed",
			entries: []entry{
				queuedEntry("w-1", KindStatus, "alice", "working"),
				queuedEntry("w-2", KindStatus, "alice", "blocked"),
				deliveredEntry("w-1"),
			},
			want: []string{"queued/w-2"},
		},
		{
			// Fail-safe: a marker naming an entry the log no longer holds retires
			// nothing, and compaction keeps rather than discards it.
			name:    "markers naming unknown IDs survive",
			entries: []entry{queuedEntry("w-1", KindStatus, "alice", "blocked"), deliveredEntry("w-9")},
			want:    []string{"queued/w-1", "delivered/w-9"},
		},
		{
			name:    "a marker naming an unknown ID survives even with nothing queued",
			entries: []entry{deliveredEntry("w-9")},
			want:    []string{"delivered/w-9"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			kept := retainPending(test.entries)
			got := make([]string, 0, len(kept))
			for _, e := range kept {
				got = append(got, e.Kind+"/"+e.ID)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("retainPending() = %v, want %v", got, test.want)
			}
			// Compaction must not change what the orchestrator is owed.
			if before, after := summarize(foldPending(test.entries)), summarize(foldPending(kept)); !reflect.DeepEqual(before, after) {
				t.Fatalf("foldPending after compaction = %+v, want %+v", after, before)
			}
			// Compaction is a fixed point: compacting a compacted log changes nothing.
			if again := retainPending(kept); !reflect.DeepEqual(again, kept) {
				t.Fatalf("retainPending is not idempotent: %+v, want %+v", again, kept)
			}
		})
	}
}
