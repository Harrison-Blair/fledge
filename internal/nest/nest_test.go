package nest_test

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestStampPreservesBodyAndDropsUnknownKeys(t *testing.T) {
	// Build a concern doc with an unknown frontmatter key and a recognizable body.
	staleFM := "---\ngenerated: 2020-01-01T00:00:00Z\ncommit: oldsha\nagent: my-agent\nfledge_version: 0.0.1\nstale_key: bad-value\n---\n"
	body := []byte("\n# Architecture\n\nSome preserved content.\n")
	content := append([]byte(staleFM), body...)

	// Write to a temp dir that looks like .fledge/nest/.
	tmp := t.TempDir()
	nestDir := filepath.Join(tmp, ".fledge", "nest")
	if err := os.MkdirAll(nestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nestDir, "architecture.md")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := nest.RefreshDoc(content, nest.Concern, "2026-01-01T00:00:00Z", "newsha", "", "0.3.0")
	if err != nil {
		t.Fatalf("RefreshDoc: %v", err)
	}

	s := string(got)

	// Derived fields must be refreshed.
	if !strings.Contains(s, "generated: 2026-01-01T00:00:00Z") {
		t.Errorf("generated not refreshed:\n%s", s)
	}
	if !strings.Contains(s, "commit: newsha") {
		t.Errorf("commit not refreshed:\n%s", s)
	}
	if !strings.Contains(s, "fledge_version: 0.3.0") {
		t.Errorf("fledge_version not refreshed:\n%s", s)
	}

	// Agent must be preserved (no override supplied).
	if !strings.Contains(s, "agent: my-agent") {
		t.Errorf("agent not preserved:\n%s", s)
	}

	// Unknown key must be dropped.
	if strings.Contains(s, "stale_key") {
		t.Errorf("stale_key not dropped:\n%s", s)
	}

	// Body must be byte-identical.
	_, gotBody, err := spec.SplitFrontmatter(got)
	if err != nil {
		t.Fatalf("SplitFrontmatter on result: %v", err)
	}
	if !bytes.Equal(gotBody, body) {
		t.Errorf("body not preserved:\ngot:  %q\nwant: %q", gotBody, body)
	}

	// --agent override replaces stored agent.
	got2, err := nest.RefreshDoc(content, nest.Concern, "2026-01-01T00:00:00Z", "newsha", "override-agent", "0.3.0")
	if err != nil {
		t.Fatalf("RefreshDoc with override: %v", err)
	}
	s2 := string(got2)
	if !strings.Contains(s2, "agent: override-agent") {
		t.Errorf("agent override not applied:\n%s", s2)
	}
	if strings.Contains(s2, "agent: my-agent") {
		t.Errorf("stored agent leaked despite override:\n%s", s2)
	}

	// Scout kind: module preserved, authored refreshed, no generated/commit fields.
	scoutFM := "---\nmodule: cli\nauthored: 2020-01-01T00:00:00Z\nagent: fledge-context-scout\nfledge_version: 0.0.1\nextra: dropped\n---\n"
	scoutBody := []byte("\n# Module: cli\n\nSome scout content.\n")
	scoutContent := append([]byte(scoutFM), scoutBody...)

	got3, err := nest.RefreshDoc(scoutContent, nest.Scout, "2026-06-01T00:00:00Z", "", "", "0.3.0")
	if err != nil {
		t.Fatalf("RefreshDoc scout: %v", err)
	}
	s3 := string(got3)
	if !strings.Contains(s3, "module: cli") {
		t.Errorf("scout module not preserved:\n%s", s3)
	}
	if !strings.Contains(s3, "authored: 2026-06-01T00:00:00Z") {
		t.Errorf("scout authored not refreshed:\n%s", s3)
	}
	if strings.Contains(s3, "generated:") {
		t.Errorf("scout result must not have generated field:\n%s", s3)
	}
	if strings.Contains(s3, "extra:") {
		t.Errorf("extra key not dropped from scout:\n%s", s3)
	}
	_, gotBody3, err := spec.SplitFrontmatter(got3)
	if err != nil {
		t.Fatalf("SplitFrontmatter on scout result: %v", err)
	}
	if !bytes.Equal(gotBody3, scoutBody) {
		t.Errorf("scout body not preserved:\ngot:  %q\nwant: %q", gotBody3, scoutBody)
	}
}

