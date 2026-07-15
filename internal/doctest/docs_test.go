// Package doctest holds small structural tests over root documentation
// files (README.md, RELEASING.md) — substring/section checks, not full
// snapshots, so ordinary prose edits don't break them.
package doctest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readRoot reads a file relative to the repo root (two levels up from this
// package: internal/doctest -> internal -> <root>).
func readRoot(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// commandsSection extracts the README's "## Commands" section (up to the
// next "## " heading) so assertions about the command table don't
// accidentally match unrelated prose elsewhere in the file.
func commandsSection(t *testing.T, readme string) string {
	t.Helper()
	start := strings.Index(readme, "## Commands")
	if start == -1 {
		t.Fatalf("README.md has no '## Commands' section")
	}
	rest := readme[start+len("## Commands"):]
	end := strings.Index(rest, "\n## ")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

// upgradingSection extracts the README's "## Upgrading" section (up to the
// next "## " heading).
func upgradingSection(t *testing.T, readme string) string {
	t.Helper()
	start := strings.Index(readme, "## Upgrading")
	if start == -1 {
		t.Fatalf("README.md has no '## Upgrading' section")
	}
	rest := readme[start+len("## Upgrading"):]
	end := strings.Index(rest, "\n## ")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

// TestReadmeDocumentsUpdateCommand pins FC-5: README's Commands table
// references `fledge update`, and the Upgrading section covers binary
// self-update (as distinct from scaffold refresh).
func TestReadmeDocumentsUpdateCommand(t *testing.T) {
	readme := readRoot(t, "README.md")

	cmds := commandsSection(t, readme)
	if !strings.Contains(cmds, "fledge update") {
		t.Errorf("README.md Commands section does not mention `fledge update`:\n%s", cmds)
	}

	upgrading := upgradingSection(t, readme)
	if !strings.Contains(upgrading, "fledge update") {
		t.Errorf("README.md Upgrading section does not mention `fledge update` (binary self-update):\n%s", upgrading)
	}
}

// TestReleasingDocCoversScaffoldRefresh pins FC-6: RELEASING.md exists at
// the repo root and documents the fledge init --refresh scaffold-stamp
// step of the release process.
func TestReleasingDocCoversScaffoldRefresh(t *testing.T) {
	releasing := readRoot(t, "RELEASING.md")

	if !strings.Contains(releasing, "fledge init --refresh") {
		t.Errorf("RELEASING.md does not mention `fledge init --refresh`")
	}
	if !strings.Contains(releasing, "scaffold.json") {
		t.Errorf("RELEASING.md does not mention committing the scaffold.json stamp")
	}
}
