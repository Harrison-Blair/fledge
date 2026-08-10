package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRegularOpensAndCreatesRegularFiles(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "created")

	file, err := OpenRegular(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("OpenRegular(create) error = %v", err)
	}
	if _, err := file.WriteString("contents"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenRegular(path, os.O_RDONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenRegular(reopen) error = %v", err)
	}
	defer reopened.Close()
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "contents" {
		t.Fatalf("contents = %q, %v; want %q", contents, err, "contents")
	}
}

func TestOpenRegularReportsMissingFile(t *testing.T) {
	t.Parallel()
	_, err := OpenRegular(filepath.Join(t.TempDir(), "absent"), os.O_RDONLY, 0o600)
	if !os.IsNotExist(err) {
		t.Fatalf("OpenRegular() error = %v, want a not-exist error", err)
	}
}

func TestWriteFileAtomicCreatesAndReplaces(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "record.json")

	if err := WriteFileAtomic(path, []byte("first value"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomic(create) error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}

	// A shorter replacement must leave no suffix from the previous contents.
	if err := WriteFileAtomic(path, []byte("short"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomic(replace) error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "short" {
		t.Fatalf("contents = %q, %v; want %q", contents, err, "short")
	}

	// No temporary files may be left behind.
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "record.json" {
		t.Fatalf("directory entries = %v; want only record.json", entries)
	}
}

func TestWriteFileAtomicWrapsRenameFailureAndRemovesTemp(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	// A non-empty directory at the destination path makes the final rename fail,
	// exercising the "replace" error branch after the temporary file is written.
	path := filepath.Join(directory, "target")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "child"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := WriteFileAtomic(path, []byte("data"), 0o600)
	if err == nil {
		t.Fatal("WriteFileAtomic() error = nil, want a rename failure")
	}
	if !strings.Contains(err.Error(), "replace") || !strings.Contains(err.Error(), path) {
		t.Errorf("WriteFileAtomic() error = %v, want a wrapped 'replace' error naming %q", err, path)
	}

	// The sibling temporary file must not survive a failed replacement.
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("leftover temporary file %q after failed rename", entry.Name())
		}
	}
}
