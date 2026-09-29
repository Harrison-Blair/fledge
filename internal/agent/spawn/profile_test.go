package spawn

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/memory"
	"github.com/Harrison-Blair/fledge/internal/lib/profiles"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func builtinProfile(t *testing.T, name string) profiles.Profile {
	t.Helper()
	p, err := profiles.Load(context.Background(), t.TempDir(), name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// profileRepo creates a Git checkout holding one .fledge/profiles file.
func profileRepo(t *testing.T, file, content string) string {
	t.Helper()
	root := t.TempDir()
	if b, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatal(err, string(b))
	}
	path := filepath.Join(root, ".fledge", "profiles", file)
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func profileOptions(profile string) Options {
	o := validOptions()
	o.Profile, o.Pane = profile, "w1:p1"
	return o
}

// profileSpawn scripts a spawn into w1:p1 that starts kind with args and
// submits exactly text as the first prompt.
func profileSpawn(t *testing.T, kind string, args []string, text string) *spawner {
	return profileSpawnIn(t, "", kind, args, text)
}

// profileSpawnIn is profileSpawn invoked from cwd; a repository cwd adds
// registration's lookup of the calling agent.
func profileSpawnIn(t *testing.T, cwd, kind string, args []string, text string) *spawner {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	calls := []call{{Method: "session.snapshot", Result: snapshot()}, labeled(p), {Method: "agent.start", Params: map[string]any{"name": "worker", "kind": kind, "pane_id": "w1:p1", "args": args, "timeout_ms": 30000}, Result: started(p)}, waitCall("worker", p, "idle")}
	if cwd != "" {
		calls = append(calls, senderCall())
	}
	calls = append(calls, senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": text}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	s := fake(t, calls...)
	if cwd != "" {
		s.Cwd = cwd
	}
	return s
}

// Every built-in role launches on any harness with exactly the caller's
// native arguments; a profile never adds a model, permissions, or flags.
func TestEveryBuiltinSendsItsBriefWithNoImplicitLaunchArguments(t *testing.T) {
	for _, name := range []string{"debugger", "implementer", "integrator", "orchestrator", "planner", "researcher", "reviewer", "verifier"} {
		for _, kind := range []string{"claude", "pi", "codex"} {
			t.Run(name+"/"+kind, func(t *testing.T) {
				o := profileOptions(name)
				o.Harness = kind
				o.Prompt, o.PromptSet = "Do #14.", true
				s := profileSpawn(t, kind, []string{}, header+builtinProfile(t, name).Brief()+noMemories+"\n\nDo #14.")
				out := s.run(context.Background(), o, nil)
				r := out.Result.(*Result)
				if out.Status != "success" || !r.Prompted || !r.PromptRequested || r.Harness != kind {
					t.Fatalf("%+v %+v", out, out.Error)
				}
				if r.Profile == nil || r.Profile.Name != name || r.Profile.Source != "builtin" || r.Profile.Path != nil {
					t.Fatalf("%+v", r.Profile)
				}
			})
		}
	}
}

func TestProfileKeepsExplicitModelAndNativeArguments(t *testing.T) {
	o := profileOptions("implementer")
	o.Model, o.Args = "sonnet", []string{"--permission-mode", "bypassPermissions"}
	s := profileSpawn(t, "claude", []string{"--model", "sonnet", "--permission-mode", "bypassPermissions"}, header+builtinProfile(t, "implementer").Brief()+noMemories)
	if out := s.run(context.Background(), o, nil); out.Status != "success" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

// The role brief, shared protocol, and memory index each arrive once, before
// the task, in one prompt under one sender header.
func TestProfileBriefIsInjectedOnceUnderOneHeader(t *testing.T) {
	o := profileOptions("reviewer")
	o.File, o.FileSet = "-", true
	want := header + builtinProfile(t, "reviewer").Brief() + noMemories + "\n\nfrom file\n"
	for _, once := range []string{header, "## Mission\n", "## Fledge protocol\n", "## Project memory\n", "from file"} {
		if strings.Count(want, once) != 1 {
			t.Fatalf("%q appears %d times in %q", once, strings.Count(want, once), want)
		}
	}
	s := profileSpawn(t, "claude", []string{}, want)
	if out := s.run(context.Background(), o, strings.NewReader("from file\n")); out.Status != "success" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

func TestProfileRequiresExplicitHarness(t *testing.T) {
	o := profileOptions("reviewer")
	o.Harness = ""
	out := fake(t).run(context.Background(), o, nil)
	if out.Status != "rejected" || out.Error.Phase != "validation" || len(out.Effects) != 0 || !strings.Contains(out.Error.Message, "--harness is required") {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

func TestProfileFailuresRejectBeforeHerdr(t *testing.T) {
	for _, tc := range []struct {
		name, file, content, want string
		set                       func(*Options)
	}{
		{name: "unknown profile", set: func(o *Options) { o.Profile = "nope" }, want: "unknown profile"},
		{name: "blank override", file: "reviewer.md", content: "\n \n", want: "blank"},
		{name: "legacy override", file: "reviewer.toml", content: "schema_version = 1\nharness = \"claude\"\n", want: "legacy TOML profile"},
		{name: "legacy custom", file: "scout.toml", content: "schema_version = 1\n", set: func(o *Options) { o.Profile = "scout" }, want: "legacy TOML profile"},
		{name: "no-wait", set: func(o *Options) { o.NoWait = true }, want: "--no-wait"},
		{name: "no-wait with custom", file: "scout.md", content: "Scout.\n", set: func(o *Options) { o.Profile, o.NoWait = "scout", true }, want: "--no-wait"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := profileOptions("reviewer")
			if tc.set != nil {
				tc.set(&o)
			}
			s := fake(t)
			if tc.file != "" {
				s.Cwd = profileRepo(t, tc.file, tc.content)
			}
			out := s.run(context.Background(), o, nil)
			if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Phase != "validation" || len(out.Effects) != 0 || !strings.Contains(out.Error.Message, tc.want) {
				t.Fatalf("%+v %+v", out, out.Error)
			}
		})
	}
}

// A repository file replaces the built-in entirely and comes from the
// invoking checkout, not the --cwd destination.
func TestProfileComesFromInvokingCheckoutNotCwd(t *testing.T) {
	invoking := profileRepo(t, "reviewer.md", "Invoking role.\n")
	destination := profileRepo(t, "reviewer.md", "Destination role.\n")
	o := profileOptions("reviewer")
	o.Pane, o.Workspace, o.Cwd = "", "new workspace", destination
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	p.AgentStatus = "idle"
	want := header + profiles.Profile{Role: "Invoking role.\n"}.Brief() + noMemories
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.current", Result: herdr.PaneResult{Type: "pane_current", Pane: herdrscript.Pane("w1:p1", "w1", "w1:t1")}}, call{Method: "workspace.create", Result: herdr.CreatedResult{Type: "workspace_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p}}, namedTab(p), labeled(p), call{Method: "agent.start", Params: map[string]any{"name": "worker", "kind": "claude", "pane_id": "w2:p1", "args": []string{}, "timeout_ms": 30000}, Result: started(p)}, waitCall("worker", p, "idle"), senderCall(), senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": want}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	s.Cwd = invoking
	out := s.run(context.Background(), o, nil)
	if out.Status != "success" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if r := out.Result.(*Result); r.Profile.Source != "repo" || *r.Profile.Path != filepath.Join(invoking, ".fledge", "profiles", "reviewer.md") {
		t.Fatalf("%+v", r.Profile)
	}
}

func TestSpawnWithoutProfileIsUnchanged(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Params: map[string]any{"name": "worker", "kind": "claude", "pane_id": "w1:p1", "args": []string{}, "timeout_ms": 30000}, Result: started(p)}, waitCall("worker", p, "idle"))
	o := validOptions()
	o.Pane = "w1:p1"
	out := s.run(context.Background(), o, nil)
	if r := out.Result.(*Result); out.Status != "success" || r.Profile != nil || r.PromptRequested || r.Prompted {
		t.Fatalf("%+v", out)
	}
}

func TestProfileNameIsRecordedOnTheAgentRecord(t *testing.T) {
	cwd := identitytest.Repository(t)
	s := profileSpawnIn(t, cwd, "claude", []string{}, header+builtinProfile(t, "reviewer").Brief()+noMemories)
	out := s.run(context.Background(), profileOptions("reviewer"), nil)
	r := out.Result.(*Result)
	if out.Status != "success" || !r.Registered {
		t.Fatalf("%+v", out)
	}
	if rec := stored(t, cwd, *r.ID); rec.Profile == nil || *rec.Profile != "reviewer" {
		t.Fatalf("%+v", rec)
	}
}

// The spawn result names the profile without retired fields, and a spawn
// reports no read effects.
func TestSpawnProfileResultHasNoRetiredFields(t *testing.T) {
	s := profileSpawn(t, "claude", []string{}, header+builtinProfile(t, "reviewer").Brief()+noMemories)
	out := s.run(context.Background(), profileOptions("reviewer"), nil)
	b, err := json.Marshal(out.Result)
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Profile map[string]any }
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if keys := slices.Sorted(maps.Keys(got.Profile)); !slices.Equal(keys, []string{"name", "path", "source"}) {
		t.Fatalf("%s", b)
	}
	for _, e := range out.Effects {
		if e.Kind == "read" {
			t.Fatalf("%+v", out.Effects)
		}
	}
}

// noMemories is the Project memory block, after a brief's final newline,
// where the repository has no memories, or outside any repository.
const noMemories = "\n## Project memory\nNo project memories yet."

// oneMemory is the Project memory block for a repository holding only alpha.
const oneMemory = "\n## Project memory\nRead one in full with `fledge memory get --name <name>`.\n\n- [alpha](alpha.md) — First fact"

func addMemory(t *testing.T, cwd string, m memory.Memory) {
	t.Helper()
	if err := memory.Add(context.Background(), cwd, m, &libagent.Outcome{}); err != nil {
		t.Fatal(err)
	}
}

func alpha(t *testing.T, cwd string) {
	addMemory(t, cwd, memory.Memory{Name: "alpha", Description: "First fact", Type: "user", Body: "a"})
}

func TestProfileBriefEndsWithProjectMemoryIndex(t *testing.T) {
	reviewer := builtinProfile(t, "reviewer")
	cwd := identitytest.Repository(t)
	addMemory(t, cwd, memory.Memory{Name: "herdr-socket", Description: "Herdr commands need socket access", Type: "project", Body: "b"})
	alpha(t, cwd)
	want := header + reviewer.Brief() + "\n## Project memory\nRead one in full with `fledge memory get --name <name>`.\n\n" +
		"- [alpha](alpha.md) — First fact\n- [herdr-socket](herdr-socket.md) — Herdr commands need socket access\n\nReview it."
	s := profileSpawnIn(t, cwd, "claude", []string{}, want)
	o := profileOptions("reviewer")
	o.Prompt, o.PromptSet = "Review it.", true
	if out := s.run(context.Background(), o, nil); out.Status != "success" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

func TestProfileBriefNotesMissingMemories(t *testing.T) {
	cwd := identitytest.Repository(t)
	s := profileSpawnIn(t, cwd, "claude", []string{}, header+builtinProfile(t, "reviewer").Brief()+noMemories)
	if out := s.run(context.Background(), profileOptions("reviewer"), nil); out.Status != "success" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".fledge", "memories")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

// An existing --pane may sit in another repository than the caller's; memory
// comes from the pane's own directory.
func TestProfileMemoryComesFromPaneCwd(t *testing.T) {
	dir := repository(t)
	alpha(t, dir)
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.Cwd = &dir
	snap := snapshot()
	snap.Snapshot.Panes = []herdr.Pane{p}
	p.AgentStatus = "idle"
	s := fake(t, call{Method: "session.snapshot", Result: snap}, labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), senderCall(), senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": header + builtinProfile(t, "reviewer").Brief() + oneMemory}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	s.Cwd = identitytest.Repository(t)
	if out := s.run(context.Background(), profileOptions("reviewer"), nil); out.Status != "success" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

func TestProfileMemoryComesFromCwd(t *testing.T) {
	destination := repository(t)
	alpha(t, destination)
	o := profileOptions("reviewer")
	o.Pane, o.Workspace, o.Cwd = "", "new workspace", destination
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	p.AgentStatus = "idle"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.current", Result: herdr.PaneResult{Type: "pane_current", Pane: herdrscript.Pane("w1:p1", "w1", "w1:t1")}}, call{Method: "workspace.create", Result: herdr.CreatedResult{Type: "workspace_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p}}, namedTab(p), labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), senderCall(), senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": header + builtinProfile(t, "reviewer").Brief() + oneMemory}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	s.Cwd = identitytest.Repository(t)
	if out := s.run(context.Background(), o, nil); out.Status != "success" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

// A worker in a new linked checkout sees its repository's primary memories.
func TestProfileMemoryComesFromWorktreePrimaryCheckout(t *testing.T) {
	root := repository(t)
	alpha(t, root)
	path := filepath.Join(root, ".fledge", "worktrees", "worker")
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	p.AgentStatus = "idle"
	o := profileOptions("reviewer")
	o.Pane, o.Worktree = "", "new"
	create := call{Method: "worktree.create", Result: herdr.CreatedResult{Type: "worktree_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p, Worktree: herdr.Worktree{Path: path}}, Before: checkout(t, root, "worker", path)}
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "worktree.list", Result: newWorktreeListing(root)}, create, namedTab(p), labeled(p), call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), callerNotAgent(), senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": header + builtinProfile(t, "reviewer").Brief() + oneMemory}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	s.Cwd = root
	if out := s.run(context.Background(), o, nil); out.Status != "success" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

// A worker placed in a linked checkout sees the primary checkout's memories.
func TestMemoryBriefReadsPrimaryCheckoutFromLinkedCheckout(t *testing.T) {
	root := repository(t)
	linked := filepath.Join(root, ".fledge", "worktrees", "feat")
	if b, err := exec.Command("git", "-C", root, "worktree", "add", "-q", "-b", "feat", linked).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	alpha(t, root)
	if got := memoryBrief(context.Background(), linked); got != oneMemory[1:] {
		t.Fatalf("%q", got)
	}
}

func TestMemoryBriefReportsUnreadableMemories(t *testing.T) {
	root := repository(t)
	alpha(t, root)
	if err := os.WriteFile(filepath.Join(root, ".fledge", "memories", "broken.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := memoryBrief(context.Background(), root); !strings.HasPrefix(got, "## Project memory\nProject memory could not be read: ") || !strings.Contains(got, "broken.md") {
		t.Fatalf("%q", got)
	}
}
