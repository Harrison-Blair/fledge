package spec

import (
	"embed"
	"strings"
)

//go:embed templates/requirement.md templates/task.md
var templatesFS embed.FS

// RequirementBody returns the skeleton body for a new requirement.
func RequirementBody(id, title string) []byte {
	return renderTemplate("templates/requirement.md", map[string]string{
		"{{ID}}": id, "{{TITLE}}": title,
	})
}

// TaskBody returns the skeleton body for a new task.
func TaskBody(id, title, reqID string) []byte {
	return renderTemplate("templates/task.md", map[string]string{
		"{{ID}}": id, "{{TITLE}}": title, "{{REQ}}": reqID,
	})
}

func renderTemplate(name string, subs map[string]string) []byte {
	b, err := templatesFS.ReadFile(name)
	if err != nil {
		panic(err) // embedded; cannot fail at runtime
	}
	s := "\n" + string(b)
	for k, v := range subs {
		s = strings.ReplaceAll(s, k, v)
	}
	return []byte(s)
}
