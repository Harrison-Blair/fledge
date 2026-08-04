package wake

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const testSession = "fledge-test-0a1b2c3d"

var testStart = time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)

// steppedClock advances one second per reading so entries have distinct,
// deterministic timestamps.
func steppedClock() func() time.Time {
	var guard sync.Mutex
	current := testStart
	return func() time.Time {
		guard.Lock()
		defer guard.Unlock()
		current = current.Add(time.Second)
		return current
	}
}

func sequentialIDs() func() (string, error) {
	var guard sync.Mutex
	count := 0
	return func() (string, error) {
		guard.Lock()
		defer guard.Unlock()
		count++
		return fmt.Sprintf("w-%08d", count), nil
	}
}

// scriptedIDs hands out fixed IDs in order, so a test can force the generator
// to repeat one.
func scriptedIDs(ids ...string) func() (string, error) {
	var guard sync.Mutex
	next := 0
	return func() (string, error) {
		guard.Lock()
		defer guard.Unlock()
		if next >= len(ids) {
			return "", errors.New("scripted ID generator is exhausted")
		}
		id := ids[next]
		next++
		return id, nil
	}
}

func testLedger(t *testing.T, root string) *Ledger {
	t.Helper()
	return New(root, testSession, WithClock(steppedClock()), WithIDGenerator(sequentialIDs()))
}

func mustAppend(t *testing.T, ledger *Ledger, kind Kind, key, reason string) Record {
	t.Helper()
	record, err := ledger.Append(kind, key, reason)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func mustPending(t *testing.T, ledger *Ledger) []Record {
	t.Helper()
	records, err := ledger.Pending()
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func mustReadDir(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func mustWriteFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func appendRaw(t *testing.T, path, text string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureCreatesWatchDirectory(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	if err := ledger.Ensure(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(ledger.watchPath())
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", ledger.watchPath())
	}
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("%s permission = %04o, want 0700", ledger.watchPath(), got)
		}
	}
	if _, err := os.Stat(ledger.logPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Ensure created a ledger file: %v", err)
	}
}

func TestEnsureLeavesTheStateRootBrowsable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions are not available on Windows")
	}

	// .fledge is user-facing: project.Init creates it 0755 and people browse
	// it. Only the state below it belongs to the ledger.
	root := t.TempDir()
	stateRoot := filepath.Join(root, ".fledge")
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := testLedger(t, root).Ensure(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("%s permission = %04o, want the project's 0755 left alone", stateRoot, got)
	}
}

func TestInvalidSessionNameFailsBeforeCreatingDirectories(t *testing.T) {
	root := t.TempDir()
	ledger := New(root, "../escape")
	if err := ledger.Ensure(); err == nil {
		t.Fatal("Ensure accepted an invalid session name")
	}
	if _, err := os.Stat(filepath.Join(root, ".fledge")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state directory was created: %v", err)
	}
}

func TestAppendedWakesSurviveReopen(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	first := mustAppend(t, ledger, KindStatus, "alice", "blocked: needs permission")
	second := mustAppend(t, ledger, KindEvent, "%3", "working -> blocked")

	if first.ID != "w-00000001" || second.ID != "w-00000002" {
		t.Fatalf("Append IDs = %q, %q", first.ID, second.ID)
	}
	if !first.Time.Equal(testStart.Add(time.Second)) {
		t.Fatalf("first wake time = %s, want %s", first.Time, testStart.Add(time.Second))
	}

	want := []Record{first, second}
	if got := mustPending(t, ledger); !reflect.DeepEqual(got, want) {
		t.Fatalf("Pending() = %+v, want %+v", got, want)
	}

	// A watcher that died before delivering replays the same wakes from disk.
	reopened := New(root, testSession)
	if got := mustPending(t, reopened); !reflect.DeepEqual(got, want) {
		t.Fatalf("Pending() after reopen = %+v, want %+v", got, want)
	}
}

func TestPendingIsEmptyBeforeAnyWake(t *testing.T) {
	records := mustPending(t, testLedger(t, t.TempDir()))
	if len(records) != 0 {
		t.Fatalf("Pending() = %+v, want none", records)
	}
}

func TestAppendRejectsUnknownKind(t *testing.T) {
	root := t.TempDir()
	if _, err := testLedger(t, root).Append(Kind("nap"), "alice", "reason"); err == nil {
		t.Fatal("Append accepted an unknown wake kind")
	}
}

