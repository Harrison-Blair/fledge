package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitDiscoverAndIdempotency(t *testing.T) {
	root := t.TempDir()
	first, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Initialized || first.MarkerPath != filepath.Join(first.ProjectRoot, ".fledge", "config.json") {
		t.Fatalf("unexpected first result: %#v", first)
	}
	second, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if second.Initialized {
		t.Fatal("second initialization rewrote a valid marker")
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := Discover(nested)
	if err != nil {
		t.Fatal(err)
	}
	if info.Root != first.ProjectRoot {
		t.Fatalf("root = %q, want %q", info.Root, first.ProjectRoot)
	}
}

func TestInitPreservesAndMaintainsFledgeGitignore(t *testing.T) {
	root := t.TempDir()
	fledgeDir := filepath.Join(root, ".fledge")
	if err := os.MkdirAll(fledgeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ignore := filepath.Join(fledgeDir, ".gitignore")
	if err := os.WriteFile(ignore, []byte("/keep/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ignore)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "/keep/\n/logs/\n/tmp/\n" {
		t.Fatalf("gitignore = %q", data)
	}
}

func TestEnsureRuntimeIgnoredAddsOnlyMissingEntries(t *testing.T) {
	root := t.TempDir()
	fledgeDir := filepath.Join(root, ".fledge")
	if err := os.Mkdir(fledgeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ignore := filepath.Join(fledgeDir, ".gitignore")
	if err := os.WriteFile(ignore, []byte("/tmp/\n/custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureRuntimeIgnored(root); err != nil {
		t.Fatal(err)
	}
	if err := EnsureRuntimeIgnored(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ignore)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "/tmp/\n/custom\n/logs/\n"; got != want {
		t.Fatalf("gitignore = %q, want %q", got, want)
	}
}

func TestResetTempDirRecursivelyCleansAndSecuresDirectory(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	tempDir := TempDir(root)
	nested := filepath.Join(tempDir, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "artifact"), []byte("discard"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tempDir, 0o777); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		got, err := ResetTempDir(root)
		if err != nil {
			t.Fatal(err)
		}
		if got != tempDir || !filepath.IsAbs(got) {
			t.Fatalf("temp dir = %q, want absolute %q", got, tempDir)
		}
		entries, err := os.ReadDir(tempDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("temp directory is not empty: %#v", entries)
		}
		info, err := os.Stat(tempDir)
		if err != nil {
			t.Fatal(err)
		}
		if gotMode := info.Mode().Perm(); gotMode != 0o700 {
			t.Fatalf("temp permissions = %o, want 700", gotMode)
		}
	}
}

func TestResetTempDirRemovesLeafSymlinkWithoutFollowingIt(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	kept := filepath.Join(outside, "keep")
	if err := os.WriteFile(kept, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, TempDir(root)); err != nil {
		t.Fatal(err)
	}

	if _, err := ResetTempDir(root); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(kept); err != nil || string(data) != "safe" {
		t.Fatalf("outside target changed: data=%q err=%v", data, err)
	}
	info, err := os.Lstat(TempDir(root))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("temp path was not recreated as a directory: %v", info.Mode())
	}
}

func TestInitOptionalRelativePathAndCanonicalSymlink(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	t.Chdir(parent)
	result, err := Init("link")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(target)
	if result.ProjectRoot != want {
		t.Fatalf("root = %q, want %q", result.ProjectRoot, want)
	}
	info, err := Discover(link)
	if err != nil || info.Root != want {
		t.Fatalf("discover = %#v, %v", info, err)
	}
}

func TestInitRejectsInvalidExistingMarkerWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	markerDir := filepath.Join(root, ".fledge")
	if err := os.Mkdir(markerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(markerDir, "config.json")
	original := []byte(`{"schema_version":99}`)
	if err := os.WriteFile(marker, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(root); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unexpected error: %v", err)
	}
	after, _ := os.ReadFile(marker)
	if string(after) != string(original) {
		t.Fatal("invalid marker was overwritten")
	}
}

func TestDiscoverUsesClosestMarker(t *testing.T) {
	outer := t.TempDir()
	if _, err := Init(outer); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(outer, "inner")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(inner); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(inner, "x")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := Discover(nested)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(inner)
	if info.Root != want {
		t.Fatalf("root = %q, want closest %q", info.Root, want)
	}
}

func TestDiscoverRejectsInvalidClosestMarkerInsteadOfSkippingIt(t *testing.T) {
	outer := t.TempDir()
	if _, err := Init(outer); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(outer, "inner")
	if err := os.MkdirAll(filepath.Join(inner, ".fledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, ".fledge", "config.json"), []byte("{broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(inner); err == nil || !strings.Contains(err.Error(), "decode Fledge marker") {
		t.Fatalf("invalid closest marker was skipped: %v", err)
	}
}

func TestDiscoverUninitializedIsActionable(t *testing.T) {
	root := t.TempDir()
	// TMPDIR may itself be project-local when the suite is run through
	// Fledge. Treat this isolated directory as HOME so discovery deliberately
	// stops before reaching any marker above the test fixture.
	t.Setenv("HOME", root)
	_, err := Discover(root)
	if err == nil || !strings.Contains(err.Error(), "fledge init") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiscoverDoesNotUseHomeAsProjectRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	markerDir := filepath.Join(home, ".fledge")
	if err := os.Mkdir(markerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(markerDir, "config.json"), []byte("{\"schema_version\":1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(home, "project", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(nested); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("home marker was unexpectedly discovered: %v", err)
	}
	if _, err := Discover(home); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("home itself was unexpectedly discovered: %v", err)
	}
	if _, err := Init(home); err == nil {
		t.Fatal("home directory was accepted as a project root")
	}
}

func TestSessionNameStable(t *testing.T) {
	got := SessionName("/tmp/repo")
	if want := "fledge-repo-b6fe87a9"; got != want {
		t.Fatalf("SessionName() = %q, want %q", got, want)
	}
	if got := SessionName("/tmp/My Project!"); got[:18] != "fledge-my-project-" {
		t.Fatalf("SessionName() did not normalize slug: %q", got)
	}
}

func TestWorkspaceLabelPreservesProjectFolderName(t *testing.T) {
	if got := WorkspaceLabel("/source/My Project"); got != "My Project" {
		t.Fatalf("WorkspaceLabel() = %q, want %q", got, "My Project")
	}
	if got := WorkspaceLabel(string(filepath.Separator)); got != "project" {
		t.Fatalf("root WorkspaceLabel() = %q, want %q", got, "project")
	}
}

func TestValidateSession(t *testing.T) {
	if err := ValidateSession("bad/name"); err == nil {
		t.Fatal("expected invalid session error")
	}
	if err := ValidateSession("team-session"); err != nil {
		t.Fatal(err)
	}
}
