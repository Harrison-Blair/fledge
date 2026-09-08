package record

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"fledge/internal/profile"
	"fledge/internal/session/sessiontest"
	"fledge/internal/session/types"
)

func TestProfileSnapshotRoundTripIsExact(t *testing.T) {
	root := sessiontest.NewProject(t)
	configured := profile.Profile{
		Name:         "fledge-test",
		Description:  "Test snapshot",
		Instructions: "line one\nline two without a trailing newline",
		Defaults: profile.Defaults{
			Harness: "claude",
			Model:   "claude-test",
			Args:    []string{"--effort", "high"},
		},
	}
	choice := types.AgentChoice{
		Harness: "codex",
		Model:   "gpt-test",
		Args:    []string{"-c", `model_reasoning_effort="high"`},
		Profile: &configured,
	}

	record, err := Create(root, choice, MaxSessionLength, nil, bytes.NewReader([]byte{1, 2, 3, 4}), time.Now())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	wantProfileJSON := `{"schema_version":1,"name":"fledge-test","description":"Test snapshot","instructions":"line one\nline two without a trailing newline","defaults":{"harness":"claude","model":"claude-test","args":["--effort","high"]}}` + "\n"
	assertFileContents(t, filepath.Join(record.Path, profileFileName), wantProfileJSON)
	assertFileContents(t, filepath.Join(record.Path, instructionsFileName), configured.Instructions)
	assertFileContents(t, filepath.Join(record.Path, "pending.json"), `{"schema_version":1,"harness":"codex","model":"gpt-test","args":["-c","model_reasoning_effort=\"high\""]}`+"\n")

	records, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Load() returned %d records, want 1", len(records))
	}
	loaded := records[0]
	if !sameProfile(loaded.Profile, &configured) {
		t.Fatalf("Load() profile = %#v, want %#v", loaded.Profile, configured)
	}
	if loaded.PendingChoice == nil || loaded.PendingChoice.Harness != choice.Harness || loaded.PendingChoice.Model != choice.Model ||
		!reflect.DeepEqual(loaded.PendingChoice.Args, choice.Args) || !sameProfile(loaded.PendingChoice.Profile, &configured) {
		t.Fatalf("Load() pending choice = %#v, want %#v", loaded.PendingChoice, choice)
	}
	path, err := ProfileInstructionsPath(loaded)
	if err != nil {
		t.Fatalf("ProfileInstructionsPath() error = %v", err)
	}
	if want := filepath.Join(record.Path, instructionsFileName); path != want {
		t.Fatalf("ProfileInstructionsPath() = %q, want %q", path, want)
	}
}

func TestCreateWithoutProfilePublishesNoProfileSidecars(t *testing.T) {
	root := sessiontest.NewProject(t)
	record, err := Create(root, types.AgentChoice{Harness: "pi"}, MaxSessionLength, nil, bytes.NewReader([]byte{1, 2, 3, 4}), time.Now())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	for _, name := range []string{profileFileName, instructionsFileName} {
		if _, err := os.Lstat(filepath.Join(record.Path, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Lstat(%s) error = %v, want not exist", name, err)
		}
	}
	path, err := ProfileInstructionsPath(record)
	if err != nil || path != "" {
		t.Fatalf("ProfileInstructionsPath() = %q, %v, want empty path and nil error", path, err)
	}
}

func TestClearPendingRetainsProfileSnapshot(t *testing.T) {
	root := sessiontest.NewProject(t)
	configured := profile.Profile{Name: "fledge-test", Description: "Pinned", Instructions: "remain pinned\n"}
	record, err := Create(root, types.AgentChoice{Harness: "claude", Profile: &configured}, MaxSessionLength, nil, bytes.NewReader([]byte{1, 2, 3, 4}), time.Now())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := ClearPending(record); err != nil {
		t.Fatalf("ClearPending() error = %v", err)
	}

	records, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(records) != 1 || records[0].PendingChoice != nil || !sameProfile(records[0].Profile, &configured) {
		t.Fatalf("Load() after ClearPending() = %#v", records)
	}
	path, err := ProfileInstructionsPath(records[0])
	if err != nil {
		t.Fatalf("ProfileInstructionsPath() error = %v", err)
	}
	assertFileContents(t, path, configured.Instructions)
}

func TestProfileSnapshotIsIndependentFromManagedRegistry(t *testing.T) {
	root := sessiontest.NewProject(t)
	current, ok := profile.Get(profile.OrchestratorName)
	if !ok {
		t.Fatal("managed orchestrator profile is missing")
	}
	pinned := current
	pinned.Description = "session-pinned old description"
	pinned.Instructions = "session-pinned old instructions\n"
	pinned.Defaults.Args = []string{"--session-pinned"}

	record, err := Create(root, types.AgentChoice{Harness: "codex", Profile: &pinned}, MaxSessionLength, nil, bytes.NewReader([]byte{1, 2, 3, 4}), time.Now())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	// Mutating the caller's snapshot after publication simulates a later managed
	// registry version and must not change the session artifact.
	pinned.Description = current.Description
	pinned.Instructions = current.Instructions
	pinned.Defaults.Args[0] = "--changed"

	records, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	loaded := records[0]
	if loaded.Profile.Description != "session-pinned old description" || loaded.Profile.Instructions != "session-pinned old instructions\n" ||
		!reflect.DeepEqual(loaded.Profile.Defaults.Args, []string{"--session-pinned"}) {
		t.Fatalf("Load() profile followed current value instead of pinned snapshot: %#v", loaded.Profile)
	}
	path, err := ProfileInstructionsPath(loaded)
	if err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, path, "session-pinned old instructions\n")
	if record.Profile.Instructions != "session-pinned old instructions\n" || record.PendingChoice.Profile.Instructions != "session-pinned old instructions\n" {
		t.Fatal("Create() returned profile storage shared with caller")
	}
}

