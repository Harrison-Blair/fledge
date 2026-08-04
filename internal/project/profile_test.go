package project

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestEnsureGeneratedOrchestratorPromptReusesRefreshesAndProtectsFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Harnesses resolve the returned reference from their own working
	// directory, so it must be absolute.
	wantPath := filepath.Join(root, ".fledge", "profiles", "generated", "orchestrator.md")
	path, err := EnsureGeneratedOrchestratorPrompt(root, "first\npolicy")
	if err != nil || path != wantPath {
		t.Fatalf("EnsureGeneratedOrchestratorPrompt() = %q, %v; want %q", path, err, wantPath)
	}
	oldTime := time.Unix(123, 0)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureGeneratedOrchestratorPrompt(root, "first\npolicy"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || !info.ModTime().Equal(oldTime) {
		t.Fatalf("reused prompt mode/time = %o, %v; want 600 and %v", info.Mode().Perm(), info.ModTime(), oldTime)
	}
	if _, err := EnsureGeneratedOrchestratorPrompt(root, "refreshed\npolicy"); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "refreshed\npolicy" {
		t.Fatalf("refreshed prompt = %q, %v", contents, err)
	}
}

func TestEnsureGeneratedOrchestratorPromptRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	root := t.TempDir()
	path := filepath.Join(root, ".fledge", "profiles", "generated", "orchestrator.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureGeneratedOrchestratorPrompt(root, "replacement"); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("EnsureGeneratedOrchestratorPrompt() error = %v, want symlink rejection", err)
	}
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != "unchanged" {
		t.Fatalf("symlink target = %q, %v; want unchanged", contents, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("symlink target mode = %v; want 644", info.Mode().Perm())
	}
}

func TestLoadOrchestratorProfileAcceptsSupportedTOMLStrings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		contents     string
		instructions string
	}{
		{
			name:         "basic string and comments",
			contents:     "# profile\ninstructions = \"line one\\nline two\" # editable\nschema_version = 1\n",
			instructions: "line one\nline two",
		},
		{
			name:         "literal string",
			contents:     "schema_version=1\ninstructions='use \\ literally'\n",
			instructions: `use \ literally`,
		},
		{
			name:         "multiline basic string",
			contents:     "schema_version = 1\ninstructions = \"\"\"\r\nfirst\r\nsecond \"quoted\"\"\"\"\n",
			instructions: "first\nsecond \"quoted\"",
		},
		{
			name:         "multiline literal string",
			contents:     "instructions = '''\nfirst\\nsecond\n'''\nschema_version = 1\n",
			instructions: "first\\nsecond\n",
		},
		{
			name:         "multiline basic continuation",
			contents:     "schema_version=1\ninstructions=\"\"\"\nline one \\\n    line two\n\"\"\"\n",
			instructions: "line one line two\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeProfile(t, root, test.contents)

			profile, err := LoadOrchestratorProfile(root)
			if err != nil {
				t.Fatalf("LoadOrchestratorProfile() error = %v", err)
			}
			if profile.SchemaVersion != SchemaVersion || profile.Instructions != test.instructions {
				t.Errorf("profile = %#v, want version %d and instructions %q", profile, SchemaVersion, test.instructions)
			}
		})
	}
}

func TestLoadOrchestratorProfileRejectsInvalidSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
		want     string
	}{
		{name: "unknown key", contents: "schema_version=1\ninstructions='ok'\nmodel='x'\n", want: "unknown key"},
		{name: "duplicate key", contents: "schema_version=1\nschema_version=1\ninstructions='ok'\n", want: "duplicate key"},
		{name: "missing version", contents: "instructions='ok'\n", want: "missing key \"schema_version\""},
		{name: "missing instructions", contents: "schema_version=1\n", want: "missing key \"instructions\""},
		{name: "unsupported version", contents: "schema_version=2\ninstructions='ok'\n", want: "unsupported schema_version 2"},
		{name: "empty instructions", contents: "schema_version=1\ninstructions='  '\n", want: "must not be empty"},
		{name: "non-integer version", contents: "schema_version='1'\ninstructions='ok'\n", want: "expected a positive integer"},
		{name: "unquoted instructions", contents: "schema_version=1\ninstructions=hello\n", want: "expected a quoted string"},
		{name: "unterminated string", contents: "schema_version=1\ninstructions='hello\n", want: "unterminated string"},
		{name: "bad escape", contents: "schema_version=1\ninstructions=\"bad\\q\"\n", want: "invalid string escape"},
		{name: "trailing tokens", contents: "schema_version=1 nope\ninstructions='ok'\n", want: "unexpected content"},
		{name: "table syntax", contents: "[profile]\nschema_version=1\ninstructions='ok'\n", want: "expected a bare key"},
		{name: "literal control character", contents: "schema_version=1\ninstructions='bad\x00value'\n", want: "invalid control character"},
		{name: "literal invalid UTF-8", contents: "schema_version=1\ninstructions='bad\xffvalue'\n", want: "invalid UTF-8"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeProfile(t, root, test.contents)

			_, err := LoadOrchestratorProfile(root)
			if err == nil {
				t.Fatal("LoadOrchestratorProfile() error = nil, want error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("LoadOrchestratorProfile() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestLoadOrchestratorProfileReportsMissingFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_, err := LoadOrchestratorProfile(root)
	if err == nil {
		t.Fatal("LoadOrchestratorProfile() error = nil, want error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		// The contextual error must retain the filesystem cause for callers.
		t.Errorf("LoadOrchestratorProfile() error = %v, want os.ErrNotExist cause", err)
	}
}

func TestProfilePathUsesProjectRoot(t *testing.T) {
	t.Parallel()

	root := filepath.Join("root", "project")
	want := filepath.Join(root, stateDirectory, profilesDir, profileFilename)
	if got := profilePath(root); got != want {
		t.Errorf("profilePath() = %q, want %q", got, want)
	}
}
