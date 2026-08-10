package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInitCreatesProjectFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gotRoot, err := Init(root)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if gotRoot != root {
		t.Errorf("Init() root = %q, want %q", gotRoot, root)
	}

	config, err := LoadConfig(root)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.SchemaVersion != SchemaVersion {
		t.Errorf("schema version = %d, want %d", config.SchemaVersion, SchemaVersion)
	}

	profile, err := LoadOrchestratorProfile(root)
	if err != nil {
		t.Fatalf("LoadOrchestratorProfile() error = %v", err)
	}
	if profile != (OrchestratorProfile{SchemaVersion: SchemaVersion, Instructions: DefaultOrchestratorInstructions}) {
		t.Errorf("profile = %#v, want default profile", profile)
	}

	assertFileContents(t, filepath.Join(root, stateDirectory, ".gitignore"), ignoreContents)
	assertFileContents(t, filepath.Join(root, ".codex", "rules", "fledge.rules"), codexRulesContents)
	for _, path := range []string{
		filepath.Join(root, stateDirectory, configFilename),
		filepath.Join(root, stateDirectory, profilesDir, profileFilename),
		filepath.Join(root, stateDirectory, ".gitignore"),
		filepath.Join(root, ".codex", "rules", "fledge.rules"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o644 {
			t.Errorf("%s permissions = %o, want 644", path, got)
		}
	}
}

func TestInitKeepsUserFacingDirectoriesReadable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if _, err := Init(root); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	for _, dir := range []string{
		filepath.Join(root, stateDirectory),
		filepath.Join(root, stateDirectory, profilesDir),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o755 {
			t.Errorf("%s permissions = %o, want 755", dir, got)
		}
	}
}

func TestInitPreservesConflictingCodexRulesAndReturnsError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, ".codex", "rules", "fledge.rules")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const contents = "# keep my rules\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Init(root); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("Init() error = %v, want Codex rule conflict", err)
	}
	assertFileContents(t, path, contents)
	if _, err := os.Stat(filepath.Join(root, stateDirectory, configFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("config error = %v, want not exist", err)
	}
}

func TestInitPreservesCustomizedGeneratedCodexRules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, ".codex", "rules", "fledge.rules")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	contents := codexRulesContents + "# customized\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Init(root); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("Init() error = %v, want Codex rule conflict", err)
	}
	assertFileContents(t, path, contents)
}

func TestDefaultOrchestratorInstructionsUseInjectedMessaging(t *testing.T) {
	t.Parallel()

	for _, want := range []string{
		"fledge agent message send <recipient> <text>",
		"fledge agent message reply <message-id> <text>",
		"fledge agent task assign/progress/blocked/needs-decision/resume/complete/fail/cancel/list/show",
		"--can-delegate",
		"--parent-task",
		"Ordinary messages always wake",
		"Never poll",
		"direct Herdr commands",
	} {
		if !strings.Contains(DefaultOrchestratorInstructions, want) {
			t.Errorf("DefaultOrchestratorInstructions = %q, want containing %q", DefaultOrchestratorInstructions, want)
		}
	}
	for _, unwanted := range []string{
		"fledge agent message inbox",
		"fledge agent message ack",
		"acknowledge",
	} {
		if strings.Contains(strings.ToLower(DefaultOrchestratorInstructions), unwanted) {
			t.Errorf("DefaultOrchestratorInstructions = %q, want no %q guidance", DefaultOrchestratorInstructions, unwanted)
		}
	}
}

func TestInitIsIdempotentAndPreservesExistingFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	profileContents := "# keep this comment\ninstructions = 'custom instructions'\nschema_version = 1\n"
	configContents := "{ \"schema_version\" : 1 }\n"
	customIgnoreContents := "session.json\nlocal-cache/\n"
	writeProjectFile(t, root, configFilename, configContents)
	writeProfile(t, root, profileContents)
	writeStateFile(t, root, ".gitignore", customIgnoreContents)

	for range 2 {
		if _, err := Init(root); err != nil {
			t.Fatalf("Init() error = %v", err)
		}
	}

	assertFileContents(t, filepath.Join(root, stateDirectory, configFilename), configContents)
	assertFileContents(t, filepath.Join(root, stateDirectory, profilesDir, profileFilename), profileContents)
	assertFileContents(t, filepath.Join(root, stateDirectory, ".gitignore"), customIgnoreContents+"session.lock\npreferences.json\nlogs/\ntmp/\nprofiles/generated/\n")
}

