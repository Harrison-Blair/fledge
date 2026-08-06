package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newPreferencesRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, stateDirectory), 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPreferencesRoundTrip(t *testing.T) {
	t.Parallel()

	root := newPreferencesRoot(t)
	first := preferences{Version: preferencesVersion, Harness: "claude", Model: "opus"}
	if err := writePreferences(root, first); err != nil {
		t.Fatal(err)
	}

	value, found, err := readPreferences(root)
	if err != nil || !found || value != first {
		t.Fatalf("readPreferences() = %#v, %v, %v; want %#v, true, nil", value, found, err, first)
	}

	info, err := os.Stat(preferencesPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("preferences file mode = %v; want %v", mode, os.FileMode(0o600))
	}

	// Loosen the existing file's mode: the atomic replacement must land a fresh
	// 0600 temp file, not inherit whatever mode the old destination carried.
	if err := os.Chmod(preferencesPath(root), 0o644); err != nil {
		t.Fatal(err)
	}

	second := preferences{Version: preferencesVersion, Harness: "codex"}
	if err := writePreferences(root, second); err != nil {
		t.Fatal(err)
	}

	value, found, err = readPreferences(root)
	if err != nil || !found || value != second {
		t.Fatalf("readPreferences() = %#v, %v, %v; want %#v, true, nil", value, found, err, second)
	}

	info, err = os.Stat(preferencesPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("replaced preferences file mode = %v; want %v", mode, os.FileMode(0o600))
	}

	leftovers, err := filepath.Glob(filepath.Join(root, stateDirectory, ".preferences-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("temporary preferences files left behind: %v", leftovers)
	}
}

func TestWritePreferencesRejectsSymlinkWithoutTouchingTarget(t *testing.T) {
	t.Parallel()

	root := newPreferencesRoot(t)
	target := filepath.Join(root, "sentinel.json")
	const sentinel = "do not touch\n"
	if err := os.WriteFile(target, []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, preferencesPath(root)); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if err := writePreferences(root, preferences{Version: preferencesVersion, Harness: "claude"}); err == nil {
		t.Fatal("writePreferences() accepted a symlinked destination")
	}
	if contents, err := os.ReadFile(target); err != nil || string(contents) != sentinel {
		t.Fatalf("sentinel target = %q, %v; want unchanged %q", contents, err, sentinel)
	}
}

func TestReadPreferencesMissing(t *testing.T) {
	t.Parallel()

	value, found, err := readPreferences(t.TempDir())
	if err != nil || found || value != (preferences{}) {
		t.Fatalf("readPreferences() = %#v, %v, %v; want zero value, false, nil", value, found, err)
	}
}

func TestReadPreferencesInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
	}{
		{name: "corrupt json", contents: "{not json"},
		{name: "wrong version", contents: `{"version":2,"harness":"claude"}`},
		{name: "empty harness", contents: `{"version":1,"harness":""}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := newPreferencesRoot(t)
			path := preferencesPath(root)
			if err := os.WriteFile(path, []byte(test.contents), 0o600); err != nil {
				t.Fatal(err)
			}

			value, found, err := readPreferences(root)
			if err == nil {
				t.Fatalf("readPreferences() = %#v, %v, nil; want error", value, found)
			}
			if found || value != (preferences{}) {
				t.Fatalf("readPreferences() = %#v, %v; want zero value, false", value, found)
			}
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("readPreferences() error = %q; want it to contain %q", err, path)
			}
		})
	}
}
