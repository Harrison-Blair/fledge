package memory

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
)

// Types are the kinds of memory, in help order.
var Types = []string{"user", "feedback", "project", "reference"}

// Memory is one fact: a kebab-case name that is also its file's basename, a
// one-line description for the index, a type, and a Markdown body.
type Memory struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Body        string `json:"body"`
}

var slug = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidateName checks a --name value.
func ValidateName(name string) error {
	if len(name) > 64 || !slug.MatchString(name) {
		return cli.Invalid("--name must be a kebab-case slug of lowercase letters and digits, at most 64 characters")
	}
	return nil
}

// ValidateType checks a --type value.
func ValidateType(kind string) error {
	if !slices.Contains(Types, kind) {
		return cli.Invalid("--type must be one of %s", strings.Join(Types, ", "))
	}
	return nil
}

// Validate checks every field of m.
func Validate(m Memory) error {
	if err := ValidateName(m.Name); err != nil {
		return err
	}
	if m.Description == "" || m.Description != strings.TrimSpace(m.Description) || strings.ContainsAny(m.Description, "\r\n") || !utf8.ValidString(m.Description) {
		return cli.Invalid("--description must be one nonempty line without surrounding whitespace")
	}
	if err := ValidateType(m.Type); err != nil {
		return err
	}
	if strings.TrimSpace(m.Body) == "" || !utf8.ValidString(m.Body) {
		return cli.Invalid("body must be nonempty UTF-8")
	}
	return nil
}

// Format renders m as its file: frontmatter, a blank line, and the body
// ending in a newline.
func Format(m Memory) []byte {
	body := m.Body
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return fmt.Appendf(nil, "---\nname: %s\ndescription: %s\ntype: %s\n---\n\n%s", m.Name, m.Description, m.Type, body)
}

// Parse reads the file of the memory named name, requiring exactly the
// frontmatter Format writes and a valid memory.
func Parse(name string, data []byte) (Memory, error) {
	text := string(data)
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return Memory{}, fmt.Errorf("missing frontmatter")
	}
	head, body, ok := strings.Cut(rest, "\n---\n\n")
	if !ok {
		return Memory{}, fmt.Errorf("unterminated frontmatter")
	}
	fields := map[string]string{}
	for _, line := range strings.Split(head, "\n") {
		key, value, ok := strings.Cut(line, ": ")
		if _, seen := fields[key]; !ok || seen || !slices.Contains([]string{"name", "description", "type"}, key) {
			return Memory{}, fmt.Errorf("invalid frontmatter line %q", line)
		}
		fields[key] = value
	}
	m := Memory{Name: fields["name"], Description: fields["description"], Type: fields["type"], Body: body}
	if len(fields) != 3 {
		return Memory{}, fmt.Errorf("frontmatter needs name, description, and type")
	}
	if m.Name != name {
		return Memory{}, fmt.Errorf("frontmatter name %q does not match the file name", m.Name)
	}
	if err := Validate(m); err != nil {
		return Memory{}, err
	}
	return m, nil
}

// Index renders one line per memory, ordered by name.
func Index(ms []Memory) string {
	ms = slices.Clone(ms)
	slices.SortFunc(ms, func(a, b Memory) int { return strings.Compare(a.Name, b.Name) })
	var b strings.Builder
	for _, m := range ms {
		fmt.Fprintf(&b, "- [%s](%s.md) — %s\n", m.Name, m.Name, m.Description)
	}
	return b.String()
}
