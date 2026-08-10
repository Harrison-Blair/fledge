package lifecycle

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
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
	instructionsPath := filepath.Join(root, ".fledge", "profiles", "generated", "orchestrator.md")
	const original = ` {"instructions":["AGENTS.md"],"theme":"dark"} `
	runtime, err := prepareOpenCodeRuntime(root, testSessionName, instructionsPath, original)
	if err != nil {
		t.Fatal(err)
	}

	tempSessionDir := fsutil.TempSession(root, testSessionName)
	assertProtectedFile(t, filepath.Join(tempSessionDir, openCodeEnvironmentFile), original)
	if _, err := os.Stat(filepath.Join(fsutil.Session(root, testSessionName), openCodeInstructionsFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("per-session prompt artifact error = %v, want absent", err)
	}
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
	if _, err := prepareOpenCodeRuntime(root, testSessionName, generatedPromptFile(root), "{}"); err != nil {
		t.Fatal(err)
	}
	auditPath := filepath.Join(fsutil.Session(root, testSessionName), "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(auditPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(auditPath, []byte("audit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeOpenCodeRuntime(root, testSessionName); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fsutil.TempSession(root, testSessionName), openCodeEnvironmentFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("runtime environment error = %v, want removed", err)
	}
	if contents, err := os.ReadFile(auditPath); err != nil || string(contents) != "audit\n" {
		t.Fatalf("audit contents = %q, %v; want preserved", contents, err)
	}
}

func TestOpenCodeLegacyRuntimeFallbackAndCleanup(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	legacyDir := fsutil.Session(root, testSessionName)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	const original = `{"legacy":true}`
	for name, contents := range map[string]string{
		openCodeEnvironmentFile:  original,
		openCodeInstructionsFile: "legacy prompt",
	} {
		if err := os.WriteFile(filepath.Join(legacyDir, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	environment, err := openCodePaneEnvironment(root, testSessionName)
	if err != nil || environment[openCodeConfigEnvironment] != original {
		t.Fatalf("legacy environment = %#v, %v", environment, err)
	}
	if err := removeOpenCodeRuntime(root, testSessionName); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{openCodeEnvironmentFile, openCodeInstructionsFile} {
		if _, err := os.Stat(filepath.Join(legacyDir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("legacy artifact %s error = %v, want removed", name, err)
		}
	}
}

func TestWriteProtectedFileTruncatesPreviousContents(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "runtime", openCodeEnvironmentFile)
	if err := writeProtectedFile(path, []byte("a long initial configuration value")); err != nil {
		t.Fatal(err)
	}
	// Overwriting with shorter contents must leave no suffix from the previous
	// value: the O_TRUNC open discards it.
	if err := writeProtectedFile(path, []byte("short")); err != nil {
		t.Fatal(err)
	}
	assertProtectedFile(t, path, "short")
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
