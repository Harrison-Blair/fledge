package bootstrap

import (
	"strings"
	"testing"
)

// TestTeamLoopDocDescribesCompactAdvisory: team-loop.md documents the
// `/compact`-is-safe-now advisory tied to digest completion — covering all
// three phase digests — explicitly framed as user-facing guidance with no
// automated trigger.
func TestTeamLoopDocDescribesCompactAdvisory(t *testing.T) {
	data, err := FS.ReadFile("adapters/claude/team-loop.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	for _, want := range []string{
		"`/compact` is safe to run now",
		"digest-planning.md",
		"digest-foraging.md",
		"digest-implementation.md",
		"user-facing guidance only",
		"no automated trigger",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("team-loop.md missing compact-advisory wording %q", want)
		}
	}
}
