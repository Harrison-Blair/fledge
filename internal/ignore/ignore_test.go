package ignore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		name     string
		patterns string
		path     string
		isDir    bool
		want     bool
	}{
		{"comment and blank lines are inert", "# *.go\n\n", "main.go", false, false},
		{"bare name matches at root", ".fledge/", ".fledge", true, true},
		{"bare name matches at any depth", "vendor/", "a/b/vendor", true, true},
		{"dir-only pattern skips files", "vendor/", "vendor", false, false},
		{"extension glob at any depth", "*.log", "a/b/c.log", false, true},
		{"star does not cross a slash", "a/*.go", "a/b/c.go", false, false},
		{"leading slash anchors to root", "/bin", "bin", true, true},
		{"leading slash excludes deeper", "/bin", "pkg/bin", true, false},
		{"interior slash anchors", "docs/ref", "a/docs/ref", true, false},
		{"question mark matches one char", "?.go", "a.go", false, true},
		{"question mark does not cross a slash", "?.go", "ab.go", false, false},
		{"char class", "[abc].go", "b.go", false, true},
		{"negated char class", "[!abc].go", "b.go", false, false},

		{"negation re-includes, last match wins", "*.log\n!keep.log", "keep.log", false, false},
		{"order matters, re-exclude wins", "!keep.log\n*.log", "keep.log", false, true},
		{"negation leaves siblings ignored", "*.log\n!keep.log", "other.log", false, true},

		{"deep wildcard spans directories", "docs/**/tmp", "docs/a/b/tmp", false, true},
		{"deep wildcard spans zero directories", "docs/**/tmp", "docs/tmp", false, true},
		{"deep wildcard stays under its root", "docs/**/tmp", "other/tmp", false, false},
		{"leading deep wildcard", "**/tmp", "a/b/tmp", false, true},
		{"trailing deep wildcard excludes contents", "docs/**", "docs/a.md", false, true},
		{"trailing deep wildcard spares the dir itself", "docs/**", "docs", true, false},

		{"escaped hash is literal", `\#notes`, "#notes", false, true},
		{"escaped bang is literal", `\!notes`, "!notes", false, true},
		{"trailing spaces are trimmed", "*.log   ", "a.log", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := Parse(strings.NewReader(tt.patterns), t.TempDir())
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.patterns, err)
			}
			if got := m.Match(tt.path, tt.isDir); got != tt.want {
				t.Errorf("Match(%q, isDir=%v) with patterns %q = %v, want %v",
					tt.path, tt.isDir, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestParseFileMissingIsEmpty(t *testing.T) {
	root := t.TempDir()
	m, err := ParseFile(filepath.Join(root, "nope"), root)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if m.Match("anything", false) {
		t.Error("missing ignore file should ignore nothing")
	}
}

// write puts contents at root/name and returns root.
func write(t *testing.T, root, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIncludeDirective(t *testing.T) {
	tests := []struct {
		name      string
		gitignore string
		patterns  string
		path      string
		want      bool
	}{
		{"included patterns apply", "*.log\n", includePrefix + " .gitignore\n", "a.log", true},
		{"local patterns still apply", "*.log\n", includePrefix + " .gitignore\nbin\n", "bin", true},
		{"a later line overrides the include", "*.log\n", includePrefix + " .gitignore\n!keep.log\n", "keep.log", false},
		{"an earlier line loses to the include", "*.log\n", "!keep.log\n" + includePrefix + " .gitignore\n", "keep.log", true},
		{"tab separator works", "*.log\n", includePrefix + "\t.gitignore\n", "a.log", true},
		{"spaced hash stays a comment", "*.log\n", "# include .gitignore\n", "a.log", false},
		{"non-separator suffix stays a comment", "*.log\n", includePrefix + "s-are-neat\n", "a.log", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, ".gitignore", tt.gitignore)

			m, err := Parse(strings.NewReader(tt.patterns), root)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.patterns, err)
			}
			if got := m.Match(tt.path, false); got != tt.want {
				t.Errorf("Match(%q) with patterns %q = %v, want %v", tt.path, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestIncludeNested(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a", includePrefix+" b\n")
	write(t, root, "b", "*.log\n")

	m, err := ParseFile(filepath.Join(root, "a"), root)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if !m.Match("x.log", false) {
		t.Error("patterns from a nested include should apply")
	}
}

func TestIncludeErrors(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"missing target", map[string]string{"a": includePrefix + " nope\n"}, "no such file"},
		{"self cycle", map[string]string{"a": includePrefix + " a\n"}, "include cycle"},
		{"mutual cycle", map[string]string{"a": includePrefix + " b\n", "b": includePrefix + " a\n"}, "include cycle"},
		{"no path given", map[string]string{"a": includePrefix + " \n"}, "needs a path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, contents := range tt.files {
				write(t, root, name, contents)
			}

			_, err := ParseFile(filepath.Join(root, "a"), root)
			if err == nil {
				t.Fatalf("ParseFile: want error containing %q, got nil", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ParseFile: error %q, want it to contain %q", err, tt.want)
			}
		})
	}
}
