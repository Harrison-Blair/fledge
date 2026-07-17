package cli

import (
	"encoding/json"
	"strings"
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

// TestAwaitReturnsOnAppearance: change-wait, record absent at start, appears
// mid-poll, await returns it, no error.
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

	result, err := pollAwait(read, *clock, "dir", "subj", "status", false, 0, false)
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

// TestAwaitChangeWaitStillDetectsPayloadChange: record present at start,
// changes mid-poll, change-wait (--exists not set) returns the new value.
// Guards FTHR-073's shipped change-wait behavior against regression.
func TestAwaitChangeWaitStillDetectsPayloadChange(t *testing.T) {
	clock, _ := fakeClock()
	calls := 0
	read := func(dir, subject, kind string) (*ledger.Record, error) {
		calls++
		if calls < 3 {
			return &ledger.Record{Subject: subject, Kind: kind, Payload: json.RawMessage(`{"note":"first"}`)}, nil
		}
		return &ledger.Record{Subject: subject, Kind: kind, Payload: json.RawMessage(`{"note":"second"}`)}, nil
	}

	result, err := pollAwait(read, *clock, "dir", "subj", "status", false, 0, false)
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

	result, err := pollAwait(read, *clock, "dir", "subj", "status", false, 3*time.Second, true)
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

// TestAwaitExistsReturnsImmediatelyWhenPresent: existence-wait, record
// already present at call time — the exact condition that deadlocks
// change-wait today. Must return on the very first read, no sleep.
func TestAwaitExistsReturnsImmediatelyWhenPresent(t *testing.T) {
	clock, _ := fakeClock()
	calls := 0
	read := func(dir, subject, kind string) (*ledger.Record, error) {
		calls++
		return &ledger.Record{Subject: subject, Kind: kind, Payload: json.RawMessage(`{"result":"pass"}`)}, nil
	}

	result, err := pollAwait(read, *clock, "dir", "subj", "verdict", true, 0, false)
	if err != nil {
		t.Fatalf("pollAwait returned error: %v", err)
	}
	if result.timedOut {
		t.Errorf("timedOut = true, want false")
	}
	if result.record == nil {
		t.Fatalf("record = nil, want the present record")
	}
	if calls != 1 {
		t.Errorf("read called %d times, want exactly 1 (no baseline sample, no poll needed)", calls)
	}
}

// TestAwaitExistsReturnsOnAppearance: existence-wait, record absent at call
// time, appears mid-poll, await returns it.
func TestAwaitExistsReturnsOnAppearance(t *testing.T) {
	clock, _ := fakeClock()
	calls := 0
	read := func(dir, subject, kind string) (*ledger.Record, error) {
		calls++
		if calls < 3 {
			return nil, &ledger.NotFoundError{Subject: subject, Kind: kind}
		}
		return &ledger.Record{Subject: subject, Kind: kind, Payload: json.RawMessage(`{"message":"blocked"}`)}, nil
	}

	result, err := pollAwait(read, *clock, "dir", "subj", "escalation", true, 0, false)
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

// TestAwaitExistsIgnoresIdenticalPayloadRewrite: existence-wait never samples
// a baseline or consults the payload, so a record whose payload is
// byte-identical on every read (the identical-payload rewrite defect that
// hides a change from change-wait) still satisfies existence-wait on the
// very first read. A bounded timeout guards against a real hang if this
// regresses to payload comparison.
func TestAwaitExistsIgnoresIdenticalPayloadRewrite(t *testing.T) {
	clock, _ := fakeClock()
	calls := 0
	read := func(dir, subject, kind string) (*ledger.Record, error) {
		calls++
		return &ledger.Record{Subject: subject, Kind: kind, Payload: json.RawMessage(`{"result":"pass"}`)}, nil
	}

	result, err := pollAwait(read, *clock, "dir", "subj", "verdict", true, 5*time.Second, true)
	if err != nil {
		t.Fatalf("pollAwait returned error: %v", err)
	}
	if result.timedOut {
		t.Fatalf("timedOut = true, want false (existence-wait must not wait for a payload change)")
	}
	if result.record == nil {
		t.Fatalf("record = nil, want the present record")
	}
	if calls != 1 {
		t.Errorf("read called %d times, want exactly 1 (payload never consulted)", calls)
	}
}

// TestAwaitExistsTimesOut: fake clock advances past --timeout with the
// record never appearing, in existence-wait mode.
func TestAwaitExistsTimesOut(t *testing.T) {
	clock, _ := fakeClock()
	calls := 0
	read := func(dir, subject, kind string) (*ledger.Record, error) {
		calls++
		return nil, &ledger.NotFoundError{Subject: subject, Kind: kind}
	}

	result, err := pollAwait(read, *clock, "dir", "subj", "verdict", true, 3*time.Second, true)
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

// TestAwaitRequiresTimeoutBothModes: omitting --timeout is a usage error on
// both the existence-wait and change-wait paths. Checked before repo.Find(),
// so it needs no repo and can be called directly.
func TestAwaitRequiresTimeoutBothModes(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"change-wait", []string{"some-subject", "--kind", "status"}},
		{"exists", []string{"some-subject", "--kind", "verdict", "--exists"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runAwait(c.args)
			if got != ExitUsage {
				t.Errorf("runAwait(%v) = %d, want ExitUsage (%d)", c.args, got, ExitUsage)
			}
		})
	}
}

// TestAwaitUsageTextNamesPerKindModes: await's registered usage/help text
// states the correct wait mode per record kind, so the guidance cannot
// silently drift from the behavior it describes.
func TestAwaitUsageTextNamesPerKindModes(t *testing.T) {
	usage := commands["await"].usage
	for _, want := range []string{"--exists", "verdict", "escalation", "status"} {
		if !strings.Contains(usage, want) {
			t.Errorf("await usage text = %q, want it to mention %q", usage, want)
		}
	}
}