func TestMarkDeliveredRemovesWakesFromPending(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	first := mustAppend(t, ledger, KindStatus, "alice", "blocked")
	second := mustAppend(t, ledger, KindDead, "bob", "vanished")

	if err := ledger.MarkDelivered([]string{first.ID}, "m-1"); err != nil {
		t.Fatal(err)
	}
	got := mustPending(t, ledger)
	if !reflect.DeepEqual(got, []Record{second}) {
		t.Fatalf("Pending() after delivery = %+v, want %+v", got, []Record{second})
	}

	if err := ledger.MarkDelivered([]string{second.ID}, "m-2"); err != nil {
		t.Fatal(err)
	}
	if got := mustPending(t, New(root, testSession)); len(got) != 0 {
		t.Fatalf("Pending() after delivering everything = %+v, want none", got)
	}

	contents, err := os.ReadFile(ledger.logPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), `"message_id":"m-1"`) || !strings.Contains(string(contents), `"message_id":"m-2"`) {
		t.Fatalf("ledger is missing delivery records:\n%s", contents)
	}
}

// Draining is the ledger's whole purpose: whatever Pending hands out, feeding
// exactly those IDs back to MarkDelivered must leave nothing behind. Collapsed
// duplicates must not resurface with the stale reasons they superseded.
func TestDeliveringPendingWakesQuiescesTheLedger(t *testing.T) {
	tests := []struct {
		name  string
		wakes []struct {
			kind   Kind
			key    string
			reason string
		}
	}{
		{
			name: "collapsed duplicates",
			wakes: []struct {
				kind   Kind
				key    string
				reason string
			}{
				{kind: KindStatus, key: "alice", reason: "working"},
				{kind: KindStatus, key: "alice", reason: "blocked: needs permission"},
			},
		},
		{
			name: "heartbeat run",
			wakes: []struct {
				kind   Kind
				key    string
				reason string
			}{
				{kind: KindHeartbeat, key: "600", reason: "first beat"},
				{kind: KindHeartbeat, key: "1200", reason: "second beat"},
				{kind: KindHeartbeat, key: "2400", reason: "third beat"},
			},
		},
		{
			name: "mixed kinds and keys",
			wakes: []struct {
				kind   Kind
				key    string
				reason string
			}{
				{kind: KindStatus, key: "alice", reason: "working"},
				{kind: KindEvent, key: "%3", reason: "working -> blocked"},
				{kind: KindStatus, key: "alice", reason: "blocked"},
				{kind: KindDead, key: "bob", reason: "vanished"},
				{kind: KindHeartbeat, key: "600", reason: "quiet"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			ledger := testLedger(t, root)
			for _, wake := range test.wakes {
				mustAppend(t, ledger, wake.kind, wake.key, wake.reason)
			}

			drained := mustPending(t, ledger)
			ids := make([]string, 0, len(drained))
			for _, record := range drained {
				ids = append(ids, record.IDs...)
			}
			if err := ledger.MarkDelivered(ids, "m-1"); err != nil {
				t.Fatal(err)
			}

			if got := mustPending(t, New(root, testSession)); len(got) != 0 {
				t.Fatalf("Pending() after delivering every drained wake = %+v, want none", summarize(got))
			}
		})
	}
}

// The engine's steady state: drain, then wake again for the same worker. A
// later cycle must report only what is newly owed — never an entry an earlier
// cycle already retired, and never a stale reason.
func TestRepeatedDrainCyclesReportOnlyFreshWakes(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)

	mustAppend(t, ledger, KindStatus, "alice", "working")
	superseded := mustAppend(t, ledger, KindStatus, "alice", "blocked")
	firstCycle := mustPending(t, ledger)
	if len(firstCycle) != 1 || firstCycle[0].Reason != "blocked" || len(firstCycle[0].IDs) != 2 {
		t.Fatalf("cycle 1 Pending() = %+v, want one record naming both entries", summarize(firstCycle))
	}
	drain(t, ledger, "m-1")

	fresh := mustAppend(t, ledger, KindStatus, "alice", "failed")
	secondCycle := mustPending(t, New(root, testSession))
	want := []pendingSummary{{ID: fresh.ID, IDs: []string{fresh.ID}, WakeKind: KindStatus, Key: "alice", Reason: "failed"}}
	if got := summarize(secondCycle); !reflect.DeepEqual(got, want) {
		t.Fatalf("cycle 2 Pending() = %+v, want %+v", got, want)
	}
	for _, id := range secondCycle[0].IDs {
		if id == superseded.ID {
			t.Fatalf("cycle 2 re-listed the retired entry %q", id)
		}
	}

	drain(t, ledger, "m-2")
	if got := mustPending(t, New(root, testSession)); len(got) != 0 {
		t.Fatalf("cycle 3 Pending() = %+v, want none", summarize(got))
	}
}

