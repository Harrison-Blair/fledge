package cleanup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/gitstatus"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
	"github.com/Harrison-Blair/fledge/internal/lib/worktree"
)

type call = herdrscript.Call

func s(v string) *string { return &v }

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=t@example.com"}, args...)...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, b)
	}
	return string(b)
}

// repo is a primary checkout on main with a dev branch and a managed linked
// checkout "topic" created from dev, clean and merged into dev.
type repo struct{ root, topic string }

func newRepo(t *testing.T) repo {
	t.Helper()
	t.Setenv("HERDR_SESSION", "")
	root, _ := filepath.EvalSymlinks(t.TempDir())
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "commit", "-qm", "initial", "--allow-empty")
	git(t, root, "branch", "dev")
	topic := filepath.Join(root, ".fledge", "worktrees", "topic")
	os.MkdirAll(filepath.Join(root, ".fledge"), 0o755)
	os.WriteFile(filepath.Join(root, ".fledge", ".gitignore"), []byte("*\n"), 0o644)
	git(t, root, "worktree", "add", "-q", "-b", "topic", topic, "dev")
	return repo{root, topic}
}

// listing lists the primary checkout, open in w1, and extra checkouts; each
// extra is open in the workspace mapped to its path, or closed.
func (r repo) listing(open map[string]string, extra ...string) herdr.WorktreeListResult {
	l := herdr.WorktreeListResult{Type: "worktree_list", Worktrees: []herdr.Worktree{{Path: r.root, Branch: s("main"), OpenWorkspaceID: s("w1")}}}
	for _, p := range extra {
		w := herdr.Worktree{Path: p, Branch: s(filepath.Base(p))}
		if ws, ok := open[p]; ok {
			w.OpenWorkspaceID = s(ws)
		}
		l.Worktrees = append(l.Worktrees, w)
	}
	l.Source.RepoRoot = r.root
	return l
}

// agent is a live claude agent named name in pane (workspace ws) hosting terminal.
func agent(pane, ws, terminal, name, status string) herdr.AgentDetails {
	p := herdrscript.Pane(pane, ws, ws+":t1")
	h := "claude"
	p.AgentStatus, p.Agent, p.Name = status, &h, &name
	a := herdrscript.Info(p).Agent
	a.TerminalID = terminal
	return a
}

// registered numbers registrations, so records order by registration even
// within one second.
var registered int

