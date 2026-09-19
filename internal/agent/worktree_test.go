package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
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
	os.WriteFile(filepath.Join(managed, ".gitignore"), []byte("# retained\n!keep\n"), 0644)
	out := Outcome{Effects: []Effect{}}
	path, err := prepareWorktree(context.Background(), root, "feature/topic", &out)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(managed, "feature", "topic") {
		t.Fatal(path)
	}
	b, _ := os.ReadFile(filepath.Join(managed, ".gitignore"))
	if string(b) != "# retained\n!keep\n*\n" {
		t.Fatalf("%q", b)
	}
	if _, err = os.Stat(filepath.Join(root, ".gitignore")); !os.IsNotExist(err) {
		t.Fatal("root ignore changed")
	}
}
func TestManagedWorktreeRejectsSymlinksAndCollisions(t *testing.T) {
	for _, kind := range []string{"managed", "branch parent", "ignore", "destination", "branch"} {
		t.Run(kind, func(t *testing.T) {
			root := repository(t)
			managed := filepath.Join(root, ".fledge", "worktrees")
			os.MkdirAll(managed, 0755)
			outside := t.TempDir()
			switch kind {
			case "managed":
				os.Remove(managed)
				os.Symlink(outside, managed)
			case "branch parent":
				os.Symlink(outside, filepath.Join(managed, "feature"))
			case "ignore":
				os.WriteFile(filepath.Join(outside, "keep"), []byte("untouched"), 0644)
				os.Symlink(filepath.Join(outside, "keep"), filepath.Join(managed, ".gitignore"))
			case "destination":
				os.MkdirAll(filepath.Join(managed, "feature", "topic"), 0755)
			case "branch":
				if err := exec.Command("git", "-C", root, "branch", "feature/topic").Run(); err != nil {
					t.Fatal(err)
				}
			}
			out := Outcome{}
			if _, err := prepareWorktree(context.Background(), root, "feature/topic", &out); err == nil {
				t.Fatal("accepted unsafe path")
			}
			if len(out.Effects) > 0 {
				t.Fatalf("mutated before collision rejection: %+v", out.Effects)
			}
		})
	}
}
func TestWorktreeOpenUsesPathSourceAndCreatesTabWhenAlreadyOpen(t *testing.T) {
	path := t.TempDir()
	p := pane("w1:p2", "w1", "w1:t2")
	cwd := path
	p.Cwd = &cwd
	old := pane("w1:p1", "w1", "w1:t1")
	open := true
	o := validOptions()
	o.Worktree = path
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "worktree.list", params: map[string]any{"cwd": path}, result: herdr.WorktreeListResult{Type: "worktree_list", Source: struct {
		RepoRoot string `json:"repo_root"`
	}{RepoRoot: path}, Worktrees: []herdr.Worktree{}}}, call{method: "worktree.open", params: map[string]any{"cwd": path, "path": path, "focus": false}, result: herdr.CreatedResult{Type: "worktree_opened", Workspace: herdr.Workspace{ID: "w1"}, Tab: herdr.Tab{ID: "w1:t1", WorkspaceID: "w1"}, RootPane: old, Worktree: herdr.Worktree{Path: path}, AlreadyOpen: &open}}, call{method: "tab.create", params: map[string]any{"workspace_id": "w1", "cwd": path, "focus": false}, result: herdr.CreatedResult{Type: "tab_created", Tab: herdr.Tab{ID: "w1:t2", WorkspaceID: "w1"}, RootPane: p}}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"))
	out := s.Spawn(context.Background(), o)
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
	for _, e := range out.Effects {
		if e.Kind == "workspace" && e.Action == "created" {
			t.Fatal("claimed existing workspace created")
		}
	}
}
func TestBranchShorthandsRejected(t *testing.T) {
	root := repository(t)
	for _, branch := range []string{"@{-1}", "../escape", "HEAD", "-flag"} {
		out := Outcome{}
		if _, err := prepareWorktree(context.Background(), root, branch, &out); err == nil {
			t.Fatalf("accepted %s", branch)
		}
	}
}
func TestIgnoreWithoutNewline(t *testing.T) {
	root := repository(t)
	path := filepath.Join(root, ".fledge", "worktrees")
	os.MkdirAll(path, 0755)
	os.WriteFile(filepath.Join(path, ".gitignore"), []byte("# preserve"), 0644)
	out := Outcome{}
	if _, err := prepareWorktree(context.Background(), root, "topic", &out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(path, ".gitignore"))
	if !strings.HasSuffix(string(b), "\n*\n") {
		t.Fatalf("%q", b)
	}
}
func TestWorktreeCreateExplicitSourceAndManagedPrimary(t *testing.T) {
	root := repository(t)
	path := filepath.Join(root, ".fledge", "worktrees", "feature", "topic")
	p := pane("w2:p1", "w2", "w2:t1")
	p.Cwd = &path
	listing := herdr.WorktreeListResult{Type: "worktree_list", Source: struct {
		RepoRoot string `json:"repo_root"`
	}{RepoRoot: root}, Worktrees: []herdr.Worktree{}}
	o := validOptions()
	o.Worktree = "new"
	o.Workspace = "main"
	o.Cwd = "/must-not-be-used"
	o.Branch = "feature/topic"
	o.Base = "HEAD"
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "worktree.list", params: map[string]any{"workspace_id": "w1"}, result: listing}, call{method: "worktree.create", params: map[string]any{"workspace_id": "w1", "branch": "feature/topic", "base": "HEAD", "path": path, "focus": false}, result: herdr.CreatedResult{Type: "worktree_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p, Worktree: herdr.Worktree{Path: path}}}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"))
	s.Cwd = filepath.Join(root, "linked-checkout")
	out := s.Spawn(context.Background(), o)
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
	if got := out.Result.(*SpawnResult).WorktreePath; got == nil || *got != path {
		t.Fatal(got)
	}
}
func TestWorktreeFailurePreservesLocalEffects(t *testing.T) {
	root := repository(t)
	o := validOptions()
	o.Worktree = "new"
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "worktree.list", params: map[string]any{"cwd": root}, result: herdr.WorktreeListResult{Type: "worktree_list", Source: struct {
		RepoRoot string `json:"repo_root"`
	}{RepoRoot: root}, Worktrees: []herdr.Worktree{}}}, call{method: "worktree.create", err: &herdr.Error{Code: "git_error", Message: "failed"}})
	s.Cwd = root
	out := s.Spawn(context.Background(), o)
	if out.Status != "partial" || out.Error.Phase != "worktree.create" || len(out.Effects) == 0 {
		t.Fatal(out)
	}
	if _, err := os.Stat(filepath.Join(root, ".fledge", "worktrees", ".gitignore")); err != nil {
		t.Fatal(err)
	}
}
func TestBranchValidationRuntimeErrors(t *testing.T) {
	root := repository(t)
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		out := Outcome{}
		_, err := prepareWorktree(ctx, root, "valid", &out)
		out.fail(err, "preflight", false)
		if out.ExitCode() != 1 {
			t.Fatalf("%+v", out)
		}
	})
	t.Run("git missing", func(t *testing.T) {
		t.Setenv("PATH", "")
		out := Outcome{}
		_, err := prepareWorktree(context.Background(), root, "valid", &out)
		out.fail(err, "preflight", false)
		if out.ExitCode() != 1 {
			t.Fatalf("%+v", out)
		}
	})
}
func TestNewWorktreeFromLinkedCheckoutUsesPrimaryRoot(t *testing.T) {
	root := repository(t)
	linked := filepath.Join(t.TempDir(), "linked")
	if b, err := exec.Command("git", "-C", root, "worktree", "add", "-qb", "linked", linked).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	path := filepath.Join(root, ".fledge", "worktrees", "worker")
	p := pane("w2:p1", "w2", "w2:t1")
	o := validOptions()
	o.Worktree = "new"
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "worktree.list", params: map[string]any{"cwd": linked}, result: herdr.WorktreeListResult{Type: "worktree_list", Source: struct {
		RepoRoot string `json:"repo_root"`
	}{RepoRoot: root}, Worktrees: []herdr.Worktree{}}}, call{method: "worktree.create", params: map[string]any{"cwd": linked, "branch": "worker", "path": path, "focus": false}, result: herdr.CreatedResult{Type: "worktree_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p, Worktree: herdr.Worktree{Path: path}}}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"))
	s.Cwd = linked
	out := s.Spawn(context.Background(), o)
	if out.Status != "success" {
		t.Fatal(out)
	}
	if _, err := os.Stat(filepath.Join(linked, ".fledge")); !os.IsNotExist(err) {
		t.Fatal("created managed paths inside linked checkout")
	}
}
func TestBranchNamespaceCollisionsBeforeWrites(t *testing.T) {
	for _, tc := range []struct{ existing, requested string }{{"feature", "feature/topic"}, {"feature/topic", "feature"}} {
		t.Run(tc.requested, func(t *testing.T) {
			root := repository(t)
			if err := exec.Command("git", "-C", root, "branch", tc.existing).Run(); err != nil {
				t.Fatal(err)
			}
			out := Outcome{}
			_, err := prepareWorktree(context.Background(), root, tc.requested, &out)
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
func TestNewlyOpenedWorktreeRenamesOnlyInitialTab(t *testing.T) {
	path := t.TempDir()
	p := pane("w2:p1", "w2", "w2:t1")
	p.Cwd = &path
	opened := false
	o := validOptions()
	o.Worktree = path
	o.Tab = "tasks"
	s := fake(t, call{method: "session.snapshot", result: snapshot()}, call{method: "worktree.list", params: map[string]any{"cwd": path}, result: herdr.WorktreeListResult{Type: "worktree_list", Source: struct {
		RepoRoot string `json:"repo_root"`
	}{RepoRoot: path}, Worktrees: []herdr.Worktree{}}}, call{method: "worktree.open", params: map[string]any{"cwd": path, "path": path, "focus": false}, result: herdr.CreatedResult{Type: "worktree_opened", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "1"}, RootPane: p, Worktree: herdr.Worktree{Path: path}, AlreadyOpen: &opened}}, call{method: "tab.rename", params: map[string]any{"tab_id": "w2:t1", "label": "tasks"}, result: herdr.TabResult{Type: "tab_info", Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "tasks"}}}, call{method: "agent.start", result: started(p)}, waitCall("worker", p, "idle"))
	if out := s.Spawn(context.Background(), o); out.Status != "success" {
		t.Fatal(out)
	}
}
func TestIgnoreUpdateAppendsWithoutRewritingExistingBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	observed := []byte("# preserved\n")
	existing := append(append([]byte{}, observed...), []byte("# additional existing bytes\n")...)
	if err := os.WriteFile(path, existing, 0644); err != nil {
		t.Fatal(err)
	}
	out := Outcome{}
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
			out := Outcome{}
			err := writeIgnoreRule(tc.writer, []byte("*\n"), Effect{Action: tc.action, Kind: "file", Path: "ignore"}, &out)
			if err == nil {
				t.Fatal("lost write or close error")
			}
			if len(out.Effects) != tc.effects {
				t.Fatalf("effects: %+v", out.Effects)
			}
		})
	}
}
