// Package scaffold creates and refreshes the .fledge directory.
package scaffold

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/ignore"
)

// DirName is the workspace directory fledge keeps its state in.
const DirName = ".fledge"

// subdirs are created under DirName on every init. Slash-separated; parents
// are created implicitly.
var subdirs = []string{
	"pluma/plumage",
	"pluma/feathers",
	"locks",
	"context",
	"flocks",
}

// IgnoreName is the ignore file fledge keeps inside DirName. Its patterns are
// relative to the workspace root, not to DirName.
const IgnoreName = ".fledgeignore"

const ignoreTemplate = `# Paths fledge ignores, one glob per line.
# Syntax follows .gitignore conventions.
#
# "#include <path>" splices another ignore file in at that point, resolved
# from this workspace root. Uncomment the next line to also honor .gitignore:
# #include .gitignore
.fledge/
.git/
`

// AgentsName is the agent config file fledge stubs out inside DirName. It must
// stay equal to agentcfg.FileName, which reads it; agentcfg imports this
// package for DirName, so the constant cannot live there without a cycle.
const AgentsName = "agents.json"

// catalogName must stay equal to agentcfg.CatalogName, mirrored here for the
// same cycle reason as AgentsName. The scaffold never writes the catalog —
// init regenerates it wholesale from the installed integrations — but it is
// per-machine state, so it belongs in .gitignore with the other runtime paths.
const catalogName = "catalog.json"

// agentsTemplate ships one illustrative entry rather than a comment, which
// JSON has no room for. The operator edits or replaces it.
const agentsTemplate = `{
  "example": {
    "integration": "claude",
    "permission_mode": "plan"
  }
}
`

// Ensure creates the .fledge tree under root, creating anything missing and
// leaving anything that already exists untouched. It reports whether the
// .fledge directory was already present, so callers can tell the user this was
// a refresh rather than a first-time init.
func Ensure(root string) (existed bool, err error) {
	base := filepath.Join(root, DirName)

	if _, err := os.Stat(base); err == nil {
		existed = true
	} else if !os.IsNotExist(err) {
		return false, err
	}

	for _, sub := range subdirs {
		if err := os.MkdirAll(filepath.Join(base, filepath.FromSlash(sub)), 0o755); err != nil {
			return existed, err
		}
	}

	// Never clobber a file the user has edited.
	if err := writeIfAbsent(filepath.Join(base, IgnoreName), ignoreTemplate); err != nil {
		return existed, err
	}
	if err := writeIfAbsent(filepath.Join(base, AgentsName), agentsTemplate); err != nil {
		return existed, err
	}

	return existed, nil
}

// GitignoreEntries are the runtime-state paths appended to an existing
// .gitignore on init: per-session state that should never be committed.
var GitignoreEntries = []string{
	DirName + "/locks/",
	DirName + "/flocks/",
	DirName + "/" + catalogName,
}

// gitignoreHeader titles the block of GitignoreEntries in a .gitignore, and
// is how a later init finds the block to update it in place.
const gitignoreHeader = "# fledge"

// EnsureGitignore appends GitignoreEntries to root's .gitignore, skipping any
// whose path the file already ignores (a broader pattern like ".fledge/"
// counts). A missing .gitignore is left missing — fledge does not impose git
// conventions on a tree that has none. It returns the entries it appended.
func EnsureGitignore(root string) ([]string, error) {
	name := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	m, err := ignore.Parse(strings.NewReader(string(data)), root)
	if err != nil {
		return nil, err
	}

	var added []string
	for _, entry := range GitignoreEntries {
		p := strings.TrimSuffix(entry, "/")
		covered := false
		if strings.HasSuffix(entry, "/") {
			covered = ignoredAsDir(m, p)
		} else {
			// A file entry is covered by an exact match or by any ignored
			// ancestor directory.
			covered = m.Match(p, false) || ignoredAsDir(m, path.Dir(p))
		}
		if !covered {
			added = append(added, entry)
		}
	}
	if len(added) == 0 {
		return nil, nil
	}

	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	if start := headerLine(content); start >= 0 {
		// The block exists: grow it in place rather than appending a
		// second one. It ends at the first blank line.
		lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
		end := start + 1
		for end < len(lines) && strings.TrimSpace(lines[end]) != "" {
			end++
		}
		updated := make([]string, 0, len(lines)+len(added))
		updated = append(updated, lines[:end]...)
		updated = append(updated, added...)
		updated = append(updated, lines[end:]...)
		content = strings.Join(updated, "\n") + "\n"
	} else {
		// One blank line sets the block apart, unless the file already
		// ends with one (or is empty).
		if content != "" && !strings.HasSuffix(content, "\n\n") {
			content += "\n"
		}
		content += gitignoreHeader + "\n" + strings.Join(added, "\n") + "\n"
	}

	return added, os.WriteFile(name, []byte(content), 0o644)
}

// headerLine returns the index of the gitignoreHeader line, or -1.
func headerLine(content string) int {
	for i, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == gitignoreHeader {
			return i
		}
	}
	return -1
}

// ignoredAsDir reports whether the slash-separated directory p, or any
// ancestor, is matched. Ancestors matter because ignoring a directory ignores
// everything beneath it, which Matcher does not model on its own.
func ignoredAsDir(m *ignore.Matcher, p string) bool {
	for ; p != "."; p = path.Dir(p) {
		if m.Match(p, true) {
			return true
		}
	}
	return false
}

// writeIfAbsent seeds path with content, leaving an existing file alone.
func writeIfAbsent(path, content string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.WriteFile(path, []byte(content), 0o644)
	} else if err != nil {
		return err
	}
	return nil
}
