package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestOpenRegularRejectsDirectory(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	file, err := OpenRegular(directory, os.O_RDONLY, 0o600)
	if err == nil {
		_ = file.Close()
		t.Fatal("OpenRegular() accepted a directory")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("OpenRegular() error = %v, want a regular-file rejection", err)
	}
}

func TestOpenRegularRejectsSymlinkWithoutTouchingTarget(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	link := filepath.Join(directory, "link")
	if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	file, err := OpenRegular(link, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err == nil {
		_ = file.Close()
		t.Fatal("OpenRegular() opened a symlink")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("OpenRegular() error = %v, want a symlink rejection", err)
	}
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != "unchanged" {
		t.Fatalf("symlink target = %q, %v; want it untouched", contents, err)
	}
}

// TestValidateOpenedRejectsReplacedFile covers the window OpenRegular closes
// after the open returns: the descriptor is fine, but path no longer names it.
func TestValidateOpenedRejectsReplacedFile(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "log")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := OpenRegular(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := validateOpened(file, path); err != nil {
		t.Fatalf("validateOpened() before replacement error = %v", err)
	}

	replacement := filepath.Join(directory, "replacement")
	if err := os.WriteFile(replacement, []byte("planted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	err = validateOpened(file, path)
	if err == nil || !strings.Contains(err.Error(), "changed while opening") {
		t.Fatalf("validateOpened() after replacement error = %v, want a replacement rejection", err)
	}
}

func TestValidateOpenedRejectsSymlinkedPath(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "log")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := OpenRegular(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(directory, "elsewhere"), path); err != nil {
		t.Fatal(err)
	}
	err = validateOpened(file, path)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("validateOpened() error = %v, want a symlink rejection", err)
	}
}

func TestRejectSymlink(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	regular := filepath.Join(directory, "regular")
	if err := os.WriteFile(regular, []byte("contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RejectSymlink(regular); err != nil {
		t.Fatalf("RejectSymlink(regular file) = %v, want nil", err)
	}
	if err := RejectSymlink(filepath.Join(directory, "absent")); err != nil {
		t.Fatalf("RejectSymlink(absent path) = %v, want nil", err)
	}
	if err := RejectSymlink(directory); err != nil {
		t.Fatalf("RejectSymlink(directory) = %v, want nil", err)
	}

	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	err := RejectSymlink(link)
	if err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("RejectSymlink(symlink) = %v, want a symlink rejection", err)
	}
}

func TestSyncDirectory(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	if err := SyncDirectory(directory); err != nil {
		t.Fatalf("SyncDirectory() = %v, want nil", err)
	}
	err := SyncDirectory(filepath.Join(directory, "absent"))
	if err == nil || !strings.Contains(err.Error(), "open directory") {
		t.Fatalf("SyncDirectory(absent) = %v, want an open failure", err)
	}
}
