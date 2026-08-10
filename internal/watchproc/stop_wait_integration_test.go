package watchproc

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
)

// Environment keys the parent test uses to hand the re-executed helper the
// session it must contend for. The helper only runs its body when the gate is
// set, so a normal `go test` treats TestStopWaitHelperProcess as a no-op pass.
const (
	stopWaitHelperGate = "FLEDGE_STOP_WAIT_HELPER"
	stopWaitHelperRoot = "FLEDGE_STOP_WAIT_ROOT"
	stopWaitHelperSess = "FLEDGE_STOP_WAIT_SESSION"
)

// waitForFileToAppear polls for a path under a bounded deadline. The stat and
// the deadline are the correctness mechanism; the sleep is only a poll cadence,
// never the thing being asserted.
func waitForFileToAppear(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("file %q never appeared within %s", path, timeout)
}

// TestStopWaitHelperProcess is the child entry point that the terminate path
// integration test re-executes. Without the gate variable it returns
// immediately so it costs nothing during an ordinary run. With the gate set it
// becomes a genuine second process that holds the singleton flock across an
// OS boundary, which is what makes acquire in the parent observe
// errAlreadyRunning and drive Stop down the terminate branch.
func TestStopWaitHelperProcess(t *testing.T) {
	if os.Getenv(stopWaitHelperGate) != "1" {
		return
	}
	root := os.Getenv(stopWaitHelperRoot)
	session := os.Getenv(stopWaitHelperSess)
	statePath := fsutil.TempSession(root, session)
	lockPath := filepath.Join(statePath, lockFilename)
	pidPath := filepath.Join(statePath, pidFilename)
	readyPath := filepath.Join(statePath, "child.ready")

	owner, err := acquire(lockPath)
	if err != nil {
		os.Exit(11)
	}
	// writePID records this child's PID, which differs from the parent's, so
	// stopAttempt neither refuses to terminate itself nor treats the lock as
	// held-without-a-PID.
	if err := writePID(pidPath); err != nil {
		os.Exit(12)
	}

	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)

	// Announce readiness only after the flock is held and the PID is recorded,
	// so the parent never races Stop ahead of a contended lock.
	if err := os.WriteFile(readyPath, []byte("ready\n"), 0o600); err != nil {
		os.Exit(13)
	}

	<-term
	// Release the advisory lock first, then remove the PID file. Because the
	// removal always follows the release, any directory event the waiter
	// observes after arming its watch is guaranteed to accompany a lock that is
	// already free, so waitForRelease resolves rather than hanging.
	_ = owner.release()
	_ = os.Remove(pidPath)
	os.Exit(0)
}

