// Package brief owns task brief text: its validation and an advisory
// template skeleton of six second-level Markdown sections.
package brief

import (
	"fmt"
	"strings"
	"unicode/utf8"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

// Sections are the advisory template's `## ` headings, in order.
var Sections = []string{"Objective", "Acceptance criteria", "Scope", "Known facts", "Deliverables", "Constraints"}

var hints = []string{
	"what outcome is wanted, in one paragraph",
	"concrete, testable checks that decide completion",
	"allowed files or areas, and what is explicitly out",
	"established facts the worker would otherwise rediscover, with file:line where known",
	"the return contract: evidence to report, findings or decisions to return, what may remain undone",
	"rules: authorization, commit policy, who to report to, what to escalate",
}

// Skeleton is an unfilled brief: every heading followed by a one-line HTML
// comment hint. The template is optional; the skeleton itself is valid text.
func Skeleton() string {
	var b strings.Builder
	for i, s := range Sections {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "## %s\n<!-- %s -->\n", s, hints[i])
	}
	return b.String()
}

// Validate reports whether text can be a brief: valid UTF-8, not only
// Unicode whitespace, and free of NUL. It never alters the text.
func Validate(text string) error {
	switch {
	case !utf8.ValidString(text):
		return libagent.Invalid("brief must be valid UTF-8")
	case strings.ContainsRune(text, 0):
		return libagent.Invalid("brief must not contain NUL")
	case strings.TrimSpace(text) == "":
		return libagent.Invalid("brief must not be blank")
	}
	return nil
}
