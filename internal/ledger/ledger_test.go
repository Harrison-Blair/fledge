package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWriteReadRoundtrip(t *testing.T) {
	dir := t.TempDir() + "/ledger" // Write must create the dir
	in := StatusRecord{Note: "running tests", Expect: "5m0s", UpdatedAt: "2026-07-16T12:00:00Z"}
	if _, err := Write(dir, "fledge-brooder-adelie", KindStatus, in); err != nil {
		t.Fatal(err)
	}
	got, err := Read(dir, "fledge-brooder-adelie", KindStatus)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "fledge-brooder-adelie" || got.Kind != KindStatus {
		t.Errorf("envelope = %+v, want subject fledge-brooder-adelie kind %s", got, KindStatus)
	}
	if _, err := time.Parse(time.RFC3339, got.Timestamp); err != nil {
		t.Errorf("Timestamp = %q, want RFC3339: %v", got.Timestamp, err)
	}
	var out StatusRecord
	if err := got.Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("payload = %+v, want %+v", out, in)
	}
}

// TestWriteOverwritesPriorRecord pins the latest-value-only addressing: one
// file per (subject, kind), no history.
func TestWriteOverwritesPriorRecord(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "adelie", KindStatus, StatusRecord{Note: "first"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir, "adelie", KindStatus, StatusRecord{Note: "second"}); err != nil {
		t.Fatal(err)
	}
	got, err := Read(dir, "adelie", KindStatus)
	if err != nil {
		t.Fatal(err)
	}
	var out StatusRecord
	if err := got.Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Note != "second" {
		t.Errorf("payload = %+v, want the latest write (note second)", out)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "adelie.status.json" {
		t.Errorf("dir entries = %v, want exactly one adelie.status.json (no history, no temp file)", entries)
	}
}

// TestWriteRejectsInvalidSubject pins the address-space contract from FC-1:
// a record always lands at .fledge/ledger/<subject>.<kind>.json and never
// outside dir. Subjects are rejected, never sanitized into something else.
func TestWriteRejectsInvalidSubject(t *testing.T) {
	subjects := map[string]string{
		"empty":            "",
		"dotdot":           "..",
		"parent traversal": "../escaped",
		"deep traversal":   "../../escaped",
		"slash":            "a/b",
		"leading slash":    "/abs",
		"backslash":        `a\b`,
		"embedded dotdot":  "a/../../b",
		"dot":              ".",
	}
	for name, subject := range subjects {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "ledger")

			_, err := Write(dir, subject, KindStatus, StatusRecord{Note: "pwn"})
			var ise *InvalidSubjectError
			if !errors.As(err, &ise) {
				t.Fatalf("Write(subject=%q): want *InvalidSubjectError, got %v (%T)", subject, err, err)
			}

			// Nothing may be created anywhere under root: not in the ledger
			// dir, not outside it.
			var found []string
			filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					found = append(found, path)
				}
				return nil
			})
			if len(found) != 0 {
				t.Errorf("Write(subject=%q) created files %v, want none", subject, found)
			}
		})
	}
}

// TestReadRejectsInvalidSubject: Read shares the same address space, so it
// must refuse to resolve a path outside dir rather than reading a stray file.
func TestReadRejectsInvalidSubject(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "ledger")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A file that a traversing subject would otherwise reach.
	outside := filepath.Join(root, "escaped.status.json")
	if err := os.WriteFile(outside, []byte(`{"subject":"escaped","kind":"status"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, subject := range []string{"", "..", "../escaped", "a/b", "/abs", `a\b`} {
		_, err := Read(dir, subject, KindStatus)
		var ise *InvalidSubjectError
		if !errors.As(err, &ise) {
			t.Errorf("Read(subject=%q): want *InvalidSubjectError, got %v (%T)", subject, err, err)
		}
	}
}

// TestValidSubjectsAccepted guards the rejection above from over-reach: the
// worker names this ledger actually addresses must still be accepted.
func TestValidSubjectsAccepted(t *testing.T) {
	for _, subject := range []string{
		"fledge-brooder-adelie",
		"fledge-skua-emperor2",
		"team-lead",
		"FTHR-072",
		"a.b", // dots are fine; only path elements are not
	} {
		dir := t.TempDir()
		if _, err := Write(dir, subject, KindStatus, StatusRecord{Note: "n"}); err != nil {
			t.Errorf("Write(subject=%q): unexpected error %v", subject, err)
			continue
		}
		if _, err := Read(dir, subject, KindStatus); err != nil {
			t.Errorf("Read(subject=%q): unexpected error %v", subject, err)
		}
	}
}

func TestReadMissingRecord(t *testing.T) {
	dir := t.TempDir()
	_, err := Read(dir, "nobody", KindStatus)
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("Read of never-written record: want *NotFoundError, got %v (%T)", err, err)
	}
	if nf.Subject != "nobody" || nf.Kind != KindStatus {
		t.Errorf("NotFoundError = %+v, want subject nobody kind status", nf)
	}
}

// TestReadCorruptRecord mirrors lock's corrupt-brood-file tolerance: a
// malformed file reports a typed corrupt-record error, never a panic.
func TestReadCorruptRecord(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"not json", "not json at all"},
		{"empty", ""},
		{"truncated", `{"subject":"adelie","kind":"stat`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "adelie.status.json"), []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Read(dir, "adelie", KindStatus)
			var ce *CorruptError
			if !errors.As(err, &ce) {
				t.Fatalf("want *CorruptError, got %v (%T)", err, err)
			}
		})
	}
}

