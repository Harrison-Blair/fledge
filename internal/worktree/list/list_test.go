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
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
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
		{Path: "/repo/.fledge/worktrees/x", Dirty: "unknown", Merged: "unknown", Managed: true, Owner: &Owner{ID: "0a1b2c3d", Name: s("alpha"), Pane: "w1:p2"}, OwnerCount: 2},
		{Path: "/repo/.fledge/worktrees/y", Branch: s("y"), Dirty: "no", Merged: "no", Managed: true, Owner: &Owner{ID: "4e5f6a7b", Pane: "w1:p3"}, OwnerCount: 1},
	}}
	var b bytes.Buffer
	if err := (libagent.Outcome{Status: "success", Result: r}).Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	want := "PATH BRANCH WORKSPACE DIRTY MERGED MANAGED OWNER /repo (primary) main w1 no yes no - " +
		"/repo/.fledge/worktrees/x (detached) - unknown unknown yes alpha (0a1b2c3d) +1 /repo/.fledge/worktrees/y y - no no yes 4e5f6a7b"
	if got := strings.Join(strings.Fields(b.String()), " "); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	herdrscript.CheckOutputFailures(t, Render, libagent.Outcome{Result: r})
}

// record stores an agent record naming terminal with worktree path at when.
func record(t *testing.T, root, terminal, name, worktree, at string) string {
	t.Helper()
	s, err := identity.OpenStore(context.Background(), root, &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Create(identity.Kind, func(id string) any {
		return identity.Record{ID: id, Name: s2(name), Pane: "w1:" + terminal, WorkspaceID: "w1", TerminalID: terminal, RegisteredAt: at, RegisteredBy: "spawn", WorktreePath: &worktree}
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func s2(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func liveAgents(terminals ...string) call {
	agents := []herdr.AgentDetails{}
	for _, term := range terminals {
		agents = append(agents, herdr.AgentDetails{Pane: herdr.Pane{PaneID: "w1:" + term}, TerminalID: term})
	}
	return call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": agents}}
}

func TestListReportsLiveOwners(t *testing.T) {
	t.Setenv("HERDR_SESSION", "")
	f := newFixture(t)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(f.root, link); err != nil {
		t.Fatal(err)
	}
	primary := record(t, f.root, "t1", "", link+"/", "2026-01-01T00:00:00Z")
	merged := record(t, f.root, "t2", "alpha", f.merged, "2026-01-01T00:00:00Z")
	record(t, f.root, "t9", "gone", f.unmerged, "2026-01-01T00:00:00Z")
	record(t, f.root, "t3", "late", f.detached, "2026-03-01T00:00:00Z")
	early := record(t, f.root, "t4", "early", f.detached, "2026-02-01T00:00:00Z")
	c := herdrscript.Client(t, call{Method: "worktree.list", Result: f.listing()}, liveAgents("t1", "t2", "t3", "t4"))
	out := Run(context.Background(), c, Options{Cwd: f.root})
	r := out.Result.(Result)
	want := []struct {
		id, name string
		count    int
	}{{primary, "", 1}, {merged, "alpha", 1}, {"", "", 0}, {early, "early", 2}}
	for i, w := range want {
		row := r.Worktrees[i]
		if w.id == "" {
			if row.Owner != nil || row.OwnerCount != 0 {
				t.Errorf("row %d: stale owner %+v", i, row.Owner)
			}
			continue
		}
		if row.Owner == nil || row.Owner.ID != w.id || str(row.Owner.Name) != str(s2(w.name)) || row.Owner.Pane == "" || row.OwnerCount != w.count {
			t.Errorf("row %d: got %+v %d want %+v", i, row.Owner, row.OwnerCount, w)
		}
	}
}

func TestListWithoutStoreCreatesNothing(t *testing.T) {
	f := newFixture(t)
	c := herdrscript.Client(t, call{Method: "worktree.list", Result: f.listing()})
	if out := Run(context.Background(), c, Options{Cwd: f.root}); out.Status != "success" || out.Result.(Result).Worktrees[0].Owner != nil {
		t.Fatalf("%+v", out)
	}
	if _, err := os.Stat(filepath.Join(f.root, ".fledge", "state")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state directory created: %v", err)
	}
}

func TestListOwnersUnavailableWhenAgentListFails(t *testing.T) {
	t.Setenv("HERDR_SESSION", "")
	f := newFixture(t)
	record(t, f.root, "t1", "alpha", f.merged, "2026-01-01T00:00:00Z")
	c := herdrscript.Client(t, call{Method: "worktree.list", Result: f.listing()}, call{Method: "agent.list", Err: errors.New("offline")})
	out := Run(context.Background(), c, Options{Cwd: f.root})
	if out.Status != "success" || out.Result.(Result).Worktrees[1].Owner != nil {
		t.Fatalf("%+v", out)
	}
}

func TestListLeavesIncompleteStoreUnchanged(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.root, ".fledge", "state")
	os.MkdirAll(dir, 0o700)
	if err := os.WriteFile(filepath.Join(dir, identity.Kind), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	c := herdrscript.Client(t, call{Method: "worktree.list", Result: f.listing()})
	out := Run(context.Background(), c, Options{Cwd: f.root})
	if out.Status != "success" || out.Result.(Result).Worktrees[1].Owner != nil {
		t.Fatalf("%+v", out)
	}
	if list, _ := os.ReadDir(dir); len(list) != 1 || list[0].Name() != identity.Kind {
		t.Fatalf("state entries changed: %v", list)
	}
}

// A configured base branch that does not exist makes every merged check
// unknown and says why, instead of falling back to another branch.
func TestListMissingConfiguredBaseBranch(t *testing.T) {
	f := newFixture(t)
	git(t, f.root, "config", "fledge.baseBranch", "dev")
	c := herdrscript.Client(t, call{Method: "worktree.list", Result: f.listing()})
	out := Run(context.Background(), c, Options{Cwd: f.root})
	r := out.Result.(Result)
	if r.DefaultBranch != nil || r.DefaultBranchError == nil || !strings.Contains(*r.DefaultBranchError, "refs/heads/dev") {
		t.Fatalf("%+v", r)
	}
	for _, row := range r.Worktrees {
		if row.Merged != "unknown" {
			t.Errorf("%s merged %s", row.Path, row.Merged)
		}
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "MERGED is unknown: "+*r.DefaultBranchError) {
		t.Fatalf("%q", b.String())
	}
	herdrscript.CheckOutputFailures(t, Render, libagent.Outcome{Result: r})
}

func TestListUsesConfiguredBaseBranch(t *testing.T) {
	f := newFixture(t)
	git(t, f.root, "branch", "dev", "main~1")
	git(t, f.root, "config", "fledge.baseBranch", "dev")
	c := herdrscript.Client(t, call{Method: "worktree.list", Result: f.listing()})
	r := Run(context.Background(), c, Options{Cwd: f.root}).Result.(Result)
	if r.DefaultBranch == nil || *r.DefaultBranch != "dev" || r.DefaultBranchError != nil {
		t.Fatalf("%+v", r)
	}
	if row := r.Worktrees[1]; row.Merged != "no" {
		t.Fatalf("merged into main but not dev: %+v", row)
	}
}
