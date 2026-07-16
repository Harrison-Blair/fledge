package bootstrap

import (
	"strings"
	"testing"
)

// TestWorkerProtocolsStub: worker-protocols.md is a stub index — no per-role
// "## " sections remain, and it links to all three per-role files.
func TestWorkerProtocolsStub(t *testing.T) {
	data, err := FS.ReadFile("core/skills/fledge-orchestrate/worker-protocols.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)

	for _, heading := range []string{"## Incubator", "## Brooder", "## Skua"} {
		if strings.Contains(doc, heading) {
			t.Errorf("worker-protocols.md must no longer contain the %q section heading", heading)
		}
	}

	for _, file := range []string{"incubator.md", "brooder.md", "skua.md"} {
		if !strings.Contains(doc, file) {
			t.Errorf("worker-protocols.md stub must link to %q", file)
		}
	}
}