func TestHeartbeatsCollapseWithinEachDrainCycle(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)

	mustAppend(t, ledger, KindHeartbeat, "600", "quiet for 600s")
	firstBeat := mustAppend(t, ledger, KindHeartbeat, "1200", "quiet for 1200s")
	firstCycle := mustPending(t, ledger)
	if len(firstCycle) != 1 || firstCycle[0].ID != firstBeat.ID || len(firstCycle[0].IDs) != 2 {
		t.Fatalf("cycle 1 Pending() = %+v, want one beat naming both entries", summarize(firstCycle))
	}
	drain(t, ledger, "m-1")

	mustAppend(t, ledger, KindHeartbeat, "2400", "quiet for 2400s")
	latestBeat := mustAppend(t, ledger, KindHeartbeat, "4800", "quiet for 4800s")
	secondCycle := mustPending(t, New(root, testSession))
	want := []pendingSummary{{
		ID: latestBeat.ID, IDs: []string{secondCycle[0].IDs[0], latestBeat.ID},
		WakeKind: KindHeartbeat, Key: "4800", Reason: "quiet for 4800s",
	}}
	if got := summarize(secondCycle); !reflect.DeepEqual(got, want) || len(secondCycle[0].IDs) != 2 {
		t.Fatalf("cycle 2 Pending() = %+v, want one beat naming only the two fresh entries", got)
	}
	if secondCycle[0].IDs[0] == firstBeat.ID {
		t.Fatal("cycle 2 re-listed a retired beat")
	}

	drain(t, ledger, "m-2")
	if got := mustPending(t, New(root, testSession)); len(got) != 0 {
		t.Fatalf("cycle 3 Pending() = %+v, want none", summarize(got))
	}
}

func TestMarkDeliveredRejectsBlankMessageIDAndIgnoresEmptyIDs(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	record := mustAppend(t, ledger, KindStatus, "alice", "blocked")

	if err := ledger.MarkDelivered([]string{record.ID}, "  "); err == nil {
		t.Fatal("MarkDelivered accepted a blank message ID")
	}
	if err := ledger.MarkDelivered(nil, "m-1"); err != nil {
		t.Fatal(err)
	}
	if got := mustPending(t, ledger); len(got) != 1 {
		t.Fatalf("Pending() = %+v, want the undelivered wake", got)
	}
}

func TestRepeatedWakesCollapseThroughTheLedger(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	superseded := mustAppend(t, ledger, KindStatus, "alice", "working")
	firstBeat := mustAppend(t, ledger, KindHeartbeat, "600", "no activity for 600s")
	latest := mustAppend(t, ledger, KindStatus, "alice", "blocked: needs permission")
	beat := mustAppend(t, ledger, KindHeartbeat, "1200", "no activity for 1200s")

	got := summarize(mustPending(t, New(root, testSession)))
	want := []pendingSummary{
		{ID: latest.ID, IDs: []string{superseded.ID, latest.ID}, WakeKind: KindStatus, Key: "alice", Reason: "blocked: needs permission"},
		{ID: beat.ID, IDs: []string{firstBeat.ID, beat.ID}, WakeKind: KindHeartbeat, Key: "1200", Reason: "no activity for 1200s"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Pending() = %+v, want %+v", got, want)
	}
}

func TestRepairsOnlyUnterminatedTail(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	record := mustAppend(t, ledger, KindStatus, "alice", "blocked")
	partial := `{"kind":"queued","id":"w-00000009","wake_kind":"dead"`
	appendRaw(t, ledger.logPath(), partial)

	got := mustPending(t, New(root, testSession))
	if !reflect.DeepEqual(got, []Record{record}) {
		t.Fatalf("Pending() after tail repair = %+v, want %+v", got, []Record{record})
	}
	contents, err := os.ReadFile(ledger.logPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), partial) {
		t.Fatalf("torn tail survived the repair:\n%s", contents)
	}

	// The repaired ledger still accepts new wakes.
	next := mustAppend(t, ledger, KindDead, "bob", "vanished")
	if got := summarize(mustPending(t, ledger)); len(got) != 2 || got[1].ID != next.ID {
		t.Fatalf("Pending() after repair and append = %+v", got)
	}
}

