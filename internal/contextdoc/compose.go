package contextdoc

import (
	"errors"
	"fmt"
	"strings"
)

// RequestTemplate holds the operator-editable instruction sections injected
// into an analyzer request by composition.
type RequestTemplate struct {
	Before string
	After  string
}

// ParseRequestTemplate extracts the instruction sections from an
// operator-editable template document. Each section is delimited by exact
// <instructions_before> and <instructions_after> XML tags that must occur
// exactly once; text outside the tags is ignored. Section content is
// whitespace-trimmed and must be nonempty.
func ParseRequestTemplate(data []byte) (RequestTemplate, error) {
	before, err := extractTemplateSection(string(data), "instructions_before")
	if err != nil {
		return RequestTemplate{}, err
	}
	after, err := extractTemplateSection(string(data), "instructions_after")
	if err != nil {
		return RequestTemplate{}, err
	}
	return RequestTemplate{Before: before, After: after}, nil
}

func extractTemplateSection(document, tag string) (string, error) {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	start := strings.Index(document, open)
	if start < 0 {
		return "", fmt.Errorf("template is missing the %s tag", open)
	}
	if strings.Contains(document[start+len(open):], open) {
		return "", fmt.Errorf("template contains more than one %s tag", open)
	}
	rest := document[start+len(open):]
	end := strings.Index(rest, close)
	if end < 0 {
		return "", fmt.Errorf("template is missing the %s tag", close)
	}
	if strings.Contains(rest[end+len(close):], close) {
		return "", fmt.Errorf("template contains more than one %s tag", close)
	}
	content := strings.TrimSpace(rest[:end])
	if content == "" {
		return "", fmt.Errorf("template section %s must be nonempty", open)
	}
	return content, nil
}

// ComposeAnalyzerRequest sets the request's instruction fields from the
// template, substituting the fixed {group_id}, {purpose}, and {worksheet_path}
// placeholders with the request's own values. Any other {...} text passes
// through verbatim. A template that references {worksheet_path} requires a
// nonempty worksheetPath. Recomposition overwrites previously composed
// instructions.
func ComposeAnalyzerRequest(request AnalyzerRequest, template RequestTemplate, worksheetPath string) (AnalyzerRequest, error) {
	substitute := func(section string) (string, error) {
		if strings.Contains(section, "{worksheet_path}") && worksheetPath == "" {
			return "", errors.New("template references {worksheet_path}; pass --worksheet or remove the placeholder")
		}
		section = strings.ReplaceAll(section, "{group_id}", request.GroupID)
		section = strings.ReplaceAll(section, "{purpose}", request.Purpose)
		return strings.ReplaceAll(section, "{worksheet_path}", worksheetPath), nil
	}
	before, err := substitute(template.Before)
	if err != nil {
		return AnalyzerRequest{}, err
	}
	after, err := substitute(template.After)
	if err != nil {
		return AnalyzerRequest{}, err
	}
	request.InstructionsBefore = before
	request.InstructionsAfter = after
	return request, nil
}

// ComposeWorksheet stamps the operator-editable worksheet template for one
// request, substituting {group_id}, {purpose}, and {files}. {files} renders as
// a Markdown checklist of the request's assigned files. Any other {...} text
// passes through verbatim.
func ComposeWorksheet(request AnalyzerRequest, template []byte) (string, error) {
	if strings.TrimSpace(string(template)) == "" {
		return "", errors.New("worksheet template must be nonempty")
	}
	var files strings.Builder
	for i, file := range request.Files {
		if i > 0 {
			files.WriteString("\n")
		}
		fmt.Fprintf(&files, "- [ ] `%s` (%d bytes)", file.Path, file.Size)
	}
	worksheet := string(template)
	worksheet = strings.ReplaceAll(worksheet, "{group_id}", request.GroupID)
	worksheet = strings.ReplaceAll(worksheet, "{purpose}", request.Purpose)
	return strings.ReplaceAll(worksheet, "{files}", files.String()), nil
}
