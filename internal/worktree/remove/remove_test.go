package remove

import (
	"bytes"
	"context"
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

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=t@example.com"}, args...)...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, b)
	}
	return string(b)
}
func s(v string) *string { return &v }

// repo is a primary checkout on main with a linked checkout "topic" that is
// merged and clean unless changed by the test.
type repo struct{ root, topic string }

func newRepo(t *testing.T) repo {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "commit", "-qm", "initial", "--allow-empty")
	topic := filepath.Join(root, ".fledge", "worktrees", "topic")
	os.MkdirAll(filepath.Join(root, ".fledge"), 0o755)
	os.WriteFile(filepath.Join(root, ".fledge", ".gitignore"), []byte("*\n"), 0o644)
	git(t, root, "worktree", "add", "-q", "-b", "topic", topic)
	return repo{root, topic}
}

func (r repo) listing(open bool) herdr.WorktreeListResult {
	topic := herdr.Worktree{Path: r.topic, Branch: s("topic")}
	if open {
		topic.OpenWorkspaceID = s("w2")
	}
	l := herdr.WorktreeListResult{Type: "worktree_list", Worktrees: []herdr.Worktree{{Path: r.root, Branch: s("main"), OpenWorkspaceID: s("w1")}, topic}}
	l.Source.RepoRoot = r.root
	return l
}

func agents(workspaces ...string) herdr.AgentListResult {
	r := herdr.AgentListResult{Type: "agent_list", Agents: []herdr.Pane{}}
	for i, ws := range workspaces {
		p := herdrscript.Pane(ws+":p"+string(rune('1'+i)), ws, ws+":t1")
		p.AgentStatus = "idle"
		r.Agents = append(r.Agents, p)
	}
	return r
}

func removed(path string, force bool) map[string]any {
	return map[string]any{"type": "worktree_removed", "workspace_id": "w2", "path": path, "forced": force}
}

func TestRequiresExactlyOneTarget(t *testing.T) {
	for _, o := range []Options{{}, {Path: "/p", Branch: "b"}} {
		out := Run(context.Background(), herdrscript.Client(t), o)
		if out.ExitCode() != 2 || out.Status != "rejected" {
			t.Fatalf("%+v", out)
		}
	}
}

func TestRefusesPrimaryCheckout(t *testing.T) {
	r := newRepo(t)
	for _, o := range []Options{{Path: r.root, Force: true}, {Branch: "main", Force: true}} {
		o.Cwd = r.root
		out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: r.listing(true)}), o)
		if out.ExitCode() != 2 || !strings.Contains(out.Error.Message, "primary") {
			t.Fatalf("%+v", out)
		}
	}
}

func TestRefusesUnknownTarget(t *testing.T) {
	r := newRepo(t)
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: r.listing(true)}), Options{Branch: "missing", Cwd: r.root})
	if out.ExitCode() != 2 {
		t.Fatalf("%+v", out)
	}
}

func TestLiveAgentGuardIgnoresForce(t *testing.T) {
	r := newRepo(t)
	out := Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Result: r.listing(true)},
		call{Method: "agent.list", Result: agents("w1", "w2")},
	), Options{Path: r.topic, Force: true, Cwd: r.root})
	if out.ExitCode() != 2 || out.Error.Phase != "guard" || !strings.Contains(out.Error.Message, "w2:p2") || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
	if _, err := os.Stat(r.topic); err != nil {
		t.Fatal("checkout removed")
	}
}

// An agent that appears after the first check is caught by the recheck made
// immediately before worktree.remove.
func TestLiveAgentRecheckedBeforeRemoval(t *testing.T) {
	r := newRepo(t)
	out := Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Result: r.listing(true)},
		call{Method: "agent.list", Result: agents("w1")},
		call{Method: "agent.list", Result: agents("w2")},
	), Options{Branch: "topic", Cwd: r.root})
	if out.ExitCode() != 2 || out.Error.Phase != "guard" {
		t.Fatalf("%+v", out)
	}
}

func TestDirtyOrUnmergedRequiresForce(t *testing.T) {
	for _, kind := range []string{"dirty", "unmerged", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			r := newRepo(t)
			listing := r.listing(false)
			switch kind {
			case "dirty":
				os.WriteFile(filepath.Join(r.topic, "untracked"), []byte("x"), 0o644)
			case "unmerged":
				git(t, r.topic, "commit", "-qm", "work", "--allow-empty")
			case "unknown":
				git(t, r.root, "branch", "-m", "main", "trunk")
			}
			out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: listing}), Options{Branch: "topic", Cwd: r.root})
			if out.ExitCode() != 2 || out.Error.Phase != "guard" || !strings.Contains(out.Error.Message, "--force") {
				t.Fatalf("%+v", out)
			}
			if _, err := os.Stat(r.topic); err != nil {
				t.Fatal("checkout removed")
			}
			out = Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: listing}), Options{Branch: "topic", Cwd: r.root, Force: true})
			if out.Status != "success" {
				t.Fatalf("forced: %+v", out)
			}
			if _, err := os.Stat(r.topic); !os.IsNotExist(err) {
				t.Fatal("forced removal kept checkout")
			}
		})
	}
}

