package spawn

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
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
func profileRepo(t *testing.T, name, content string) string {
	t.Helper()
	root := t.TempDir()
	if b, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatal(err, string(b))
	}
	path := filepath.Join(root, ".fledge", "profiles", name+".toml")
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func profileOptions(profile string) Options {
	o := validOptions()
	o.Harness, o.Profile, o.Pane = "", profile, "w1:p1"
	return o
}

// profileSpawn scripts a spawn into w1:p1 that starts kind with args and,
// when text is nonempty, submits exactly that first prompt.
func profileSpawn(t *testing.T, kind string, args []string, text string) *spawner {
	return profileSpawnIn(t, "", kind, args, text)
}

// profileSpawnIn is profileSpawn invoked from cwd; a repository cwd adds
// registration's lookup of the calling agent.
func profileSpawnIn(t *testing.T, cwd, kind string, args []string, text string) *spawner {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	calls := []call{{Method: "session.snapshot", Result: snapshot()}, {Method: "agent.start", Params: map[string]any{"name": "worker", "kind": kind, "pane_id": "w1:p1", "args": args, "timeout_ms": 30000}, Result: started(p)}, waitCall("worker", p, "idle")}
	if cwd != "" {
		calls = append(calls, senderCall())
	}
	if text != "" {
		calls = append(calls, senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": text}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	}
	s := fake(t, calls...)
	if cwd != "" {
		s.Cwd = cwd
	}
	return s
}

func TestProfileSuppliesLaunchSettingsAndPrefixesRoleToPrompt(t *testing.T) {
	planner := builtinProfile(t, "planner")
	o := profileOptions("planner")
	o.Prompt, o.PromptSet = "Plan #14.", true
	s := profileSpawn(t, "pi", []string{"--model", "openai-codex/gpt-6-astra", "--thinking", "xhigh"}, header+planner.Brief()+"\n\nPlan #14.")
	out := s.run(context.Background(), o, nil)
	r := out.Result.(*Result)
	if out.Status != "success" || !r.Prompted || !r.PromptRequested || r.Harness != "pi" {
		t.Fatalf("%+v", out)
	}
	if r.Profile == nil || r.Profile.Name != "planner" || r.Profile.Source != "builtin" || r.Profile.Path != nil || r.Profile.Base != nil {
		t.Fatalf("%+v", r.Profile)
	}
}

func TestProfileRoleAloneIsTheFirstPrompt(t *testing.T) {
	reviewer := builtinProfile(t, "reviewer")
	s := profileSpawn(t, "pi", []string{"--model", "openai-codex/gpt-6-astra"}, header+reviewer.Brief())
	out := s.run(context.Background(), profileOptions("reviewer"), nil)
	if r := out.Result.(*Result); out.Status != "success" || !r.Prompted || !r.PromptRequested {
		t.Fatalf("%+v", out)
	}
}

func TestProfileRoleComposesWithStdinFile(t *testing.T) {
	reviewer := builtinProfile(t, "reviewer")
	o := profileOptions("reviewer")
	o.File, o.FileSet = "-", true
	s := profileSpawn(t, "pi", []string{"--model", "openai-codex/gpt-6-astra"}, header+reviewer.Brief()+"\n\nfrom file\n")
	if out := s.run(context.Background(), o, strings.NewReader("from file\n")); out.Status != "success" {
		t.Fatalf("%+v", out)
	}
}

func TestProfileWithoutRoleOrTaskSendsNothing(t *testing.T) {
	root := profileRepo(t, "quiet", "schema_version = 1\nharness = \"claude\"\n")
	s := profileSpawnIn(t, root, "claude", []string{}, "")
	out := s.run(context.Background(), profileOptions("quiet"), nil)
	if r := out.Result.(*Result); out.Status != "success" || r.Prompted || r.PromptRequested {
		t.Fatalf("%+v", out)
	}
	if r := out.Result.(*Result); r.Profile.Source != "repo" || *r.Profile.Path != filepath.Join(root, ".fledge", "profiles", "quiet.toml") {
		t.Fatalf("%+v", r.Profile)
	}
}

func TestProfilePrecedence(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Options)
		kind string
		args []string
	}{
		{"profile only", func(*Options) {}, "pi", []string{"--model", "openai-codex/gpt-6-astra", "--thinking", "xhigh"}},
		{"same harness keeps everything", func(o *Options) { o.Harness = "pi" }, "pi", []string{"--model", "openai-codex/gpt-6-astra", "--thinking", "xhigh"}},
		{"other harness drops model and args", func(o *Options) { o.Harness = "claude" }, "claude", []string{}},
		{"other harness with explicit model", func(o *Options) { o.Harness, o.Model = "claude", "sonnet" }, "claude", []string{"--model", "sonnet"}},
		{"explicit model replaces", func(o *Options) { o.Model = "gpt-5" }, "pi", []string{"--model", "gpt-5", "--thinking", "xhigh"}},
		{"explicit args replace", func(o *Options) { o.Args = []string{"--search"} }, "pi", []string{"--model", "openai-codex/gpt-6-astra", "--search"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := profileOptions("planner")
			tc.set(&o)
			s := profileSpawn(t, tc.kind, tc.args, header+builtinProfile(t, "planner").Brief())
			out := s.run(context.Background(), o, nil)
			if r := out.Result.(*Result); out.Status != "success" || r.Harness != tc.kind {
				t.Fatalf("%+v", out)
			}
		})
	}
}

