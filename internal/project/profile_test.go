package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