// MarkDelivered reads before it appends so a torn tail is repaired first;
// appending onto an unterminated line would fuse garbage and JSON into one
// complete but unparsable line, wedging the ledger and losing the delivery.
func TestMarkDeliveredRepairsATornTailBeforeAppending(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	record := mustAppend(t, ledger, KindStatus, "alice", "blocked")
	partial := `{"kind":"queued","id":"w-00000009","wake_kind":"dead"`
	appendRaw(t, ledger.logPath(), partial)

	if err := ledger.MarkDelivered([]string{record.ID}, "m-1"); err != nil {
		t.Fatal(err)
	}
	got, err := New(root, testSession).Pending()
	if err != nil {
		t.Fatalf("Pending() after delivering onto a torn tail: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Pending() = %+v, want none", summarize(got))
	}
	contents, err := os.ReadFile(ledger.logPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), partial) {
		t.Fatalf("torn tail survived MarkDelivered:\n%s", contents)
	}
}

func TestMarkDeliveredRejectsUnusableIDsWithoutWriting(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
	}{
		{name: "blank ID", ids: []string{""}},
		{name: "whitespace ID", ids: []string{"   "}},
		{name: "ID with a line break", ids: []string{"w-1\nw-2"}},
		{name: "one unusable ID among valid ones", ids: []string{"w-00000001", ""}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			ledger := testLedger(t, root)
			mustAppend(t, ledger, KindStatus, "alice", "blocked")
			before, err := os.ReadFile(ledger.logPath())
			if err != nil {
				t.Fatal(err)
			}

			if err := ledger.MarkDelivered(test.ids, "m-1"); err == nil {
				t.Fatal("MarkDelivered accepted an unusable wake ID")
			}
			after, err := os.ReadFile(ledger.logPath())
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatalf("MarkDelivered wrote to the ledger before failing:\n%s", after)
			}
			if _, err := ledger.Pending(); err != nil {
				t.Fatalf("Pending() after a rejected delivery: %v", err)
			}
		})
	}
}

func TestAppendSkipsReusedIDsAndRejectsUnusableOnes(t *testing.T) {
	t.Run("reused ID is skipped", func(t *testing.T) {
		root := t.TempDir()
		ledger := New(root, testSession, WithClock(steppedClock()), WithIDGenerator(scriptedIDs("w-1", "w-1", "w-2")))
		first := mustAppend(t, ledger, KindStatus, "alice", "working")
		second := mustAppend(t, ledger, KindStatus, "bob", "working")
		if first.ID != "w-1" || second.ID != "w-2" {
			t.Fatalf("Append IDs = %q, %q, want w-1 and w-2", first.ID, second.ID)
		}
		if got := mustPending(t, ledger); len(got) != 2 {
			t.Fatalf("Pending() = %+v, want both wakes", summarize(got))
		}
	})

	t.Run("exhausted generator fails", func(t *testing.T) {
		root := t.TempDir()
		ledger := New(root, testSession, WithClock(steppedClock()), WithIDGenerator(func() (string, error) { return "w-1", nil }))
		mustAppend(t, ledger, KindStatus, "alice", "working")
		if _, err := ledger.Append(KindStatus, "bob", "working"); err == nil {
			t.Fatal("Append accepted a duplicate wake ID")
		}
	})

	invalid := []struct {
		name string
		id   string
	}{
		{name: "blank", id: "  "},
		{name: "line break", id: "w-1\nw-2"},
	}
	for _, test := range invalid {
		t.Run("rejects an "+test.name+" ID", func(t *testing.T) {
			root := t.TempDir()
			ledger := New(root, testSession, WithClock(steppedClock()), WithIDGenerator(func() (string, error) { return test.id, nil }))
			if _, err := ledger.Append(KindStatus, "alice", "working"); err == nil {
				t.Fatal("Append accepted an unusable wake ID")
			}
			if _, err := os.Stat(ledger.logPath()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("Append wrote a ledger despite an unusable ID: %v", err)
			}
		})
	}
}

