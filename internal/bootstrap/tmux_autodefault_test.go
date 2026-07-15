package bootstrap

import (
	"strings"
	"testing"
)

// TestTmuxPreconditionAutoResolves: team-loop.md's tmux section no longer
// gates on a confirm-gate — it auto-resolves (panes if tmux present, degraded
// in-process teammates if not) and reports which path was taken as one line
// of non-blocking run narration.
func TestTmuxPreconditionAutoResolves(t *testing.T) {
	data, err := FS.ReadFile("adapters/claude/team-loop.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	for _, old := range []string{
		"surfaces this via a `confirm-gate`",
		"stop and restart inside tmux (recommended)",
	} {
		if strings.Contains(body, old) {
			t.Errorf("team-loop.md still contains old gating language %q", old)
		}
	}

	for _, want := range []string{
		"no `confirm-gate`",
		"tmux detected — spawning teammates in panes",
		"tmux not detected — proceeding degraded, in-process teammates",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("team-loop.md missing expected auto-resolve wording %q", want)
		}
	}
}

// TestImplementationPreconditionCarveOut: implementation.md §1's harness
// piping preconditions bullet no longer states a blanket "never silently
// proceed" — it must carve out auto-resolving preconditions while still
// applying to gated ones (e.g. permission-mode).
func TestImplementationPreconditionCarveOut(t *testing.T) {
	data, err := FS.ReadFile("core/skills/fledge-orchestrate/implementation.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	old := "If a precondition is unmet, your piping file states the fallback (commonly: proceed degraded with in-process teammates, or stop and restart). Never silently proceed past a precondition your piping file says to surface."
	if strings.Contains(body, old) {
		t.Errorf("implementation.md still contains the old unqualified sentence")
	}

	if !strings.Contains(body, "auto-resolve") {
		t.Error("implementation.md missing wording that some preconditions auto-resolve without a gate")
	}
	if !strings.Contains(body, "never silently proceed") {
		t.Error("implementation.md missing scoped 'never silently proceed' instruction for gated preconditions")
	}
}

// TestPermissionModeUnchanged: guards FC-5 — the permission-mode paragraph
// and its confirm-gate wording in team-loop.md's "Spawning and addressing
// teammates" section must survive this feather byte-for-byte.
func TestPermissionModeUnchanged(t *testing.T) {
	data, err := FS.ReadFile("adapters/claude/team-loop.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	for _, want := range []string{
		"Teammates inherit your permission mode at spawn",
		"`implementation.md` §1 surfaces the current mode via a `confirm-gate`",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("team-loop.md missing permission-mode wording %q", want)
		}
	}
}
