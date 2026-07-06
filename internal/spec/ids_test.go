package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNextID(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{"empty dir", nil, "TASK-001"},
		{"sequential", []string{"TASK-001-a.md", "TASK-002-b.md"}, "TASK-003"},
		{"gaps use max not count", []string{"TASK-001-a.md", "TASK-007-b.md"}, "TASK-008"},
		{"wide ids keep width", []string{"TASK-1042-a.md"}, "TASK-1043"},
		{"ignores non-matching files", []string{"README.md", "TASK-002-b.md", "notes.txt"}, "TASK-003"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := NextID(dir, "TASK")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("NextID = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNextIDMissingDir(t *testing.T) {
	got, err := NextID(filepath.Join(t.TempDir(), "nope"), "REQ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "REQ-001" {
		t.Errorf("NextID = %q, want REQ-001", got)
	}
}

func TestKebab(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Deterministic CLI", "deterministic-cli"},
		{"Wire graph: waves & cycles", "wire-graph-waves-cycles"},
		{"  spaces   everywhere  ", "spaces-everywhere"},
		{"already-kebab", "already-kebab"},
		{"Ünïcode Títle", "ünïcode-títle"},
		{"123 numbers", "123-numbers"},
	}
	for _, tt := range tests {
		if got := Kebab(tt.in); got != tt.want {
			t.Errorf("Kebab(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