func TestRejectsCompletedCorruptLines(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	mustAppend(t, ledger, KindStatus, "alice", "blocked")
	appendRaw(t, ledger.logPath(), `{"kind":"queued","id":"w-00000009","wake_kind":"nap","time":"2026-08-04T12:00:00Z"}`+"\n")

	if _, err := ledger.Pending(); !errors.Is(err, ErrCorruptLog) {
		t.Fatalf("Pending() error = %v, want ErrCorruptLog", err)
	}
}

func TestConcurrentAppendsAreSerialized(t *testing.T) {
	root := t.TempDir()
	const writers = 64
	var wait sync.WaitGroup
	failures := make(chan error, writers)
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := New(root, testSession).Append(KindStatus, fmt.Sprintf("agent-%d", index), fmt.Sprintf("blocked-%d", index))
			failures <- err
		}(index)
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}

	records := mustPending(t, New(root, testSession))
	if len(records) != writers {
		t.Fatalf("Pending() returned %d wakes, want %d", len(records), writers)
	}
	seenIDs := make(map[string]bool, writers)
	seenKeys := make(map[string]bool, writers)
	for _, record := range records {
		if seenIDs[record.ID] {
			t.Fatalf("duplicate wake ID %q", record.ID)
		}
		seenIDs[record.ID] = true
		seenKeys[record.Key] = true
	}
	if len(seenKeys) != writers {
		t.Fatalf("Pending() covered %d distinct keys, want %d", len(seenKeys), writers)
	}
}

func TestLedgerOperationsWaitForTheLock(t *testing.T) {
	root := t.TempDir()
	ledger := testLedger(t, root)
	if err := ledger.Ensure(); err != nil {
		t.Fatal(err)
	}
	unlock, err := ledger.acquireLock(ledger.lockPath())
	if err != nil {
		t.Fatal(err)
	}

	blocked := make(chan error, 1)
	go func() {
		_, err := New(root, testSession).Append(KindStatus, "alice", "blocked")
		blocked <- err
	}()
	select {
	case err := <-blocked:
		t.Fatalf("Append finished while another holder had the lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-blocked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Append did not finish after the lock was released")
	}
	if got := mustPending(t, ledger); len(got) != 1 {
		t.Fatalf("Pending() = %+v, want the queued wake", got)
	}
}

func TestRejectsLedgerLockAndMarkerSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	tests := []struct {
		name      string
		path      func(*Ledger) string
		operation func(*Ledger) error
	}{
		{
			name:      "ledger",
			path:      func(l *Ledger) string { return l.logPath() },
			operation: func(l *Ledger) error { _, err := l.Append(KindStatus, "alice", "blocked"); return err },
		},
		{
			name:      "lock",
			path:      func(l *Ledger) string { return l.lockPath() },
			operation: func(l *Ledger) error { _, err := l.Pending(); return err },
		},
		{
			name:      "markers",
			path:      func(l *Ledger) string { return l.markersPath() },
			operation: func(l *Ledger) error { return l.SaveMarkers(Markers{HeartbeatStreak: 2}) },
		},
		{
			name:      "markers read",
			path:      func(l *Ledger) string { return l.markersPath() },
			operation: func(l *Ledger) error { _, err := l.LoadMarkers(); return err },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ledger := testLedger(t, t.TempDir())
			if err := ledger.Ensure(); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "target")
			if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, test.path(ledger)); err != nil {
				t.Fatal(err)
			}
			if err := test.operation(ledger); err == nil || !strings.Contains(err.Error(), "symlink") {
				t.Fatalf("operation error = %v, want symlink rejection", err)
			}
			contents, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if string(contents) != "unchanged" {
				t.Fatalf("symlink target was modified: %q", contents)
			}
		})
	}
}

func TestRejectsSymlinkedWatchDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	ledger := testLedger(t, t.TempDir())
	if err := os.MkdirAll(filepath.Dir(ledger.watchPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), ledger.watchPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Append(KindStatus, "alice", "blocked"); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Append error = %v, want symlink rejection", err)
	}
}

func TestPermissionsAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions are not available on Windows")
	}
	ledger := testLedger(t, t.TempDir())
	mustAppend(t, ledger, KindStatus, "alice", "blocked")
	if err := ledger.SaveMarkers(Markers{}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{ledger.watchPath(), ledger.logPath(), ledger.lockPath(), ledger.markersPath()} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0o600)
		if info.IsDir() {
			want = 0o700
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s permission = %04o, want %04o", path, got, want)
		}
	}
}