func TestProfileFailuresRejectBeforeHerdr(t *testing.T) {
	for _, tc := range []struct {
		name, file, content, want string
		set                       func(*Options)
	}{
		{name: "unknown profile", set: func(o *Options) { o.Profile = "nope" }, want: "unknown profile"},
		{name: "invalid override", file: "reviewer", content: "schema_version = 1\nmodle = \"x\"\n", want: "modle"},
		{name: "no harness", file: "scout", content: "schema_version = 1\n[sections]\nmission = \"r\"\n", set: func(o *Options) { o.Profile = "scout" }, want: "--harness"},
		{name: "native model conflict", file: "pinned", content: "schema_version = 1\nharness = \"claude\"\nargs = [\"--model\", \"a\"]\n", set: func(o *Options) { o.Profile, o.Model = "pinned", "b" }, want: "conflicts with --model"},
		{name: "no-wait with role", set: func(o *Options) { o.NoWait = true }, want: "--no-wait"},
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

func TestProfileNoWaitWithoutRoleIsAllowed(t *testing.T) {
	o := profileOptions("quiet")
	o.NoWait = true
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "agent.start", Result: started(p)}, callerNotAgent())
	s.Cwd = profileRepo(t, "quiet", "schema_version = 1\nextends = \"builtin:reviewer\"\nprotocol = false\nreads = []\n[sections]\nmission = \"\"\nworkflow = \"\"\nnever = \"\"\nreport = \"\"\n")
	if out := s.run(context.Background(), o, nil); out.Status != "success" {
		t.Fatalf("%+v", out)
	}
}

// Profiles come from the invoking checkout, not the --cwd destination.
func TestProfileComesFromInvokingCheckoutNotCwd(t *testing.T) {
	invoking := profileRepo(t, "reviewer", "schema_version = 1\nmodel = \"invoking\"\n")
	destination := profileRepo(t, "reviewer", "schema_version = 1\nmodel = \"destination\"\n")
	o := profileOptions("reviewer")
	o.Pane, o.Workspace, o.Cwd = "", "new workspace", destination
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	p.AgentStatus = "idle"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.current", Result: herdr.PaneResult{Type: "pane_current", Pane: herdrscript.Pane("w1:p1", "w1", "w1:t1")}}, call{Method: "workspace.create", Result: herdr.CreatedResult{Type: "workspace_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p}}, call{Method: "agent.start", Params: map[string]any{"name": "worker", "kind": "pi", "pane_id": "w2:p1", "args": []string{"--model", "invoking"}, "timeout_ms": 30000}, Result: started(p)}, waitCall("worker", p, "idle"), senderCall(), senderCall(), call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	s.Cwd = invoking
	if out := s.run(context.Background(), o, nil); out.Status != "success" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

func TestSpawnWithoutProfileIsUnchanged(t *testing.T) {
	s := profileSpawn(t, "claude", []string{}, "")
	o := validOptions()
	o.Pane = "w1:p1"
	out := s.run(context.Background(), o, nil)
	if r := out.Result.(*Result); out.Status != "success" || r.Profile != nil || r.PromptRequested {
		t.Fatalf("%+v", out)
	}
	if !reflect.DeepEqual(out.Result.(*Result).Harness, "claude") {
		t.Fatal(out.Result)
	}
}

func TestProfileNameIsRecordedOnTheAgentRecord(t *testing.T) {
	reviewer := builtinProfile(t, "reviewer")
	cwd := identitytest.Repository(t)
	s := profileSpawnIn(t, cwd, "pi", []string{"--model", "openai-codex/gpt-6-astra"}, header+reviewer.Brief())
	out := s.run(context.Background(), profileOptions("reviewer"), nil)
	r := out.Result.(*Result)
	if out.Status != "success" || !r.Registered {
		t.Fatalf("%+v", out)
	}
	if rec := stored(t, cwd, *r.ID); rec.Profile == nil || *rec.Profile != "reviewer" {
		t.Fatalf("%+v", rec)
	}
}

const readsProfile = "schema_version = 1\nharness = \"claude\"\nreads = [\"present.md\", \"gone.md\"]\n[sections]\nmission = \"Do it.\"\n"

// readsBrief is the brief of the reads profile in root once gone.md is dropped.
func readsBrief(t *testing.T, root string) string {
	t.Helper()
	p, err := profiles.Load(context.Background(), root, "reader")
	if err != nil {
		t.Fatal(err)
	}
	p.Reads = []string{"present.md"}
	return p.Brief()
}

func writeFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
}