func TestLoadRejectsInvalidProfileSidecars(t *testing.T) {
	valid := `{"schema_version":1,"name":"fledge-test","description":"test","instructions":"pinned\n","defaults":{"harness":"","model":"","args":[]}}`
	tests := []struct {
		name         string
		profileJSON  string
		instructions *string
		want         string
	}{
		{name: "unknown field", profileJSON: `{"schema_version":1,"name":"fledge-test","description":"test","instructions":"pinned\n","defaults":{"harness":"","model":"","args":[]},"extra":true}`, instructions: stringPointer("pinned\n"), want: `unknown field "extra"`},
		{name: "duplicate field", profileJSON: `{"schema_version":1,"name":"fledge-test","name":"again","description":"test","instructions":"pinned\n","defaults":{"harness":"","model":"","args":[]}}`, instructions: stringPointer("pinned\n"), want: `duplicate field "name"`},
		{name: "null field", profileJSON: `{"schema_version":1,"name":null,"description":"test","instructions":"pinned\n","defaults":{"harness":"","model":"","args":[]}}`, instructions: stringPointer("pinned\n"), want: "profile name must not be null"},
		{name: "nested unknown field", profileJSON: `{"schema_version":1,"name":"fledge-test","description":"test","instructions":"pinned\n","defaults":{"harness":"","model":"","args":[],"extra":true}}`, instructions: stringPointer("pinned\n"), want: `unknown field "extra"`},
		{name: "nested duplicate field", profileJSON: `{"schema_version":1,"name":"fledge-test","description":"test","instructions":"pinned\n","defaults":{"harness":"","harness":"","model":"","args":[]}}`, instructions: stringPointer("pinned\n"), want: `duplicate field "harness"`},
		{name: "nested null field", profileJSON: `{"schema_version":1,"name":"fledge-test","description":"test","instructions":"pinned\n","defaults":{"harness":"","model":"","args":null}}`, instructions: stringPointer("pinned\n"), want: "profile defaults args must not be null"},
		{name: "null argument", profileJSON: `{"schema_version":1,"name":"fledge-test","description":"test","instructions":"pinned\n","defaults":{"harness":"","model":"","args":[null]}}`, instructions: stringPointer("pinned\n"), want: "profile defaults args[0] must not be null"},
		{name: "corrupt artifact", profileJSON: valid, instructions: stringPointer("changed\n"), want: "does not match"},
		{name: "missing artifact", profileJSON: valid, want: "must either both exist or both be absent"},
		{name: "orphan artifact", instructions: stringPointer("pinned\n"), want: "must either both exist or both be absent"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := sessiontest.NewProject(t)
			dir := sessiontest.WriteRecord(t, root, "record", `{"schema_version":1,"herdr_session_name":"record","created_at":"2026-08-24T14:15:16Z"}`)
			if test.profileJSON != "" {
				if err := os.WriteFile(filepath.Join(dir, profileFileName), []byte(test.profileJSON), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if test.instructions != nil {
				if err := os.WriteFile(filepath.Join(dir, instructionsFileName), []byte(*test.instructions), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Load(root); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestLoadRejectsProfileSymlinksAndNonRegularFiles(t *testing.T) {
	for _, name := range []string{profileFileName, instructionsFileName} {
		t.Run(name+" symlink", func(t *testing.T) {
			root := sessiontest.NewProject(t)
			dir := sessiontest.WriteRecord(t, root, "record", `{"schema_version":1,"herdr_session_name":"record","created_at":"2026-08-24T14:15:16Z"}`)
			target := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "is a symlink") {
				t.Fatalf("Load() error = %v, want symlink rejection", err)
			}
		})

		t.Run(name+" directory", func(t *testing.T) {
			root := sessiontest.NewProject(t)
			dir := sessiontest.WriteRecord(t, root, "record", `{"schema_version":1,"herdr_session_name":"record","created_at":"2026-08-24T14:15:16Z"}`)
			if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "not a regular file") {
				t.Fatalf("Load() error = %v, want nonregular rejection", err)
			}
		})
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("ReadFile(%q) = %q, want %q", path, data, want)
	}
}

func stringPointer(value string) *string {
	return &value
}
