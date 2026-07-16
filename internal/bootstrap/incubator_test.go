package bootstrap

import (
	"strings"
	"testing"
)

// incubatorDoc returns the embedded incubator.md contents.
func incubatorDoc(t *testing.T) string {
	t.Helper()
	data, err := FS.ReadFile("core/skills/fledge-orchestrate/incubator.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestIncubatorDocSections: incubator.md exists, is titled "# Incubator", and
// contains its four subsection headings.
func TestIncubatorDocSections(t *testing.T) {
	doc := incubatorDoc(t)

	if !strings.HasPrefix(doc, "# Incubator") {
		t.Errorf("incubator.md must start with \"# Incubator\", got %q", firstLine(doc))
	}

	for _, heading := range []string{
		"### Relay envelope",
		"### Communication rules",
		"### Drafting",
		"### Lifecycle",
	} {
		if !strings.Contains(doc, heading) {
			t.Errorf("incubator.md must contain the %q subsection heading", heading)
		}
	}
}

// TestIncubatorDocDescribesScratchpadBatching: incubator.md documents the
// scratchpad-batching mechanism — the batchable rule, the scratchpad path,
// the single GATE review reuse, and that it covers both plumage and feather
// interrogation.
func TestIncubatorDocDescribesScratchpadBatching(t *testing.T) {
	doc := incubatorDoc(t)

	if !strings.Contains(doc, "### Scratchpad batching") {
		t.Errorf("incubator.md must contain the %q subsection heading", "### Scratchpad batching")
	}

	for _, phrase := range []string{
		"independent leaves",
		".fledge/scratch/",
		"one `GATE review`",
		"plumage interrogation",
		"feather interrogation",
	} {
		if !strings.Contains(doc, phrase) {
			t.Errorf("incubator.md must contain %q", phrase)
		}
	}
}

// firstLine returns the first line of s, for error messages.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i != -1 {
		return s[:i]
	}
	return s
}
