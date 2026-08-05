package watch

import (
	"context"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

// TestCycleInvokesRefreshWithSnapshot proves the injected Refresh hook is handed
// the fresh snapshot the poll cycle read, so lifecycle transitions rebuild the
// context report.
func TestCycleInvokesRefreshWithSnapshot(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	var got []herdr.Snapshot
	h.engine.Refresh = func(_ context.Context, snapshot herdr.Snapshot) {
		got = append(got, snapshot)
	}
	h.snapshot(worker("builder", "p1", "working"))

	h.runCycle()

	if len(got) != 1 {
		t.Fatalf("Refresh called %d times, want 1", len(got))
	}
	names := map[string]bool{}
	for _, agent := range got[0].Agents {
		if agent.Name != nil {
			names[*agent.Name] = true
		}
	}
	if !names["orchestrator"] || !names["builder"] {
		t.Errorf("refresh snapshot agents = %v, want orchestrator and builder", names)
	}
}

// TestCycleWithoutRefreshHookIsSafe proves the hook is optional: a nil Refresh
// never panics the cycle.
func TestCycleWithoutRefreshHookIsSafe(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.engine.Refresh = nil
	h.snapshot(worker("builder", "p1", "working"))

	h.runCycle() // must not panic
}
