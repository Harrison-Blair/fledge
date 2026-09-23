package spawn

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
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
func branchOf(t *testing.T, root string) string {
	t.Helper()
	b, err := exec.Command("git", "-C", root, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func newWorktreeListing(root string) herdr.WorktreeListResult {
	return herdr.WorktreeListResult{Type: "worktree_list", Source: struct {
		RepoRoot string `json:"repo_root"`
	}{RepoRoot: root}, Worktrees: []herdr.Worktree{}}
}

// An explicit --base is recorded as given, since it names the ref the
// checkout was created from.
func TestNewWorktreeRecordsExplicitBase(t *testing.T) {
	root := repository(t)
	path := filepath.Join(root, ".fledge", "worktrees", "worker")
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	o := validOptions()
	o.Worktree = "new"
	o.Base = "dev"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "worktree.list", Result: newWorktreeListing(root)}, call{Method: "worktree.create", Params: map[string]any{"cwd": root, "branch": "worker", "base": "dev", "path": path, "focus": false}, Result: herdr.CreatedResult{Type: "worktree_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p, Worktree: herdr.Worktree{Path: path}}}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), callerNotAgent())
	s.Cwd = root
	out := s.run(context.Background(), o, nil)
	if out.Status != "success" {
		t.Fatal(out)
	}
	if rec := stored(t, root, *out.Result.(*Result).ID); !rec.WorktreeCreated || rec.WorktreeBase == nil || *rec.WorktreeBase != "dev" {
		t.Fatalf("%+v", rec)
	}
}

// Opening an existing checkout associates it with the agent but records no
// creation, so cleanup never removes a checkout the agent merely borrowed.
func TestOpenedWorktreeRecordsNoCreation(t *testing.T) {
	root := repository(t)
	path := filepath.Join(root, ".fledge", "worktrees", "existing")
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	opened := false
	o := validOptions()
	o.Worktree = path
	o.Cwd = root
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "worktree.list", Result: newWorktreeListing(root)}, call{Method: "worktree.open", Result: herdr.CreatedResult{Type: "worktree_opened", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p, Worktree: herdr.Worktree{Path: path}, AlreadyOpen: &opened}}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), callerNotAgent())
	s.Cwd = root
	out := s.run(context.Background(), o, nil)
	if out.Status != "success" {
		t.Fatal(out)
	}
	if rec := stored(t, root, *out.Result.(*Result).ID); rec.WorktreePath == nil || *rec.WorktreePath != path || rec.WorktreeCreated || rec.WorktreeBase != nil {
		t.Fatalf("%+v", rec)
	}
}

