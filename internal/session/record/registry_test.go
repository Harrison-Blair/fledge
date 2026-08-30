package record

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fledge/internal/profile"
	"fledge/internal/session/sessiontest"
	"fledge/internal/session/types"
)

func TestLoadMissingSessionsDirectoryIsEmpty(t *testing.T) {
	root := sessiontest.NewProject(t)

	records, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("Load() returned %d records, want 0", len(records))
	}
}

func TestLoadValidRecord(t *testing.T) {
	root := sessiontest.NewProject(t)
	name := "fledge-My.Project-0123abcd"
	recordDir := sessiontest.WriteRecord(t, root, name, `{"schema_version":1,"herdr_session_name":"`+name+`","created_at":"2026-08-24T14:15:16Z"}`)

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
			root := sessiontest.NewProject(t)
			sessiontest.WriteRecord(t, root, test.dirName, test.config)
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
		root := sessiontest.NewProject(t)
		target := t.TempDir()
		if err := os.Symlink(target, filepath.Join(root, ".fledge", "sessions")); err != nil {
			t.Fatal(err)
		}
		assertLoadSymlinkError(t, root)
	})

	t.Run("record directory", func(t *testing.T) {
		root := sessiontest.NewProject(t)
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
		root := sessiontest.NewProject(t)
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

	record, err := Create(root, types.AgentChoice{}, MaxSessionLength, nil, bytes.NewReader([]byte{0xab, 0xcd, 0xef, 0x12}), createdAt)
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
	for path, want := range map[string]string{
		"claim.json":   "{\"schema_version\":1}\n",
		"pending.json": "{\"schema_version\":1,\"harness\":\"\",\"model\":\"\",\"args\":[]}\n",
	} {
		data, err := os.ReadFile(filepath.Join(record.Path, path))
		if err != nil {
			t.Fatalf("read published %s: %v", path, err)
		}
		if string(data) != want {
			t.Fatalf("%s = %q, want %q", path, data, want)
		}
	}

	records, err := Load(root)
	if err != nil {
		t.Fatalf("Load() after Create() error = %v", err)
	}
	if len(records) != 1 || records[0].HerdrSessionName != record.HerdrSessionName {
		t.Fatalf("Load() after Create() = %#v", records)
	}
}

func TestLoadClaimAndPendingMetadata(t *testing.T) {
	root := sessiontest.NewProject(t)
	name := "claimed"
	dir := sessiontest.WriteRecord(t, root, name, `{"schema_version":1,"herdr_session_name":"claimed","created_at":"2026-08-24T14:15:16Z"}`)
	if err := os.WriteFile(filepath.Join(dir, "claim.json"), []byte(`{"schema_version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pending.json"), []byte(`{"schema_version":1,"harness":"claude","model":"opus","args":["--effort","high"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !records[0].Claimed || records[0].PendingChoice == nil {
		t.Fatalf("Load() records = %#v", records)
	}
	choice := records[0].PendingChoice
	if choice.Harness != "claude" || choice.Model != "opus" || len(choice.Args) != 2 || choice.Args[0] != "--effort" || choice.Args[1] != "high" || choice.Profile != nil {
		t.Fatalf("Load() pending choice = %#v", choice)
	}
	if err := ClearPending(records[0]); err != nil {
		t.Fatal(err)
	}
	if err := ClearPending(records[0]); err != nil {
		t.Fatalf("ClearPending() second call: %v", err)
	}
}

func TestLoadLegacyPendingMetadataWithoutArgs(t *testing.T) {
	root := sessiontest.NewProject(t)
	dir := sessiontest.WriteRecord(t, root, "claimed", `{"schema_version":1,"herdr_session_name":"claimed","created_at":"2026-08-24T14:15:16Z"}`)
	if err := os.WriteFile(filepath.Join(dir, "claim.json"), []byte(`{"schema_version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pending.json"), []byte(`{"schema_version":1,"harness":"claude","model":"opus"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(records) != 1 || records[0].PendingChoice == nil || len(records[0].PendingChoice.Args) != 0 {
		t.Fatalf("Load() records = %#v", records)
	}
}

func TestLoadRejectsOrphanAndNullSidecars(t *testing.T) {
	for _, test := range []struct{ name, claim, pending, want string }{
		{name: "orphan", pending: `{"schema_version":1,"harness":"","model":"","args":[]}`, want: "without a claim"},
		{name: "null", claim: `{"schema_version":null}`, want: "must not be null"},
		{name: "model only", claim: `{"schema_version":1}`, pending: `{"schema_version":1,"harness":"","model":"opus","args":[]}`, want: "requires harness"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := sessiontest.NewProject(t)
			dir := sessiontest.WriteRecord(t, root, "record", `{"schema_version":1,"herdr_session_name":"record","created_at":"2026-08-24T14:15:16Z"}`)
			if test.claim != "" {
				if err := os.WriteFile(filepath.Join(dir, "claim.json"), []byte(test.claim), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if test.pending != "" {
				if err := os.WriteFile(filepath.Join(dir, "pending.json"), []byte(test.pending), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Load(root); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadRejectsInvalidSidecars(t *testing.T) {
	tests := []struct {
		name string
		file string
		data string
		want string
	}{
		{name: "malformed claim", file: "claim.json", data: "{", want: "EOF"},
		{name: "claim unknown field", file: "claim.json", data: `{"schema_version":1,"extra":true}`, want: `unknown field "extra"`},
		{name: "claim duplicate field", file: "claim.json", data: `{"schema_version":1,"schema_version":1}`, want: `duplicate field "schema_version"`},
		{name: "claim missing field", file: "claim.json", data: `{}`, want: "must contain schema_version"},
		{name: "pending unknown field", file: "pending.json", data: `{"schema_version":1,"harness":"","model":"","args":[],"extra":true}`, want: `unknown field "extra"`},
		{name: "pending duplicate field", file: "pending.json", data: `{"schema_version":1,"harness":"","harness":"","model":"","args":[]}`, want: `duplicate field "harness"`},
		{name: "pending missing field", file: "pending.json", data: `{"schema_version":1,"harness":""}`, want: "must contain schema_version, harness, and model"},
		{name: "pending null field", file: "pending.json", data: `{"schema_version":1,"harness":null,"model":"","args":[]}`, want: "must not be null"},
		{name: "pending null args", file: "pending.json", data: `{"schema_version":1,"harness":"","model":"","args":null}`, want: "pending args must not be null"},
		{name: "pending null argument", file: "pending.json", data: `{"schema_version":1,"harness":"","model":"","args":[null]}`, want: "args[0] must not be null"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := sessiontest.NewProject(t)
			dir := sessiontest.WriteRecord(t, root, "record", `{"schema_version":1,"herdr_session_name":"record","created_at":"2026-08-24T14:15:16Z"}`)
			if test.file == "pending.json" {
				if err := os.WriteFile(filepath.Join(dir, "claim.json"), []byte(`{"schema_version":1}`), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(dir, test.file), []byte(test.data), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(root); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadRejectsInvalidSidecarFileTypes(t *testing.T) {
	for _, sidecar := range []string{"claim.json", "pending.json"} {
		t.Run(sidecar+" symlink", func(t *testing.T) {
			root := sessiontest.NewProject(t)
			dir := sessiontest.WriteRecord(t, root, "record", `{"schema_version":1,"herdr_session_name":"record","created_at":"2026-08-24T14:15:16Z"}`)
			target := filepath.Join(t.TempDir(), sidecar)
			if err := os.WriteFile(target, []byte(`{"schema_version":1}`), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(dir, sidecar)); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "is a symlink") {
				t.Fatalf("Load() error = %v, want symlink rejection", err)
			}
		})

		t.Run(sidecar+" directory", func(t *testing.T) {
			root := sessiontest.NewProject(t)
			dir := sessiontest.WriteRecord(t, root, "record", `{"schema_version":1,"herdr_session_name":"record","created_at":"2026-08-24T14:15:16Z"}`)
			if err := os.Mkdir(filepath.Join(dir, sidecar), 0o755); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "not a regular file") {
				t.Fatalf("Load() error = %v, want nonregular rejection", err)
			}
		})
	}
}

func TestLoadRejectsMultipleClaims(t *testing.T) {
	root := sessiontest.NewProject(t)
	for _, name := range []string{"first", "second"} {
		dir := sessiontest.WriteRecord(t, root, name, `{"schema_version":1,"herdr_session_name":"`+name+`","created_at":"2026-08-24T14:15:16Z"}`)
		if err := os.WriteFile(filepath.Join(dir, "claim.json"), []byte(`{"schema_version":1}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "multiple claimed records") {
		t.Fatalf("Load() error = %v, want multiple-claims rejection", err)
	}
}

func TestClaimPublishesOnlyAfterPreparingTemporaryFile(t *testing.T) {
	root := sessiontest.NewProject(t)
	dir := sessiontest.WriteRecord(t, root, "record", `{"schema_version":1,"herdr_session_name":"record","created_at":"2026-08-24T14:15:16Z"}`)
	record := Record{HerdrSessionName: "record", Path: dir}

	err := claim(record, func(temporaryPath, finalPath string) error {
		if _, err := os.Lstat(finalPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("claim final path before publish error = %v, want not exist", err)
		}
		data, err := os.ReadFile(temporaryPath)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), "{\"schema_version\":1}\n"; got != want {
			t.Fatalf("temporary claim = %q, want %q", got, want)
		}
		info, err := os.Stat(temporaryPath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Fatalf("temporary claim mode = %#o, want 0644", info.Mode().Perm())
		}
		return os.Link(temporaryPath, finalPath)
	})
	if err != nil {
		t.Fatalf("claim() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "claim.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "{\"schema_version\":1}\n"; got != want {
		t.Fatalf("claim.json = %q, want %q", got, want)
	}
	assertNoClaimTemporaryFiles(t, dir)
}

func TestClaimDoesNotOverwriteAndCleansTemporaryFile(t *testing.T) {
	root := sessiontest.NewProject(t)
	dir := sessiontest.WriteRecord(t, root, "record", `{"schema_version":1,"herdr_session_name":"record","created_at":"2026-08-24T14:15:16Z"}`)
	claimPath := filepath.Join(dir, "claim.json")
	if err := os.WriteFile(claimPath, []byte("existing claim\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record := Record{HerdrSessionName: "record", Path: dir}
	if err := Claim(record); err == nil || !strings.Contains(err.Error(), "already claimed") {
		t.Fatalf("Claim() error = %v, want already-claimed error", err)
	}
	data, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "existing claim\n"; got != want {
		t.Fatalf("claim.json changed to %q, want %q", got, want)
	}
	assertNoClaimTemporaryFiles(t, dir)
}

func TestClaimCleansTemporaryFileAfterPublishFailure(t *testing.T) {
	root := sessiontest.NewProject(t)
	dir := sessiontest.WriteRecord(t, root, "record", `{"schema_version":1,"herdr_session_name":"record","created_at":"2026-08-24T14:15:16Z"}`)
	want := errors.New("link failed")
	err := claim(Record{HerdrSessionName: "record", Path: dir}, func(string, string) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("claim() error = %v, want wrapped %v", err, want)
	}
	if _, err := os.Lstat(filepath.Join(dir, "claim.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("claim final path error = %v, want not exist", err)
	}
	assertNoClaimTemporaryFiles(t, dir)
}

func TestCreateRetriesGlobalAndLocalCollisions(t *testing.T) {
	rootParent := t.TempDir()
	root := filepath.Join(rootParent, "project")
	if err := os.MkdirAll(filepath.Join(root, ".fledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	sessiontest.WriteRecord(t, root, "fledge-project-00000001", `{"schema_version":1,"herdr_session_name":"fledge-project-00000001","created_at":"2026-08-24T14:15:16Z"}`)
	unavailable := map[string]struct{}{
		"fledge-project-00000002": {},
	}
	entropy := bytes.NewReader([]byte{
		0, 0, 0, 1,
		0, 0, 0, 2,
		0, 0, 0, 3,
	})

	record, err := Create(root, types.AgentChoice{}, MaxSessionLength, unavailable, entropy, time.Now())
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
	root := sessiontest.NewProject(t)
	want := errors.New("rename failed")
	configured := profile.Profile{Name: "fledge-test", Instructions: "pinned\n"}

	_, err := create(root, types.AgentChoice{Profile: &configured}, MaxSessionLength, nil, bytes.NewReader([]byte{1, 2, 3, 4}), time.Now(), func(string, string) error {
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

func assertLoadSymlinkError(t *testing.T, root string) {
	t.Helper()
	_, err := Load(root)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Load() error = %v, want symlink error", err)
	}
}

func assertNoClaimTemporaryFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".claim-") {
			t.Fatalf("temporary claim %q remains", entry.Name())
		}
	}
}
