package nest_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/nest"
	"github.com/Harrison-Blair/fledge/internal/spec"
)

func TestConcernFrontmatterKeyOrder(t *testing.T) {
	d := nest.Doc{
		Kind:          nest.Concern,
		Generated:     "2026-01-01T00:00:00Z",
		Commit:        "abc123",
		Agent:         "fledge-forager",
		FledgeVersion: "0.2.1",
	}
	fm := d.Frontmatter()
	s := string(fm)

	if !strings.HasPrefix(s, "---\n") {
		t.Errorf("frontmatter must start with ---\\n, got: %q", s)
	}
	if !strings.HasSuffix(s, "---\n") {
		t.Errorf("frontmatter must end with ---\\n, got: %q", s)
	}

	// Keys must appear in fixed order: generated, commit, agent, fledge_version.
	keys := []string{"generated:", "commit:", "agent:", "fledge_version:"}
	prev := 0
	for _, k := range keys {
		i := strings.Index(s, k)
		if i < 0 {
			t.Errorf("key %q not found in concern frontmatter:\n%s", k, s)
			continue
		}
		if i < prev {
			t.Errorf("key %q out of order (at %d, prev=%d):\n%s", k, i, prev, s)
		}
		prev = i
	}
}

func TestScoutFrontmatterKeyOrder(t *testing.T) {
	d := nest.Doc{
		Kind:          nest.Scout,
		Module:        "internal/spec",
		Authored:      "2026-01-01T00:00:00Z",
		Agent:         "fledge-context-scout",
		FledgeVersion: "0.2.1",
	}
	fm := d.Frontmatter()
	s := string(fm)

	if !strings.HasPrefix(s, "---\n") {
		t.Errorf("frontmatter must start with ---\\n, got: %q", s)
	}
	if !strings.HasSuffix(s, "---\n") {
		t.Errorf("frontmatter must end with ---\\n, got: %q", s)
	}

	// Keys must appear in fixed order: module, authored, agent, fledge_version.
	keys := []string{"module:", "authored:", "agent:", "fledge_version:"}
	prev := 0
	for _, k := range keys {
		i := strings.Index(s, k)
		if i < 0 {
			t.Errorf("key %q not found in scout frontmatter:\n%s", k, s)
			continue
		}
		if i < prev {
			t.Errorf("key %q out of order (at %d, prev=%d):\n%s", k, i, prev, s)
		}
		prev = i
	}
}

func TestRenderPreservesBody(t *testing.T) {
	body := []byte("\n# Architecture\n\nSome content here.\n")
	d := nest.Doc{
		Kind:          nest.Concern,
		Generated:     "2026-01-01T00:00:00Z",
		Commit:        "abc123",
		Agent:         "fledge-forager",
		FledgeVersion: "0.2.1",
		Body:          body,
	}
	rendered := d.Render()
	_, got, err := spec.SplitFrontmatter(rendered)
	if err != nil {
		t.Fatalf("SplitFrontmatter failed: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("body not preserved:\ngot:  %q\nwant: %q", got, body)
	}
}

func TestYAMLScalarQuoting(t *testing.T) {
	tests := []struct {
		input      string
		wantQuoted bool
	}{
		{"simple", false},
		{"plain-with-hyphens", false},
		{"needs: colon", true},   // colon is outside safe charset
		{"true", true},           // YAML boolean keyword
		{"null", true},           // YAML null keyword
		{"", true},               // empty string → ""
		{"123", true},            // numeric → quoted
	}
	for _, tt := range tests {
		got := spec.YAMLScalar(tt.input)
		isQuoted := strings.HasPrefix(got, `"`)
		if isQuoted != tt.wantQuoted {
			t.Errorf("YAMLScalar(%q) = %q; wantQuoted=%v", tt.input, got, tt.wantQuoted)
		}
	}
}
