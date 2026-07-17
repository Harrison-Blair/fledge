package bootstrap

import (
	"strings"
	"testing"
)

// TestForagingDocDescribesDigestWrite: foraging.md's Commissioner
// verify-and-release paragraph instructs the commissioner to write
// `.fledge/scratch/digest-foraging.md` as part of closing out foraging.
func TestForagingDocDescribesDigestWrite(t *testing.T) {
	data, err := FS.ReadFile("core/skills/fledge-orchestrate/foraging.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	// FTHR-075 changed the gating signal from the forager's final message to
	// its ledger `status` record reaching "done", so the paragraph marker
	// changed to match — the digest-write duty itself (asserted below) is
	// unchanged.
	marker := "**On done, verify and release.**"
	idx := strings.Index(body, marker)
	if idx == -1 {
		t.Fatalf("foraging.md missing verify-and-release paragraph marker %q", marker)
	}
	// The paragraph runs from the marker to the next blank line.
	para := body[idx:]
	if end := strings.Index(para, "\n\n"); end != -1 {
		para = para[:end]
	}

	for _, want := range []string{
		".fledge/scratch/digest-foraging.md",
		"commissioner",
	} {
		if !strings.Contains(para, want) {
			t.Errorf("foraging.md verify-and-release paragraph missing digest wording %q", want)
		}
	}
}