func TestWorktreeOpenUsesPathSourceAndCreatesTabWhenAlreadyOpen(t *testing.T) {
	path := t.TempDir()
	p := herdrscript.Pane("w1:p2", "w1", "w1:t2")
	cwd := path
	p.Cwd = &cwd
	old := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	open := true
	o := validOptions()
	o.Worktree = path
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "worktree.list", Params: map[string]any{"cwd": path}, Result: herdr.WorktreeListResult{Type: "worktree_list", Source: struct {
		RepoRoot string `json:"repo_root"`
	}{RepoRoot: path}, Worktrees: []herdr.Worktree{}}}, call{Method: "worktree.open", Params: map[string]any{"cwd": path, "path": path, "focus": false}, Result: herdr.CreatedResult{Type: "worktree_opened", Workspace: herdr.Workspace{ID: "w1"}, Tab: herdr.Tab{ID: "w1:t1", WorkspaceID: "w1"}, RootPane: old, Worktree: herdr.Worktree{Path: path}, AlreadyOpen: &open}}, call{Method: "tab.create", Params: map[string]any{"workspace_id": "w1", "cwd": path, "focus": false}, Result: herdr.CreatedResult{Type: "tab_created", Tab: herdr.Tab{ID: "w1:t2", WorkspaceID: "w1"}, RootPane: p}}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	out := s.run(context.Background(), o, nil)
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
	for _, e := range out.Effects {
		if e.Kind == "workspace" && e.Action == "created" {
			t.Fatal("claimed existing workspace created")
		}
	}
}
func TestWorktreeCreateExplicitSourceAndManagedPrimary(t *testing.T) {
	root := repository(t)
	path := filepath.Join(root, ".fledge", "worktrees", "feature", "topic")
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
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
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "worktree.list", Params: map[string]any{"workspace_id": "w1"}, Result: listing}, call{Method: "worktree.create", Params: map[string]any{"workspace_id": "w1", "branch": "feature/topic", "base": "HEAD", "path": path, "focus": false}, Result: herdr.CreatedResult{Type: "worktree_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p, Worktree: herdr.Worktree{Path: path}}}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	s.Cwd = filepath.Join(root, "linked-checkout")
	out := s.run(context.Background(), o, nil)
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
	if got := out.Result.(*Result).WorktreePath; got == nil || *got != path {
		t.Fatal(got)
	}
}
func TestWorktreeNewSourceUsesResolvedRelativeCwd(t *testing.T) {
	root := repository(t)
	callerCwd := filepath.Dir(root)
	path := filepath.Join(root, ".fledge", "worktrees", "worker")
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	p.Cwd = &path
	o := validOptions()
	o.Worktree = "new"
	o.Cwd = filepath.Base(root)
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "worktree.list", Params: map[string]any{"cwd": root}, Result: herdr.WorktreeListResult{Type: "worktree_list", Source: struct {
		RepoRoot string `json:"repo_root"`
	}{RepoRoot: root}, Worktrees: []herdr.Worktree{}}}, call{Method: "worktree.create", Params: map[string]any{"cwd": root, "branch": "worker", "path": path, "focus": false}, Result: herdr.CreatedResult{Type: "worktree_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p, Worktree: herdr.Worktree{Path: path}}}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	s.Cwd = callerCwd
	out := s.run(context.Background(), o, nil)
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
	if got := out.Result.(*Result).WorktreePath; got == nil || *got != path {
		t.Fatal(got)
	}
}
func TestWorktreeFailurePreservesLocalEffects(t *testing.T) {
	root := repository(t)
	o := validOptions()
	o.Worktree = "new"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "worktree.list", Params: map[string]any{"cwd": root}, Result: herdr.WorktreeListResult{Type: "worktree_list", Source: struct {
		RepoRoot string `json:"repo_root"`
	}{RepoRoot: root}, Worktrees: []herdr.Worktree{}}}, call{Method: "worktree.create", Err: &herdr.Error{Code: "git_error", Message: "failed"}})
	s.Cwd = root
	out := s.run(context.Background(), o, nil)
	if out.Status != "partial" || out.Error.Phase != "worktree.create" || len(out.Effects) == 0 {
		t.Fatal(out)
	}
	if _, err := os.Stat(filepath.Join(root, ".fledge", ".gitignore")); err != nil {
		t.Fatal(err)
	}
}
func TestNewWorktreeFromLinkedCheckoutUsesPrimaryRoot(t *testing.T) {
	root := repository(t)
	linked := filepath.Join(t.TempDir(), "linked")
	if b, err := exec.Command("git", "-C", root, "worktree", "add", "-qb", "linked", linked).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	path := filepath.Join(root, ".fledge", "worktrees", "worker")
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	o := validOptions()
	o.Worktree = "new"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "worktree.list", Params: map[string]any{"cwd": linked}, Result: herdr.WorktreeListResult{Type: "worktree_list", Source: struct {
		RepoRoot string `json:"repo_root"`
	}{RepoRoot: root}, Worktrees: []herdr.Worktree{}}}, call{Method: "worktree.create", Params: map[string]any{"cwd": root, "branch": "worker", "path": path, "focus": false}, Result: herdr.CreatedResult{Type: "worktree_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p, Worktree: herdr.Worktree{Path: path}}}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), callerNotAgent())
	s.Cwd = linked
	out := s.run(context.Background(), o, nil)
	if out.Status != "success" {
		t.Fatal(out)
	}
	// The record lands in the primary checkout's store and names the checkout,
	// created by this spawn from the primary checkout's branch.
	rec := stored(t, root, *out.Result.(*Result).ID)
	if rec.WorktreePath == nil || *rec.WorktreePath != path || !rec.WorktreeCreated || rec.WorktreeBase == nil || *rec.WorktreeBase != branchOf(t, root) {
		t.Fatalf("%+v", rec)
	}
	if _, err := os.Stat(filepath.Join(linked, ".fledge")); !os.IsNotExist(err) {
		t.Fatal("created managed paths inside linked checkout")
	}
}
func TestNewlyOpenedWorktreeRenamesOnlyInitialTab(t *testing.T) {
	path := t.TempDir()
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	p.Cwd = &path
	opened := false
	o := validOptions()
	o.Worktree = path
	o.Tab = "tasks"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "worktree.list", Params: map[string]any{"cwd": path}, Result: herdr.WorktreeListResult{Type: "worktree_list", Source: struct {
		RepoRoot string `json:"repo_root"`
	}{RepoRoot: path}, Worktrees: []herdr.Worktree{}}}, call{Method: "worktree.open", Params: map[string]any{"cwd": path, "path": path, "focus": false}, Result: herdr.CreatedResult{Type: "worktree_opened", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "1"}, RootPane: p, Worktree: herdr.Worktree{Path: path}, AlreadyOpen: &opened}}, call{Method: "tab.rename", Params: map[string]any{"tab_id": "w2:t1", "label": "tasks"}, Result: herdr.TabResult{Type: "tab_info", Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2", Label: "tasks"}}}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	if out := s.run(context.Background(), o, nil); out.Status != "success" {
		t.Fatal(out)
	}
}
