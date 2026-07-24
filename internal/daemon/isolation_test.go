package daemon_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/client"
	"github.com/Harrison-Blair/fledge/internal/protocol"
)

// Two flocks in one workspace keep separate species pools: each hands out the
// first species of a type, because neither can see the other's roster.
func TestFlocksHaveSeparateSpeciesPools(t *testing.T) {
	root := workspace(t)
	stopA := start(t, root, "alpha")
	defer stopA()
	stopB := start(t, root, "bravo")
	defer stopB()

	nameA := register(t, root, "alpha", "worker")
	nameB := register(t, root, "bravo", "worker")

	if nameA != nameB {
		t.Fatalf("flocks assigned %q and %q; each flock's pool should start over", nameA, nameB)
	}
}

// A message sent inside one flock is never visible in another.
func TestMessagesDoNotCrossFlocks(t *testing.T) {
	root := workspace(t)
	stopA := start(t, root, "alpha")
	defer stopA()
	stopB := start(t, root, "bravo")
	defer stopB()

	// The same agent name exists in both flocks, so a leak would land.
	nameA := register(t, root, "alpha", "worker")
	nameB := register(t, root, "bravo", "worker")
	opsA := register(t, root, "alpha", "ops")
	if nameA != nameB {
		t.Fatalf("precondition: names differ (%q, %q)", nameA, nameB)
	}

	if _, err := client.Do(root, "alpha", protocol.Request{
		Op: protocol.OpSend, From: opsA, To: nameA, Body: "for alpha only",
	}); err != nil {
		t.Fatalf("send in alpha: %v", err)
	}

	// bravo's waiter must time out: the message belongs to alpha.
	_, err := client.Do(root, "bravo", protocol.Request{
		Op: protocol.OpWait, As: nameB, TimeoutMS: 200,
	})
	if err == nil {
		t.Fatal("bravo received a message sent in alpha")
	}

	// And alpha must still hold it, proving the timeout was isolation and not
	// the message having been lost.
	resp, err := client.Do(root, "alpha", protocol.Request{
		Op: protocol.OpWait, As: nameA, TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatalf("wait in alpha: %v", err)
	}
	if resp.Message == nil || resp.Message.Body != "for alpha only" {
		t.Fatalf("alpha got %+v, want its own message", resp.Message)
	}
}

// One flock's daemon going away leaves the others serving.
func TestFlockDaemonDeathDoesNotDisturbOthers(t *testing.T) {
	root := workspace(t)
	stopA := start(t, root, "alpha")
	stopB := start(t, root, "bravo")
	defer stopB()

	nameB := register(t, root, "bravo", "worker")

	stopA()

	if client.Running(root, "alpha") {
		t.Fatal("alpha still listening after its daemon stopped")
	}
	if !client.Running(root, "bravo") {
		t.Fatal("bravo stopped serving when alpha's daemon went away")
	}

	// bravo is not merely listening; it still works and kept its roster.
	resp, err := client.Do(root, "bravo", protocol.Request{Op: protocol.OpList})
	if err != nil {
		t.Fatalf("list in bravo: %v", err)
	}
	if len(resp.Agents) != 1 || resp.Agents[0].Name != nameB {
		t.Fatalf("bravo roster is %+v, want just %s", resp.Agents, nameB)
	}
}

// A flock with no daemon is a hard error even while another flock is up, so a
// missing flock can never silently fall through to a running one.
func TestUnstartedFlockIsHardError(t *testing.T) {
	root := workspace(t)
	stop := start(t, root, "alpha")
	defer stop()

	_, err := client.Do(root, "bravo", protocol.Request{Op: protocol.OpList})
	if !errors.Is(err, client.ErrNotRunning) {
		t.Fatalf("got %v, want %v", err, client.ErrNotRunning)
	}
}

// Each flock replays only its own journal across a restart.
func TestRestartReplaysOnlyItsOwnFlock(t *testing.T) {
	root := workspace(t)
	stopA := start(t, root, "alpha")
	stopB := start(t, root, "bravo")
	defer stopB()

	// bravo registers a different type on purpose: both flocks hand out the
	// same first species per type, so same-type agents would share a name and
	// a leaked registration would silently collapse on replay instead of
	// showing up as an extra agent.
	register(t, root, "alpha", "worker")
	register(t, root, "alpha", "worker")
	register(t, root, "bravo", "reviewer")

	stopA()
	time.Sleep(10 * time.Millisecond)
	stopA2 := start(t, root, "alpha")
	defer stopA2()

	resp, err := client.Do(root, "alpha", protocol.Request{Op: protocol.OpList})
	if err != nil {
		t.Fatalf("list in alpha: %v", err)
	}
	if len(resp.Agents) != 2 {
		t.Fatalf("alpha replayed %d agents, want its own 2", len(resp.Agents))
	}
	for _, a := range resp.Agents {
		if a.Type != "worker" {
			t.Fatalf("alpha replayed a %s agent from another flock's journal", a.Type)
		}
	}
}
