package brief

import (
	"errors"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

func TestSections(t *testing.T) {
	want := []string{"Objective", "Acceptance criteria", "Scope", "Known facts", "Deliverables", "Constraints"}
	if strings.Join(Sections, "|") != strings.Join(want, "|") {
		t.Fatal(Sections)
	}
}

// Any nonblank, NUL-free UTF-8 text is a brief; the template is advisory.
func TestValidateAccepts(t *testing.T) {
	for label, text := range map[string]string{
		"one line":        "one line",
		"skeleton":        Skeleton(),
		"single rune":     "x",
		"padded":          "  \n\tpadded text\n\n",
		"off template":    "## Notes\nanything\n## Scope\n",
		"unicode":         "résumé → 完成",
		"space-led rune":  "　x ",
		"comment only":    "<!-- hint -->",
		"control not NUL": "a\x01b",
	} {
		if err := Validate(text); err != nil {
			t.Errorf("%s: %v", label, err)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	for label, c := range map[string]struct{ text, message string }{
		"empty":           {"", "brief must not be blank"},
		"ascii blank":     {" \t\r\n\v\f", "brief must not be blank"},
		"unicode blank":   {"  　\u0085 \n", "brief must not be blank"},
		"NUL":             {"a\x00b", "brief must not contain NUL"},
		"only NUL":        {"\x00", "brief must not contain NUL"},
		"invalid UTF-8":   {"ok \xff", "brief must be valid UTF-8"},
		"surrogate bytes": {"\xed\xa0\x80", "brief must be valid UTF-8"},
	} {
		err := Validate(c.text)
		var input *libagent.InputError
		if !errors.As(err, &input) || input.Message != c.message {
			t.Errorf("%s: %v", label, err)
		}
	}
}

func TestSkeletonHasEveryHeadingWithAHint(t *testing.T) {
	lines := strings.Split(strings.TrimSpace(Skeleton()), "\n")
	var headings []string
	for i, line := range lines {
		if name, ok := strings.CutPrefix(line, "## "); ok {
			headings = append(headings, name)
			if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "<!-- ") || !strings.HasSuffix(lines[i+1], " -->") {
				t.Fatalf("%s has no hint: %q", name, Skeleton())
			}
		}
	}
	if strings.Join(headings, "|") != strings.Join(Sections, "|") {
		t.Fatal(Skeleton())
	}
}
