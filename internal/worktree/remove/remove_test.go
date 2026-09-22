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
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
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
			out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: listing}, agentList()), Options{Branch: "topic", Cwd: r.root})
			if out.ExitCode() != 2 || out.Error.Phase != "guard" || !strings.Contains(out.Error.Message, "--force") {
				t.Fatalf("%+v", out)
			}
			if _, err := os.Stat(r.topic); err != nil {
				t.Fatal("checkout removed")
			}
			out = Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: listing}, agentList(), agentList()), Options{Branch: "topic", Cwd: r.root, Force: true})
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
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: r.listing(false)}, agentList(), agentList()), Options{Path: r.topic, Cwd: r.root})
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
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: r.listing(false)}, agentList(), agentList()), Options{Path: r.topic, Cwd: r.root})
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

// agentAt is a live agent in pane w9:p1 of another workspace, with cwd and
// terminal as given.
func agentAt(cwd, terminal string) herdr.AgentDetails {
	p := herdrscript.Pane("w9:p1", "w9", "w9:t1")
	p.Cwd, p.AgentStatus = &cwd, "idle"
	return herdr.AgentDetails{Pane: p, TerminalID: terminal}
}
func agentList(agents ...herdr.AgentDetails) call {
	if agents == nil {
		agents = []herdr.AgentDetails{}
	}
	return call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": agents}}
}

// An agent anywhere whose cwd is the checkout, or inside it, blocks removal
// whether or not the checkout is open, and --force does not override it.
func TestAgentWorkingInCheckoutBlocksRemoval(t *testing.T) {
	for _, open := range []bool{false, true} {
		r := newRepo(t)
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(r.topic, link); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(r.topic, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, cwd := range []string{r.topic, filepath.Join(r.topic, "sub"), filepath.Join(link, "sub") + "/"} {
			out := Run(context.Background(), herdrscript.Client(t,
				call{Method: "worktree.list", Result: r.listing(open)},
				agentList(agentAt(cwd, "t9")),
			), Options{Path: r.topic, Force: true, Cwd: r.root})
			if out.ExitCode() != 2 || out.Error.Phase != "guard" || !strings.Contains(out.Error.Message, "w9:p1") || !strings.Contains(out.Error.Message, "--force does not override") {
				t.Fatalf("open=%v cwd=%s: %+v", open, cwd, out)
			}
			if _, err := os.Stat(r.topic); err != nil {
				t.Fatal("checkout removed")
			}
		}
	}
}

// A sibling path that merely shares the checkout's prefix is not inside it.
func TestAgentInSiblingPathDoesNotBlock(t *testing.T) {
	r := newRepo(t)
	sibling := r.topic + "ure"
	os.MkdirAll(sibling, 0o755)
	out := Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Result: r.listing(false)},
		agentList(agentAt(sibling, "t9")),
		agentList(agentAt(sibling, "t9")),
	), Options{Path: r.topic, Cwd: r.root})
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
}

// A live registered agent whose recorded worktree is the checkout blocks
// removal even when its pane's cwd is elsewhere; a record whose terminal is no
// longer live does not.
func TestRegisteredAgentBlocksRemoval(t *testing.T) {
	t.Setenv("HERDR_SESSION", "")
	r := newRepo(t)
	st, err := identity.OpenStore(context.Background(), r.root, &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []string{"t1", "t2"} {
		if _, err := st.Create(identity.Kind, func(id string) any {
			name := "owner-" + term
			return identity.Record{ID: id, Name: &name, Pane: "w9:p1", WorkspaceID: "w9", TerminalID: term, RegisteredAt: "2026-01-01T00:00:00Z", RegisteredBy: "spawn", WorktreePath: &r.topic}
		}); err != nil {
			t.Fatal(err)
		}
	}
	out := Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Result: r.listing(false)},
		agentList(agentAt(r.root, "t1")),
	), Options{Path: r.topic, Force: true, Cwd: r.root})
	if out.ExitCode() != 2 || out.Error.Phase != "guard" || !strings.Contains(out.Error.Message, "owner-t1") {
		t.Fatalf("%+v", out)
	}
	out = Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Result: r.listing(false)},
		agentList(agentAt(r.root, "t3")),
		agentList(agentAt(r.root, "t3")),
	), Options{Path: r.topic, Force: true, Cwd: r.root})
	if out.Status != "success" {
		t.Fatalf("stale record blocked removal: %+v", out)
	}
}

