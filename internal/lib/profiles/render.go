package profiles

import (
	_ "embed"
	"strings"
)

// shared is the Fledge protocol block every protocol profile renders.
//
//go:embed builtin/protocol.md
var shared string

// Brief renders the profile as Markdown: one H2 block per non-empty part in
// a fixed order, separated by blank lines. Texts render byte for byte.
func (p Profile) Brief() string {
	reads := ""
	if len(p.Reads) > 0 {
		reads = "Read these files in your working directory before starting: `" + strings.Join(p.Reads, "`, `") + "`."
	}
	protocol := []string{p.Sections.Protocol}
	if p.Protocol {
		protocol = []string{shared, p.Sections.Protocol}
	}
	var blocks []string
	for _, part := range []struct {
		heading string
		texts   []string
	}{
		{"Mission", []string{p.Sections.Mission}},
		{"Read first", []string{reads}},
		{"Workflow", []string{p.Sections.Workflow}},
		{"Always", []string{p.Sections.Always}},
		{"Never", []string{p.Sections.Never}},
		{"Fledge protocol", protocol},
		{"Report", []string{p.Sections.Report}},
	} {
		var texts []string
		for _, t := range part.texts {
			if t != "" {
				texts = append(texts, t)
			}
		}
		if len(texts) > 0 {
			blocks = append(blocks, "## "+part.heading+"\n"+paragraphs(texts))
		}
	}
	return paragraphs(blocks)
}

// paragraphs joins texts with a blank line, adding only the newlines a text
// does not already end with.
func paragraphs(texts []string) string {
	var b strings.Builder
	for i, t := range texts {
		if i > 0 && strings.HasSuffix(texts[i-1], "\n") {
			b.WriteString("\n")
		} else if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(t)
	}
	return b.String()
}
