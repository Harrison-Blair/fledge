package bootstrap

import (
	"strings"
	"testing"
)

// implementationDocSection returns the body of implementation.md between the
// given heading and the next `## ` heading (or EOF).
func implementationDocSection(t *testing.T, heading string) string {
	t.Helper()
	data, err := FS.ReadFile("core/skills/fledge-orchestrate/implementation.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)

	idx := strings.Index(doc, heading)
	if idx == -1 {
		t.Fatalf("implementation.md missing heading %q", heading)
	}
	section := doc[idx+len(heading):]
	if end := strings.Index(section, "\n## "); end != -1 {
		section = section[:end]
	}
	return section
}

// TestImplementationDocDescribesDigestWrite: implementation.md step 5 ("End of
// run") documents writing the implementation-phase digest to
// `.fledge/scratch/digest-implementation.md`, overwriting any prior one.
func TestImplementationDocDescribesDigestWrite(t *testing.T) {
	section := implementationDocSection(t, "## 5. End of run")

	for _, want := range []string{
		".fledge/scratch/digest-implementation.md",
		"overwrit",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("implementation.md step 5 missing digest-write wording %q", want)
		}
	}
}

// TestImplementationDocDescribesDigestRead: implementation.md step 1 ("Resolve
// scope") documents best-effort reading a prior planning digest at
// `.fledge/scratch/digest-planning.md` if present.
func TestImplementationDocDescribesDigestRead(t *testing.T) {
	section := implementationDocSection(t, "## 1. Resolve scope")

	for _, want := range []string{
		".fledge/scratch/digest-planning.md",
		"best-effort",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("implementation.md step 1 missing digest-read wording %q", want)
		}
	}
}
