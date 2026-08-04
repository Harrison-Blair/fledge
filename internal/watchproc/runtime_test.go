package watchproc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/statedir"
	"github.com/Harrison-Blair/fledge/internal/wake"
	"github.com/Harrison-Blair/fledge/internal/watch"
)

const testSession = "fledge-watchproc-1234abcd"

func testConfig() watch.Config {
	return watch.Config{
		Enabled:                 true,
		PollIntervalSeconds:     1,
		IdlePollIntervalSeconds: 1,
		HeartbeatSeconds:        0,
		EventStream:             false,
	}
}

type staticHerdr struct {
	mu            sync.Mutex
	protocol      int
	protocolErr   error
	sessions      [][]herdr.Session
	listCalls     int
	snapshot      herdr.Snapshot
	snapshotCalls int
}

func (h *staticHerdr) Protocol(context.Context) (int, error) {
	return h.protocol, h.protocolErr
}

func (h *staticHerdr) List(context.Context) ([]herdr.Session, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	index := h.listCalls
	h.listCalls++
	if len(h.sessions) == 0 {
		return nil, nil
	}
	if index >= len(h.sessions) {
		index = len(h.sessions) - 1
	}
	return h.sessions[index], nil
}

func (h *staticHerdr) Snapshot(context.Context, string) (herdr.Snapshot, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.snapshotCalls++
	return h.snapshot, nil
}

func (h *staticHerdr) counts() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.listCalls, h.snapshotCalls
}

func TestRunForegroundLogsToOutputAndOwnerOnlyFile(t *testing.T) {
	root := t.TempDir()
	client := &staticHerdr{sessions: [][]herdr.Session{{{Name: testSession, Running: false}}}}
	var output bytes.Buffer

	err := Run(context.Background(), Options{
		Root: root, Session: testSession, Config: testConfig(), Herdr: client,
		Deliver: func(context.Context, string) (string, error) { return "msg-unused", nil },
		Output:  &output,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	logPath := filepath.Join(statedir.Session(root, testSession), LogFilename)
	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"stdout": output.String(), "watch log": string(contents)} {
		if !strings.Contains(text, "session "+testSession+" is gone; watcher exiting") {
			t.Fatalf("%s = %q, want engine decision", name, text)
		}
	}
	assertPermission(t, logPath, 0o600)
	assertPermission(t, filepath.Join(statedir.WatchSession(root, testSession), lockFilename), 0o600)
	assertPermission(t, filepath.Join(statedir.WatchSession(root, testSession), beaconFilename), 0o600)
	if _, err := os.Stat(filepath.Join(statedir.WatchSession(root, testSession), pidFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("watch.pid after exit: %v, want absent", err)
	}
}

func TestRunDaemonExitsNilWhenSingletonIsHeld(t *testing.T) {
	root := t.TempDir()
	if err := ensureStateDirectories(root, testSession); err != nil {
		t.Fatal(err)
	}
	lock, err := acquire(filepath.Join(statedir.WatchSession(root, testSession), lockFilename))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()

	err = Run(context.Background(), Options{
		Root: root, Session: testSession, Config: testConfig(), Herdr: &staticHerdr{},
		Deliver: func(context.Context, string) (string, error) { return "", nil },
		Output:  io.Discard, Daemon: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
}

func TestRunAttachedPrintsBacklogTailsCompleteLinesAndStopsWithOwner(t *testing.T) {
	root := t.TempDir()
	if err := ensureStateDirectories(root, testSession); err != nil {
		t.Fatal(err)
	}
	if err := ensureLogDirectory(root, testSession); err != nil {
		t.Fatal(err)
	}
	lock, err := acquire(filepath.Join(statedir.WatchSession(root, testSession), lockFilename))
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(statedir.Session(root, testSession), LogFilename)
	file, err := openOwned(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(file, "line%02d\n", i)
	}
	_, _ = io.WriteString(file, "partial")
	_ = file.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output lockedBuffer
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			Root: root, Session: testSession, Config: testConfig(), Herdr: &staticHerdr{},
			Deliver: func(context.Context, string) (string, error) { return "", nil },
			Output:  &output,
		})
	}()

	waitFor(t, func() bool { return strings.Contains(output.String(), "line60\n") })
	appendFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(appendFile, "-done\nline61\n")
	_ = appendFile.Close()
	waitFor(t, func() bool { return strings.Contains(output.String(), "partial-done\nline61\n") })

	if err := lock.release(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("attached Run() error = %v, want nil when owner exits", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("attached Run did not stop after singleton owner exited")
	}

	got := output.String()
	if strings.Contains(got, "line10\n") || !strings.HasPrefix(got, "line11\n") {
		t.Fatalf("backlog starts incorrectly: %q", got[:min(len(got), 80)])
	}
}

func TestRunWritesPIDAndTouchesBeaconDuringCycle(t *testing.T) {
	root := t.TempDir()
	started := make(chan struct{})
	client := blockingHerdr{started: started}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			Root: root, Session: testSession, Config: testConfig(), Herdr: client,
			Deliver: func(context.Context, string) (string, error) { return "", nil },
			Output:  io.Discard, Daemon: true,
		})
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("engine did not begin its first cycle")
	}

	pidPath := filepath.Join(statedir.WatchSession(root, testSession), pidFilename)
	contents, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(contents)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("watch.pid = %q", contents)
	}
	assertPermission(t, pidPath, 0o600)
	beaconPath := filepath.Join(statedir.WatchSession(root, testSession), beaconFilename)
	info, err := os.Stat(beaconPath)
	if err != nil || info.ModTime().IsZero() {
		t.Fatalf("beacon stat = %v, %v", info, err)
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context canceled", err)
	}
}

