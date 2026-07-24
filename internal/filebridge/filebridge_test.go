package filebridge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/protocol"
)

func TestRequestResponseLifecycle(t *testing.T) {
	root := t.TempDir()
	if err := ResetServer(root, "alpha"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { CloseServer(root, "alpha") })

	id, err := Submit(root, "alpha", protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := Take(root, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != id || pending[0].Request.Op != protocol.OpSpawn {
		t.Fatalf("pending = %+v", pending)
	}
	if err := Respond(root, "alpha", id, protocol.Response{Name: "pi-emperor"}); err != nil {
		t.Fatal(err)
	}
	resp, err := Await(root, "alpha", id, time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Name != "pi-emperor" {
		t.Fatalf("response = %+v", resp)
	}

	if err := CloseServer(root, "alpha"); err != nil {
		t.Fatal(err)
	}
	if Available(root, "alpha") {
		t.Fatal("closed bridge still reports available")
	}
}

// TestRespondDropsOrphanWhenClientGone covers a late response for an exchange
// whose client already gave up: without a sweep it would linger in responses/
// for the daemon's lifetime.
func TestRespondDropsOrphanWhenClientGone(t *testing.T) {
	root := t.TempDir()
	if err := ResetServer(root, "alpha"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { CloseServer(root, "alpha") })

	id, err := Submit(root, "alpha", protocol.Request{Op: protocol.OpList})
	if err != nil {
		t.Fatal(err)
	}
	// Daemon claims the request, creating the accepted marker.
	if _, err := Take(root, "alpha"); err != nil {
		t.Fatal(err)
	}
	// Client gives up before the response arrives: its Await defer Cleanup
	// removes the accepted marker (and any files for this exchange).
	Cleanup(root, "alpha", id)
	// Daemon responds late; no client will ever read or clean this up.
	if err := Respond(root, "alpha", id, protocol.Response{Name: "late"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(responseDir(root, "alpha"), id+".json")); !os.IsNotExist(err) {
		t.Fatalf("orphan response left behind: stat err=%v", err)
	}
}

// TestTakeRejectsNonPositivePID covers the upgrade window where a pre-pid
// client binary publishes a request that unmarshals to PID 0. Such a waiter
// would be unprobeable, so Take must discard it rather than park it.
func TestTakeRejectsNonPositivePID(t *testing.T) {
	root := t.TempDir()
	if err := ResetServer(root, "alpha"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { CloseServer(root, "alpha") })

	id, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	// Omitting PID unmarshals to 0, exactly like an old client's payload.
	data, err := json.Marshal(pending{ID: id, Request: protocol.Request{Op: protocol.OpWait, As: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inboxDir(root, "alpha"), id+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Take(root, "alpha")
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Take accepted a PID<=0 request: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(acceptedDir(root, "alpha"), id)); !os.IsNotExist(err) {
		t.Fatalf("accepted marker created for stale request: stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(inboxDir(root, "alpha"), id+".json")); !os.IsNotExist(err) {
		t.Fatalf("stale request not discarded: stat err=%v", err)
	}
}

// TestTakeRejectsUnsafeIDs feeds Take crafted inbox files whose embedded id is
// unsafe (path traversal, separators, non-hex). The guard must discard them,
// never returning them and never letting an accepted marker escape the tree.
func TestTakeRejectsUnsafeIDs(t *testing.T) {
	cases := []struct {
		name string
		id   string
	}{
		{"traversal", "../../../pwned"},
		{"separator", "sub/dir"},
		{"dotdot", ".."},
		{"nonhex", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
		{"short", "abcd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := ResetServer(root, "alpha"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { CloseServer(root, "alpha") })

			// Submit only ever mints valid ids, so craft the inbox file directly.
			data, err := json.Marshal(pending{ID: tc.id, Request: protocol.Request{Op: protocol.OpList}})
			if err != nil {
				t.Fatal(err)
			}
			craft := filepath.Join(inboxDir(root, "alpha"), "craft.json")
			if err := os.WriteFile(craft, data, 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := Take(root, "alpha")
			if err != nil {
				t.Fatalf("Take: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("Take accepted unsafe id %q: %+v", tc.id, got)
			}
			if _, err := os.Stat(craft); !os.IsNotExist(err) {
				t.Fatalf("unsafe request not discarded: stat err=%v", err)
			}
			// No accepted marker may have escaped the transport tree.
			assertNoStrayFile(t, root, "pwned")
		})
	}
}

// assertNoStrayFile fails if any file named name exists anywhere under root,
// which would indicate a path-traversal write escaped the .rpc tree.
func assertNoStrayFile(t *testing.T, root, name string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == name {
			t.Fatalf("stray file created by traversal: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestAwaitAbortsWhenDaemonStops covers the Available-goes-false abort in
// waitFile: a wait that will never be answered must return once the daemon's
// alive marker disappears instead of blocking forever.
func TestAwaitAbortsWhenDaemonStops(t *testing.T) {
	root := t.TempDir()
	if err := ResetServer(root, "alpha"); err != nil {
		t.Fatal(err)
	}
	id, err := Submit(root, "alpha", protocol.Request{Op: protocol.OpList})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		CloseServer(root, "alpha")
	}()
	// No accepted marker will ever appear; the abort must come from Available.
	_, err = Await(root, "alpha", id, time.Second, time.Second)
	if err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("err = %v, want daemon-stopped abort", err)
	}
}

// TestConcurrentSubmitWithLiveTake runs many concurrent Submits against a
// single Take drainer (mirroring the daemon's one serveFileRequests loop) and
// checks every request is claimed exactly once. Run under -race it guards the
// atomic publish/claim handoff.
func TestConcurrentSubmitWithLiveTake(t *testing.T) {
	root := t.TempDir()
	if err := ResetServer(root, "alpha"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { CloseServer(root, "alpha") })

	const n = 50
	ids := make(chan string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			id, err := Submit(root, "alpha", protocol.Request{Op: protocol.OpSend, Body: fmt.Sprintf("m%d", i)})
			if err != nil {
				t.Errorf("submit %d: %v", i, err)
				return
			}
			ids <- id
		}(i)
	}
	go func() { wg.Wait(); close(ids) }()

	taken := make(map[string]bool)
	deadline := time.Now().Add(5 * time.Second)
	for len(taken) < n {
		got, err := Take(root, "alpha")
		if err != nil {
			t.Fatalf("take: %v", err)
		}
		for _, p := range got {
			if taken[p.ID] {
				t.Fatalf("duplicate take of %s", p.ID)
			}
			taken[p.ID] = true
		}
		if time.Now().After(deadline) {
			t.Fatalf("took %d of %d before deadline", len(taken), n)
		}
		if len(got) == 0 {
			time.Sleep(time.Millisecond)
		}
	}

	submitted := make(map[string]bool)
	for id := range ids {
		submitted[id] = true
	}
	if len(submitted) != n {
		t.Fatalf("submitted %d distinct ids, want %d", len(submitted), n)
	}
	for id := range submitted {
		if !taken[id] {
			t.Fatalf("submitted id %s was never taken", id)
		}
	}
}
