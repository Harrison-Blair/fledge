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
	got, err := Root(context.Background(), linked)
	if err == nil {
		t.Fatalf("Root(%s) guessed %s", linked, got)
	}
	// The error names the one-time fix, which Root then honors.
	if !strings.Contains(err.Error(), "git config core.worktree <primary checkout path>") {
		t.Fatalf("%v", err)
	}
	git(t, "-C", repo, "config", "core.worktree", repo)
	if got, err := Root(context.Background(), linked); err != nil || got != repo {
		t.Fatalf("Root(%s) = %s, %v want %s", linked, got, err, repo)
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
	if string(b) != managedBlock {
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
		if !strings.HasPrefix(string(b), existing) || !strings.HasSuffix(string(b), "\n"+managedBlock) || len(b) > len(existing)+1+len(managedBlock) {
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
	want := string(existing) + managedBlock
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

// racedMkdir makes another creator win: appear puts something at path just
// before the real Mkdir runs.
func racedMkdir(t *testing.T, appear func(path string) error) {
	t.Helper()
	t.Cleanup(func() { mkdir = os.Mkdir })
	mkdir = func(path string, perm os.FileMode) error {
		if err := appear(path); err != nil {
			t.Fatal(err)
		}
		return os.Mkdir(path, perm)
	}
}

func TestMakeParentsToleratesConcurrentCreator(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "a")
	racedMkdir(t, func(path string) error { return os.Mkdir(path, 0o755) })
	out := libagent.Outcome{}
	if err := MakeParents(root, parent, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Effects) != 0 {
		t.Fatalf("losing creator recorded %+v", out.Effects)
	}
}

func TestMakeParentsRefusesSymlinkFromConcurrentCreator(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "a")
	target := t.TempDir()
	racedMkdir(t, func(path string) error { return os.Symlink(target, path) })
	out := libagent.Outcome{}
	if err := MakeParents(root, parent, &out); err == nil || len(out.Effects) != 0 {
		t.Fatalf("accepted symlink: %v %+v", err, out.Effects)
	}
}

func TestRootRejectsLinkedWorktreeOfBareRepository(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Git reports a linked worktree of a bare repository as non-bare. The bare
	// repository has no primary checkout, and Git refuses core.worktree
	// alongside core.bare, so the error must not advise setting it.
	for _, name := range []string{"repo.git", filepath.Join("named", ".git")} {
		bare := filepath.Join(base, name)
		git(t, "init", "-q", "--bare", bare)
		linked := filepath.Join(base, "linked-"+filepath.Base(filepath.Dir(bare)))
		git(t, "--git-dir", bare, "worktree", "add", "-q", "--orphan", "-b", "linked", linked)
		got, err := Root(context.Background(), linked)
		if err == nil {
			t.Fatalf("Root(%s) accepted: %s", linked, got)
		}
		if msg := err.Error(); !strings.Contains(msg, "bare repositories are not supported") || strings.Contains(msg, "core.worktree") {
			t.Fatalf("Root(%s): %v", linked, err)
		}
	}
}

const managedBlock = "*\n!/profiles/\n!/profiles/*.toml\n"

// ignored reports whether git ignores path, relative to root.
func ignored(t *testing.T, root, path string) bool {
	t.Helper()
	err := exec.Command("git", "-C", root, "check-ignore", "--no-index", "--quiet", "--", path).Run()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		t.Fatal(err)
	}
	return err == nil
}

// Profile TOML files are trackable while state, managed checkouts, the ignore
// file itself, and anything else under .fledge stay ignored, whether Ensure
// creates the file, migrates a legacy "*" file, or appends after user rules.
func TestEnsureLeavesOnlyProfileFilesTrackable(t *testing.T) {
	for _, tc := range []struct{ name, existing, want string }{
		{"fresh", "", managedBlock},
		{"legacy", "*\n", managedBlock},
		{"legacy with user rules", "# mine\n!keep\n*\n", "# mine\n!keep\n" + managedBlock},
		{"user rules after star", "*\n!keep", "*\n!keep\n" + managedBlock},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := repository(t)
			ignore := filepath.Join(root, ".fledge", ".gitignore")
			if tc.existing != "" {
				os.Mkdir(filepath.Dir(ignore), 0755)
				os.WriteFile(ignore, []byte(tc.existing), 0644)
			}
			for range 3 {
				if _, err := Ensure(root, &libagent.Outcome{}); err != nil {
					t.Fatal(err)
				}
			}
			if b, _ := os.ReadFile(ignore); string(b) != tc.want {
				t.Fatalf("%q want %q", b, tc.want)
			}
			for path, want := range map[string]bool{
				".fledge/profiles/reviewer.toml":    false,
				".fledge/.gitignore":                true,
				".fledge/state/agents/a.json":       true,
				".fledge/state/lock":                true,
				".fledge/worktrees/feat/x/file.go":  true,
				".fledge/worktrees/profiles/x.toml": true,
				".fledge/profiles/notes.md":         true,
				".fledge/profiles/sub/x.toml":       true,
				".fledge/state/profiles/x.toml":     true,
				".fledge/other.toml":                true,
			} {
				if got := ignored(t, root, path); got != want {
					t.Errorf("%s ignored = %v, want %v", path, got, want)
				}
			}
		})
	}
}
func TestEnsureRepeatedHasNoEffects(t *testing.T) {
	root := repository(t)
	dir := filepath.Join(root, ".fledge")
	os.Mkdir(dir, 0755)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0644)
	out := libagent.Outcome{}
	if _, err := Ensure(root, &out); err != nil || len(out.Effects) != 1 || out.Effects[0].Action != "updated" {
		t.Fatalf("migration: %v %+v", err, out.Effects)
	}
	out = libagent.Outcome{}
	if _, err := Ensure(root, &out); err != nil || len(out.Effects) != 0 {
		t.Fatalf("repeat: %v %+v", err, out.Effects)
	}
}
