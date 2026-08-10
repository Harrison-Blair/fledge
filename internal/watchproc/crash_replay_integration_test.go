package watchproc

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
)

// This file drives the crash-replay seam across real OS processes: a daemon
// dispatcher (a re-exec of this test binary) acquires the singleton flock, parks
// mid-drain, and is SIGKILLed while still holding the lock; a second foreground
// dispatcher must then acquire the freed lock and drive the uncertain wake to
// delivered exactly once. The only fakes are the dispatcher's WatchFile and
// Subscribe seams and a trivial in-process Herdr — the crash-replay mechanics
// under test (unix flock ownership released on death, ledger drain of an
// uncertain wake) are exercised for real.

const (
	crashChildEnv    = "FLEDGE_CRASH_CHILD"    // "block" (daemon) or "deliver" (foreground)
	crashRootEnv     = "FLEDGE_CRASH_ROOT"     // project root the child dispatches for
	crashSessionEnv  = "FLEDGE_CRASH_SESSION"  // Herdr session name
	crashSentinelEnv = "FLEDGE_CRASH_SENTINEL" // file the daemon child touches once it parks
)

// TestMain lets this test binary re-exec itself as a dispatcher subprocess. When
// the crash-child env is set the process becomes a dispatcher and never runs the
// test suite; otherwise it runs the tests normally.
func TestMain(m *testing.M) {
	if mode := os.Getenv(crashChildEnv); mode != "" {
		os.Exit(runCrashDispatcherChild(mode))
	}
	os.Exit(m.Run())
}

// crashChildHerdr is the in-process Herdr a re-exec'd dispatcher child talks to.
// No real Herdr session is needed: the seam under test is the singleton flock and
// the ledger's uncertain-wake replay, not Herdr I/O. Only PromptAgent carries
// behaviour, chosen by the child's mode.
type crashChildHerdr struct {
	deliver func(recipient string) error
}

func (crashChildHerdr) Protocol(context.Context) (int, error)         { return RequiredHerdrProtocol, nil }
func (crashChildHerdr) List(context.Context) ([]herdr.Session, error) { return nil, nil }
func (crashChildHerdr) Snapshot(context.Context, string) (herdr.Snapshot, error) {
	return herdr.Snapshot{}, nil
}
func (c crashChildHerdr) PromptAgent(_ context.Context, _, recipient, _ string) error {
	return c.deliver(recipient)
}

// runCrashDispatcherChild runs the dispatcher inside a re-exec'd child process and
// returns the process exit code. Both modes install fake WatchFile and Subscribe
// seams so runDispatcher never reaches its native fswatch or Herdr socket; the
// only real seams exercised are the singleton flock Run acquires and the ledger
// drain that replays the uncertain wake.
func runCrashDispatcherChild(mode string) int {
	root := os.Getenv(crashRootEnv)
	session := os.Getenv(crashSessionEnv)

	// A file watcher that never fires and a subscription that only unblocks on
	// teardown keep the dispatcher parked on its event loop instead of touching
	// the OS. The daemon child never even reaches the loop — it parks in drain.
	idleFiles := &fakeFiles{events: make(chan struct{}), errs: make(chan error)}
	idleSubscribe := func(streamCtx context.Context, _ []string, _ func(), _ func(herdr.Event)) error {
		<-streamCtx.Done()
		return streamCtx.Err()
	}

	var deliver func(string) error
	daemon := false
	switch mode {
	case "block":
		// The daemon records its wake attempt in drain, then parks inside delivery
		// so the parent can SIGKILL it while it still holds the singleton lock. The
		// sentinel signals that delivery has been entered — by then Run has acquired
		// the lock, written the PID file, and drain has committed the wake attempt.
		daemon = true
		sentinel := os.Getenv(crashSentinelEnv)
		deliver = func(string) error {
			_ = os.WriteFile(sentinel, []byte("delivering\n"), 0o600)
			// Park until SIGKILL. Daemon mode installed a signal handler, so the
			// runtime keeps this process alive rather than declaring a deadlock.
			select {}
		}
	case "deliver":
		deliver = func(string) error { return nil }
	default:
		return 2
	}

	err := Run(context.Background(), Options{
		Root: root, Session: session,
		Herdr:     crashChildHerdr{deliver: deliver},
		WatchFile: func(string) (FileWatcher, error) { return idleFiles, nil },
		Subscribe: idleSubscribe,
		Daemon:    daemon,
	})
	if err != nil {
		return 1
	}
	return 0
}

// crashChild owns one re-exec'd dispatcher subprocess.
type crashChild struct {
	mode   string
	cmd    *exec.Cmd
	output *bytes.Buffer
	killed bool
}

func newCrashChild(t *testing.T, mode, root, session, sentinel string) *crashChild {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(),
		crashChildEnv+"="+mode,
		crashRootEnv+"="+root,
		crashSessionEnv+"="+session,
		crashSentinelEnv+"="+sentinel,
	)
	output := &bytes.Buffer{}
	cmd.Stdout = output
	cmd.Stderr = output
	return &crashChild{mode: mode, cmd: cmd, output: output}
}

func (c *crashChild) start(t *testing.T) {
	t.Helper()
	if err := c.cmd.Start(); err != nil {
		t.Fatalf("start %s dispatcher child: %v", c.mode, err)
	}
	t.Cleanup(func() { c.kill() })
}

// kill SIGKILLs the child and reaps it. Reaping matters for the seam under test:
// the kernel releases the child's advisory flock only when the process is fully
// gone, and Wait returning guarantees that. Safe to call more than once.
func (c *crashChild) kill() {
	if c.cmd.Process == nil || c.killed {
		return
	}
	c.killed = true
	_ = c.cmd.Process.Kill()
	_ = c.cmd.Wait()
}

