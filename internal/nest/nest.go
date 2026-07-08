// Package nest provides schemas, rendering, and an embedded-template registry
// for fledge context documents written to .fledge/nest/.
package nest

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/Harrison-Blair/fledge/internal/spec"
)

//go:embed templates/concern-doc.md templates/index.md templates/scout-report.md
var templatesFS embed.FS

// Kind is the document kind: either a concern synthesis doc or a raw scout report.
type Kind string

const (
	Concern Kind = "concern"
	Scout   Kind = "scout"
)

// Doc is one context document — either a concern doc (.fledge/nest/<name>.md)
// or a raw scout report (.fledge/nest/raw/<module>.md).
type Doc struct {
	Kind Kind

	// Concern fields (Kind == Concern).
	Generated string // UTC RFC3339 timestamp
	Commit    string // full git HEAD SHA

	// Scout fields (Kind == Scout).
	Module  string // top-level module name
	Authored string // UTC RFC3339 timestamp

	// Shared.
	Agent         string
	FledgeVersion string

	Body []byte // everything after the closing ---; byte-preserved
}

// Frontmatter renders the canonical frontmatter block including both ---
// fences. Key order is fixed per kind.
func (d *Doc) Frontmatter() []byte {
	var b bytes.Buffer
	b.WriteString("---\n")
	switch d.Kind {
	case Concern:
		fmt.Fprintf(&b, "generated: %s\n", d.Generated)
		fmt.Fprintf(&b, "commit: %s\n", d.Commit)
		fmt.Fprintf(&b, "agent: %s\n", spec.YAMLScalar(d.Agent))
		fmt.Fprintf(&b, "fledge_version: %s\n", spec.YAMLScalar(d.FledgeVersion))
	case Scout:
		fmt.Fprintf(&b, "module: %s\n", spec.YAMLScalar(d.Module))
		fmt.Fprintf(&b, "authored: %s\n", d.Authored)
		fmt.Fprintf(&b, "agent: %s\n", spec.YAMLScalar(d.Agent))
		fmt.Fprintf(&b, "fledge_version: %s\n", spec.YAMLScalar(d.FledgeVersion))
	}
	b.WriteString("---\n")
	return b.Bytes()
}

// Render returns the full file bytes: canonical frontmatter + preserved body.
func (d *Doc) Render() []byte { return append(d.Frontmatter(), d.Body...) }

// ConcernBody returns the stub body for a new concern doc with the given
// display title.
func ConcernBody(title string) []byte {
	b, err := templatesFS.ReadFile("templates/concern-doc.md")
	if err != nil {
		panic(err)
	}
	s := "\n" + strings.ReplaceAll(string(b), "{{TITLE}}", title)
	return []byte(s)
}

// IndexBody returns the stub body for a new index doc.
func IndexBody() []byte {
	b, err := templatesFS.ReadFile("templates/index.md")
	if err != nil {
		panic(err)
	}
	return append([]byte("\n"), b...)
}

// ScoutBody returns the stub body for a new scout report.
func ScoutBody(module string) []byte {
	b, err := templatesFS.ReadFile("templates/scout-report.md")
	if err != nil {
		panic(err)
	}
	s := "\n" + strings.ReplaceAll(string(b), "{{MODULE}}", module)
	return []byte(s)
}

// ClearNest removes all .md files directly under contextDir and the entire
// raw/ subdirectory, then recreates raw/. contextDir itself is not removed.
func ClearNest(contextDir string) error {
	matches, err := filepath.Glob(filepath.Join(contextDir, "*.md"))
	if err != nil {
		return err
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	rawDir := filepath.Join(contextDir, "raw")
	if err := os.RemoveAll(rawDir); err != nil {
		return err
	}
	return os.MkdirAll(rawDir, 0o755)
}

// RefreshDoc rebuilds a nest document from its raw bytes b. It parses the
// existing frontmatter to recover the stored agent and module fields, constructs
// a Doc with the supplied derived values (generated, commit, version), drops
// unknown frontmatter keys, and returns the rendered bytes (canonical FM +
// original body). For Scout kind, generated is used as the authored timestamp.
// If agentOverride is non-empty it replaces the stored agent field.
func RefreshDoc(b []byte, kind Kind, generated, commit, agentOverride, version string) ([]byte, error) {
	fm, body, err := spec.SplitFrontmatter(b)
	if err != nil {
		return nil, err
	}
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(fm, &parsed); err != nil {
		return nil, fmt.Errorf("invalid frontmatter: %w", err)
	}
	agent := strFromMap(parsed, "agent")
	if agentOverride != "" {
		agent = agentOverride
	}
	module := strFromMap(parsed, "module")

	d := &Doc{
		Kind:          kind,
		Agent:         agent,
		FledgeVersion: version,
		Body:          body,
	}
	switch kind {
	case Concern:
		d.Generated = generated
		d.Commit = commit
	case Scout:
		d.Module = module
		d.Authored = generated
	}
	return d.Render(), nil
}

// strFromMap extracts a string value from a YAML-decoded map, returning ""
// when the key is absent or not a string.
func strFromMap(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
