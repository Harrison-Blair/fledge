package session

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadMissingSessionsDirectoryIsEmpty(t *testing.T) {
	root := newProject(t)

	records, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("Load() returned %d records, want 0", len(records))
	}
}

func TestLoadValidRecord(t *testing.T) {
	root := newProject(t)
	name := "fledge-My.Project-0123abcd"
	recordDir := writeRecord(t, root, name, `{"schema_version":1,"herdr_session_name":"`+name+`","created_at":"2026-08-24T14:15:16Z"}`)

	records, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Load() returned %d records, want 1", len(records))
	}
	record := records[0]
	if record.SchemaVersion != 1 || record.HerdrSessionName != name || record.Path != recordDir {
		t.Fatalf("Load() record = %#v", record)
	}
	wantTime := time.Date(2026, 8, 24, 14, 15, 16, 0, time.UTC)
	if !record.CreatedAt.Equal(wantTime) || record.CreatedAt.Location() != time.UTC {
		t.Fatalf("Load() CreatedAt = %v, want %v in UTC", record.CreatedAt, wantTime)
	}
}

func TestLoadRejectsInvalidRecords(t *testing.T) {
	tests := []struct {
		name    string
		dirName string
		config  string
		wantErr string
	}{
		{
			name:    "unknown field",
			dirName: "valid",
			config:  `{"schema_version":1,"herdr_session_name":"valid","created_at":"2026-08-24T14:15:16Z","extra":true}`,
			wantErr: `unknown field "extra"`,
		},
		{
			name:    "duplicate field",
			dirName: "valid",
			config:  `{"schema_version":1,"schema_version":1,"herdr_session_name":"valid","created_at":"2026-08-24T14:15:16Z"}`,
			wantErr: `duplicate field "schema_version"`,
		},
		{
			name:    "missing field",
			dirName: "valid",
			config:  `{"schema_version":1,"herdr_session_name":"valid"}`,
			wantErr: "must contain",
		},
		{
			name:    "trailing value",
			dirName: "valid",
			config:  `{"schema_version":1,"herdr_session_name":"valid","created_at":"2026-08-24T14:15:16Z"} {}`,
			wantErr: "unexpected trailing JSON value",
		},
		{
			name:    "unsupported schema",
			dirName: "valid",
			config:  `{"schema_version":2,"herdr_session_name":"valid","created_at":"2026-08-24T14:15:16Z"}`,
			wantErr: "unsupported schema_version 2",
		},
		{
			name:    "mismatched name",
			dirName: "valid",
			config:  `{"schema_version":1,"herdr_session_name":"different","created_at":"2026-08-24T14:15:16Z"}`,
			wantErr: "want directory name",
		},
		{
			name:    "invalid directory name",
			dirName: "not valid",
			config:  `{"schema_version":1,"herdr_session_name":"not valid","created_at":"2026-08-24T14:15:16Z"}`,
			wantErr: "not a valid Herder session name",
		},
		{
			name:    "malformed timestamp",
			dirName: "valid",
			config:  `{"schema_version":1,"herdr_session_name":"valid","created_at":"yesterday"}`,
			wantErr: "invalid created_at",
		},
		{
			name:    "non-UTC timestamp",
			dirName: "valid",
			config:  `{"schema_version":1,"herdr_session_name":"valid","created_at":"2026-08-24T10:15:16-04:00"}`,
			wantErr: "created_at is not UTC",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newProject(t)
			writeRecord(t, root, test.dirName, test.config)
			_, err := Load(root)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Load() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestLoadRejectsSymlinksInRecordPath(t *testing.T) {
	t.Run("fledge directory", func(t *testing.T) {
		root := t.TempDir()
		target := t.TempDir()
		if err := os.Symlink(target, filepath.Join(root, ".fledge")); err != nil {
			t.Fatal(err)
		}
		assertLoadSymlinkError(t, root)
	})

	t.Run("sessions directory", func(t *testing.T) {
		root := newProject(t)
		target := t.TempDir()
		if err := os.Symlink(target, filepath.Join(root, ".fledge", "sessions")); err != nil {
			t.Fatal(err)
		}
		assertLoadSymlinkError(t, root)
	})

	t.Run("record directory", func(t *testing.T) {
		root := newProject(t)
		sessions := filepath.Join(root, ".fledge", "sessions")
		if err := os.Mkdir(sessions, 0o755); err != nil {
			t.Fatal(err)
		}
		target := t.TempDir()
		if err := os.Symlink(target, filepath.Join(sessions, "valid")); err != nil {
			t.Fatal(err)
		}
		assertLoadSymlinkError(t, root)
	})

	t.Run("config file", func(t *testing.T) {
		root := newProject(t)
		recordDir := filepath.Join(root, ".fledge", "sessions", "valid")
		if err := os.MkdirAll(recordDir, 0o755); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(target, []byte(`{"schema_version":1}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(recordDir, "config.json")); err != nil {
			t.Fatal(err)
		}
		assertLoadSymlinkError(t, root)
	})
}

func TestCreatePublishesExactRecordAndRetainsExistingRecords(t *testing.T) {
	rootParent := t.TempDir()
	root := filepath.Join(rootParent, "My Project")
	if err := os.MkdirAll(filepath.Join(root, ".fledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, 8, 24, 14, 15, 16, 987654321, time.FixedZone("EDT", -4*60*60))

	record, err := Create(root, nil, bytes.NewReader([]byte{0xab, 0xcd, 0xef, 0x12}), createdAt)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if record.HerdrSessionName != "fledge-My-Project-abcdef12" {
		t.Fatalf("Create() name = %q", record.HerdrSessionName)
	}
	wantTime := time.Date(2026, 8, 24, 18, 15, 16, 0, time.UTC)
	if !record.CreatedAt.Equal(wantTime) {
		t.Fatalf("Create() time = %v, want %v", record.CreatedAt, wantTime)
	}
	data, err := os.ReadFile(filepath.Join(record.Path, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	wantJSON := `{"schema_version":1,"herdr_session_name":"fledge-My-Project-abcdef12","created_at":"2026-08-24T18:15:16Z"}` + "\n"
	if string(data) != wantJSON {
		t.Fatalf("config.json = %q, want %q", data, wantJSON)
	}

	records, err := Load(root)
	if err != nil {
		t.Fatalf("Load() after Create() error = %v", err)
	}
	if len(records) != 1 || records[0].HerdrSessionName != record.HerdrSessionName {
		t.Fatalf("Load() after Create() = %#v", records)
	}
}

func TestCreateRetriesGlobalAndLocalCollisions(t *testing.T) {
	rootParent := t.TempDir()
	root := filepath.Join(rootParent, "project")
	if err := os.MkdirAll(filepath.Join(root, ".fledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRecord(t, root, "fledge-project-00000001", `{"schema_version":1,"herdr_session_name":"fledge-project-00000001","created_at":"2026-08-24T14:15:16Z"}`)
	unavailable := map[string]struct{}{
		"fledge-project-00000002": {},
	}
	entropy := bytes.NewReader([]byte{
		0, 0, 0, 1,
		0, 0, 0, 2,
		0, 0, 0, 3,
	})

	record, err := Create(root, unavailable, entropy, time.Now())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if record.HerdrSessionName != "fledge-project-00000003" {
		t.Fatalf("Create() name = %q, want final collision-free name", record.HerdrSessionName)
	}
	records, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("Load() returned %d records, want retained old record and new record", len(records))
	}
}

func TestCreateCleansTemporaryDirectoryAfterPublishFailure(t *testing.T) {
	root := newProject(t)
	want := errors.New("rename failed")

	_, err := create(root, nil, bytes.NewReader([]byte{1, 2, 3, 4}), time.Now(), func(string, string) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("create() error = %v, want wrapped %v", err, want)
	}

	entries, err := os.ReadDir(filepath.Join(root, ".fledge"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".session-") {
			t.Fatalf("temporary entry %q remains after failure", entry.Name())
		}
	}
	sessionEntries, err := os.ReadDir(filepath.Join(root, ".fledge", "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionEntries) != 0 {
		t.Fatalf("partial session entries remain after failure: %v", sessionEntries)
	}
}

func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".fledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeRecord(t *testing.T, root, name, config string) string {
	t.Helper()
	recordDir := filepath.Join(root, ".fledge", "sessions", name)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recordDir, "config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return recordDir
}

func assertLoadSymlinkError(t *testing.T, root string) {
	t.Helper()
	_, err := Load(root)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Load() error = %v, want symlink error", err)
	}
}
