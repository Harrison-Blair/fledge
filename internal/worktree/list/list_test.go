package list

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
)

type call = herdrscript.Call

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=t@example.com"}, args...)...)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, b)
	}
}
func s(v string) *string { return &v }

// fixture is a primary checkout on main with a managed merged checkout, an
// unmanaged dirty unmerged checkout, and a detached checkout at main.
type fixture struct{ root, merged, unmerged, detached string }

func newFixture(t *testing.T) fixture {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "commit", "-qm", "initial", "--allow-empty")
	os.MkdirAll(filepath.Join(root, ".fledge"), 0o755)
	os.WriteFile(filepath.Join(root, ".fledge", ".gitignore"), []byte("*\n"), 0o644)
	outside, _ := filepath.EvalSymlinks(t.TempDir())
	f := fixture{root: root, merged: filepath.Join(root, ".fledge", "worktrees", "merged"), unmerged: filepath.Join(outside, "unmerged"), detached: filepath.Join(outside, "detached")}
	git(t, root, "worktree", "add", "-q", "-b", "merged", f.merged)
	git(t, f.merged, "commit", "-qm", "merged work", "--allow-empty")
	git(t, root, "merge", "-q", "--no-edit", "merged")
	git(t, root, "worktree", "add", "-q", "-b", "unmerged", f.unmerged)
	git(t, f.unmerged, "commit", "-qm", "unmerged work", "--allow-empty")
	os.WriteFile(filepath.Join(f.unmerged, "untracked"), []byte("x"), 0o644)
	git(t, root, "worktree", "add", "-q", "--detach", f.detached, "main")
	return f
}

func (f fixture) listing() herdr.WorktreeListResult {
	l := herdr.WorktreeListResult{Type: "worktree_list", Worktrees: []herdr.Worktree{
		{Path: f.merged, Branch: s("merged"), OpenWorkspaceID: s("w2")},
		{Path: f.root, Branch: s("main"), OpenWorkspaceID: s("w1")},
		{Path: f.unmerged, Branch: s("unmerged")},
		{Path: f.detached},
	}}
	l.Source.RepoRoot = f.root
	return l
}

func TestListInspectsEveryCheckout(t *testing.T) {
	f := newFixture(t)
	c := herdrscript.Client(t, call{Method: "worktree.list", Params: map[string]any{"cwd": f.unmerged}, Result: f.listing()})
	out := Run(context.Background(), c, Options{Cwd: f.unmerged})
	if out.Status != "success" || out.Operation != "worktree.list" {
		t.Fatalf("%+v", out)
	}
	r := out.Result.(Result)
	if r.RepoRoot != f.root || r.DefaultBranch == nil || *r.DefaultBranch != "main" {
		t.Fatalf("%+v", r)
	}
	want := []Row{
		{Path: f.root, Branch: s("main"), Primary: true, WorkspaceID: s("w1"), Dirty: "no", Merged: "yes", Managed: false},
		{Path: f.merged, Branch: s("merged"), WorkspaceID: s("w2"), Dirty: "no", Merged: "yes", Managed: true},
		{Path: f.unmerged, Branch: s("unmerged"), Dirty: "yes", Merged: "no"},
		{Path: f.detached, Dirty: "no", Merged: "yes"},
	}
	if len(r.Worktrees) != len(want) {
		t.Fatalf("%+v", r.Worktrees)
	}
	for i, w := range want {
		got := r.Worktrees[i]
		if got.Path != w.Path || str(got.Branch) != str(w.Branch) || got.Primary != w.Primary || str(got.WorkspaceID) != str(w.WorkspaceID) ||
			got.Dirty != w.Dirty || got.Merged != w.Merged || got.Managed != w.Managed || got.Owner != nil {
			t.Errorf("row %d: got %+v want %+v", i, got, w)
		}
	}
}

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestListDefaultsToProcessCwd(t *testing.T) {
	f := newFixture(t)
	c := herdrscript.Client(t, call{Method: "worktree.list", Result: f.listing()})
	c.Cwd = f.root
	c2 := herdrscript.Client(t, call{Method: "worktree.list", Params: map[string]any{"cwd": f.root}, Result: f.listing()})
	c2.Cwd = f.root
	if out := Run(context.Background(), c2, Options{}); out.Status != "success" {
		t.Fatalf("%+v", out)
	}
	if out := Run(context.Background(), c, Options{Cwd: "."}); out.Status != "success" {
		t.Fatalf("relative cwd: %+v", out)
	}
}

func TestPrunableCheckoutIsUnknown(t *testing.T) {
	f := newFixture(t)
	os.RemoveAll(f.unmerged)
	c := herdrscript.Client(t, call{Method: "worktree.list", Result: f.listing()})
	r := Run(context.Background(), c, Options{Cwd: f.root}).Result.(Result)
	if row := r.Worktrees[2]; row.Dirty != "unknown" || row.Merged != "no" {
		t.Fatalf("%+v", row)
	}
	if row := r.Worktrees[3]; row.Dirty != "no" {
		t.Fatalf("%+v", row)
	}
}

func TestListFailure(t *testing.T) {
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Err: errors.New("offline")}), Options{Cwd: "/repo"})
	if out.ExitCode() != 1 || out.Error.Phase != "worktree.list" || out.Status != "rejected" {
		t.Fatalf("%+v", out)
	}
}

func TestRender(t *testing.T) {
	r := Result{RepoRoot: "/repo", DefaultBranch: s("main"), Worktrees: []Row{
		{Path: "/repo", Branch: s("main"), Primary: true, WorkspaceID: s("w1"), Dirty: "no", Merged: "yes"},
		{Path: "/repo/.fledge/worktrees/x", Dirty: "unknown", Merged: "unknown", Managed: true},
	}}
	var b bytes.Buffer
	if err := (libagent.Outcome{Status: "success", Result: r}).Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	want := "PATH BRANCH WORKSPACE DIRTY MERGED MANAGED /repo (primary) main w1 no yes no /repo/.fledge/worktrees/x (detached) - unknown unknown yes"
	if got := strings.Join(strings.Fields(b.String()), " "); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	herdrscript.CheckOutputFailures(t, Render, libagent.Outcome{Result: r})
}