// TestCrashedDaemonMidDrainReplaysUncertainWakeExactlyOnce is the cross-process
// crash-replay integration test. It is intentionally serial: it spawns real
// processes and SIGKILLs them, which should not interleave with parallel tests.
func TestCrashedDaemonMidDrainReplaysUncertainWakeExactlyOnce(t *testing.T) {
	root := t.TempDir()
	const session = "fledge-test-1234abcd"

	// Seed the ledger with exactly one pending wake: a message for a live worker.
	store := messaging.New(root, session)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{
		Name: "worker", PaneID: "p1", Harness: "codex", Caller: messaging.UserIdentity}); err != nil {
		t.Fatal(err)
	}
	message, err := store.Create(messaging.CreateParams{
		Sender: messaging.UserIdentity, Recipient: "worker", RecipientPane: "p1", Body: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if pending, err := store.PendingWakes(); err != nil {
		t.Fatal(err)
	} else if len(pending) != 1 {
		t.Fatalf("seeded %d pending wakes, want exactly 1", len(pending))
	}

	lockPath := filepath.Join(fsutil.TempSession(root, session), lockFilename)

	// --- Daemon: acquires the singleton, parks mid-drain, then is SIGKILLed while
	// still holding the lock. ---
	sentinel := filepath.Join(t.TempDir(), "delivering")
	daemon := newCrashChild(t, "block", root, session, sentinel)
	daemon.start(t)

	// Deterministic barrier: poll for the sentinel the daemon writes from inside
	// delivery, under a bounded deadline. No fixed sleep gates correctness.
	awaitFile(t, daemon, sentinel, 30*time.Second)

	// The singleton is genuinely held right now, and the wake is uncertain: its
	// attempt is committed but no outcome exists, so it is still pending replay.
	if held, err := singletonHeld(lockPath); err != nil || !held {
		t.Fatalf("singleton not held while daemon parks mid-drain: held=%v err=%v\n%s", held, err, daemon.output)
	}
	if pending, err := store.PendingWakes(); err != nil {
		t.Fatal(err)
	} else if len(pending) != 1 || pending[0].Status != messaging.StatusUncertain {
		t.Fatalf("wake state after daemon parks = %#v, want one uncertain wake", pending)
	}

	// SIGKILL the daemon; the kernel drops its advisory flock once it is reaped.
	daemon.kill()
	awaitUnlocked(t, lockPath, 30*time.Second)

	// --- Foreground: must ACQUIRE the freed lock and drive the uncertain wake to
	// delivered exactly once. ---
	foreground := newCrashChild(t, "deliver", root, session, "")
	foreground.start(t)

	// The foreground dispatcher drains at startup; poll the ledger until the seeded
	// message is delivered, bounded so a stuck child fails loudly.
	awaitDelivered(t, foreground, store, message.ID, 30*time.Second)

	if pending, err := store.PendingWakes(); err != nil {
		t.Fatal(err)
	} else if len(pending) != 0 {
		t.Fatalf("pending wakes after replay = %#v, want none", pending)
	}

	// Exactly once: the ledger holds one wake_attempt (the daemon's; the replay is
	// idempotent by design) and one accepted wake_outcome (the foreground child's).
	attempts, outcomes, accepted := countWakeEvents(t, store.LogPath())
	if attempts != 1 {
		t.Fatalf("wake_attempt events = %d, want exactly 1 (replay must not re-attempt)", attempts)
	}
	if outcomes != 1 || !accepted {
		t.Fatalf("wake_outcome events = %d (accepted=%v), want exactly 1 accepted", outcomes, accepted)
	}

	// Foreground child owns the lock now; stop it (its Cleanup also guards this).
	foreground.kill()
}

// awaitFile polls for path under a bounded deadline. The deadline plus the file
// condition are the correctness mechanism; the sleep is only a poll interval.
func awaitFile(t *testing.T, child *crashChild, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s child never signalled %q within %s\n%s", child.mode, path, timeout, child.output)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// awaitUnlocked polls until the singleton lock can be acquired, proving the
// killed daemon released it. Bounded deadline gates correctness.
func awaitUnlocked(t *testing.T, lockPath string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		held, err := singletonHeld(lockPath)
		if err != nil {
			t.Fatalf("inspect singleton lock: %v", err)
		}
		if !held {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("killed daemon never released the singleton lock within %s", timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// awaitDelivered polls the ledger until the message reaches delivered, under a
// bounded deadline. The store re-reads the log on every call, so one instance
// observes the foreground child's cross-process writes.
func awaitDelivered(t *testing.T, child *crashChild, store *messaging.Store, messageID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		got, err := store.Get(messageID)
		if err != nil {
			t.Fatalf("read seeded message: %v", err)
		}
		if got.Status == messaging.StatusDelivered {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("message status = %s after %s, want delivered\n%s", got.Status, timeout, child.output)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// countWakeEvents tallies the durable wake_attempt and wake_outcome events in the
// ledger and reports whether the single outcome was accepted.
func countWakeEvents(t *testing.T, logPath string) (attempts, outcomes int, accepted bool) {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.SplitAfter(data, []byte{'\n'}) {
		entry, ok := messaging.DecodeLedgerLine(line)
		if !ok {
			continue
		}
		switch entry.Type {
		case "wake_attempt":
			attempts++
		case "wake_outcome":
			outcomes++
			if entry.Accepted != nil && *entry.Accepted {
				accepted = true
			}
		}
	}
	return attempts, outcomes, accepted
}
