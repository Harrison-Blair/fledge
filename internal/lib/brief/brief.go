// Package brief owns the task brief template: six fixed second-level Markdown
// sections, their validation, and a printable skeleton.
package brief

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// Sections are the required `## ` headings of a brief, in order.
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
// comment hint. It fails Validate until each section gains content.
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

// Validate reports whether text follows the template: each section heading
// exactly once, in order, with content other than blank lines and HTML
// comments on their own lines. Text before the first heading is allowed, and
// heading-like lines inside fenced code blocks are content.
func Validate(text string) error {
	var seen []string
	var open string
	filled := map[string]bool{}
	for line := range strings.Lines(text) {
		line = strings.TrimRight(line, " \t\r\n")
		if marker := fence(line); open == "" && marker != "" {
			open = marker
		} else if open != "" && strings.HasPrefix(marker, open) && strings.TrimSpace(line) == marker {
			open = ""
		} else if name, ok := strings.CutPrefix(line, "## "); ok && open == "" {
			if !slices.Contains(Sections, name) {
				return incomplete("brief has unknown section %s", name)
			}
			if slices.Contains(seen, name) {
				return incomplete("brief section %s is duplicated", name)
			}
			seen = append(seen, name)
			continue
		}
		if body := strings.TrimSpace(line); len(seen) > 0 && body != "" && !comment(body) {
			filled[seen[len(seen)-1]] = true
		}
	}
	var missing []string
	for _, s := range Sections {
		if !slices.Contains(seen, s) {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		return incomplete("brief is missing sections: %s", strings.Join(missing, ", "))
	}
	for i := 1; i < len(seen); i++ {
		if slices.Index(Sections, seen[i-1]) > slices.Index(Sections, seen[i]) {
			return incomplete("brief sections are out of order: %s before %s", seen[i-1], seen[i])
		}
	}
	for _, s := range Sections {
		if !filled[s] {
			return incomplete("brief section %s is empty", s)
		}
	}
	return nil
}

// fence returns the run of three or more backticks or tildes that opens line,
// after at most three spaces of indent, or "" when line is not a fence.
func fence(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || !strings.HasPrefix(trimmed, "```") && !strings.HasPrefix(trimmed, "~~~") {
		return ""
	}
	return trimmed[:len(trimmed)-len(strings.TrimLeft(trimmed, trimmed[:1]))]
}

// comment reports whether line is a single HTML comment and nothing else.
func comment(line string) bool {
	return len(line) >= 7 && strings.HasPrefix(line, "<!--") && strings.Index(line, "-->") == len(line)-3
}

func incomplete(format string, args ...any) error {
	return &herdr.Error{Code: "task_brief_incomplete", Message: fmt.Sprintf(format, args...)}
}
