package bootstrap

import (
	"strings"
	"testing"
)

// TestPlanningDocReferencesScratchpadBatching: planning.md steps 3 (plumage
// interrogation) and 4.1 (feather interrogation) keep their original
// one-question-at-a-time framing intact AND, in the same paragraph, reference
// the scratchpad-batching option documented in incubator.md.
func TestPlanningDocReferencesScratchpadBatching(t *testing.T) {
	data, err := FS.ReadFile("core/skills/fledge-orchestrate/planning.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)

	cases := []struct {
		step     string
		original string
	}{
		{"step 3", "one question at a time, recommended answer first"},
		{"step 4.1", "still one question at a time"},
	}
	for _, tc := range cases {
		t.Run(tc.step, func(t *testing.T) {
			idx := strings.Index(doc, tc.original)
			if idx == -1 {
				t.Fatalf("planning.md %s must still contain the one-at-a-time framing verbatim: %q", tc.step, tc.original)
			}

			// The batching reference must appear in the same paragraph as the
			// one-at-a-time framing.
			start := strings.LastIndex(doc[:idx], "\n\n")
			if start == -1 {
				start = 0
			}
			para := doc[start:]
			if end := strings.Index(doc[idx:], "\n\n"); end != -1 {
				para = doc[start : idx+end]
			}

			if !strings.Contains(para, "incubator.md") {
				t.Errorf("%s's interrogation paragraph must reference incubator.md; got %q", tc.step, para)
			}
			if !strings.Contains(para, "batch") || !strings.Contains(para, "scratchpad") {
				t.Errorf("%s's interrogation paragraph must mention scratchpad batching; got %q", tc.step, para)
			}
		})
	}
}
