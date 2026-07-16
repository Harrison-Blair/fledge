package bootstrap

import (
	"strings"
	"testing"
)

// planningDocSection returns the slice of planning.md from the given marker to
// the next top-level "## " heading (or end of document).
func planningDocSection(t *testing.T, doc, marker string) string {
	t.Helper()
	idx := strings.Index(doc, marker)
	if idx == -1 {
		t.Fatalf("planning.md must contain %q", marker)
	}
	section := doc[idx:]
	if end := strings.Index(section[1:], "\n## "); end != -1 {
		section = section[:end+1]
	}
	return section
}

// TestPlanningDocDescribesDigestWrite: planning.md step 4.7 (the closing
// report) instructs writing .fledge/scratch/digest-planning.md with the
// phase's outcome, key user decisions, and pointers to the created spec
// files.
func TestPlanningDocDescribesDigestWrite(t *testing.T) {
	data, err := FS.ReadFile("core/skills/fledge-orchestrate/planning.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)

	// Step 4.7 starts at the "After the last feather" item.
	idx := strings.Index(doc, "After the last feather")
	if idx == -1 {
		t.Fatal("planning.md must still contain step 4.7 (\"After the last feather\")")
	}
	step := doc[idx:]

	if !strings.Contains(step, ".fledge/scratch/digest-planning.md") {
		t.Fatal("step 4.7 must instruct writing .fledge/scratch/digest-planning.md")
	}
	for _, want := range []string{"outcome", "decisions", "overwriting"} {
		if !strings.Contains(step, want) {
			t.Errorf("step 4.7's digest prose must mention %q", want)
		}
	}
	if !strings.Contains(step, "pointers to the created spec files") {
		t.Error("step 4.7's digest prose must mention pointers to the created spec files")
	}
}

// TestPlanningDocDescribesDigestRead: planning.md step 1 (the freshness gate,
// the phase's opening) notes that a prior .fledge/scratch/digest-implementation.md
// should be read as grounding context, best-effort (missing file = proceed
// without it).
func TestPlanningDocDescribesDigestRead(t *testing.T) {
	data, err := FS.ReadFile("core/skills/fledge-orchestrate/planning.md")
	if err != nil {
		t.Fatal(err)
	}
	section := planningDocSection(t, string(data), "## 1. Freshness gate")

	if !strings.Contains(section, ".fledge/scratch/digest-implementation.md") {
		t.Fatal("step 1 must mention reading .fledge/scratch/digest-implementation.md")
	}
	if !strings.Contains(section, "best-effort") {
		t.Error("step 1's digest-read prose must be marked best-effort")
	}
	if !strings.Contains(section, "proceed without it") {
		t.Error("step 1's digest-read prose must say a missing digest means proceed without it")
	}
}
