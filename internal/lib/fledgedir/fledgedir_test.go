package fledgedir

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

func git(t *testing.T, args ...string) {
	t.Helper()
	if b, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, b)
	}
}
func repository(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, "-C", root, "init", "-q")
	git(t, "-C", root, "-c", "user.name=Test", "-c", "user.email=t@example.com", "commit", "-qm", "initial", "--allow-empty")
	return root
}

func TestRootResolvesPrimaryFromLinkedWorktree(t *testing.T) {
	root := repository(t)
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, "-C", root, "worktree", "add", "-qb", "linked", linked)
	os.MkdirAll(filepath.Join(linked, "sub"), 0755)
	for _, cwd := range []string{root, linked, filepath.Join(linked, "sub")} {
		got, err := Root(context.Background(), cwd)
		if err != nil {
			t.Fatal(err)
		}
		if got != root {
			t.Fatalf("Root(%s) = %s want %s", cwd, got, root)
		}
	}
}
func TestRootResolvesSeparateGitDirAndSubmodule(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(base, "repo")
	git(t, "init", "-q", "--separate-git-dir", filepath.Join(base, "gitdir"), repo)
	git(t, "-C", repo, "-c", "user.name=Test", "-c", "user.email=t@example.com", "commit", "-qm", "initial", "--allow-empty")
	linked := filepath.Join(base, "linked")
	git(t, "-C", repo, "worktree", "add", "-qb", "linked", linked)
	// A separate git dir named .git must not be mistaken for a checkout's.
	named := filepath.Join(base, "named")
	os.Mkdir(filepath.Join(base, "elsewhere"), 0755)
	git(t, "init", "-q", "--separate-git-dir", filepath.Join(base, "elsewhere", ".git"), named)

	super := repository(t)
	child := repository(t)
	git(t, "-C", super, "-c", "protocol.file.allow=always", "submodule", "add", "-q", child, "sub")
	sub := filepath.Join(super, "sub")
	os.MkdirAll(filepath.Join(sub, "deep"), 0755)

	subLinked := filepath.Join(base, "sublinked")
	git(t, "-C", sub, "worktree", "add", "-qb", "sublinked", subLinked)

	for cwd, want := range map[string]string{repo: repo, named: named, sub: sub, filepath.Join(sub, "deep"): sub, subLinked: sub} {
		got, err := Root(context.Background(), cwd)
		if err != nil {
			t.Fatalf("Root(%s): %v", cwd, err)
		}
		if got != want {
			t.Fatalf("Root(%s) = %s want %s", cwd, got, want)
		}
	}
	// Git records no path back to a separate-git-dir primary checkout, so a
	// linked worktree of one must fail rather than guess.
	if got, err := Root(context.Background(), linked); err == nil {
		t.Fatalf("Root(%s) guessed %s", linked, got)
	}
}
func TestRootRejectsCheckoutOfAnotherRepository(t *testing.T) {
	root := repository(t)
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, "-C", root, "worktree", "add", "-qb", "linked", linked)
	git(t, "-C", root, "config", "core.worktree", repository(t))
	if got, err := Root(context.Background(), linked); err == nil {
		t.Fatalf("accepted foreign checkout %s", got)
	}
}
func TestRootRejectsNonRepositoryAndBare(t *testing.T) {
	bare := filepath.Join(t.TempDir(), "bare.git")
	git(t, "init", "-q", "--bare", bare)
	// A bare repository named .git must not pass as a checkout's git directory.
	dotGit := filepath.Join(t.TempDir(), ".git")
	git(t, "init", "-q", "--bare", dotGit)
	for _, cwd := range []string{t.TempDir(), bare, dotGit} {
		if got, err := Root(context.Background(), cwd); err == nil {
			t.Fatalf("Root(%s) accepted: %s", cwd, got)
		}
	}
}

