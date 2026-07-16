package bootstrap

import (
	"strings"
	"testing"
)

// brooderDoc returns the embedded brooder.md contents.
func brooderDoc(t *testing.T) string {
	t.Helper()
	data, err := FS.ReadFile("core/skills/fledge-orchestrate/brooder.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestBrooderDocSections: brooder.md is titled "# Brooder" and contains its
// four subsection headings.
func TestBrooderDocSections(t *testing.T) {
	doc := brooderDoc(t)

	if !strings.HasPrefix(doc, "# Brooder") {
		t.Errorf("brooder.md must start with \"# Brooder\", got %q", firstLine(doc))
	}

	for _, heading := range []string{
		"### Communication rules",
		"### Protocol",
		"### When stuck",
		"### Lifecycle",
	} {
		if !strings.Contains(doc, heading) {
			t.Errorf("brooder.md must contain the %q subsection heading", heading)
		}
	}
}

// TestBrooderFixLoopInvariant: guards the fix-loop sentence against
// incidental change (formerly asserted by worker_protocols_test.go).
func TestBrooderFixLoopInvariant(t *testing.T) {
	doc := brooderDoc(t)

	fixLoop := "Do not argue a finding with the skua past one round of clarification"
	if !strings.Contains(doc, fixLoop) {
		t.Errorf("brooder.md must still contain the fix-loop sentence verbatim: %q", fixLoop)
	}
}
