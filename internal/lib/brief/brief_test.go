package brief

import (
	"errors"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// build joins headings and bodies into a brief: each pair is a heading name
// and the text under it.
func build(pairs ...string) string {
	var b strings.Builder
	for i := 0; i < len(pairs); i += 2 {
		b.WriteString("## " + pairs[i] + "\n" + pairs[i+1] + "\n\n")
	}
	return b.String()
}

func full() []string {
	var pairs []string
	for _, s := range Sections {
		pairs = append(pairs, s, "text for "+s)
	}
	return pairs
}

func TestSections(t *testing.T) {
	want := []string{"Objective", "Acceptance criteria", "Scope", "Known facts", "Deliverables", "Constraints"}
	if strings.Join(Sections, "|") != strings.Join(want, "|") {
		t.Fatal(Sections)
	}
}

func TestValidateAccepts(t *testing.T) {
	for label, text := range map[string]string{
		"plain":             build(full()...),
		"preamble":          "Title line\n\nsome preamble\n" + build(full()...),
		"subheadings":       strings.Replace(build(full()...), "text for Scope", "### Allowed\nfiles\n### Out\nnone", 1),
		"trailing ws":       strings.Replace(build(full()...), "## Objective\n", "## Objective  \t\r\n", 1),
		"inline comment":    strings.Replace(build(full()...), "text for Scope", "<!-- hint --> real text", 1),
		"between comments":  strings.Replace(build(full()...), "text for Scope", "<!-- a --> text <!-- b -->", 1),
		"comment plus text": strings.Replace(build(full()...), "text for Scope", "<!-- hint -->\nreal text", 1),
	} {
		if err := Validate(text); err != nil {
			t.Errorf("%s: %v", label, err)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	pairs := full()
	for label, c := range map[string]struct{ text, message string }{
		"one line":     {"one line", "brief is missing sections: Objective, Acceptance criteria, Scope, Known facts, Deliverables, Constraints"},
		"missing":      {build(append(append([]string{}, pairs[:4]...), pairs[6:10]...)...), "brief is missing sections: Scope, Constraints"},
		"empty":        {strings.Replace(build(pairs...), "text for Objective", "  \n\t", 1), "brief section Objective is empty"},
		"comment only": {strings.Replace(build(pairs...), "text for Objective", "<!-- what outcome is wanted -->\n  <!-- more -->  ", 1), "brief section Objective is empty"},
		"last empty":   {strings.Replace(build(pairs...), "text for Constraints", "", 1), "brief section Constraints is empty"},
		"unknown":      {build(append(append([]string{}, pairs...), "Notes", "x")...), "brief has unknown section Notes"},
		"case":         {strings.Replace(build(pairs...), "## Scope", "## scope", 1), "brief has unknown section scope"},
		"duplicate":    {build(append(append([]string{}, pairs...), "Scope", "again")...), "brief section Scope is duplicated"},
		"order":        {build(append(append(append([]string{}, pairs[:2]...), pairs[4:6]...), append(append([]string{}, pairs[2:4]...), pairs[6:]...)...)...), "brief sections are out of order: Scope before Acceptance criteria"},
		"skeleton":     {Skeleton(), "brief section Objective is empty"},
	} {
		err := Validate(c.text)
		var coded *herdr.Error
		if !errors.As(err, &coded) || coded.Code != "task_brief_incomplete" || coded.Message != c.message {
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
	if err := Validate(strings.ReplaceAll(Skeleton(), "-->", "-->\nfilled")); err != nil {
		t.Fatalf("filled skeleton: %v", err)
	}
}