func TestEnsureCreatesIgnoredDirectory(t *testing.T) {
	root := repository(t)
	out := libagent.Outcome{}
	dir, err := Ensure(root, &out)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(root, ".fledge") {
		t.Fatal(dir)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(b) != "*\n" {
		t.Fatalf("%q", b)
	}
	if len(out.Effects) != 2 || out.Effects[0] != (libagent.Effect{Action: "created", Kind: "directory", Path: dir}) || out.Effects[1] != (libagent.Effect{Action: "created", Kind: "file", Path: filepath.Join(dir, ".gitignore")}) {
		t.Fatalf("%+v", out.Effects)
	}
	if _, err = os.Stat(filepath.Join(root, ".gitignore")); !os.IsNotExist(err) {
		t.Fatal("root ignore changed")
	}
	out = libagent.Outcome{}
	if _, err = Ensure(root, &out); err != nil || len(out.Effects) != 0 {
		t.Fatalf("second ensure: %v %+v", err, out.Effects)
	}
}
func TestEnsureAppendsOnly(t *testing.T) {
	for _, existing := range []string{"# retained\n!keep\n", "# preserve"} {
		root := repository(t)
		dir := filepath.Join(root, ".fledge")
		os.Mkdir(dir, 0755)
		os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0644)
		out := libagent.Outcome{}
		if _, err := Ensure(root, &out); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if !strings.HasPrefix(string(b), existing) || !strings.HasSuffix(string(b), "\n*\n") || len(b) > len(existing)+3 {
			t.Fatalf("%q", b)
		}
		if len(out.Effects) != 1 || out.Effects[0].Action != "updated" {
			t.Fatalf("%+v", out.Effects)
		}
	}
}
func TestEnsureRejectsSymlinksWithoutMutation(t *testing.T) {
	for _, kind := range []string{"ignore", "directory", "not regular"} {
		t.Run(kind, func(t *testing.T) {
			root := repository(t)
			dir := filepath.Join(root, ".fledge")
			outside := t.TempDir()
			target := filepath.Join(outside, "keep")
			os.WriteFile(target, []byte("untouched"), 0644)
			switch kind {
			case "ignore":
				os.Mkdir(dir, 0755)
				os.Symlink(target, filepath.Join(dir, ".gitignore"))
			case "directory":
				os.Symlink(outside, dir)
			case "not regular":
				os.MkdirAll(filepath.Join(dir, ".gitignore"), 0755)
			}
			out := libagent.Outcome{}
			if _, err := Ensure(root, &out); err == nil {
				t.Fatal("accepted unsafe path")
			} else if !errors.As(err, new(*libagent.InputError)) {
				t.Fatalf("%T %v", err, err)
			}
			if len(out.Effects) != 0 {
				t.Fatalf("%+v", out.Effects)
			}
			if b, _ := os.ReadFile(target); string(b) != "untouched" {
				t.Fatalf("%q", b)
			}
		})
	}
}
func TestEnsureLeavesWorktreesIgnoreAlone(t *testing.T) {
	root := repository(t)
	legacy := filepath.Join(root, ".fledge", "worktrees", ".gitignore")
	os.MkdirAll(filepath.Dir(legacy), 0755)
	os.WriteFile(legacy, []byte("# old"), 0644)
	if _, err := Ensure(root, &libagent.Outcome{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(legacy); string(b) != "# old" {
		t.Fatalf("%q", b)
	}
}
func TestEnsureVerifiesIgnored(t *testing.T) {
	root := repository(t)
	// A later root rule cannot override .fledge/.gitignore, so simulate a
	// failed verification by making git unavailable after the writes.
	t.Setenv("PATH", "")
	out := libagent.Outcome{}
	if _, err := Ensure(root, &out); err == nil {
		t.Fatal("skipped verification")
	}
	if len(out.Effects) != 2 {
		t.Fatalf("lost effects: %+v", out.Effects)
	}
}
func TestParentsCreatedBeneathRoot(t *testing.T) {
	root := repository(t)
	parent := filepath.Join(root, "a", "b")
	if err := CheckParents(root, parent); err != nil {
		t.Fatal(err)
	}
	out := libagent.Outcome{}
	if err := MakeParents(root, parent, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Effects) != 2 || out.Effects[1].Path != parent {
		t.Fatalf("%+v", out.Effects)
	}
	os.Symlink(t.TempDir(), filepath.Join(root, "c"))
	if err := CheckParents(root, filepath.Join(root, "c", "d")); err == nil {
		t.Fatal("accepted symlinked parent")
	}
}
func TestIgnoreUpdateAppendsWithoutRewritingExistingBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	observed := []byte("# preserved\n")
	existing := append(append([]byte{}, observed...), []byte("# additional existing bytes\n")...)
	if err := os.WriteFile(path, existing, 0644); err != nil {
		t.Fatal(err)
	}
	out := libagent.Outcome{}
	if err := appendIgnoreRule(path, observed, false, &out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := string(existing) + "*\n"
	if string(got) != want {
		t.Fatalf("%q want %q", got, want)
	}
	if len(out.Effects) != 1 || out.Effects[0].Action != "updated" {
		t.Fatal(out.Effects)
	}
}

type failingIgnoreWriter struct {
	n                  int
	writeErr, closeErr error
}

func (w failingIgnoreWriter) Write([]byte) (int, error) { return w.n, w.writeErr }
func (w failingIgnoreWriter) Close() error              { return w.closeErr }
func TestIgnoreWriteEffectsReflectActualMutation(t *testing.T) {
	failure := errors.New("write failed")
	for _, tc := range []struct {
		name, action string
		writer       failingIgnoreWriter
		effects      int
	}{
		{"existing zero bytes", "updated", failingIgnoreWriter{writeErr: failure}, 0},
		{"existing partial write", "updated", failingIgnoreWriter{n: 1, writeErr: failure}, 1},
		{"new file zero bytes", "created", failingIgnoreWriter{writeErr: failure}, 1},
		{"close failure", "updated", failingIgnoreWriter{n: 2, closeErr: failure}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := libagent.Outcome{}
			err := writeIgnoreRule(tc.writer, []byte("*\n"), libagent.Effect{Action: tc.action, Kind: "file", Path: "ignore"}, &out)
			if err == nil {
				t.Fatal("lost write or close error")
			}
			if len(out.Effects) != tc.effects {
				t.Fatalf("effects: %+v", out.Effects)
			}
		})
	}
}
