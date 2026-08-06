//go:build linux

package fswatch

import (
	"encoding/binary"
	"testing"

	"golang.org/x/sys/unix"
)

// record encodes one inotify event the way the kernel does, so the filter is
// exercised against the exact layout it parses in production.
func record(name string, padding int) []byte {
	nameBytes := append([]byte(name), make([]byte, padding)...)
	event := make([]byte, unix.SizeofInotifyEvent+len(nameBytes))
	binary.NativeEndian.PutUint32(event[0:4], 1)                        // wd
	binary.NativeEndian.PutUint32(event[4:8], unix.IN_MODIFY)           // mask
	binary.NativeEndian.PutUint32(event[12:16], uint32(len(nameBytes))) // len
	copy(event[unix.SizeofInotifyEvent:], nameBytes)
	return event
}

func TestMatchedSelectsTheNamedEntry(t *testing.T) {
	t.Parallel()

	batch := append(record("other.log", 3), record("events.jsonl", 4)...)
	if !matched(batch, "events.jsonl") {
		t.Fatal("matched() missed the named entry in a multi-record batch")
	}
	if matched(record("other.log", 3), "events.jsonl") {
		t.Fatal("matched() accepted an unrelated entry")
	}
	if !matched(record("other.log", 3), "") {
		t.Fatal("matched() dropped a directory-wide change")
	}
}

// A truncated batch must resolve as a match: a spurious wake is harmless
// because every reader re-reads state, while a dropped one stalls coordination.
func TestMatchedTreatsTruncationAsAChange(t *testing.T) {
	t.Parallel()

	full := record("events.jsonl", 4)
	if !matched(full[:len(full)-3], "events.jsonl") {
		t.Fatal("matched() dropped a truncated record")
	}
}
