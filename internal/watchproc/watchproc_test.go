package watchproc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/statedir"
)

const testSession = "fledge-test-1234abcd"

func sessionRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := ensureStateDirectories(root, testSession); err != nil {
		t.Fatal(err)
	}
	if err := ensureLogDirectory(root, testSession); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestWaitReadyReturnsImmediatelyForAnExistingMarker(t *testing.T) {
	t.Parallel()

	root := sessionRoot(t)
	marker := filepath.Join(statedir.TempSession(root, testSession), readyFilename)
	if err := os.WriteFile(marker, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := WaitReady(ctx, root, testSession); err != nil {
		t.Fatalf("WaitReady() = %v", err)
	}
}

// Readiness must arrive from the filesystem notification the dispatcher's
// marker produces, with nothing on either side keeping time.
func TestWaitReadyObservesAMarkerWrittenLater(t *testing.T) {
	t.Parallel()

	root := sessionRoot(t)
	marker := filepath.Join(statedir.TempSession(root, testSession), readyFilename)
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { done <- WaitReady(ctx, root, testSession) }()
	// The watch is armed before the wait begins and re-stats afterwards, so a
	// marker written at any point still resolves.
	if err := os.WriteFile(marker, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitReady() = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("WaitReady() never observed the marker")
	}
}

func TestWaitReadyReportsCancellation(t *testing.T) {
	t.Parallel()

	root := sessionRoot(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- WaitReady(ctx, root, testSession) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WaitReady() = %v, want context cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitReady() ignored cancellation")
	}
}

func TestStopIsANoOpWithoutAWatcher(t *testing.T) {
	t.Parallel()

	// No state directory at all.
	if err := Stop(t.TempDir(), testSession); err != nil {
		t.Fatalf("Stop() with no state = %v", err)
	}
	// State but no lock file.
	root := sessionRoot(t)
	if err := Stop(root, testSession); err != nil {
		t.Fatalf("Stop() with no lock = %v", err)
	}
	// A lock file nobody holds.
	lockPath := filepath.Join(statedir.TempSession(root, testSession), lockFilename)
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Stop(root, testSession); err != nil {
		t.Fatalf("Stop() with an unheld lock = %v", err)
	}
}

func TestStopRejectsAnInvalidSessionName(t *testing.T) {
	t.Parallel()

	if err := Stop(t.TempDir(), "../escape"); err == nil {
		t.Fatal("Stop() accepted a traversing session name")
	}
	if err := Stop("", testSession); err == nil {
		t.Fatal("Stop() accepted a blank root")
	}
}

// A held lock with no PID file is reported rather than silently ignored: it
// means a watcher is running that Stop has no way to terminate.
func TestStopReportsAHeldLockWithoutAPID(t *testing.T) {
	t.Parallel()

	root := sessionRoot(t)
	lockPath := filepath.Join(statedir.TempSession(root, testSession), lockFilename)
	owner, err := acquire(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.release()
	err = Stop(root, testSession)
	if err == nil || !strings.Contains(err.Error(), "no recorded PID") {
		t.Fatalf("Stop() = %v", err)
	}
}

// Stop must not signal itself out of existence when a stale PID file names the
// running process.
func TestStopRefusesToTerminateItself(t *testing.T) {
	t.Parallel()

	root := sessionRoot(t)
	statePath := statedir.TempSession(root, testSession)
	lockPath := filepath.Join(statePath, lockFilename)
	owner, err := acquire(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.release()
	if err := writePID(filepath.Join(statePath, pidFilename)); err != nil {
		t.Fatal(err)
	}
	terminated := false
	err = stopWith(root, testSession, func(int) error { terminated = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "refuse to terminate current process") {
		t.Fatalf("stopWith() = %v", err)
	}
	if terminated {
		t.Fatal("stopWith() signalled the current process")
	}
}

func TestReadPIDRejectsMalformedFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for name, contents := range map[string]string{
		"empty":     "",
		"letters":   "not-a-pid\n",
		"negative":  "-1\n",
		"zero":      "0\n",
		"oversized": strings.Repeat("9", maxPIDFileBytes+1),
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readPID(path); err == nil {
			t.Fatalf("readPID(%s) accepted %q", name, contents)
		}
	}
	path := filepath.Join(dir, "valid")
	if err := os.WriteFile(path, []byte("4321\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if pid, err := readPID(path); err != nil || pid != 4321 {
		t.Fatalf("readPID(valid) = %d, %v", pid, err)
	}
}

// A second daemon must exit quietly rather than fight for the singleton.
func TestRunDaemonYieldsToTheRunningSingleton(t *testing.T) {
	t.Parallel()

	root := sessionRoot(t)
	lockPath := filepath.Join(statedir.TempSession(root, testSession), lockFilename)
	owner, err := acquire(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.release()
	err = Run(context.Background(), Options{Root: root, Session: testSession, Herdr: &stubHerdr{}, Daemon: true})
	if err != nil {
		t.Fatalf("Run(daemon) with the lock held = %v", err)
	}
}

func TestRunValidatesItsInputs(t *testing.T) {
	t.Parallel()

	cases := map[string]Options{
		"blank root":     {Session: testSession, Herdr: &stubHerdr{}},
		"bad session":    {Root: t.TempDir(), Session: "../escape", Herdr: &stubHerdr{}},
		"missing client": {Root: t.TempDir(), Session: testSession},
	}
	for name, options := range cases {
		if err := Run(context.Background(), options); err == nil {
			t.Fatalf("Run(%s) returned no error", name)
		}
	}
}

// followLog must end when the dispatcher that owned the log exits, which is
// only observable through the singleton state it removes on the way out.
func TestFollowLogEndsWhenTheSingletonIsReleased(t *testing.T) {
	t.Parallel()

	root := sessionRoot(t)
	logPath := filepath.Join(statedir.Session(root, testSession), LogFilename)
	if err := os.WriteFile(logPath, []byte("first line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath := statedir.TempSession(root, testSession)
	lockPath := filepath.Join(statePath, lockFilename)
	owner, err := acquire(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	pidPath := filepath.Join(statePath, pidFilename)
	if err := writePID(pidPath); err != nil {
		t.Fatal(err)
	}
	output := &syncBuffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- followLog(ctx, root, testSession, output) }()

	// Exit the way the daemon does: drop the PID file, then release the lock.
	if err := os.Remove(pidPath); err != nil {
		t.Fatal(err)
	}
	if err := owner.release(); err != nil {
		t.Fatal(err)
	}
	// Whether followLog armed its watches before or after the release decides
	// which exit path it takes, so keep disturbing the directory until it
	// returns. Only the test needs this; the follower itself is event-driven.
	deadline := time.After(10 * time.Second)
	for nudge := 0; ; nudge++ {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("followLog() = %v", err)
			}
			if !strings.Contains(output.String(), "first line") {
				t.Fatalf("output = %q", output.String())
			}
			return
		case <-deadline:
			t.Fatal("followLog() did not notice the released singleton")
		case <-time.After(20 * time.Millisecond):
			path := filepath.Join(statePath, "nudge")
			if err := os.WriteFile(path, []byte{byte(nudge)}, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// A dispatcher that exits before the follower arms its watches leaves no event
// to observe, so the follower has to notice on its own.
func TestFollowLogExitsWhenTheSingletonIsAlreadyFree(t *testing.T) {
	t.Parallel()

	root := sessionRoot(t)
	logPath := filepath.Join(statedir.Session(root, testSession), LogFilename)
	if err := os.WriteFile(logPath, []byte("only line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := &syncBuffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- followLog(ctx, root, testSession, output) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("followLog() = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("followLog() waited for a dispatcher that was never running")
	}
	if !strings.Contains(output.String(), "only line") {
		t.Fatalf("output = %q", output.String())
	}
}

// stubHerdr satisfies the client contract for the input-validation and
// singleton paths, none of which reach Herdr.
type stubHerdr struct{}

func (stubHerdr) Protocol(context.Context) (int, error) { return 0, errors.New("unused") }
func (stubHerdr) List(context.Context) ([]herdr.Session, error) {
	return nil, errors.New("unused")
}
func (stubHerdr) PromptAgent(context.Context, string, string, string) error {
	return errors.New("unused")
}

// syncBuffer collects follower output from the goroutine that produces it.
type syncBuffer struct {
	mu       sync.Mutex
	contents []byte
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.contents = append(b.contents, p...)
	return len(p), nil
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.contents)
}
