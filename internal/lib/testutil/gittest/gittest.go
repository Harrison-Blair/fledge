// Package gittest runs Git in throwaway repositories for tests. It uses only
// the standard library, so any package's tests can import it. Production
// packages must not import it.
package gittest

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// Git runs git -C dir with args and a fixed test author, fails t when Git
// fails, and returns the combined output.
func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	b, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=t@example.com"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, b)
	}
	return string(b)
}

// Repository returns a fresh Git repository, with no commits, in a new
// temporary directory whose path has its symlinks resolved.
func Repository(t testing.TB) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	Git(t, root, "init", "-q")
	return root
}

// Commit adds an empty commit named "initial" to the repository at dir.
func Commit(t testing.TB, dir string) {
	t.Helper()
	Git(t, dir, "commit", "-qm", "initial", "--allow-empty")
}
