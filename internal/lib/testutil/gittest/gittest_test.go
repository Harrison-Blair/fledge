package gittest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A repository is a fresh repository root with no commits, and its path has
// no symlinks even when the temporary directory is reached through one.
func TestRepository(t *testing.T) {
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", link)
	// A subtest, since t.TempDir keeps the directory it first made.
	t.Run("through symlink", func(t *testing.T) {
		root := Repository(t)
		if resolved, err := filepath.EvalSymlinks(root); err != nil || resolved != root {
			t.Fatalf("%q resolves to %q: %v", root, resolved, err)
		}
		if got := Git(t, root, "rev-parse", "--show-toplevel"); got != root+"\n" {
			t.Fatalf("top level %q, want %q", got, root)
		}
		if got := Git(t, root, "rev-list", "--all", "--count"); got != "0\n" {
			t.Fatalf("%q commits", got)
		}
	})
}

// Commit adds one empty commit by the fixed test author, which Git uses for
// every command.
func TestCommitUsesTestAuthor(t *testing.T) {
	root := Repository(t)
	Commit(t, root)
	if got := Git(t, root, "log", "--format=%an <%ae> %s"); got != "Test <t@example.com> initial\n" {
		t.Fatalf("%q", got)
	}
	if got := Git(t, root, "show", "--name-only", "--format=", "HEAD"); got != "" {
		t.Fatalf("commit is not empty: %q", got)
	}
}

// fatal records the first Fatalf and stops the calling goroutine, as
// testing.T does.
type fatal struct {
	testing.TB
	msg string
}

func (f *fatal) Helper() {}
func (f *fatal) Fatalf(format string, args ...any) {
	f.msg = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// A failed command fails the test with the command and Git's output.
func TestGitFailsOnError(t *testing.T) {
	f := &fatal{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		Git(f, t.TempDir(), "rev-parse", "--verify", "HEAD")
		f.msg = "returned"
	}()
	<-done
	if !strings.Contains(f.msg, "rev-parse") || !strings.Contains(f.msg, "fatal:") {
		t.Fatalf("%q", f.msg)
	}
}