// TestConcurrentWrites pins FC-2: N concurrent writers to the same
// (subject, kind) leave exactly one of the written values, never a torn or
// partial file. Same style as internal/lock's 16-way contention test.
func TestConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	const n = 16
	path := filepath.Join(dir, "adelie.status.json")

	// Seed so the readers below always have a file to observe.
	if _, err := Write(dir, "adelie", KindStatus, StatusRecord{Note: "seed"}); err != nil {
		t.Fatal(err)
	}

	var sawPartial atomic.Bool
	done := make(chan struct{})
	watcher := make(chan struct{})
	go func() {
		defer close(watcher)
		for {
			select {
			case <-done:
				return
			default:
			}
			b, err := os.ReadFile(path)
			if err == nil {
				if len(b) == 0 || json.Unmarshal(b, new(Record)) != nil {
					sawPartial.Store(true)
				}
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := Write(dir, "adelie", KindStatus, StatusRecord{Note: fmt.Sprintf("racer-%d", i)}); err != nil {
				t.Errorf("writer %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	close(done)
	<-watcher

	if sawPartial.Load() {
		t.Error("observed a zero-length or partial ledger file mid-Write")
	}

	got, err := Read(dir, "adelie", KindStatus)
	if err != nil {
		t.Fatal(err)
	}
	var out StatusRecord
	if err := got.Decode(&out); err != nil {
		t.Fatal(err)
	}
	var racer int
	if _, err := fmt.Sscanf(out.Note, "racer-%d", &racer); err != nil || racer < 1 || racer > n {
		t.Errorf("final record = %+v, want exactly one of the %d written values", out, n)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "adelie.status.json" {
		t.Errorf("dir entries = %v, want exactly one adelie.status.json (no leftover temp files)", entries)
	}
}

// TestWriteAllKinds pins that all three PLM-030 record kinds round-trip
// through the same envelope, each addressed independently.
func TestWriteAllKinds(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "adelie", KindStatus, StatusRecord{Note: "n"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir, "adelie", KindVerdict, VerdictRecord{Result: "approved", Note: "lgtm"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir, "adelie", KindEscalation, EscalationRecord{Message: "blocked on X"}); err != nil {
		t.Fatal(err)
	}

	rec, err := Read(dir, "adelie", KindVerdict)
	if err != nil {
		t.Fatal(err)
	}
	var v VerdictRecord
	if err := rec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	if v.Result != "approved" || v.Note != "lgtm" {
		t.Errorf("verdict = %+v", v)
	}

	rec, err = Read(dir, "adelie", KindEscalation)
	if err != nil {
		t.Fatal(err)
	}
	var e EscalationRecord
	if err := rec.Decode(&e); err != nil {
		t.Fatal(err)
	}
	if e.Message != "blocked on X" {
		t.Errorf("escalation = %+v", e)
	}

	// Kinds are addressed separately: the status record is untouched.
	rec, err = Read(dir, "adelie", KindStatus)
	if err != nil {
		t.Fatal(err)
	}
	var s StatusRecord
	if err := rec.Decode(&s); err != nil {
		t.Fatal(err)
	}
	if s.Note != "n" {
		t.Errorf("status = %+v, want note %q unaffected by the other kinds", s, "n")
	}
}

// TestClassifyLiveness pins PLM-035 FC-1/FC-3/FC-5: classification consults
// only lease freshness against the worker's own declared quiet period, with
// StaleAfter as the default for a lease that declares nothing. There is no
// PID input (FC-1) — the dead-PID rows PLM-030's version of this test carried
// cease to exist entirely: that failure direction is gone, not merely
// untested.
func TestClassifyLiveness(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		lastUpdated time.Time
		expect      time.Duration
		wantStalled bool
	}{
		{"fresh lease, default expect", now.Add(-time.Second), StaleAfter, false},
		{"lease just inside default expect", now.Add(-4 * time.Minute), StaleAfter, false},
		{"lease past default expect", now.Add(-6 * time.Minute), StaleAfter, true},
		{"lease far past default expect", now.Add(-time.Hour), StaleAfter, true},
		// The case PLM-035 exists for: a lease at 6 minutes, past the old
		// fixed 5-minute TTL, but well inside a 30-minute declared period —
		// impossible for the old classifier to express, let alone return
		// not-stalled for.
		{"lease past old ttl but inside a long declared period", now.Add(-6 * time.Minute), 30 * time.Minute, false},
		{"lease past a long declared period", now.Add(-31 * time.Minute), 30 * time.Minute, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stalled, reason := ClassifyLiveness(tc.lastUpdated, tc.expect, now)
			if stalled != tc.wantStalled {
				t.Errorf("ClassifyLiveness(%v, %v, %v) stalled = %v, want %v (reason %q)",
					tc.lastUpdated, tc.expect, now, stalled, tc.wantStalled, reason)
			}
			if reason == "" {
				t.Error("reason must always be non-empty")
			}
		})
	}
}

// TestStaleAfterIsFiveMinutes pins the fixed TTL from PLM-030 FC-4.
func TestStaleAfterIsFiveMinutes(t *testing.T) {
	if StaleAfter != 5*time.Minute {
		t.Errorf("StaleAfter = %v, want 5m", StaleAfter)
	}
}
