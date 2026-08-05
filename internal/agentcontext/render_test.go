package agentcontext

import (
	"strings"
	"testing"
)

func TestRenderShowsUsageAndReasons(t *testing.T) {
	t.Parallel()
	out := Render(sampleReport())
	for _, want := range []string{
		"orchestrator (claude): 1000/200000 tokens (0.50%)",
		"worker (pi): unknown (" + ReasonAwaitingFirstResponse + ")",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Render output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderKnownUsedWithoutWindow(t *testing.T) {
	t.Parallel()
	used := 1374
	report := Report{Agents: []AgentContext{{Name: "a", Harness: "pi", Status: StatusAvailable, Used: &used}}}
	out := Render(report)
	if !strings.Contains(out, "1374 tokens used (context window unknown)") {
		t.Errorf("Render output = %q, want window-unknown phrasing", out)
	}
}

func TestRenderEmpty(t *testing.T) {
	t.Parallel()
	if got := Render(Report{}); got != "No live agents.\n" {
		t.Errorf("Render(empty) = %q", got)
	}
}