// register records a in the repository, then sets its parent and a
// registration time after every earlier one.
func (r repo) register(t *testing.T, a herdr.AgentDetails, parent *string, by string, checkout *identity.Checkout) identity.Record {
	t.Helper()
	st, err := identity.OpenStore(context.Background(), r.root, &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := identity.Register(context.Background(), st, libagent.Client{}, a, by, checkout)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(identity.Kind, rec.ID, &rec, func() error {
		registered++
		rec.Parent, rec.RegisteredAt = parent, fmt.Sprintf("2026-01-01T00:%02d:%02dZ", registered/60%60, registered%60)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return rec
}

func (r repo) load(t *testing.T, id string) identity.Record {
	t.Helper()
	st, err := identity.Existing(context.Background(), r.root)
	if err != nil {
		t.Fatal(err)
	}
	var rec identity.Record
	if err := st.Get(identity.Kind, id, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

// created is the provenance a spawn records after creating the checkout at
// path from base: its branch and its marker, marking it if needed. The
// primary checkout cannot be marked and gets no marker.
func created(t *testing.T, path, base string) *identity.Checkout {
	t.Helper()
	c := &identity.Checkout{Path: path, Created: true, Base: &base, Branch: gitstatus.Branch(context.Background(), path)}
	id := worktree.Marker(context.Background(), path)
	if id == "" {
		id, _ = worktree.Mark(context.Background(), path)
	}
	c.Marker = libagent.Pointer(id)
	return c
}

func client(t *testing.T, r repo, calls ...call) libagent.Client {
	c := herdrscript.Client(t, calls...)
	c.Cwd, c.CallerPane = r.root, "w1:p1"
	return c
}

func agentList(agents ...herdr.AgentDetails) call {
	if agents == nil {
		agents = []herdr.AgentDetails{}
	}
	return call{Method: "agent.list", Result: herdr.AgentListResult{Type: "agent_list", Agents: agents}}
}

func get(a herdr.AgentDetails) call {
	return call{Method: "agent.get", Params: map[string]any{"target": a.PaneID}, Result: herdr.AgentResult{Type: "agent_info", Agent: a}}
}

// snapshot reads every file under dir, so tests can prove a run wrote nothing.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		files[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func worker(res Result, id string) Worker {
	for _, w := range res.Workers {
		if w.ID == id {
			return w
		}
	}
	return Worker{}
}

func checkout(res Result, path string) Checkout {
	for _, c := range res.Checkouts {
		if c.Path == path {
			return c
		}
	}
	return Checkout{}
}

// Selection keeps only the caller's direct, spawned, live children, and holds
// each one unless it is idle or done, has no live worker of its own, and every
// task it owns, from any creator, is verified or cancelled. A taskless worker
// is eligible only when the caller asserts it collected the results.
func TestSelectWorkers(t *testing.T) {
	caller, other := "caller00", "other000"
	rec := func(id, terminal string, parent *string, by string) identity.Record {
		return identity.Record{ID: id, TerminalID: terminal, Parent: parent, RegisteredBy: by, Harness: s("claude")}
	}
	owned := func(owner, status string) task.Record {
		return task.Record{ID: "task" + owner[:4], Owner: &owner, Status: status, CreatedBy: s(other)}
	}
	ended := rec("ended000", "t_ended", &caller, "spawn")
	ended.EndedAt = s("2026-01-01T00:00:00Z")
	records := []identity.Record{
		// A malformed record naming itself as parent is still never its own worker.
		rec("caller00", "t_caller", &caller, "spawn"),
		rec("sibling0", "t_sibling", &other, "spawn"),
		rec("adopted0", "t_adopted", &caller, "adopt"),
		rec("idle0000", "t_idle", &caller, "spawn"),
		rec("done0000", "t_done", &caller, "spawn"),
		rec("busy0000", "t_busy", &caller, "spawn"),
		rec("complete", "t_complete", &caller, "spawn"),
		rec("assigned", "t_assigned", &caller, "spawn"),
		rec("cancel00", "t_cancel", &caller, "spawn"),
		rec("taskless", "t_taskless", &caller, "spawn"),
		rec("parent00", "t_parent", &caller, "spawn"),
		rec("grandkid", "t_grandkid", s("parent00"), "spawn"),
		rec("gone0000", "t_gone", &caller, "spawn"),
		ended,
	}
	live := map[string]identity.Record{}
	var agents []herdr.AgentDetails
	for _, r := range records {
		if r.EndedAt != nil {
			continue
		}
		// gone0000's record is live, but no Herdr agent runs its terminal.
		if live[r.TerminalID] = r; r.ID == "gone0000" {
			continue
		}
		status := "idle"
		switch r.ID {
		case "done0000":
			status = "done"
		case "busy0000":
			status = "working"
		}
		agents = append(agents, agent("w1:"+r.ID, "w1", r.TerminalID, r.ID, status))
	}
	tasks := []task.Record{
		owned("idle0000", task.Verified), owned("done0000", task.Verified), owned("busy0000", task.Verified),
		owned("complete", task.Completed), owned("assigned", task.Verified), owned("cancel00", task.Cancelled),
		owned("parent00", task.Verified), owned("gone0000", task.Verified),
	}
	tasks = append(tasks, task.Record{ID: "second00", Owner: s("assigned"), Status: task.Assigned, CreatedBy: s(other)})
	for _, collected := range []bool{false, true} {
		got := map[string]string{}
		for _, w := range selectWorkers(caller, records, agents, live, tasks, collected) {
			reason := ""
			if w.Reason != nil {
				reason = *w.Reason
			}
			got[w.ID] = w.Outcome + ": " + reason
		}
		want := map[string]string{
			"idle0000": "planned: ",
			"done0000": "planned: ",
			"cancel00": "planned: ",
			"busy0000": "skipped: agent is working",
			"complete": "skipped: task taskcomp is completed",
			"assigned": "skipped: task second00 is assigned",
			"taskless": "skipped: it owns no task; pass --results-collected once its results are read",
			"parent00": "skipped: its worker grandkid is live",
			"gone0000": "skipped: no live Herdr agent hosts its terminal",
		}
		if collected {
			want["taskless"] = "planned: "
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("collected=%v:\n got %v\nwant %v", collected, got, want)
		}
	}
}

// A dry run reports the plan and writes nothing: no state files, no mutating
// Herdr requests, not even the caller's relocation to its current pane.
func TestDryRunPlansWithoutWrites(t *testing.T) {
	r := newRepo(t)
	callerAgent := agent("w1:p1", "w1", "t_caller", "orchestrator", "working")
	moved := callerAgent
	moved.PaneID = "w1:p9"
	callerRec := r.register(t, moved, nil, "adopt", nil)
	w := agent("w2:p1", "w2", "t_worker", "worker", "idle")
	wrec := r.register(t, w, &callerRec.ID, "spawn", created(t, r.topic, "dev"))
	tasktest.Seed(t, r.root, task.Record{Title: "t", Owner: &wrec.ID, Status: task.Verified})
	before := snapshot(t, filepath.Join(r.root, ".fledge", "state"))
	out := Run(context.Background(), client(t, r, get(callerAgent), agentList(callerAgent, w), call{Method: "worktree.list", Result: r.listing(map[string]string{r.topic: "w2"}, r.topic)}), Options{DryRun: true})
	if out.Status != "success" || out.Operation != "agent.cleanup" || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
	res := out.Result.(Result)
	if !res.DryRun || res.Caller != callerRec.ID || worker(res, wrec.ID).Outcome != "planned" || checkout(res, r.topic).Outcome != "planned" {
		t.Fatalf("%+v", res)
	}
	if c := checkout(res, r.topic); c.Worker != wrec.ID || *c.Base != "dev" || *c.Branch != "topic" {
		t.Fatalf("%+v", c)
	}
	if after := snapshot(t, filepath.Join(r.root, ".fledge", "state")); !reflect.DeepEqual(before, after) {
		t.Fatalf("dry run wrote state:\n%v\n%v", before, after)
	}
	if _, err := os.Stat(r.topic); err != nil {
		t.Fatal(err)
	}
}

func TestRequiresRegisteredCaller(t *testing.T) {
	r := newRepo(t)
	a := agent("w1:p1", "w1", "t_caller", "orchestrator", "idle")
	for name, calls := range map[string][]call{
		"unregistered agent": {get(a)},
		"not an agent":       {{Method: "agent.get", Err: &herdr.Error{Code: "agent_not_found", Message: "none"}}},
	} {
		t.Run(name, func(t *testing.T) {
			out := Run(context.Background(), client(t, r, calls...), Options{DryRun: true})
			if out.Error == nil || out.Error.Code != "caller_unregistered" || out.ExitCode() != 1 {
				t.Fatalf("%+v", out)
			}
		})
	}
}

// Cleanup stops an eligible worker by its record, then removes the clean
// checkout its spawn created once merged into the recorded base, keeping the
// branch.
func TestCleanupStopsWorkerAndRemovesItsCheckout(t *testing.T) {
	r := newRepo(t)
	callerAgent := agent("w1:p1", "w1", "t_caller", "orchestrator", "working")
	callerRec := r.register(t, callerAgent, nil, "adopt", nil)
	w := agent("w2:p1", "w2", "t_worker", "worker", "idle")
	wrec := r.register(t, w, &callerRec.ID, "spawn", created(t, r.topic, "dev"))
	tasktest.Seed(t, r.root, task.Record{Title: "t", Owner: &wrec.ID, Status: task.Verified})
	open := map[string]string{r.topic: "w2"}
	out := Run(context.Background(), client(t, r,
		get(callerAgent), agentList(callerAgent, w), call{Method: "worktree.list", Result: r.listing(open, r.topic)},
		get(w), call{Method: "pane.close", Params: map[string]any{"pane_id": "w2:p1"}, Result: herdrscript.OK()},
		// Closing the workspace's last pane closed the workspace.
		call{Method: "worktree.list", Result: r.listing(nil, r.topic)}, agentList(callerAgent), agentList(callerAgent),
	), Options{})
	if out.Status != "success" || out.ExitCode() != 0 {
		t.Fatalf("%+v", out)
	}
	res := out.Result.(Result)
	if worker(res, wrec.ID).Outcome != "done" || checkout(res, r.topic).Outcome != "done" {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(r.topic); !os.IsNotExist(err) {
		t.Fatal("checkout kept")
	}
	if !strings.Contains(git(t, r.root, "branch", "--list", "topic"), "topic") {
		t.Fatal("branch deleted")
	}
	if rec := r.load(t, wrec.ID); rec.EndedAt == nil {
		t.Fatalf("worker record not ended: %+v", rec)
	}
	want := []libagent.Effect{{Action: "closed", Kind: "pane", ID: "w2:p1"}, {Action: "updated", Kind: "agent_record", ID: wrec.ID}, {Action: "removed", Kind: "worktree", Path: r.topic}}
	if !reflect.DeepEqual(out.Effects, want) {
		t.Fatalf("%+v", out.Effects)
	}
}

// Each checkout is kept, with its reason, unless it is a managed checkout the
// worker's spawn created from a recorded base, clean, merged into that base,
// and used by no live agent other than the workers being stopped.
func TestCheckoutGuards(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		setup        func(t *testing.T, r repo, path *string, checkout **identity.Checkout, others *[]herdr.AgentDetails)
	}{
		{"clean and merged", "", func(*testing.T, repo, *string, **identity.Checkout, *[]herdr.AgentDetails) {}},
		{"dirty", "dirty: yes", func(t *testing.T, r repo, _ *string, _ **identity.Checkout, _ *[]herdr.AgentDetails) {
			os.WriteFile(filepath.Join(r.topic, "tracked"), []byte("x"), 0o644)
			git(t, r.topic, "add", "tracked")
		}},
		{"untracked", "dirty: yes", func(t *testing.T, r repo, _ *string, _ **identity.Checkout, _ *[]herdr.AgentDetails) {
			os.WriteFile(filepath.Join(r.topic, "untracked"), []byte("x"), 0o644)
		}},
		{"unmerged", "merged into dev: no", func(t *testing.T, r repo, _ *string, _ **identity.Checkout, _ *[]herdr.AgentDetails) {
			git(t, r.topic, "commit", "-qm", "work", "--allow-empty")
		}},
		{"merged only into main", "merged into dev: no", func(t *testing.T, r repo, _ *string, _ **identity.Checkout, _ *[]herdr.AgentDetails) {
			git(t, r.topic, "commit", "-qm", "work", "--allow-empty")
			git(t, r.root, "merge", "-q", "--ff-only", "topic")
		}},
		{"unknown base", "merged into gone: unknown", func(t *testing.T, r repo, _ *string, c **identity.Checkout, _ *[]herdr.AgentDetails) {
			*c = created(t, r.topic, "gone")
		}},
		{"no recorded base", "no recorded base branch", func(t *testing.T, r repo, _ *string, c **identity.Checkout, _ *[]herdr.AgentDetails) {
			*c = created(t, r.topic, "dev")
			(*c).Base = nil
		}},
		{"legacy record without marker", "no recorded checkout identity", func(t *testing.T, r repo, _ *string, c **identity.Checkout, _ *[]herdr.AgentDetails) {
			base := "dev"
			*c = &identity.Checkout{Path: r.topic, Created: true, Base: &base, Branch: s("topic")}
		}},
		{"empty recorded marker on an unmarked checkout", "replaced since the worker's spawn", func(t *testing.T, r repo, _ *string, c **identity.Checkout, _ *[]herdr.AgentDetails) {
			git(t, r.root, "worktree", "remove", r.topic)
			git(t, r.root, "worktree", "add", "-q", r.topic, "topic")
			base := "dev"
			*c = &identity.Checkout{Path: r.topic, Created: true, Base: &base, Branch: s("topic"), Marker: s("")}
		}},
		{"branch switched", "replaced since the worker's spawn", func(t *testing.T, r repo, _ *string, _ **identity.Checkout, _ *[]herdr.AgentDetails) {
			git(t, r.topic, "switch", "-q", "-c", "other")
		}},
		{"borrowed", "not created by its worker's spawn", func(t *testing.T, r repo, _ *string, c **identity.Checkout, _ *[]herdr.AgentDetails) {
			*c = &identity.Checkout{Path: r.topic}
		}},
		{"external", "not a managed checkout under .fledge/worktrees", func(t *testing.T, r repo, path *string, c **identity.Checkout, _ *[]herdr.AgentDetails) {
			ext, _ := filepath.EvalSymlinks(t.TempDir())
			ext = filepath.Join(ext, "ext")
			git(t, r.root, "worktree", "add", "-q", "-b", "ext", ext, "dev")
			*path, *c = ext, created(t, ext, "dev")
		}},
		{"primary", "primary checkout", func(t *testing.T, r repo, path *string, c **identity.Checkout, _ *[]herdr.AgentDetails) {
			*path, *c = r.root, created(t, r.root, "dev")
		}},
		{"used by an unrelated agent", "live agent stranger (w9:p1) is working in", func(t *testing.T, r repo, _ *string, _ **identity.Checkout, others *[]herdr.AgentDetails) {
			a := agent("w9:p1", "w9", "t_stranger", "stranger", "idle")
			a.Cwd = s(filepath.Join(r.topic))
			*others = append(*others, a)
		}},
		{"used by another registered agent", "live agent peer (w9:p2) is registered to", func(t *testing.T, r repo, _ *string, _ **identity.Checkout, others *[]herdr.AgentDetails) {
			a := agent("w9:p2", "w9", "t_peer", "peer", "idle")
			r.register(t, a, nil, "adopt", &identity.Checkout{Path: r.topic})
			*others = append(*others, a)
		}},
		{"used by an unregistered agent", "is in workspace w2", func(t *testing.T, r repo, _ *string, _ **identity.Checkout, others *[]herdr.AgentDetails) {
			*others = append(*others, agent("w2:p2", "w2", "t_unregistered", "helper", "idle"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRepo(t)
			path, c := r.topic, created(t, r.topic, "dev")
			var others []herdr.AgentDetails
			tc.setup(t, r, &path, &c, &others)
			callerAgent := agent("w1:p1", "w1", "t_caller", "orchestrator", "working")
			callerRec := r.register(t, callerAgent, nil, "adopt", nil)
			w := agent("w2:p1", "w2", "t_worker", "worker", "idle")
			wrec := r.register(t, w, &callerRec.ID, "spawn", c)
			tasktest.Seed(t, r.root, task.Record{Title: "t", Owner: &wrec.ID, Status: task.Verified})
			extra := []string{r.topic}
			if path != r.topic && path != r.root {
				extra = append(extra, path)
			}
			agents := append([]herdr.AgentDetails{callerAgent, w}, others...)
			out := Run(context.Background(), client(t, r, get(callerAgent), agentList(agents...), call{Method: "worktree.list", Result: r.listing(map[string]string{path: "w2"}, extra...)}), Options{DryRun: true})
			got := checkout(out.Result.(Result), path)
			if tc.reason == "" {
				if got.Outcome != "planned" || got.Reason != nil {
					t.Fatalf("%+v", got)
				}
				return
			}
			if got.Outcome != "skipped" || got.Reason == nil || !strings.Contains(*got.Reason, tc.reason) {
				t.Fatalf("want skipped: %s, got %+v", tc.reason, got)
			}
			if out.Status != "success" || out.ExitCode() != 0 {
				t.Fatalf("%+v", out)
			}
		})
	}
}

// A held worker keeps its checkout and is left running.
func TestHeldWorkerKeepsCheckout(t *testing.T) {
	r := newRepo(t)
	callerAgent := agent("w1:p1", "w1", "t_caller", "orchestrator", "working")
	callerRec := r.register(t, callerAgent, nil, "adopt", nil)
	w := agent("w2:p1", "w2", "t_worker", "worker", "idle")
	wrec := r.register(t, w, &callerRec.ID, "spawn", created(t, r.topic, "dev"))
	tasktest.Seed(t, r.root, task.Record{Title: "t", Owner: &wrec.ID, Status: task.Completed})
	out := Run(context.Background(), client(t, r, get(callerAgent), agentList(callerAgent, w), call{Method: "worktree.list", Result: r.listing(map[string]string{r.topic: "w2"}, r.topic)}), Options{})
	res := out.Result.(Result)
	if out.Status != "success" || worker(res, wrec.ID).Outcome != "skipped" {
		t.Fatalf("%+v", out)
	}
	if c := checkout(res, r.topic); c.Outcome != "skipped" || !strings.Contains(*c.Reason, "worker "+wrec.ID+" is held") {
		t.Fatalf("%+v", c)
	}
}

// Guards are refreshed before acting: a task assigned after planning holds
// the worker, and a worker busy again by the time it is stopped is not forced.
// Neither worker's checkout is removed.
func TestGuardsRecheckedBeforeStopping(t *testing.T) {
	for _, change := range []string{"task", "busy"} {
		t.Run(change, func(t *testing.T) {
			r := newRepo(t)
			callerAgent := agent("w1:p1", "w1", "t_caller", "orchestrator", "working")
			callerRec := r.register(t, callerAgent, nil, "adopt", nil)
			w := agent("w2:p1", "w2", "t_worker", "worker", "idle")
			wrec := r.register(t, w, &callerRec.ID, "spawn", created(t, r.topic, "dev"))
			tasktest.Seed(t, r.root, task.Record{Title: "t", Owner: &wrec.ID, Status: task.Verified})
			busy := w
			busy.AgentStatus = "working"
			plan := call{Method: "worktree.list", Result: r.listing(map[string]string{r.topic: "w2"}, r.topic)}
			calls := []call{get(callerAgent), agentList(callerAgent, w), plan}
			if change == "task" {
				calls[2].Before = func() { tasktest.Seed(t, r.root, task.Record{Title: "more", Owner: &wrec.ID, Status: task.Assigned}) }
			} else {
				calls = append(calls, get(busy))
			}
			out := Run(context.Background(), client(t, r, calls...), Options{})
			res := out.Result.(Result)
			if out.Status != "success" || len(out.Effects) != 0 || worker(res, wrec.ID).Outcome != "skipped" || checkout(res, r.topic).Outcome != "skipped" {
				t.Fatalf("%+v %+v", out, res)
			}
			if rec := r.load(t, wrec.ID); rec.EndedAt != nil {
				t.Fatal("record ended")
			}
			if _, err := os.Stat(r.topic); err != nil {
				t.Fatal("checkout removed")
			}
		})
	}
}

// A worker whose recorded pane now hosts a different terminal is never stopped:
// the replacement agent keeps running.
func TestReplacementTerminalUntouched(t *testing.T) {
	r := newRepo(t)
	callerAgent := agent("w1:p1", "w1", "t_caller", "orchestrator", "working")
	callerRec := r.register(t, callerAgent, nil, "adopt", nil)
	w := agent("w2:p1", "w2", "t_worker", "worker", "idle")
	wrec := r.register(t, w, &callerRec.ID, "spawn", nil)
	tasktest.Seed(t, r.root, task.Record{Title: "t", Owner: &wrec.ID, Status: task.Verified})
	replacement := agent("w2:p1", "w2", "t_new", "worker", "idle")
	out := Run(context.Background(), client(t, r,
		get(callerAgent), agentList(callerAgent, w),
		// By the time it is stopped, the pane hosts another terminal and the
		// worker's own terminal is gone.
		get(replacement), agentList(callerAgent, replacement),
		call{Method: "pane.list", Result: map[string]any{"type": "pane_list", "panes": []herdr.AgentDetails{callerAgent, replacement}}},
	), Options{})
	if len(out.Effects) != 0 || worker(out.Result.(Result), wrec.ID).Outcome == "done" {
		t.Fatalf("%+v", out)
	}
}

// A stop that succeeds while its checkout is kept leaves the worker archived;
// a later run still finds that checkout through the archived record and
// removes it once it is safe.
func TestRerunRemovesCheckoutOfStoppedWorker(t *testing.T) {
	r := newRepo(t)
	git(t, r.topic, "commit", "-qm", "work", "--allow-empty")
	callerAgent := agent("w1:p1", "w1", "t_caller", "orchestrator", "working")
	callerRec := r.register(t, callerAgent, nil, "adopt", nil)
	w := agent("w2:p1", "w2", "t_worker", "worker", "idle")
	wrec := r.register(t, w, &callerRec.ID, "spawn", created(t, r.topic, "dev"))
	tasktest.Seed(t, r.root, task.Record{Title: "t", Owner: &wrec.ID, Status: task.Verified})
	out := Run(context.Background(), client(t, r,
		get(callerAgent), agentList(callerAgent, w), call{Method: "worktree.list", Result: r.listing(map[string]string{r.topic: "w2"}, r.topic)},
		get(w), call{Method: "pane.close", Result: herdrscript.OK()},
	), Options{})
	res := out.Result.(Result)
	if out.Status != "success" || worker(res, wrec.ID).Outcome != "done" || checkout(res, r.topic).Outcome != "skipped" {
		t.Fatalf("%+v %+v", out, res)
	}
	git(t, r.root, "branch", "-f", "dev", "topic")
	out = Run(context.Background(), client(t, r,
		get(callerAgent), agentList(callerAgent), call{Method: "worktree.list", Result: r.listing(nil, r.topic)},
		call{Method: "worktree.list", Result: r.listing(nil, r.topic)}, agentList(callerAgent), agentList(callerAgent),
	), Options{})
	res = out.Result.(Result)
	if out.Status != "success" || len(res.Workers) != 0 || checkout(res, r.topic).Outcome != "done" {
		t.Fatalf("%+v %+v", out, res)
	}
	if _, err := os.Stat(r.topic); !os.IsNotExist(err) {
		t.Fatal("checkout kept")
	}
	// Once removed, the checkout is no longer reported.
	out = Run(context.Background(), client(t, r, get(callerAgent), agentList(callerAgent), call{Method: "worktree.list", Result: r.listing(nil)}), Options{})
	if res := out.Result.(Result); out.Status != "success" || len(res.Checkouts) != 0 {
		t.Fatalf("%+v", res)
	}
}

// A failed stop is reported as a failure with a nonzero exit, and the
// worker's checkout is left in place.
func TestStopFailureIsPartial(t *testing.T) {
	r := newRepo(t)
	callerAgent := agent("w1:p1", "w1", "t_caller", "orchestrator", "working")
	callerRec := r.register(t, callerAgent, nil, "adopt", nil)
	w := agent("w2:p1", "w2", "t_worker", "worker", "idle")
	wrec := r.register(t, w, &callerRec.ID, "spawn", created(t, r.topic, "dev"))
	w2 := agent("w3:p1", "w3", "t_worker2", "worker2", "idle")
	w2rec := r.register(t, w2, &callerRec.ID, "spawn", nil)
	for _, id := range []string{wrec.ID, w2rec.ID} {
		tasktest.Seed(t, r.root, task.Record{Title: "t", Owner: &id, Status: task.Verified})
	}
	out := Run(context.Background(), client(t, r,
		get(callerAgent), agentList(callerAgent, w, w2), call{Method: "worktree.list", Result: r.listing(map[string]string{r.topic: "w2"}, r.topic)},
		get(w), call{Method: "pane.close", Err: &herdr.Error{Code: "timeout", Message: "no answer", Uncertain: true}},
		get(w2), call{Method: "pane.close", Result: herdrscript.OK()},
	), Options{})
	res := out.Result.(Result)
	if out.Status != "unknown" || out.ExitCode() != 1 || out.Error == nil || worker(res, wrec.ID).Outcome != "failed" || worker(res, w2rec.ID).Outcome != "done" {
		t.Fatalf("%+v %+v", out, res)
	}
	if c := checkout(res, r.topic); c.Outcome != "skipped" {
		t.Fatalf("%+v", c)
	}
	if _, err := os.Stat(r.topic); err != nil {
		t.Fatal("checkout removed")
	}
}

func TestRender(t *testing.T) {
	plan := Result{DryRun: true, Caller: "caller00",
		Workers:   []Worker{{ID: "a1b2c3d4", Name: s("worker"), Outcome: "planned"}, {ID: "b1b2c3d4", Name: s("reviewer"), Outcome: "skipped", Reason: s("task 12345678 is completed")}},
		Checkouts: []Checkout{{Path: "/r/.fledge/worktrees/topic", Branch: s("topic"), Base: s("dev"), Worker: "a1b2c3d4", Outcome: "planned"}}}
	done := plan
	done.DryRun = false
	done.Workers = []Worker{{ID: "a1b2c3d4", Name: s("worker"), Outcome: "done"}}
	done.Checkouts = []Checkout{{Path: "/r/.fledge/worktrees/topic", Branch: s("topic"), Base: s("dev"), Worker: "a1b2c3d4", Outcome: "failed", Reason: s("git worktree remove: locked")}}
	for _, tc := range []struct {
		result Result
		want   string
	}{
		{plan, "Dry run: would stop 1 of 2 workers and remove 1 of 1 checkouts.\n" +
			"  stop    worker (a1b2c3d4)\n" +
			"  hold    reviewer (b1b2c3d4): task 12345678 is completed\n" +
			"  remove  /r/.fledge/worktrees/topic (branch topic, from dev)\n"},
		{done, "Stopped 1 of 1 workers and removed 0 of 1 checkouts.\n" +
			"  stopped  worker (a1b2c3d4)\n" +
			"  failed   /r/.fledge/worktrees/topic (branch topic, from dev): git worktree remove: locked\n"},
		{Result{Workers: []Worker{}, Checkouts: []Checkout{}}, "No spawned workers or created checkouts to clean up.\n"},
	} {
		var b bytes.Buffer
		if err := (libagent.Outcome{Status: "success", Result: tc.result}).Write(&b, false, Render); err != nil {
			t.Fatal(err)
		}
		if b.String() != tc.want {
			t.Fatalf("got\n%s\nwant\n%s", b.String(), tc.want)
		}
		herdrscript.CheckOutputFailures(t, Render, libagent.Outcome{Result: tc.result})
	}
}

// A live worker of the worker holds it whether registered before planning or
// between planning and the stop, so its pane is never closed under the new
// worker. (Regression from verification: descendants were not rechecked.)
func TestNewDescendantHoldsWorker(t *testing.T) {
	for _, late := range []bool{false, true} {
		name := "before_planning"
		if late {
			name = "after_planning"
		}
		t.Run(name, func(t *testing.T) {
			r := newRepo(t)
			a := agent("w1:p1", "w1", "t_caller", "orchestrator", "working")
			caller := r.register(t, a, nil, "adopt", nil)
			w := agent("w2:p1", "w2", "t_worker", "worker", "idle")
			rec := r.register(t, w, &caller.ID, "spawn", nil)
			tasktest.Seed(t, r.root, task.Record{Title: "accepted", Owner: &rec.ID, Status: task.Verified})
			addChild := func() {
				child := agent("w3:p1", "w3", "t_child", "child", "working")
				r.register(t, child, &rec.ID, "spawn", nil)
			}
			listing := agentList(a, w)
			if late {
				listing.Before = addChild
			} else {
				addChild()
			}
			// No agent.get or pane.close of the worker is scripted: stopping it fails the test.
			out := Run(context.Background(), client(t, r, get(a), listing), Options{})
			got := worker(out.Result.(Result), rec.ID)
			if out.Status != "success" || len(out.Effects) != 0 || got.Outcome != "skipped" || got.Reason == nil || !strings.Contains(*got.Reason, "is live") {
				t.Fatalf("new live descendant must hold its parent: %+v %+v", out, got)
			}
			if r.load(t, rec.ID).EndedAt != nil {
				t.Fatal("worker record ended")
			}
		})
	}
}

// endRecord ends record id as a completed agent stop would.
func (r repo) endRecord(t *testing.T, id string) {
	t.Helper()
	st, err := identity.Existing(context.Background(), r.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := identity.End(st, id); err != nil {
		t.Fatal(err)
	}
}

// A stopped worker's provenance belongs to the checkout its spawn created,
// not to its path: once that checkout is removed, one recreated at the same
// path, by hand on another branch or on the same branch, is reported as
// replaced and never removed. Only the planning calls are scripted, so any
// removal attempt fails the test. (Regression from verification.)
func TestArchivedProvenanceDoesNotOwnReplacement(t *testing.T) {
	for _, branch := range []string{"replacement", "topic"} {
		for _, dryRun := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s dry-run=%v", branch, dryRun), func(t *testing.T) {
				r := newRepo(t)
				a := agent("w1:p1", "w1", "t_caller", "orchestrator", "working")
				caller := r.register(t, a, nil, "adopt", nil)
				w := agent("w2:p1", "w2", "t_old", "old", "done")
				old := r.register(t, w, &caller.ID, "spawn", created(t, r.topic, "dev"))
				r.endRecord(t, old.ID)
				git(t, r.root, "worktree", "remove", r.topic)
				if branch == "topic" {
					git(t, r.root, "worktree", "add", "-q", r.topic, "topic")
				} else {
					git(t, r.root, "worktree", "add", "-q", "-b", branch, r.topic, "dev")
				}
				listing := r.listing(nil, r.topic)
				listing.Worktrees[1].Branch = s(branch)
				out := Run(context.Background(), client(t, r, get(a), agentList(a), call{Method: "worktree.list", Result: listing}), Options{DryRun: dryRun})
				c := checkout(out.Result.(Result), r.topic)
				if out.Status != "success" || c.Outcome != "skipped" || c.Reason == nil || !strings.Contains(*c.Reason, "replaced since the worker's spawn") {
					t.Fatalf("replacement checkout must not inherit archived provenance: %+v %+v", out, c)
				}
				if _, err := os.Stat(r.topic); err != nil {
					t.Fatal("replacement checkout deleted")
				}
			})
		}
	}
}

// When several of the caller's workers recorded the same path, only the one
// whose recorded marker matches the checkout there owns it, whichever
// registered first or last.
func TestOnlyMatchingIncarnationOwnsSharedPath(t *testing.T) {
	for _, newerOwns := range []bool{true, false} {
		t.Run(fmt.Sprintf("newer owns=%v", newerOwns), func(t *testing.T) {
			r := newRepo(t)
			a := agent("w1:p1", "w1", "t_caller", "orchestrator", "working")
			caller := r.register(t, a, nil, "adopt", nil)
			stale := created(t, r.topic, "dev")
			stale.Marker = s("0123456789abcdef0123456789abcdef")
			current := created(t, r.topic, "dev")
			first, second := stale, current
			if !newerOwns {
				first, second = current, stale
			}
			older := r.register(t, agent("w2:p1", "w2", "t_older", "older", "done"), &caller.ID, "spawn", first)
			r.endRecord(t, older.ID)
			newer := r.register(t, agent("w3:p1", "w3", "t_newer", "newer", "done"), &caller.ID, "spawn", second)
			r.endRecord(t, newer.ID)
			owner := newer.ID
			if !newerOwns {
				owner = older.ID
			}
			out := Run(context.Background(), client(t, r, get(a), agentList(a), call{Method: "worktree.list", Result: r.listing(nil, r.topic)}), Options{DryRun: true})
			res := out.Result.(Result)
			if c := checkout(res, r.topic); len(res.Checkouts) != 1 || c.Outcome != "planned" || c.Worker != owner {
				t.Fatalf("want planned under %s: %+v", owner, res.Checkouts)
			}
		})
	}
}

// A checkout replaced while its worker is being stopped (same path, same
// branch, no marker) is kept: removal rechecks the checkout's identity last,
// right before deleting. (Regression from verification.)
func TestReplacementDuringStopIsKept(t *testing.T) {
	r := newRepo(t)
	a := agent("w1:p1", "w1", "t_caller", "caller", "working")
	caller := r.register(t, a, nil, "adopt", nil)
	w := agent("w2:p1", "w2", "t_worker", "worker", "idle")
	rec := r.register(t, w, &caller.ID, "spawn", created(t, r.topic, "dev"))
	tasktest.Seed(t, r.root, task.Record{Title: "accepted", Owner: &rec.ID, Status: task.Verified})
	replace := func() {
		git(t, r.root, "worktree", "remove", r.topic)
		git(t, r.root, "worktree", "add", "-q", r.topic, "topic")
	}
	listing := r.listing(nil, r.topic)
	out := Run(context.Background(), client(t, r,
		get(a), agentList(a, w), call{Method: "worktree.list", Result: listing},
		get(w), call{Method: "pane.close", Before: replace, Result: herdrscript.OK()},
		call{Method: "worktree.list", Result: listing}, agentList(a), agentList(a),
	), Options{})
	if _, err := os.Stat(r.topic); os.IsNotExist(err) {
		t.Fatalf("replacement created during worker stop was DELETED: %+v", out.Result)
	}
	c := checkout(out.Result.(Result), r.topic)
	if out.Status != "success" || worker(out.Result.(Result), rec.ID).Outcome != "done" || c.Outcome != "skipped" || c.Reason == nil || !strings.Contains(*c.Reason, "replaced since the worker's spawn") {
		t.Fatalf("%+v %+v", out, c)
	}
}

// A checkout moved out of the managed tree while its worker is being stopped,
// its old path left as a symlink to it, keeps its marker but is kept: removal
// requires the checkout still at the planned path under .fledge/worktrees.
// (Regression from verification.)
func TestMovedOutsideManagedTreeIsKept(t *testing.T) {
	r := newRepo(t)
	a := agent("w1:p1", "w1", "t_caller", "caller", "working")
	caller := r.register(t, a, nil, "adopt", nil)
	w := agent("w2:p1", "w2", "t_worker", "worker", "idle")
	rec := r.register(t, w, &caller.ID, "spawn", created(t, r.topic, "dev"))
	tasktest.Seed(t, r.root, task.Record{Title: "accepted", Owner: &rec.ID, Status: task.Verified})
	outside := filepath.Join(t.TempDir(), "outside")
	move := func() {
		git(t, r.root, "worktree", "move", r.topic, outside)
		if err := os.Symlink(outside, r.topic); err != nil {
			t.Fatal(err)
		}
	}
	movedListing := r.listing(nil, outside)
	movedListing.Worktrees[1].Branch = s("topic")
	out := Run(context.Background(), client(t, r,
		get(a), agentList(a, w), call{Method: "worktree.list", Result: r.listing(nil, r.topic)},
		get(w), call{Method: "pane.close", Before: move, Result: herdrscript.OK()},
		call{Method: "worktree.list", Result: movedListing}, agentList(a), agentList(a),
	), Options{})
	if _, err := os.Stat(outside); os.IsNotExist(err) {
		t.Fatalf("checkout moved outside managed tree was DELETED: %+v", out.Result)
	}
	c := checkout(out.Result.(Result), r.topic)
	if out.Status != "success" || c.Outcome != "skipped" || c.Reason == nil || !strings.Contains(*c.Reason, "moved since cleanup planned it") {
		t.Fatalf("%+v %+v", out, c)
	}
}