type blockingHerdr struct{ started chan struct{} }

func (blockingHerdr) Protocol(context.Context) (int, error) { return 0, nil }
func (h blockingHerdr) List(ctx context.Context) ([]herdr.Session, error) {
	select {
	case <-h.started:
	default:
		close(h.started)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}
func (blockingHerdr) Snapshot(context.Context, string) (herdr.Snapshot, error) {
	return herdr.Snapshot{}, nil
}

func TestUnsupportedProtocolDisablesEventsButStillPolls(t *testing.T) {
	root := t.TempDir()
	config := testConfig()
	config.EventStream = true
	config.MinProtocol = 16
	client := &staticHerdr{protocol: 15, sessions: [][]herdr.Session{{{Name: testSession, Running: false}}}}
	var output bytes.Buffer

	err := Run(context.Background(), Options{
		Root: root, Session: testSession, Config: config, Herdr: client,
		Deliver: func(context.Context, string) (string, error) { return "", nil }, Output: &output,
	})
	if err != nil {
		t.Fatal(err)
	}
	lists, snapshots := client.counts()
	if lists != 1 || snapshots != 0 {
		t.Fatalf("poll calls = List %d Snapshot %d, want 1 and 0", lists, snapshots)
	}
	if !strings.Contains(output.String(), "event stream disabled") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestUnavailableSocketDisablesEventsButStillPolls(t *testing.T) {
	root := t.TempDir()
	config := testConfig()
	config.EventStream = true
	config.MinProtocol = 16
	client := &staticHerdr{
		protocol: 19,
		sessions: [][]herdr.Session{
			{{Name: testSession, Running: true, SocketPath: filepath.Join(root, "missing.sock")}},
			{{Name: testSession, Running: false}},
		},
	}
	var output bytes.Buffer

	err := Run(context.Background(), Options{
		Root: root, Session: testSession, Config: config, Herdr: client,
		Deliver: func(context.Context, string) (string, error) { return "", nil }, Output: &output,
	})
	if err != nil {
		t.Fatal(err)
	}
	lists, snapshots := client.counts()
	if lists != 2 || snapshots != 0 {
		t.Fatalf("preflight/poll calls = List %d Snapshot %d, want 2 and 0", lists, snapshots)
	}
	if !strings.Contains(output.String(), "event stream disabled") || !strings.Contains(output.String(), "socket") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestBeaconHerdrTouchesExactlyOncePerList(t *testing.T) {
	count := 0
	wrapped := beaconHerdr{
		Herdr: &staticHerdr{},
		touch: func() error { count++; return nil },
	}
	_, _ = wrapped.List(context.Background())
	_, _ = wrapped.List(context.Background())
	if count != 2 {
		t.Fatalf("beacon touches = %d, want one per List", count)
	}
}

func TestWakeLedgerMapsCorruptionAndPreservesEveryRecordID(t *testing.T) {
	root := t.TempDir()
	ledger := wake.New(root, testSession,
		wake.WithIDGenerator(sequence("w-one", "w-two")),
		wake.WithClock(func() time.Time { return time.Unix(1, 0).UTC() }),
	)
	adapter := wakeLedger{ledger: ledger}
	if _, err := adapter.Append(watch.KindStatus, "worker", "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Append(watch.KindStatus, "worker", "latest"); err != nil {
		t.Fatal(err)
	}
	records, err := adapter.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || strings.Join(records[0].IDs, ",") != "w-one,w-two" {
		t.Fatalf("pending records = %#v", records)
	}
	if err := adapter.MarkDelivered(records[0].IDs, "msg-one"); err != nil {
		t.Fatal(err)
	}
	if remaining, err := adapter.Pending(); err != nil || len(remaining) != 0 {
		t.Fatalf("remaining = %#v, err = %v", remaining, err)
	}

	corruptRoot := t.TempDir()
	corrupt := wake.New(corruptRoot, testSession)
	if err := corrupt.Ensure(); err != nil {
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(statedir.WatchSession(corruptRoot, testSession), "ledger.jsonl")
	if err := os.WriteFile(ledgerPath, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = (wakeLedger{ledger: corrupt}).Pending()
	if !errors.Is(err, watch.ErrCorruptLog) {
		t.Fatalf("Pending() error = %v, want watch.ErrCorruptLog", err)
	}
}

func TestCompletionAuditRejectsWrongRecipient(t *testing.T) {
	since := time.Unix(100, 0).UTC()
	audit := completionAudit{store: messageListerFunc(func() ([]messaging.Message, error) {
		return []messaging.Message{
			{Sender: "worker", Recipient: "user", CreatedAt: since},
		}, nil
	})}
	completed, err := audit.CompletionSince("worker", since)
	if err != nil || completed {
		t.Fatalf("CompletionSince() = %v, %v, want false", completed, err)
	}
}

func TestCompletionAuditRejectsMessageBeforeSince(t *testing.T) {
	since := time.Unix(100, 0).UTC()
	audit := completionAudit{store: messageListerFunc(func() ([]messaging.Message, error) {
		return []messaging.Message{
			{Sender: "worker", Recipient: "orchestrator", CreatedAt: since.Add(-time.Nanosecond)},
		}, nil
	})}
	completed, err := audit.CompletionSince("worker", since)
	if err != nil || completed {
		t.Fatalf("CompletionSince() = %v, %v, want false", completed, err)
	}
}

func TestCompletionAuditAcceptsExactSinceBoundary(t *testing.T) {
	since := time.Unix(100, 0).UTC()
	audit := completionAudit{store: messageListerFunc(func() ([]messaging.Message, error) {
		return []messaging.Message{
			{Sender: "worker", Recipient: "orchestrator", CreatedAt: since},
		}, nil
	})}
	completed, err := audit.CompletionSince("worker", since)
	if err != nil || !completed {
		t.Fatalf("CompletionSince() = %v, %v, want true", completed, err)
	}
}

type messageListerFunc func() ([]messaging.Message, error)

func (f messageListerFunc) List() ([]messaging.Message, error) { return f() }

func sequence(values ...string) func() (string, error) {
	index := 0
	return func() (string, error) {
		value := values[index]
		index++
		return value, nil
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(contents []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(contents)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func assertPermission(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s permissions = %#o, want %#o", path, got, want)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met")
}