// A closed checkout is rechecked immediately before git removes it.
func TestClosedCheckoutRecheckedBeforeRemoval(t *testing.T) {
	r := newRepo(t)
	out := Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Result: r.listing(false)},
		agentList(),
		agentList(agentAt(r.topic, "t9")),
	), Options{Path: r.topic, Force: true, Cwd: r.root})
	if out.ExitCode() != 2 || out.Error.Phase != "guard" {
		t.Fatalf("%+v", out)
	}
	if _, err := os.Stat(r.topic); err != nil {
		t.Fatal("checkout removed")
	}
}

// A configured base branch that does not exist refuses removal with its reason.
func TestMissingConfiguredBaseBranchExplainsRefusal(t *testing.T) {
	r := newRepo(t)
	git(t, r.root, "config", "fledge.baseBranch", "dev")
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: r.listing(false)}, agentList()), Options{Branch: "topic", Cwd: r.root})
	if out.ExitCode() != 2 || out.Error.Phase != "guard" || !strings.Contains(out.Error.Message, "merged: unknown (git config fledge.baseBranch names refs/heads/dev") {
		t.Fatalf("%+v", out)
	}
}

// An unreadable agent record fails the guard closed, even with --force.
func TestUnreadableAgentRecordBlocksRemoval(t *testing.T) {
	t.Setenv("HERDR_SESSION", "")
	r := newRepo(t)
	st, err := identity.OpenStore(context.Background(), r.root, &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.Create(identity.Kind, func(id string) any { return identity.Record{ID: id, TerminalID: "t1"} })
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(r.root, ".fledge", "state", identity.Kind, id+"*"))
	if len(files) != 1 {
		t.Fatalf("record files %v", files)
	}
	if err := os.WriteFile(files[0], []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Result: r.listing(false)},
		agentList(),
	), Options{Path: r.topic, Force: true, Cwd: r.root})
	if out.ExitCode() == 0 || out.Error.Phase != "guard" || !strings.Contains(out.Error.Message, "repair or remove the bad record under .fledge/state") {
		t.Fatalf("%+v", out)
	}
	if _, err := os.Stat(r.topic); err != nil {
		t.Fatal("checkout removed")
	}
}

// A checkout that Herdr reports through a symlink still matches an agent
// whose cwd is its real path.
func TestAgentInSymlinkedCheckoutBlocksRemoval(t *testing.T) {
	for _, open := range []bool{false, true} {
		r := newRepo(t)
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(r.topic, link); err != nil {
			t.Fatal(err)
		}
		listing := r.listing(open)
		listing.Worktrees[1].Path = link
		out := Run(context.Background(), herdrscript.Client(t,
			call{Method: "worktree.list", Result: listing},
			agentList(agentAt(r.topic, "t9")),
		), Options{Branch: "topic", Force: true, Cwd: r.root})
		if out.ExitCode() != 2 || out.Error.Phase != "guard" || !strings.Contains(out.Error.Message, "w9:p1") {
			t.Fatalf("open=%v: %+v", open, out)
		}
		if _, err := os.Stat(r.topic); err != nil {
			t.Fatal("checkout removed")
		}
	}
}

// A record left in a terminal by a different harness does not block removal.
func TestRecordOfDifferentHarnessDoesNotBlockRemoval(t *testing.T) {
	t.Setenv("HERDR_SESSION", "")
	r := newRepo(t)
	st, err := identity.OpenStore(context.Background(), r.root, &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	codex, claude := "codex", "claude"
	if _, err := st.Create(identity.Kind, func(id string) any {
		return identity.Record{ID: id, Pane: "w9:p1", WorkspaceID: "w9", Harness: &codex, TerminalID: "t1", RegisteredAt: "2026-01-01T00:00:00Z", RegisteredBy: "spawn", WorktreePath: &r.topic}
	}); err != nil {
		t.Fatal(err)
	}
	live := agentAt(r.root, "t1")
	live.Agent = &claude
	out := Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Result: r.listing(false)},
		agentList(live),
		agentList(live),
	), Options{Path: r.topic, Force: true, Cwd: r.root})
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
}

