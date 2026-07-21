package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCreatesTree(t *testing.T) {
	root := t.TempDir()

	existed, err := Ensure(root)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if existed {
		t.Error("existed = true on a fresh directory, want false")
	}

	for _, want := range []string{
		"pluma/plumage",
		"pluma/feathers",
		"locks",
		"context",
		"flocks",
		".fledgeignore",
		AgentsName,
	} {
		if _, err := os.Stat(filepath.Join(root, DirName, filepath.FromSlash(want))); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

func TestEnsureRefreshPreservesIgnoreAndRestoresDirs(t *testing.T) {
	root := t.TempDir()
	if _, err := Ensure(root); err != nil {
		t.Fatalf("first Ensure: %v", err)
	}

	ignore := filepath.Join(root, DirName, ".fledgeignore")
	if err := os.WriteFile(ignore, []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(root, DirName, AgentsName)
	if err := os.WriteFile(agents, []byte("{\"edited\":{}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, DirName, "locks")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, DirName, "pluma", "plumage")); err != nil {
		t.Fatal(err)
	}

	existed, err := Ensure(root)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if !existed {
		t.Error("existed = false on refresh, want true")
	}

	for _, want := range []string{"locks", "pluma/plumage"} {
		if _, err := os.Stat(filepath.Join(root, DirName, filepath.FromSlash(want))); err != nil {
			t.Errorf("refresh did not restore %s: %v", want, err)
		}
	}
	got, err := os.ReadFile(ignore)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "edited\n" {
		t.Errorf("refresh clobbered .fledgeignore: got %q", got)
	}
	got, err = os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{\"edited\":{}}\n" {
		t.Errorf("refresh clobbered %s: got %q", AgentsName, got)
	}
}

func TestEnsureGitignore(t *testing.T) {
	write := func(t *testing.T, root, content string) string {
		t.Helper()
		name := filepath.Join(root, ".gitignore")
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return name
	}
	read := func(t *testing.T, name string) string {
		t.Helper()
		got, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return string(got)
	}

	t.Run("missing file is a no-op", func(t *testing.T) {
		root := t.TempDir()
		added, err := EnsureGitignore(root)
		if err != nil {
			t.Fatalf("EnsureGitignore: %v", err)
		}
		if added != nil {
			t.Errorf("added = %v, want nil", added)
		}
		if _, err := os.Stat(filepath.Join(root, ".gitignore")); !os.IsNotExist(err) {
			t.Errorf(".gitignore was created: %v", err)
		}
	})

	t.Run("appends both entries", func(t *testing.T) {
		root := t.TempDir()
		name := write(t, root, "bin/\n")
		added, err := EnsureGitignore(root)
		if err != nil {
			t.Fatalf("EnsureGitignore: %v", err)
		}
		if len(added) != 2 {
			t.Fatalf("added = %v, want both entries", added)
		}
		if got, want := read(t, name), "bin/\n\n# fledge\n.fledge/locks/\n.fledge/flocks/\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("adds newline before appending", func(t *testing.T) {
		root := t.TempDir()
		name := write(t, root, "bin/")
		if _, err := EnsureGitignore(root); err != nil {
			t.Fatalf("EnsureGitignore: %v", err)
		}
		if got, want := read(t, name), "bin/\n\n# fledge\n.fledge/locks/\n.fledge/flocks/\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("skips entries already present", func(t *testing.T) {
		root := t.TempDir()
		name := write(t, root, ".fledge/locks/\n")
		added, err := EnsureGitignore(root)
		if err != nil {
			t.Fatalf("EnsureGitignore: %v", err)
		}
		if len(added) != 1 || added[0] != ".fledge/flocks/" {
			t.Errorf("added = %v, want only .fledge/flocks/", added)
		}
		if got, want := read(t, name), ".fledge/locks/\n\n# fledge\n.fledge/flocks/\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("broader pattern covers both", func(t *testing.T) {
		root := t.TempDir()
		name := write(t, root, ".fledge/\n")
		added, err := EnsureGitignore(root)
		if err != nil {
			t.Fatalf("EnsureGitignore: %v", err)
		}
		if added != nil {
			t.Errorf("added = %v, want nil", added)
		}
		if got, want := read(t, name), ".fledge/\n"; got != want {
			t.Errorf("file changed: got %q, want %q", got, want)
		}
	})

	t.Run("no extra blank after a trailing blank line", func(t *testing.T) {
		root := t.TempDir()
		name := write(t, root, "bin/\n\n")
		if _, err := EnsureGitignore(root); err != nil {
			t.Fatalf("EnsureGitignore: %v", err)
		}
		if got, want := read(t, name), "bin/\n\n# fledge\n.fledge/locks/\n.fledge/flocks/\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("updates an existing block in place", func(t *testing.T) {
		root := t.TempDir()
		name := write(t, root, "# fledge\n.fledge/locks/\n")
		if _, err := EnsureGitignore(root); err != nil {
			t.Fatalf("EnsureGitignore: %v", err)
		}
		if got, want := read(t, name), "# fledge\n.fledge/locks/\n.fledge/flocks/\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("updates a block in the middle of the file", func(t *testing.T) {
		root := t.TempDir()
		name := write(t, root, "# fledge\n.fledge/locks/\n\nbin/\n")
		if _, err := EnsureGitignore(root); err != nil {
			t.Fatalf("EnsureGitignore: %v", err)
		}
		if got, want := read(t, name), "# fledge\n.fledge/locks/\n.fledge/flocks/\n\nbin/\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		root := t.TempDir()
		name := write(t, root, "")
		if _, err := EnsureGitignore(root); err != nil {
			t.Fatal(err)
		}
		before := read(t, name)
		added, err := EnsureGitignore(root)
		if err != nil {
			t.Fatal(err)
		}
		if added != nil {
			t.Errorf("second call added %v, want nil", added)
		}
		if got := read(t, name); got != before {
			t.Errorf("second call changed file: got %q, want %q", got, before)
		}
	})
}