func TestOpenCheckoutRemovedThroughHerdr(t *testing.T) {
	for _, force := range []bool{false, true} {
		r := newRepo(t)
		out := Run(context.Background(), herdrscript.Client(t,
			call{Method: "worktree.list", Params: map[string]any{"cwd": r.root}, Result: r.listing(true)},
			call{Method: "agent.list", Result: agents("w1")},
			call{Method: "agent.list", Result: agents("w1")},
			call{Method: "worktree.remove", Params: map[string]any{"workspace_id": "w2", "force": force}, Result: removed(r.topic, force)},
		), Options{Path: r.topic, Cwd: r.root, Force: force})
		if out.Status != "success" || out.Operation != "worktree.remove" {
			t.Fatalf("%+v", out)
		}
		res := out.Result.(Result)
		if res.Path != r.topic || *res.Branch != "topic" || res.ClosedWorkspaceID == nil || *res.ClosedWorkspaceID != "w2" {
			t.Fatalf("%+v", res)
		}
		want := []libagent.Effect{{Action: "removed", Kind: "worktree", Path: r.topic}, {Action: "closed", Kind: "workspace", ID: "w2"}}
		if len(out.Effects) != 2 || out.Effects[0] != want[0] || out.Effects[1] != want[1] {
			t.Fatalf("%+v", out.Effects)
		}
	}
}

func TestHerdrRemoveFailureIsUnknownWhenUncertain(t *testing.T) {
	r := newRepo(t)
	out := Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Result: r.listing(true)},
		call{Method: "agent.list", Result: agents()},
		call{Method: "agent.list", Result: agents()},
		call{Method: "worktree.remove", Result: map[string]any{"type": "other"}},
	), Options{Path: r.topic, Cwd: r.root})
	if out.Status != "unknown" || out.Error.Phase != "worktree.remove" {
		t.Fatalf("%+v", out)
	}
}

func TestClosedCheckoutRemovedWithGit(t *testing.T) {
	r := newRepo(t)
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: r.listing(false)}), Options{Path: r.topic, Cwd: r.root})
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
	if _, err := os.Stat(r.topic); !os.IsNotExist(err) {
		t.Fatal("checkout kept")
	}
	if strings.Contains(git(t, r.root, "worktree", "list"), r.topic) {
		t.Fatal("git still lists the checkout")
	}
	if !strings.Contains(git(t, r.root, "branch", "--list", "topic"), "topic") {
		t.Fatal("branch deleted")
	}
	res := out.Result.(Result)
	if res.ClosedWorkspaceID != nil || len(out.Effects) != 1 || out.Effects[0] != (libagent.Effect{Action: "removed", Kind: "worktree", Path: r.topic}) {
		t.Fatalf("%+v %+v", res, out.Effects)
	}
}

func TestGitRemoveFailure(t *testing.T) {
	r := newRepo(t)
	git(t, r.root, "worktree", "lock", r.topic)
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: r.listing(false)}), Options{Path: r.topic, Cwd: r.root})
	if out.ExitCode() != 1 || out.Error.Phase != "git worktree remove" || out.Status != "rejected" {
		t.Fatalf("%+v", out)
	}
}

func TestRender(t *testing.T) {
	ws := "w2"
	for _, tc := range []struct {
		result Result
		want   string
	}{
		{Result{Path: "/r/t", Branch: s("topic"), ClosedWorkspaceID: &ws}, "Removed worktree /r/t and closed workspace w2; kept branch topic.\n"},
		{Result{Path: "/r/t"}, "Removed worktree /r/t (detached HEAD).\n"},
		{Result{Path: "/r/t", Branch: s("topic")}, "Removed worktree /r/t; kept branch topic.\n"},
	} {
		var b bytes.Buffer
		if err := (libagent.Outcome{Status: "success", Result: tc.result}).Write(&b, false, Render); err != nil {
			t.Fatal(err)
		}
		if b.String() != tc.want {
			t.Fatalf("got %q want %q", b.String(), tc.want)
		}
		herdrscript.CheckOutputFailures(t, Render, libagent.Outcome{Result: tc.result})
	}
}
