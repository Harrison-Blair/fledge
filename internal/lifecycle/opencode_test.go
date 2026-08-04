package lifecycle

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/statedir"
)

func TestMergeOpenCodeConfig(t *testing.T) {
	t.Parallel()
	const path = "/project/.fledge/logs/session/policy.md"
	tests := []struct {
		name     string
		original string
		want     map[string]any
	}{
		{name: "empty", want: map[string]any{"instructions": []any{path}}},
		{
			name:     "populated",
			original: `{"model":"provider/model","permission":{"bash":"allow"},"instructions":["AGENTS.md"]}`,
			want: map[string]any{
				"model": "provider/model", "permission": map[string]any{"bash": "allow"},
				"instructions": []any{"AGENTS.md", path},
			},
		},
		{
			name:     "duplicate instruction",
			original: `{"instructions":["AGENTS.md","` + path + `"]}`,
			want:     map[string]any{"instructions": []any{"AGENTS.md", path}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			gotJSON, err := mergeOpenCodeConfig(test.original, path)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(gotJSON), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("merged config = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestMergeOpenCodeConfigRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	for _, original := range []string{"{", `[]`, `{"instructions":"AGENTS.md"}`, `{} {}`} {
		if _, err := mergeOpenCodeConfig(original, "/policy.md"); err == nil || !strings.Contains(err.Error(), openCodeConfigEnvironment) {
			t.Errorf("mergeOpenCodeConfig(%q) error = %v", original, err)
		}
	}
}

func TestPrepareOpenCodeRuntimeWritesProtectedSnapshots(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const instructions = "line one\nquoted \"text\" and 雪"
	const original = ` {"instructions":["AGENTS.md"],"theme":"dark"} `
	runtime, err := prepareOpenCodeRuntime(root, testSessionName, instructions, original)
	if err != nil {
		t.Fatal(err)
	}

	sessionDir := statedir.Session(root, testSessionName)
	instructionsPath := filepath.Join(sessionDir, openCodeInstructionsFile)
	assertProtectedFile(t, instructionsPath, instructions)
	assertProtectedFile(t, filepath.Join(sessionDir, openCodeEnvironmentFile), original)
	if got := runtime.paneEnvironment[openCodeConfigEnvironment]; got != original {
		t.Fatalf("pane environment = %q, want exact original %q", got, original)
	}
	var merged map[string]any
	if err := json.Unmarshal([]byte(runtime.serverEnvironment[openCodeConfigEnvironment]), &merged); err != nil {
		t.Fatal(err)
	}
	if got := merged["instructions"]; !reflect.DeepEqual(got, []any{"AGENTS.md", instructionsPath}) {
		t.Fatalf("merged instructions = %#v", got)
	}
}

func TestRemoveOpenCodeRuntimePreservesAuditLogs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := prepareOpenCodeRuntime(root, testSessionName, "policy", "{}"); err != nil {
		t.Fatal(err)
	}
	auditPath := filepath.Join(statedir.Session(root, testSessionName), "messages.jsonl")
	if err := os.WriteFile(auditPath, []byte("audit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeOpenCodeRuntime(root, testSessionName); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{openCodeInstructionsFile, openCodeEnvironmentFile} {
		if _, err := os.Stat(filepath.Join(statedir.Session(root, testSessionName), name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("runtime artifact %s error = %v, want removed", name, err)
		}
	}
	if contents, err := os.ReadFile(auditPath); err != nil || string(contents) != "audit\n" {
		t.Fatalf("audit contents = %q, %v; want preserved", contents, err)
	}
}

func assertProtectedFile(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != want {
		t.Fatalf("contents of %s = %q, %v; want %q", path, contents, err, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("permissions of %s = %o, want 600", path, got)
	}
}
