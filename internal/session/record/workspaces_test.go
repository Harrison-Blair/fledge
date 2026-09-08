package record

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWorkspacesAbsentReturnsEmptyMap(t *testing.T) {
	ids, err := ReadWorkspaces(t.TempDir())
	if err != nil {
		t.Fatalf("ReadWorkspaces() error = %v", err)
	}
	if ids == nil || len(ids) != 0 {
		t.Fatalf("ReadWorkspaces() = %#v, want empty non-nil map", ids)
	}
}

func TestWorkspacesRoundTripDeterministicAndReplacement(t *testing.T) {
	recordPath := t.TempDir()
	if err := WriteWorkspaces(recordPath, map[string]string{"worker": "ws-2", "manager": "ws-1"}); err != nil {
		t.Fatalf("WriteWorkspaces(first): %v", err)
	}

	// Map keys are marshaled in sorted order, so the encoding is deterministic
	// regardless of the caller's insertion order.
	data, err := os.ReadFile(filepath.Join(recordPath, WorkspacesFileName))
	if err != nil {
		t.Fatal(err)
	}
	wantJSON := `{"schema_version":1,"workspaces":{"manager":"ws-1","worker":"ws-2"}}` + "\n"
	if string(data) != wantJSON {
		t.Fatalf("workspaces.json = %q, want %q", data, wantJSON)
	}

	info, err := os.Lstat(filepath.Join(recordPath, WorkspacesFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o644 {
		t.Fatalf("workspaces.json mode = %v", info.Mode())
	}

	// Replacement fully supersedes the previous map: the dropped role is gone.
	if err := WriteWorkspaces(recordPath, map[string]string{"manager": "ws-9"}); err != nil {
		t.Fatalf("WriteWorkspaces(second): %v", err)
	}
	got, err := ReadWorkspaces(recordPath)
	if err != nil {
		t.Fatalf("ReadWorkspaces(): %v", err)
	}
	if len(got) != 1 || got["manager"] != "ws-9" {
		t.Fatalf("ReadWorkspaces() = %#v, want {manager:ws-9}", got)
	}

	matches, err := filepath.Glob(filepath.Join(recordPath, ".workspaces-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary workspaces files = %q, error = %v", matches, err)
	}
}

func TestWriteWorkspacesEmptyMapWritesEmptyObject(t *testing.T) {
	recordPath := t.TempDir()
	if err := WriteWorkspaces(recordPath, nil); err != nil {
		t.Fatalf("WriteWorkspaces(nil): %v", err)
	}
	data, err := os.ReadFile(filepath.Join(recordPath, WorkspacesFileName))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"schema_version":1,"workspaces":{}}` + "\n"; string(data) != want {
		t.Fatalf("workspaces.json = %q, want %q", data, want)
	}
	ids, err := ReadWorkspaces(recordPath)
	if err != nil {
		t.Fatalf("ReadWorkspaces(): %v", err)
	}
	if ids == nil || len(ids) != 0 {
		t.Fatalf("ReadWorkspaces() = %#v, want empty non-nil map", ids)
	}
}

func TestWorkspacesPreservesArbitraryValidRoles(t *testing.T) {
	recordPath := t.TempDir()
	// Roles the current code has no constant for still round-trip intact, so
	// future managed roles need no schema changes. Also exercises the pattern
	// boundaries: single letter, and a full 32-character role.
	roles := map[string]string{
		"future-role-7":               "ws-a",
		"a":                           "ws-b",
		"z" + strings.Repeat("a", 31): "ws-c", // 32 chars: the pattern's upper bound
	}
	if err := WriteWorkspaces(recordPath, roles); err != nil {
		t.Fatalf("WriteWorkspaces(): %v", err)
	}
	got, err := ReadWorkspaces(recordPath)
	if err != nil {
		t.Fatalf("ReadWorkspaces(): %v", err)
	}
	if len(got) != len(roles) {
		t.Fatalf("ReadWorkspaces() = %#v, want %#v", got, roles)
	}
	for role, id := range roles {
		if got[role] != id {
			t.Fatalf("ReadWorkspaces()[%q] = %q, want %q", role, got[role], id)
		}
	}
}

func TestReadWorkspacesReturnsIndependentMaps(t *testing.T) {
	recordPath := t.TempDir()
	if err := WriteWorkspaces(recordPath, map[string]string{"manager": "ws-1"}); err != nil {
		t.Fatalf("WriteWorkspaces(): %v", err)
	}
	first, err := ReadWorkspaces(recordPath)
	if err != nil {
		t.Fatalf("ReadWorkspaces(first): %v", err)
	}
	// No shared cache: mutating one read's result cannot leak into the next.
	first["manager"] = "tampered"
	first["injected"] = "ws-x"
	second, err := ReadWorkspaces(recordPath)
	if err != nil {
		t.Fatalf("ReadWorkspaces(second): %v", err)
	}
	if len(second) != 1 || second["manager"] != "ws-1" {
		t.Fatalf("ReadWorkspaces(second) = %#v, want {manager:ws-1}", second)
	}
}

func TestWriteWorkspacesRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		ids  map[string]string
		want string
	}{
		{name: "uppercase role", ids: map[string]string{"Manager": "ws-1"}, want: `invalid workspace role "Manager"`},
		{name: "leading digit", ids: map[string]string{"1role": "ws-1"}, want: "invalid workspace role"},
		{name: "underscore", ids: map[string]string{"a_b": "ws-1"}, want: "invalid workspace role"},
		{name: "empty role", ids: map[string]string{"": "ws-1"}, want: "invalid workspace role"},
		{name: "too long role", ids: map[string]string{strings.Repeat("a", 33): "ws-1"}, want: "invalid workspace role"},
		{name: "empty id", ids: map[string]string{"manager": ""}, want: `workspace "manager" must not be empty`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recordPath := t.TempDir()
			err := WriteWorkspaces(recordPath, test.ids)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("WriteWorkspaces() error = %v, want containing %q", err, test.want)
			}
			// A rejected write publishes nothing and leaves no temporary files.
			if _, statErr := os.Lstat(filepath.Join(recordPath, WorkspacesFileName)); !os.IsNotExist(statErr) {
				t.Fatalf("workspaces.json stat error = %v, want not exist", statErr)
			}
			matches, err := filepath.Glob(filepath.Join(recordPath, ".workspaces-*"))
			if err != nil || len(matches) != 0 {
				t.Fatalf("temporary workspaces files = %q, error = %v", matches, err)
			}
		})
	}
}

func TestWorkspacesRejectInvalidUTF8(t *testing.T) {
	t.Run("read raw invalid byte", func(t *testing.T) {
		recordPath := t.TempDir()
		// A raw 0xff byte inside the JSON string is accepted by the JSON scanner
		// but is not valid UTF-8; json.Unmarshal would silently replace it with
		// U+FFFD, so ReadWorkspaces must reject rather than normalize it.
		raw := []byte("{\"schema_version\":1,\"workspaces\":{\"manager\":\"ws-\xff\"}}")
		if err := os.WriteFile(filepath.Join(recordPath, WorkspacesFileName), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := ReadWorkspaces(recordPath)
		if err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
			t.Fatalf("ReadWorkspaces() error = %v, want invalid-UTF-8 rejection", err)
		}
		if got != nil {
			t.Fatalf("ReadWorkspaces() = %#v, want nil map on rejection (no normalization)", got)
		}
	})

	t.Run("write invalid caller string", func(t *testing.T) {
		recordPath := t.TempDir()
		// A Go string may hold invalid UTF-8; json.Marshal would rewrite it to
		// U+FFFD, breaking the exact round trip, so WriteWorkspaces must reject.
		err := WriteWorkspaces(recordPath, map[string]string{"manager": "ws-\xff"})
		if err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
			t.Fatalf("WriteWorkspaces() error = %v, want invalid-UTF-8 rejection", err)
		}
		// A rejected write publishes nothing (no normalized bytes on disk) and
		// leaves no temporary files behind.
		if _, statErr := os.Lstat(filepath.Join(recordPath, WorkspacesFileName)); !os.IsNotExist(statErr) {
			t.Fatalf("workspaces.json stat error = %v, want not exist (no publication)", statErr)
		}
		matches, err := filepath.Glob(filepath.Join(recordPath, ".workspaces-*"))
		if err != nil || len(matches) != 0 {
			t.Fatalf("temporary workspaces files = %q, error = %v", matches, err)
		}
	})
}

func TestReadWorkspacesRejectsMalformedState(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "malformed json", data: "{", want: "EOF"},
		{name: "not an object", data: `["manager"]`, want: "must be a JSON object"},
		{name: "unknown top field", data: `{"schema_version":1,"workspaces":{},"extra":true}`, want: `unknown field "extra"`},
		{name: "duplicate top field", data: `{"schema_version":1,"schema_version":1,"workspaces":{}}`, want: `duplicate field "schema_version"`},
		{name: "missing workspaces", data: `{"schema_version":1}`, want: "must contain"},
		{name: "trailing value", data: `{"schema_version":1,"workspaces":{}} {}`, want: "unexpected trailing JSON value"},
		{name: "unsupported schema", data: `{"schema_version":2,"workspaces":{}}`, want: "unsupported schema_version 2"},
		{name: "null schema", data: `{"schema_version":null,"workspaces":{}}`, want: "schema_version must not be null"},
		{name: "null workspaces", data: `{"schema_version":1,"workspaces":null}`, want: "workspaces must not be null"},
		{name: "workspaces not object", data: `{"schema_version":1,"workspaces":[]}`, want: "workspaces must be a JSON object"},
		{name: "null id", data: `{"schema_version":1,"workspaces":{"manager":null}}`, want: `workspace "manager" must not be null`},
		{name: "empty id", data: `{"schema_version":1,"workspaces":{"manager":""}}`, want: `workspace "manager" must not be empty`},
		{name: "non-string id", data: `{"schema_version":1,"workspaces":{"manager":5}}`, want: `workspace "manager"`},
		{name: "invalid role", data: `{"schema_version":1,"workspaces":{"Manager":"ws-1"}}`, want: `invalid workspace role "Manager"`},
		{name: "duplicate role", data: `{"schema_version":1,"workspaces":{"manager":"ws-1","manager":"ws-2"}}`, want: `duplicate workspace role "manager"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recordPath := t.TempDir()
			if err := os.WriteFile(filepath.Join(recordPath, WorkspacesFileName), []byte(test.data), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := ReadWorkspaces(recordPath)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ReadWorkspaces() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestReadWorkspacesRejectsUnsafeTargets(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		recordPath := t.TempDir()
		target := filepath.Join(t.TempDir(), WorkspacesFileName)
		if err := os.WriteFile(target, []byte(`{"schema_version":1,"workspaces":{}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(recordPath, WorkspacesFileName)); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadWorkspaces(recordPath); err == nil || !strings.Contains(err.Error(), "is a symlink") {
			t.Fatalf("ReadWorkspaces() error = %v, want symlink rejection", err)
		}
	})

	t.Run("directory", func(t *testing.T) {
		recordPath := t.TempDir()
		if err := os.Mkdir(filepath.Join(recordPath, WorkspacesFileName), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadWorkspaces(recordPath); err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("ReadWorkspaces() error = %v, want nonregular rejection", err)
		}
	})
}

func TestWriteWorkspacesCleansTemporaryFileAfterPublishFailure(t *testing.T) {
	recordPath := t.TempDir()
	// A directory at the publish target makes the final rename fail, so the
	// deferred cleanup is the only thing that can remove the temporary file.
	if err := os.Mkdir(filepath.Join(recordPath, WorkspacesFileName), 0o755); err != nil {
		t.Fatal(err)
	}
	err := WriteWorkspaces(recordPath, map[string]string{"manager": "ws-1"})
	if err == nil || !strings.Contains(err.Error(), "publish") {
		t.Fatalf("WriteWorkspaces() error = %v, want publish failure", err)
	}
	matches, err := filepath.Glob(filepath.Join(recordPath, ".workspaces-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary workspaces files = %q, error = %v", matches, err)
	}
}

func TestWriteWorkspacesMissingRecordDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	err := WriteWorkspaces(missing, map[string]string{"manager": "ws-1"})
	if err == nil || !strings.Contains(err.Error(), "create temporary file") {
		t.Fatalf("WriteWorkspaces() error = %v, want create-temporary-file failure", err)
	}
}