// symlinked returns r's listing as Herdr reports it for a cwd reached through
// a symlink to the primary checkout: repo_root repeats the symlinked path
// while checkout paths are real.
func (r repo) symlinked(t *testing.T, open bool) (string, herdr.WorktreeListResult) {
	t.Helper()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(r.root, link); err != nil {
		t.Fatal(err)
	}
	l := r.listing(open)
	l.Source.RepoRoot = link
	return link, l
}

// Fledge's own guard refuses the primary checkout reached through a symlink.
func TestRefusesPrimaryCheckoutThroughSymlink(t *testing.T) {
	r := newRepo(t)
	link, l := r.symlinked(t, false)
	for _, o := range []Options{{Path: link, Force: true}, {Branch: "main", Force: true}} {
		o.Cwd = link
		out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: l}), o)
		if out.ExitCode() != 2 || out.Error.Phase != "guard" || !strings.Contains(out.Error.Message, "primary") {
			t.Fatalf("%+v: %+v", o, out)
		}
	}
}

// --path finds a checkout named through a symlink, absolute or relative to
// the process working directory.
func TestPathThroughSymlinkFindsCheckout(t *testing.T) {
	for name, relative := range map[string]bool{"absolute": false, "relative": true} {
		t.Run(name, func(t *testing.T) {
			r := newRepo(t)
			link, l := r.symlinked(t, false)
			path := filepath.Join(link, ".fledge", "worktrees", "topic")
			if relative {
				t.Chdir(link)
				path = filepath.Join(".fledge", "worktrees", "topic")
			}
			out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: l}, agentList(), agentList()), Options{Path: path, Cwd: link})
			if out.Status != "success" || out.Result.(Result).Path != r.topic {
				t.Fatalf("%+v", out)
			}
			if _, err := os.Stat(r.topic); !os.IsNotExist(err) {
				t.Fatal("checkout kept")
			}
		})
	}
}

// --path through a symlink finds a checkout whose directory is gone.
func TestPathThroughSymlinkFindsMissingCheckout(t *testing.T) {
	r := newRepo(t)
	link, l := r.symlinked(t, true)
	if err := os.RemoveAll(r.topic); err != nil {
		t.Fatal(err)
	}
	out := Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Result: l},
		agentList(),
		agentList(),
		call{Method: "worktree.remove", Params: map[string]any{"workspace_id": "w2", "force": true}, Result: removed(r.topic, true)},
	), Options{Path: filepath.Join(link, ".fledge", "worktrees", "topic"), Cwd: link, Force: true})
	if out.Status != "success" || out.Result.(Result).Path != r.topic {
		t.Fatalf("%+v", out)
	}
}

// Removal inspects only its target: no git command runs in, or names, another
// linked checkout of the repository.
func TestInspectsOnlyTargetCheckout(t *testing.T) {
	r := newRepo(t)
	other := filepath.Join(r.root, ".fledge", "worktrees", "other")
	git(t, r.root, "worktree", "add", "-q", "-b", "other", other)
	l := r.listing(false)
	l.Worktrees = append(l.Worktrees, herdr.Worktree{Path: other, Branch: s("other")})
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "git.log")
	script := "#!/bin/sh\necho \"$*\" >> \"" + log + "\"\nexec \"" + real + "\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: l}, agentList(), agentList()), Options{Branch: "topic", Cwd: r.root})
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "-C "+r.topic+" status") {
		t.Fatalf("target checkout not inspected; git log:\n%s", b)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if strings.Contains(line, other) || strings.Contains(line, "refs/heads/other") || (strings.Contains(line, " status ") && !strings.Contains(line, r.topic)) {
			t.Errorf("inspected another checkout: git %s", line)
		}
	}
}
