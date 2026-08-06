package trace

import (
	"os"
	"path/filepath"
	"testing"
)

const wakeLine = `{"version":1,"type":"wake_requested","at":"2026-08-05T20:41:09Z","session_id":"s","wake_id":"w-31a4","wake_kind":"message","task_id":"m-9f2c","recipient":"impl-worker","recipient_pane":"%12","body":"x"}` + "\n"

const progressLine = `{"version":1,"type":"task_progress","at":"2026-08-05T20:44:03Z","session_id":"s","task_id":"t-77bd","task_status":"active","detail":"build green","actor":"impl-worker"}` + "\n"

func writeLedger(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A line still being appended must be left alone until it is whole, or the
// reader consumes half an event and never sees the rest.
func TestReadIgnoresATornTrailingLineUntilItCompletes(t *testing.T) {
	t.Parallel()

	partial := progressLine[:40]
	path := writeLedger(t, wakeLine+partial)
	entries, offset, err := Read(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].WakeID != "w-31a4" {
		t.Fatalf("entries = %#v", entries)
	}
	if offset != int64(len(wakeLine)) {
		t.Fatalf("offset = %d, want %d", offset, len(wakeLine))
	}
	if err := os.WriteFile(path, []byte(wakeLine+progressLine), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, offset, err = Read(path, offset)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].TaskID != "t-77bd" || entries[0].Actor != "impl-worker" {
		t.Fatalf("resumed entries = %#v", entries)
	}
	if offset != int64(len(wakeLine)+len(progressLine)) {
		t.Fatalf("resumed offset = %d", offset)
	}
}

func TestReadSkipsUndecodableLinesInsteadOfFailing(t *testing.T) {
	t.Parallel()

	path := writeLedger(t, "garbage not json\n"+wakeLine)
	entries, offset, err := Read(path, 0)
	if err != nil {
		t.Fatalf("Read() = %v, want an undecodable line to be skipped", err)
	}
	if len(entries) != 1 || entries[0].WakeID != "w-31a4" {
		t.Fatalf("entries = %#v", entries)
	}
	if offset != int64(len("garbage not json\n")+len(wakeLine)) {
		t.Fatalf("offset = %d", offset)
	}
}

func TestReadHandlesAnAbsentOrRewrittenLedger(t *testing.T) {
	t.Parallel()

	entries, offset, err := Read(filepath.Join(t.TempDir(), "missing.jsonl"), 12)
	if err != nil || entries != nil || offset != 12 {
		t.Fatalf("Read(missing) = %#v, %d, %v", entries, offset, err)
	}
	// A ledger that shrank was replaced, so the reader restarts from its head
	// rather than reading from an offset that now points into the middle.
	path := writeLedger(t, wakeLine)
	entries, offset, err = Read(path, int64(len(wakeLine)*4))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || offset != int64(len(wakeLine)) {
		t.Fatalf("entries/offset after truncation = %#v / %d", entries, offset)
	}
}
