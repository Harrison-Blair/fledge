package install_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/update/install"
)

func executable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fledge")
	if err := os.WriteFile(path, []byte("original"), 0751); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveExecutableFollowsSymlink(t *testing.T) {
	real := executable(t)
	link := filepath.Join(t.TempDir(), "fledge-link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	got, err := install.ResolveExecutable(link)
	if err != nil || got != real {
		t.Fatalf("ResolveExecutable() = %q, %v; want %q", got, err, real)
	}
}

func TestResolveExecutableDefaultsToRunningBinary(t *testing.T) {
	running, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(running)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := install.ResolveExecutable(""); err != nil || got != want {
		t.Fatalf("ResolveExecutable(\"\") = %q, %v; want %q", got, err, want)
	}
}

func TestResolveExecutableMissingTarget(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(missing, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{missing, link} {
		if _, err := install.ResolveExecutable(path); err == nil || !strings.HasPrefix(err.Error(), "update: resolve executable: ") {
			t.Fatalf("ResolveExecutable(%q) error = %v", path, err)
		}
	}
}

func TestReplacePreservesModeAndLeavesNoTemporaryFiles(t *testing.T) {
	path := executable(t)
	if err := install.Replace(path, []byte("new binary")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new binary" {
		t.Fatalf("binary = %q", got)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0751 {
		t.Fatalf("mode = %v, %v", info, err)
	}
	files, _ := os.ReadDir(filepath.Dir(path))
	if len(files) != 1 {
		t.Fatalf("temporary files remain: %v", files)
	}
}

func TestReplaceMissingExecutable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "fledge")
	if err := install.Replace(missing, []byte("new")); err == nil || !strings.HasPrefix(err.Error(), "update: stat executable: ") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("replace created the missing executable: %v", err)
	}
}

func TestReplacementFailurePreservesOriginal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses filesystem permissions")
	}
	path := executable(t)
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })
	if err := install.Replace(path, []byte("new")); err == nil || !strings.Contains(err.Error(), "write permission") {
		t.Fatalf("error = %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "original" {
		t.Fatalf("modified original: %q", got)
	}
}