// TestStopTerminatesAndWaitsForRelease drives the full terminate-and-wait path
// of Stop against a real second process: a re-executed helper holds the
// singleton flock and records its PID, Stop signals it through the production
// terminateProcess seam, and waitForRelease blocks on real fswatch directory
// notifications until the helper drops the lock and exits.
func TestStopTerminatesAndWaitsForRelease(t *testing.T) {
	root := sessionRoot(t)
	statePath := fsutil.TempSession(root, testSession)
	readyPath := filepath.Join(statePath, "child.ready")

	cmd := exec.Command(os.Args[0], "-test.run=^TestStopWaitHelperProcess$", "-test.timeout=90s")
	cmd.Env = append(os.Environ(),
		stopWaitHelperGate+"=1",
		stopWaitHelperRoot+"="+root,
		stopWaitHelperSess+"="+testSession,
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper process: %v", err)
	}

	// A single Wait, performed on a goroutine, reaps the child exactly once.
	// Cleanup only needs to guarantee the child dies if the test bails early;
	// the buffered channel keeps the goroutine from leaking either way.
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	// The helper writes its readiness marker only once it holds the flock, so
	// this is what guarantees Stop meets a genuinely contended singleton.
	waitForFileToAppear(t, readyPath, 20*time.Second)

	if err := Stop(root, testSession); err != nil {
		t.Fatalf("Stop() over a live watcher = %v; helper output: %s", err, output.String())
	}

	select {
	case err := <-waitErr:
		if err != nil {
			t.Fatalf("helper process exited with %v; output: %s", err, output.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("helper process did not exit after Stop released it; output: %s", output.String())
	}
}

// TestStopWithInjectedTerminateReleasesTheLock exercises stopAttempt's
// terminate branch through the stopWith seam without a second process: the
// lock is held on a separate descriptor (so acquire reports errAlreadyRunning),
// a foreign PID is recorded, and the injected terminate stands in for the kill
// by dropping the lock. stopWith must invoke terminate and return nil.
func TestStopWithInjectedTerminateReleasesTheLock(t *testing.T) {
	t.Parallel()

	root := sessionRoot(t)
	statePath := fsutil.TempSession(root, testSession)
	lockPath := filepath.Join(statePath, lockFilename)
	pidPath := filepath.Join(statePath, pidFilename)

	owner, err := acquire(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	t.Cleanup(func() {
		if !released {
			_ = owner.release()
		}
	})

	// A PID that is valid and distinct from this process, so stopAttempt clears
	// both the self-termination guard and the held-without-PID guard.
	foreignPID := os.Getpid() + 1
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(foreignPID)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	terminated := 0
	terminate := func(pid int) error {
		terminated++
		if pid != foreignPID {
			t.Errorf("terminate(%d), want %d", pid, foreignPID)
		}
		released = true
		return owner.release()
	}

	if err := stopWith(root, testSession, terminate); err != nil {
		t.Fatalf("stopWith() = %v", err)
	}
	if terminated != 1 {
		t.Fatalf("terminate called %d times, want 1", terminated)
	}
	if held, err := singletonHeld(lockPath); err != nil || held {
		t.Fatalf("singletonHeld() = %v, %v; want false, nil after release", held, err)
	}
}

// TestWaitForReleaseResolvesWhenTheLockDrops covers waitForRelease's success
// path: a held lock keeps it blocking, and once the lock is released the
// directory notifications drive it to observe the release and return nil.
func TestWaitForReleaseResolvesWhenTheLockDrops(t *testing.T) {
	t.Parallel()

	root := sessionRoot(t)
	statePath := fsutil.TempSession(root, testSession)
	lockPath := filepath.Join(statePath, lockFilename)

	owner, err := acquire(lockPath)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- waitForRelease(lockPath, 15*time.Second) }()

	// Release the lock, then keep nudging the watched directory with real
	// filesystem events under a bounded deadline. Whichever iteration of
	// waitForRelease runs next observes the freed lock; the nudges guarantee at
	// least one event reaches the watch after it is armed.
	if err := owner.release(); err != nil {
		t.Fatal(err)
	}
	nudge := filepath.Join(statePath, "nudge")
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(12 * time.Second)
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("waitForRelease() = %v", err)
			}
			return
		case <-deadline:
			t.Fatal("waitForRelease() never observed the release")
		case <-ticker.C:
			_ = os.WriteFile(nudge, []byte("x"), 0o600)
			_ = os.Remove(nudge)
		}
	}
}

// TestWaitForReleaseTimesOutWhenTheLockIsHeld covers waitForRelease's timeout
// branch: a lock that is never released must surface the bounded "did not
// release" error rather than blocking forever.
func TestWaitForReleaseTimesOutWhenTheLockIsHeld(t *testing.T) {
	t.Parallel()

	root := sessionRoot(t)
	statePath := fsutil.TempSession(root, testSession)
	lockPath := filepath.Join(statePath, lockFilename)

	owner, err := acquire(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.release() })

	err = waitForRelease(lockPath, 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not release") {
		t.Fatalf("waitForRelease() = %v, want a 'did not release' timeout error", err)
	}
}
