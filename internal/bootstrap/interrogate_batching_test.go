package bootstrap

import (
	"strings"
	"testing"
)

// TestInterrogateSkillDocumentsBatchingException: fledge-interrogate/SKILL.md
// keeps its original one-question-at-a-time instruction intact AND, in the
// same vicinity, documents the delegated-incubator exception for batching
// resolved questions via incubator.md's scratchpad mechanism.
func TestInterrogateSkillDocumentsBatchingException(t *testing.T) {
	data, err := FS.ReadFile("core/skills/fledge-interrogate/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)

	const original = "Ask the questions one at a time, waiting for feedback on each question before continuing. Asking multiple questions at once is bewildering."
	idx := strings.Index(doc, original)
	if idx == -1 {
		t.Fatalf("SKILL.md must still contain the original one-at-a-time sentence verbatim: %q", original)
	}

	// The batching exception must appear in the same vicinity: the remainder
	// of the paragraph the original sentence lives in.
	rest := doc[idx+len(original):]
	if end := strings.Index(rest, "\n\n"); end != -1 {
		rest = rest[:end]
	}

	if !strings.Contains(rest, "incubator.md") {
		t.Errorf("the paragraph containing the one-at-a-time sentence must reference incubator.md; got %q", rest)
	}
	if !strings.Contains(rest, "batch") || !strings.Contains(rest, "scratchpad") {
		t.Errorf("the paragraph containing the one-at-a-time sentence must mention batching via the scratchpad mechanism; got %q", rest)
	}
}
