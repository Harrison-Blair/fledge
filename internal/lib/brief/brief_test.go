package brief

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
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
		"NUL and invalid": {"\x00\xff", "brief must be valid UTF-8"},
		"NUL and blank":   {" \x00 ", "brief must not contain NUL"},
	} {
		err := Validate(c.text)
		var input *cli.InputError
		if !errors.As(err, &input) || input.Message != c.message {
			t.Errorf("%s: %v", label, err)
		}
	}
}

// CheckText reports the first broken rule as a plain, unprefixed error, in
// the order UTF-8, NUL, blank.
func TestCheckText(t *testing.T) {
	for text, want := range map[string]string{
		"ok":        "",
		"\x00\xff":  "must be valid UTF-8",
		" \x00 ":    "must not contain NUL",
		" \u3000\n": "must not be blank",
	} {
		err := CheckText(text)
		if got := fmt.Sprint(err); want == "" && err != nil || want != "" && got != want {
			t.Errorf("%q: %v", text, err)
		}
		if errors.As(err, new(*cli.InputError)) {
			t.Errorf("%q: classified %T", text, err)
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