// writeDoc renders a concern doc with the given body and commit into dir.
func writeDoc(t *testing.T, dir, name, commit string, body []byte) {
	t.Helper()
	d := &nest.Doc{
		Kind:          nest.Concern,
		Generated:     "2026-01-01T00:00:00Z",
		Commit:        commit,
		Agent:         "fledge-forager",
		FledgeVersion: "0.5.4",
		Body:          body,
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), d.Render(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// scaffold writes every concern doc as its template stub, stamped to commit.
func scaffold(t *testing.T, dir, commit string) {
	t.Helper()
	for _, name := range nest.ConcernDocs {
		if name == "index" {
			writeDoc(t, dir, name, commit, nest.IndexBody())
		} else {
			writeDoc(t, dir, name, commit, nest.ConcernBody(nest.Title(name)))
		}
	}
}

func TestIsStub(t *testing.T) {
	if !nest.IsStub("architecture", nest.ConcernBody(nest.Title("architecture"))) {
		t.Error("freshly rendered concern body must be detected as a stub")
	}
	if !nest.IsStub("index", nest.IndexBody()) {
		t.Error("freshly rendered index body must be detected as a stub")
	}
	if nest.IsStub("architecture", []byte("\n# Architecture\n\nReal synthesized content.\n")) {
		t.Error("filled-in body must not be detected as a stub")
	}
	// A concern body under the wrong doc name is not that doc's stub.
	if nest.IsStub("modules", nest.ConcernBody(nest.Title("architecture"))) {
		t.Error("architecture stub must not count as the modules stub")
	}
}

func TestStatus(t *testing.T) {
	const head = "abc123def456"

	// Fresh scaffold: every doc is a stub, index stamped to HEAD → incomplete.
	dir := t.TempDir()
	scaffold(t, dir, head)
	res := nest.Status(dir, head)
	if res.Complete {
		t.Errorf("freshly scaffolded nest must be incomplete: %+v", res)
	}
	if len(res.StubDocs) != len(nest.ConcernDocs) {
		t.Errorf("all %d docs should be stubs, got %v", len(nest.ConcernDocs), res.StubDocs)
	}
	if !res.IndexCommitMatches {
		t.Errorf("scaffolded index is stamped to HEAD, want IndexCommitMatches=true: %+v", res)
	}

	// Fill every doc with real content, index stamped to HEAD → complete.
	dir = t.TempDir()
	for _, name := range nest.ConcernDocs {
		writeDoc(t, dir, name, head, []byte("\n# "+nest.Title(name)+"\n\nReal synthesized content.\n"))
	}
	res = nest.Status(dir, head)
	if !res.Complete {
		t.Errorf("fully synthesized nest must be complete: %+v", res)
	}
	if len(res.StubDocs) != 0 || len(res.MissingDocs) != 0 {
		t.Errorf("no stubs/missing expected: %+v", res)
	}

	// Filled docs but index stamped to an old commit → incomplete via freshness.
	dir = t.TempDir()
	for _, name := range nest.ConcernDocs {
		commit := head
		if name == "index" {
			commit = "staleSHA"
		}
		writeDoc(t, dir, name, commit, []byte("\n# "+nest.Title(name)+"\n\nReal content.\n"))
	}
	res = nest.Status(dir, head)
	if res.Complete {
		t.Errorf("stale index must make the nest incomplete: %+v", res)
	}
	if res.IndexCommitMatches {
		t.Errorf("stale index commit must not match HEAD: %+v", res)
	}

	// Missing concern doc → listed in MissingDocs, incomplete.
	dir = t.TempDir()
	for _, name := range nest.ConcernDocs {
		if name == "domain" {
			continue // omit one doc
		}
		writeDoc(t, dir, name, head, []byte("\n# "+nest.Title(name)+"\n\nReal content.\n"))
	}
	res = nest.Status(dir, head)
	if res.Complete {
		t.Errorf("missing doc must make the nest incomplete: %+v", res)
	}
	found := false
	for _, m := range res.MissingDocs {
		if m == "domain" {
			found = true
		}
	}
	if !found {
		t.Errorf("domain should be reported missing: %+v", res)
	}
}

func TestYAMLScalarQuoting(t *testing.T) {
	tests := []struct {
		input      string
		wantQuoted bool
	}{
		{"simple", false},
		{"plain-with-hyphens", false},
		{"needs: colon", true}, // colon is outside safe charset
		{"true", true},         // YAML boolean keyword
		{"null", true},         // YAML null keyword
		{"", true},             // empty string → ""
		{"123", true},          // numeric → quoted
	}
	for _, tt := range tests {
		got := spec.YAMLScalar(tt.input)
		isQuoted := strings.HasPrefix(got, `"`)
		if isQuoted != tt.wantQuoted {
			t.Errorf("YAMLScalar(%q) = %q; wantQuoted=%v", tt.input, got, tt.wantQuoted)
		}
	}
}