// checkSkippedRead asserts a successful spawn skipped only gone.md in dir.
func checkSkippedRead(t *testing.T, out libagent.Outcome, dir string) {
	t.Helper()
	if out.Status != "success" || !out.Result.(*Result).Prompted {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	var skipped []libagent.Effect
	for _, e := range out.Effects {
		if e.Action == "skipped" {
			skipped = append(skipped, e)
		}
	}
	if want := []libagent.Effect{{Action: "skipped", Kind: "read", Path: "gone.md"}}; !reflect.DeepEqual(skipped, want) {
		t.Fatalf("%+v", out.Effects)
	}
	var b strings.Builder
	if err := Render(&b, out); err != nil {
		t.Fatal(err)
	}
	if line := "skipped read: gone.md (not found in " + dir + ")\n"; strings.Count(b.String(), "skipped read:") != 1 || !strings.Contains(b.String(), line) {
		t.Fatalf("%q", b.String())
	}
}

func TestProfileReadsResolveUnderCallerDirectory(t *testing.T) {
	root := profileRepo(t, "reader", readsProfile)
	writeFile(t, root, "present.md")
	o := profileOptions("reader")
	o.Prompt, o.PromptSet = "Go.", true
	s := profileSpawnIn(t, root, "claude", []string{}, header+readsBrief(t, root)+"\n\nGo.")
	checkSkippedRead(t, s.run(context.Background(), o, nil), root)
}

func TestProfileReadsResolveUnderCwd(t *testing.T) {
	root := profileRepo(t, "reader", readsProfile)
	writeFile(t, root, "gone.md")
	destination := t.TempDir()
	writeFile(t, destination, "present.md")
	o := profileOptions("reader")
	o.Pane, o.Workspace, o.Cwd = "", "new workspace", destination
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	p.AgentStatus = "idle"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "pane.current", Result: herdr.PaneResult{Type: "pane_current", Pane: herdrscript.Pane("w1:p1", "w1", "w1:t1")}}, call{Method: "workspace.create", Result: herdr.CreatedResult{Type: "workspace_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p}}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), senderCall(), senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": header + readsBrief(t, root)}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	s.Cwd = root
	checkSkippedRead(t, s.run(context.Background(), o, nil), destination)
}

func TestProfileReadsResolveUnderWorktree(t *testing.T) {
	root := repository(t)
	os.MkdirAll(filepath.Join(root, ".fledge", "profiles"), 0755)
	if err := os.WriteFile(filepath.Join(root, ".fledge", "profiles", "reader.toml"), []byte(readsProfile), 0644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "gone.md")
	path := filepath.Join(root, ".fledge", "worktrees", "worker")
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	p.AgentStatus = "idle"
	o := profileOptions("reader")
	o.Pane, o.Worktree = "", "new"
	add := checkout(t, root, "worker", path)
	create := call{Method: "worktree.create", Result: herdr.CreatedResult{Type: "worktree_created", Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p, Worktree: herdr.Worktree{Path: path}}, Before: func() { add(); writeFile(t, path, "present.md") }}
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "worktree.list", Result: newWorktreeListing(root)}, create, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), callerNotAgent(), senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": header + readsBrief(t, root)}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	s.Cwd = root
	checkSkippedRead(t, s.run(context.Background(), o, nil), path)
}

const readsOnlyProfile = "schema_version = 1\nharness = \"claude\"\nreads = [\"present.md\"]\n"

// A brief made only of reads that are all missing is empty, so --no-wait is
// allowed and the reads are still reported.
func TestProfileNoWaitAllowedWhenEveryReadIsMissing(t *testing.T) {
	o := profileOptions("quiet")
	o.NoWait = true
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "agent.start", Result: started(p)}, callerNotAgent())
	s.Cwd = profileRepo(t, "quiet", readsOnlyProfile)
	out := s.run(context.Background(), o, nil)
	if out.Status != "success" || out.Result.(*Result).PromptRequested {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if !slices.Contains(out.Effects, libagent.Effect{Action: "skipped", Kind: "read", Path: "present.md"}) {
		t.Fatalf("%+v", out.Effects)
	}
}

// Reads are checked before placement under an existing --worktree checkout
// or an absolute --cwd, so a read present only there rejects --no-wait.
func TestProfileNoWaitRejectedWhenReadIsPresentInTargetDirectory(t *testing.T) {
	for name, set := range map[string]func(*Options, string){
		"worktree": func(o *Options, dir string) { o.Worktree = dir },
		"cwd":      func(o *Options, dir string) { o.Workspace, o.Cwd = "new workspace", dir },
	} {
		t.Run(name, func(t *testing.T) {
			target := t.TempDir()
			writeFile(t, target, "present.md")
			o := profileOptions("quiet")
			o.Pane, o.NoWait = "", true
			set(&o, target)
			s := fake(t)
			s.Cwd = profileRepo(t, "quiet", readsOnlyProfile)
			out := s.run(context.Background(), o, nil)
			if out.Status != "rejected" || out.Error.Phase != "validation" || len(out.Effects) != 0 || !strings.Contains(out.Error.Message, "--no-wait") {
				t.Fatalf("%+v %+v", out, out.Error)
			}
		})
	}
}