func TestInitPreservesExistingProfileWhenCreatingOtherMetadata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	profileContents := "schema_version = 1\ninstructions = 'keep me'\n"
	writeProfile(t, root, profileContents)

	if _, err := Init(root); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	assertFileContents(t, filepath.Join(root, stateDirectory, profilesDir, profileFilename), profileContents)
	if _, err := LoadConfig(root); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
}

func TestInitMigratesOnlyExactLegacyIgnore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
		want string
	}{
		{name: "exact legacy contents", give: legacyIgnoreContents, want: ignoreContents},
		{name: "customized legacy contents", give: legacyIgnoreContents + "!profiles/\n", want: legacyIgnoreContents + "!profiles/\n" + ignoreContents},
		{name: "legacy CRLF contents", give: "*\r\n!.gitignore\r\n", want: "*\r\n!.gitignore\r\n" + ignoreContents},
		{name: "empty custom file", give: "", want: ignoreContents},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeStateFile(t, root, ".gitignore", test.give)

			if _, err := Init(root); err != nil {
				t.Fatalf("Init() error = %v", err)
			}

			assertFileContents(t, filepath.Join(root, stateDirectory, ".gitignore"), test.want)
		})
	}
}

func TestInitRejectsMalformedExistingMetadataBeforeWriting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config string
	}{
		{name: "invalid JSON", config: "{"},
		{name: "unknown field", config: `{"schema_version":1,"other":true}`},
		{name: "missing version", config: `{}`},
		{name: "unsupported version", config: `{"schema_version":2}`},
		{name: "trailing value", config: `{"schema_version":1} {}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeProjectFile(t, root, configFilename, test.config)

			if _, err := Init(root); err == nil {
				t.Fatal("Init() error = nil, want malformed config error")
			}
			if _, err := os.Stat(filepath.Join(root, stateDirectory, profilesDir)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("profiles directory error = %v, want not exist", err)
			}
		})
	}
}

func TestInitRejectsMalformedProfileWithoutOverwritingIt(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	malformed := "schema_version = 1\ninstructions = [\"not supported\"]\n"
	writeProfile(t, root, malformed)

	if _, err := Init(root); err == nil {
		t.Fatal("Init() error = nil, want malformed profile error")
	}
	assertFileContents(t, filepath.Join(root, stateDirectory, profilesDir, profileFilename), malformed)
	if _, err := os.Stat(filepath.Join(root, stateDirectory, configFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("config error = %v, want not exist", err)
	}
}

func TestInitRequiresExistingDirectory(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	if _, err := Init(filepath.Join(parent, "missing")); err == nil {
		t.Fatal("Init(missing) error = nil, want error")
	}
	file := filepath.Join(parent, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(file); err == nil {
		t.Fatal("Init(file) error = nil, want error")
	}
}

func TestFindSearchesUpwardAndReturnsCanonicalNearestRoot(t *testing.T) {
	t.Parallel()

	outer := t.TempDir()
	if _, err := Init(outer); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(outer, "one", "project")
	leaf := filepath.Join(inner, "deep", "leaf")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(inner); err != nil {
		t.Fatal(err)
	}

	got, err := Find(leaf)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if got != inner {
		t.Errorf("Find() = %q, want nearest root %q", got, inner)
	}

	link := filepath.Join(t.TempDir(), "linked-leaf")
	if err := os.Symlink(leaf, link); err != nil {
		t.Fatal(err)
	}
	got, err = Find(link)
	if err != nil {
		t.Fatalf("Find(symlink) error = %v", err)
	}
	if got != inner {
		t.Errorf("Find(symlink) = %q, want %q", got, inner)
	}
}

func TestFindRejectsInvalidNearestMarker(t *testing.T) {
	t.Parallel()

	outer := t.TempDir()
	if _, err := Init(outer); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(outer, "inner")
	leaf := filepath.Join(inner, "leaf")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, inner, configFilename, `{"schema_version":99}`)

	if got, err := Find(leaf); err == nil {
		t.Fatalf("Find() = %q, nil; want invalid marker error", got)
	} else if !strings.Contains(err.Error(), "unsupported schema_version 99") {
		t.Errorf("Find() error = %v, want version error", err)
	}
}

func TestFindSkipsDirectoriesWithoutMarker(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	leaf := filepath.Join(root, "has-state-dir", "leaf")
	if err := os.MkdirAll(filepath.Join(root, "has-state-dir", stateDirectory), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Find(leaf)
	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("Find() error = %v, want ErrNotInitialized", err)
	}
}

func TestEnsureRuntimeIgnoreAppendsMissingRuntimeEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
		want string
	}{
		{
			name: "custom user entries preserved and runtime entries appended",
			give: "session.json\nlocal-cache/\n",
			want: "session.json\nlocal-cache/\n" + "session.lock\npreferences.json\nlogs/\ntmp/\nprofiles/generated/\n",
		},
		{
			name: "exact legacy contents migrated wholesale",
			give: legacyIgnoreContents,
			want: ignoreContents,
		},
		{
			name: "no trailing newline still gains a separator",
			give: "session.json",
			want: "session.json\n" + "session.lock\npreferences.json\nlogs/\ntmp/\nprofiles/generated/\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeStateFile(t, root, ".gitignore", test.give)

			if err := EnsureRuntimeIgnore(root); err != nil {
				t.Fatalf("EnsureRuntimeIgnore() error = %v", err)
			}
			assertFileContents(t, filepath.Join(root, stateDirectory, ".gitignore"), test.want)
		})
	}
}

func TestEnsureRuntimeIgnoreIsNoOpWhenAllEntriesPresent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeStateFile(t, root, ".gitignore", ignoreContents)
	path := filepath.Join(root, stateDirectory, ".gitignore")

	// A no-op must not rewrite the file, so pin an old mtime and confirm it holds.
	old := time.Unix(1000, 0)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if err := EnsureRuntimeIgnore(root); err != nil {
		t.Fatalf("EnsureRuntimeIgnore() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Errorf("gitignore was rewritten (mtime = %v, want %v); expected a no-op", info.ModTime(), old)
	}
	assertFileContents(t, path, ignoreContents)
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != want {
		t.Errorf("%s contents = %q, want %q", path, contents, want)
	}
}

func writeProjectFile(t *testing.T, root, name, contents string) {
	t.Helper()
	writeStateFile(t, root, name, contents)
}

func writeStateFile(t *testing.T, root, name, contents string) {
	t.Helper()
	dir := filepath.Join(root, stateDirectory)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeProfile(t *testing.T, root, contents string) {
	t.Helper()
	dir := filepath.Join(root, stateDirectory, profilesDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, profileFilename), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindDoesNotUseHomeAsProjectRoot(t *testing.T) {
	home := t.TempDir()
	// os.UserHomeDir reads HOME on Unix and USERPROFILE on Windows, so set both
	// to keep the test correct cross-platform.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeStateFile(t, home, configFilename, "{\n  \"schema_version\": 1\n}\n")

	nested := filepath.Join(home, "project", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Find(nested); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("Find(nested) home marker was unexpectedly discovered: %v", err)
	}
	if _, err := Find(home); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("Find(home) home itself was unexpectedly discovered: %v", err)
	}
	if _, err := Init(home); err == nil {
		t.Fatal("Init(home) accepted the home directory as a project root")
	} else if !strings.Contains(err.Error(), "the home directory cannot be a Fledge project root") {
		t.Fatalf("Init(home) error = %v, want home-root rejection", err)
	}
}
