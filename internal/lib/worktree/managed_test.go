package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

func repository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=Test", "-c", "user.email=t@example.com", "commit", "-qm", "initial", "--allow-empty"}} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, b)
		}
	}
	return root
}
func TestManagedWorktreePreparation(t *testing.T) {
	root := repository(t)
	managed := filepath.Join(root, ".fledge", "worktrees")
	os.MkdirAll(managed, 0755)
	os.WriteFile(filepath.Join(root, ".fledge", ".gitignore"), []byte("# retained\n!keep\n"), 0644)
	os.WriteFile(filepath.Join(managed, ".gitignore"), []byte("# legacy"), 0644)
	out := libagent.Outcome{Effects: []libagent.Effect{}}
	path, err := Prepare(context.Background(), root, "feature/topic", &out)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(managed, "feature", "topic") {
		t.Fatal(path)
	}
	b, _ := os.ReadFile(filepath.Join(root, ".fledge", ".gitignore"))
	if string(b) != "# retained\n!keep\n*\n!/profiles/\n!/profiles/*.toml\n" {
		t.Fatalf("%q", b)
	}
	if b, _ = os.ReadFile(filepath.Join(managed, ".gitignore")); string(b) != "# legacy" {
		t.Fatalf("legacy ignore changed: %q", b)
	}
	if _, err = os.Stat(filepath.Join(root, ".gitignore")); !os.IsNotExist(err) {
		t.Fatal("root ignore changed")
	}
}
func TestManagedWorktreeRejectsSymlinksAndCollisions(t *testing.T) {
	for _, kind := range []string{"fledge", "managed", "branch parent", "ignore", "destination", "branch"} {
		t.Run(kind, func(t *testing.T) {
			root := repository(t)
			managed := filepath.Join(root, ".fledge", "worktrees")
			os.MkdirAll(managed, 0755)
			outside := t.TempDir()
			switch kind {
			case "fledge":
				os.RemoveAll(filepath.Join(root, ".fledge"))
				os.Symlink(outside, filepath.Join(root, ".fledge"))
			case "managed":
				os.Remove(managed)
				os.Symlink(outside, managed)
			case "branch parent":
				os.Symlink(outside, filepath.Join(managed, "feature"))
			case "ignore":
				os.WriteFile(filepath.Join(outside, "keep"), []byte("untouched"), 0644)
				os.Symlink(filepath.Join(outside, "keep"), filepath.Join(root, ".fledge", ".gitignore"))
			case "destination":
				os.MkdirAll(filepath.Join(managed, "feature", "topic"), 0755)
			case "branch":
				if err := exec.Command("git", "-C", root, "branch", "feature/topic").Run(); err != nil {
					t.Fatal(err)
				}
			}
			out := libagent.Outcome{}
			if _, err := Prepare(context.Background(), root, "feature/topic", &out); err == nil {
				t.Fatal("accepted unsafe path")
			}
			if len(out.Effects) > 0 {
				t.Fatalf("mutated before collision rejection: %+v", out.Effects)
			}
		})
	}
}
func TestBranchShorthandsRejected(t *testing.T) {
	root := repository(t)
	for _, branch := range []string{"@{-1}", "../escape", "HEAD", "-flag"} {
		out := libagent.Outcome{}
		if _, err := Prepare(context.Background(), root, branch, &out); err == nil {
			t.Fatalf("accepted %s", branch)
		}
	}
}
func TestIgnoreWithoutNewline(t *testing.T) {
	root := repository(t)
	path := filepath.Join(root, ".fledge")
	os.MkdirAll(path, 0755)
	os.WriteFile(filepath.Join(path, ".gitignore"), []byte("# preserve"), 0644)
	out := libagent.Outcome{}
	if _, err := Prepare(context.Background(), root, "topic", &out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(path, ".gitignore"))
	if string(b) != "# preserve\n*\n!/profiles/\n!/profiles/*.toml\n" {
		t.Fatalf("%q", b)
	}
}
func TestBranchValidationRuntimeErrors(t *testing.T) {
	root := repository(t)
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		out := libagent.Outcome{}
		_, err := Prepare(ctx, root, "valid", &out)
		out.Fail(err, "preflight", false)
		if out.ExitCode() != 1 {
			t.Fatalf("%+v", out)
		}
	})
	t.Run("git missing", func(t *testing.T) {
		t.Setenv("PATH", "")
		out := libagent.Outcome{}
		_, err := Prepare(context.Background(), root, "valid", &out)
		out.Fail(err, "preflight", false)
		if out.ExitCode() != 1 {
			t.Fatalf("%+v", out)
		}
	})
}
func TestBranchNamespaceCollisionsBeforeWrites(t *testing.T) {
	for _, tc := range []struct{ existing, requested string }{{"feature", "feature/topic"}, {"feature/topic", "feature"}} {
		t.Run(tc.requested, func(t *testing.T) {
			root := repository(t)
			if err := exec.Command("git", "-C", root, "branch", tc.existing).Run(); err != nil {
				t.Fatal(err)
			}
			out := libagent.Outcome{}
			_, err := Prepare(context.Background(), root, tc.requested, &out)
			if err == nil {
				t.Fatal("accepted branch namespace collision")
			}
			if len(out.Effects) != 0 {
				t.Fatalf("mutated before rejection: %+v", out.Effects)
			}
			if _, err := os.Stat(filepath.Join(root, ".fledge")); !os.IsNotExist(err) {
				t.Fatal("created managed directory before rejecting collision")
			}
		})
	}
}

func TestManaged(t *testing.T) {
	root := "/repo"
	for path, want := range map[string]bool{
		"/repo/.fledge/worktrees/topic":         true,
		"/repo/.fledge/worktrees/feature/topic": true,
		"/repo/.fledge/worktrees":               false,
		"/repo/.fledge/worktreesX/topic":        false,
		"/repo/.fledge/topic":                   false,
		"/repo":                                 false,
		"/elsewhere/.fledge/worktrees/topic":    false,
	} {
		if got := Managed(root, path); got != want {
			t.Errorf("Managed(%s) = %v, want %v", path, got, want)
		}
	}
}
