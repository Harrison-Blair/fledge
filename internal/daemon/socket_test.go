package daemon_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/client"
	"github.com/Harrison-Blair/fledge/internal/daemon"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

// Two daemons for the same flock must never both come up: the stale-socket
// reclaim (probe, unlink, bind) has to be serialized, or a losing interleaving
// unlinks the winner's live socket and forks the journal and state authority.
// A burst of concurrent New calls widens the interleaving window enough to trip
// the non-atomic version reliably.
func TestConcurrentNewElectsOneWinner(t *testing.T) {
	root := workspace(t)

	const n = 12
	for trial := 0; trial < 30; trial++ {
		var wg sync.WaitGroup
		daemons := make([]*daemon.Daemon, n)
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func(i int) {
				defer wg.Done()
				if d, err := daemon.New(root, testFlock); err == nil {
					daemons[i] = d
				}
			}(i)
		}
		wg.Wait()

		winners := 0
		for _, d := range daemons {
			if d != nil {
				winners++
				d.Close()
			}
		}
		if winners != 1 {
			t.Fatalf("trial %d: %d daemons came up for one flock, want exactly 1", trial, winners)
		}
	}
}

// The flock directory can hold a readiness digest whose file path is
// bearer-equivalent, so the directory and journal must not be group/other
// readable. An older fledge left them at 0o755/0o644; starting must tighten
// them, not just create fresh ones correctly.
func TestFlockDirAndJournalPermissions(t *testing.T) {
	root := workspace(t)

	dir := filepath.Join(root, ".fledge", "flocks", testFlock)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	jp := filepath.Join(dir, "journal.jsonl")
	if err := os.WriteFile(jp, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(jp, 0o644); err != nil {
		t.Fatal(err)
	}

	defer start(t, root, testFlock)()

	di, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := di.Mode().Perm(); got != 0o700 {
		t.Fatalf("flock dir mode = %o, want 700", got)
	}

	ji, err := os.Stat(jp)
	if err != nil {
		t.Fatal(err)
	}
	if got := ji.Mode().Perm(); got != 0o600 {
		t.Fatalf("journal mode = %o, want 600", got)
	}
}

// Sockets live outside the workspace, so the workspace's identity is what
// keeps two checkouts running the same flock name apart. This is what replaces
// the uniqueness the in-repo path used to give for free.
func TestSocketPathsDifferPerWorkspace(t *testing.T) {
	a := daemon.SocketPath(t.TempDir(), "flock1")
	b := daemon.SocketPath(t.TempDir(), "flock1")

	if a == b {
		t.Fatalf("two workspaces share socket %s; flocks of the same name would collide", a)
	}
}

// The same workspace must resolve to the same socket however it is spelled,
// or a client invoked with a relative root could not reach its own daemon.
func TestSocketPathIsStableAcrossEquivalentRoots(t *testing.T) {
	root := t.TempDir()

	direct := daemon.SocketPath(root, "flock1")
	indirect := daemon.SocketPath(filepath.Join(root, "sub", ".."), "flock1")

	if direct != indirect {
		t.Fatalf("same workspace resolved to %s and %s", direct, indirect)
	}
}

// deepWorkspace scaffolds a workspace whose absolute path is over 100
// characters — long enough that the old in-workspace socket path could not be
// bound at all.
func deepWorkspace(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	for len(root) <= 100 {
		root = filepath.Join(root, "nested-directory-level")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	return root
}

// The case the move exists to fix: a workspace too deep for a unix socket path
// still runs a daemon end to end.
func TestDeepWorkspaceRunsDaemon(t *testing.T) {
	root := deepWorkspace(t)
	if len(root) <= 100 {
		t.Fatalf("precondition: workspace %s is only %d characters", root, len(root))
	}
	defer start(t, root, testFlock)()

	name := register(t, root, testFlock, "engineer")
	ops := register(t, root, testFlock, "ops")

	sent, err := client.Do(root, testFlock, protocol.Request{
		Op: protocol.OpSend, From: ops, To: name, Body: "deep workspace",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	got, err := client.Do(root, testFlock, protocol.Request{
		Op: protocol.OpWait, As: name, TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if got.Message == nil || got.Message.ID != sent.ID {
		t.Fatalf("delivered %+v, want message %s", got.Message, sent.ID)
	}
}

// The worst legal case — the longest flock name in a very deep workspace —
// still fits darwin's sun_path, the stricter of the two platforms.
func TestWorstCaseSocketPathFitsDarwinLimit(t *testing.T) {
	// 104 bytes of sun_path, one of which is the terminating NUL.
	const darwinMax = 103

	root := deepWorkspace(t)
	name := strings.Repeat("a", flock.MaxName)

	sock := daemon.SocketPath(root, name)
	if len(sock) > darwinMax {
		t.Fatalf("socket path %s is %d characters, over the darwin limit of %d", sock, len(sock), darwinMax)
	}
	if err := daemon.CheckSocketPath(root, name); err != nil {
		t.Fatalf("CheckSocketPath rejected the worst legal case: %v", err)
	}
}
