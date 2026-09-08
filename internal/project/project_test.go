package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesExactProjectFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	got, err := Init(root)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if got != root {
		t.Fatalf("Init() = %q, want %q", got, root)
	}

	assertContents(t, filepath.Join(root, stateDir, configFile), configContents)
	assertContents(t, filepath.Join(root, stateDir, ".gitignore"), ignoreContents)
	entries, err := os.ReadDir(filepath.Join(root, stateDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf(".fledge entries = %d, want 2", len(entries))
	}
}

func TestInitDoesNotRequireGit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if _, err := Init(root); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".git error = %v, want not exist", err)
	}
}

func TestInitCanonicalizesSymlinkedDirectory(t *testing.T) {
	t.Parallel()

	realRoot := t.TempDir()
	link := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Fatal(err)
	}

	got, err := Init(link)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if got != realRoot {
		t.Fatalf("Init() = %q, want %q", got, realRoot)
	}
	assertContents(t, filepath.Join(realRoot, stateDir, configFile), configContents)
}

func TestInitRequiresExistingDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "missing"), file} {
		if _, err := Init(path); err == nil {
			t.Errorf("Init(%q) error = nil, want error", path)
		}
	}
}

func TestInitRejectsAnyExistingMarkerWithoutMutation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{
			name: "directory",
			setup: func(t *testing.T, path string) {
				t.Helper()
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "keep"), []byte("mine"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "file",
			setup: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(path, []byte("mine"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlink",
			setup: func(t *testing.T, path string) {
				t.Helper()
				if err := os.Symlink(filepath.Join(t.TempDir(), "target"), path); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			marker := filepath.Join(root, stateDir)
			test.setup(t, marker)
			before, err := os.Lstat(marker)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := Init(root); err == nil {
				t.Fatal("Init() error = nil, want already exists error")
			}
			after, err := os.Lstat(marker)
			if err != nil {
				t.Fatalf("existing marker was removed: %v", err)
			}
			if after.Mode() != before.Mode() || after.Size() != before.Size() {
				t.Fatalf("marker changed: before=%v after=%v", before, after)
			}
		})
	}
}

func TestInitCleansUpPartialWrites(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writes := 0
	writeFailure := errors.New("simulated write failure")
	writeFile := func(path string, data []byte, mode os.FileMode) error {
		writes++
		if writes == 2 {
			if err := os.WriteFile(path, data[:2], mode); err != nil {
				t.Fatal(err)
			}
			return writeFailure
		}
		return os.WriteFile(path, data, mode)
	}

	if _, err := initProject(root, writeFile, os.RemoveAll); !errors.Is(err, writeFailure) {
		t.Fatalf("initProject() error = %v, want %v", err, writeFailure)
	}
	if _, err := os.Lstat(filepath.Join(root, stateDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".fledge error = %v, want cleaned up", err)
	}
}

func TestFindReturnsCanonicalNearestRoot(t *testing.T) {
	t.Parallel()

	outer := t.TempDir()
	if _, err := Init(outer); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(outer, "nested")
	leaf := filepath.Join(inner, "one", "two")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(inner); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(t.TempDir(), "linked-leaf")
	if err := os.Symlink(leaf, link); err != nil {
		t.Fatal(err)
	}
	got, err := Find(link)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if got != inner {
		t.Fatalf("Find() = %q, want nearest root %q", got, inner)
	}
}

func TestFindRejectsInvalidNearestBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		marker func(*testing.T, string)
		want   string
	}{
		{name: "marker file", marker: writeMarkerFile, want: "is not a directory"},
		{name: "marker symlink", marker: writeMarkerSymlink, want: "must not be a symlink"},
		{name: "missing config", marker: writeMarkerDirectory, want: "config.json"},
		{name: "config symlink", marker: writeMarkerWithConfigSymlink, want: "must not be a symlink"},
		{name: "malformed config", marker: writeMarkerWithConfig("{"), want: "parse Fledge config"},
		{name: "unsupported config", marker: writeMarkerWithConfig(`{"schema_version":2}`), want: "unsupported schema_version 2"},
		{name: "unknown field", marker: writeMarkerWithConfig(`{"schema_version":1,"extra":true}`), want: "unknown field"},
		{name: "duplicate field", marker: writeMarkerWithConfig(`{"schema_version":1,"schema_version":1}`), want: "duplicate field"},
		{name: "trailing content", marker: writeMarkerWithConfig(`{"schema_version":1}{}`), want: "unexpected trailing content"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
			test.marker(t, filepath.Join(inner, stateDir))

			if got, err := Find(leaf); err == nil {
				t.Fatalf("Find() = %q, nil; want boundary error", got)
			} else if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Find() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestFindWithoutProjectReturnsSentinel(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	leaf := filepath.Join(root, "leaf")
	if err := os.Mkdir(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(leaf); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("Find() error = %v, want ErrNotInitialized", err)
	}
}

func assertContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func writeMarkerDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeMarkerFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeMarkerSymlink(t *testing.T, path string) {
	t.Helper()
	if err := os.Symlink(t.TempDir(), path); err != nil {
		t.Fatal(err)
	}
}

func writeMarkerWithConfig(contents string) func(*testing.T, string) {
	return func(t *testing.T, path string) {
		t.Helper()
		writeMarkerDirectory(t, path)
		if err := os.WriteFile(filepath.Join(path, configFile), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeMarkerWithConfigSymlink(t *testing.T, path string) {
	t.Helper()
	writeMarkerDirectory(t, path)
	target := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(target, []byte(configContents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(path, configFile)); err != nil {
		t.Fatal(err)
	}
}
