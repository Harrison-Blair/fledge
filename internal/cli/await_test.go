package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/ledger"
)

// fakeClock advances instantly on sleep, so unit tests exercise the polling
// loop's logic without paying real wall-clock cost.
func fakeClock() (*awaitClock, *time.Time) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	clock := &awaitClock{
		now:   func() time.Time { return now },
		sleep: func(d time.Duration) { now = now.Add(d) },
	}
	return clock, &now
}

// TestAwaitReturnsOnAppearance: record absent at start, appears mid-poll,
// await returns it, no error.
func TestAwaitReturnsOnAppearance(t *testing.T) {
	clock, _ := fakeClock()
	calls := 0
	read := func(dir, subject, kind string) (*ledger.Record, error) {
		calls++
		if calls < 3 {
			return nil, &ledger.NotFoundError{Subject: subject, Kind: kind}
		}
		return &ledger.Record{Subject: subject, Kind: kind, Payload: json.RawMessage(`{"pid":1}`)}, nil
	}

	result, err := pollAwait(read, *clock, "dir", "subj", "status", 0, false)
	if err != nil {
		t.Fatalf("pollAwait returned error: %v", err)
	}
	if result.timedOut {
		t.Errorf("timedOut = true, want false")
	}
	if result.record == nil {
		t.Fatalf("record = nil, want the appeared record")
	}
	if calls < 3 {
		t.Errorf("read called %d times, want at least 3 (poll until appearance)", calls)
	}
}

// TestAwaitReturnsOnChange: record present at start, changes mid-poll, await
// returns the new value.
func TestAwaitReturnsOnChange(t *testing.T) {
	clock, _ := fakeClock()
	calls := 0
	read := func(dir, subject, kind string) (*ledger.Record, error) {
		calls++
		if calls < 3 {
			return &ledger.Record{Subject: subject, Kind: kind, Payload: json.RawMessage(`{"note":"first"}`)}, nil
		}
		return &ledger.Record{Subject: subject, Kind: kind, Payload: json.RawMessage(`{"note":"second"}`)}, nil
	}

	result, err := pollAwait(read, *clock, "dir", "subj", "status", 0, false)
	if err != nil {
		t.Fatalf("pollAwait returned error: %v", err)
	}
	if result.timedOut {
		t.Errorf("timedOut = true, want false")
	}
	if result.record == nil || string(result.record.Payload) != `{"note":"second"}` {
		t.Fatalf("record = %+v, want the changed payload", result.record)
	}
}

// TestAwaitTimesOutNoChange: fake clock advances past --timeout with no
// change; returns the ExitTimeout-equivalent outcome and the last-known
// record (absent, so nil).
func TestAwaitTimesOutNoChange(t *testing.T) {
	clock, _ := fakeClock()
	calls := 0
	read := func(dir, subject, kind string) (*ledger.Record, error) {
		calls++
		return nil, &ledger.NotFoundError{Subject: subject, Kind: kind}
	}

	result, err := pollAwait(read, *clock, "dir", "subj", "status", 3*time.Second, true)
	if err != nil {
		t.Fatalf("pollAwait returned error: %v", err)
	}
	if !result.timedOut {
		t.Errorf("timedOut = false, want true")
	}
	if result.record != nil {
		t.Errorf("record = %+v, want nil (never appeared)", result.record)
	}
	if calls < 2 {
		t.Errorf("read called %d times, want at least 2 (polled before timing out)", calls)
	}
}
