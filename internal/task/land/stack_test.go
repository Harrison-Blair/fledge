package land

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// setupLayer is the fixture with a stack parent of two layers: lower, on
// branch low (main and one file), and the task, verified on branch up (low
// and one file). lower is stored as mod leaves it.
func setupLayer(t *testing.T, mod func(lower *task.Record)) (f fixture, parent, lower string) {
	t.Helper()
	f = setup(t, task.ModeLocal)
	git(t, f.repo, "checkout", "-qb", "low", "main")
	commitFile(t, f.repo, "lower", "lower\n")
	git(t, f.repo, "checkout", "-qb", "up")
	f.head = commitFile(t, f.repo, "upper", "upper\n")
	git(t, f.repo, "checkout", "-q", "main")
	parent = tasktest.Seed(t, f.repo, task.Record{Title: "p", Brief: "b", Status: task.Created, Shape: task.ShapeStack, CreatedAt: *task.Now()})
	r := task.Record{Title: "lower", Brief: "b", Status: task.Verified, Parent: &parent, Branch: tasktest.Ptr("low"), CreatedAt: *task.Now()}
	if mod != nil {
		mod(&r)
	}
	lower = tasktest.Seed(t, f.repo, r)
	f.id = tasktest.Seed(t, f.repo, task.Record{Title: "upper", Brief: "b", Status: task.Verified, Owner: &f.worker.ID, Verifier: &f.boss.ID,
		VerifiedAt: task.Now(), VerifiedCommit: &f.head, DeliveryMode: task.ModeLocal, Parent: &parent, After: []string{lower},
		Branch: tasktest.Ptr("up"), CreatedAt: *task.Now()})
	return f, parent, lower
}

// measure runs measure for the task of f, as the stack path does for a layer.
func (f fixture) measure(t *testing.T) (*lander, error) {
	t.Helper()
	r := f.load(t)
	l := &lander{s: tasktest.Store(t, f.repo), root: f.repo, r: r, mode: r.DeliveryMode, commit: *r.VerifiedCommit}
	return l, l.measure(context.Background())
}

func TestLayerSizeUsesLayerBase(t *testing.T) {
	f, _, _ := setupLayer(t, nil)
	l, err := f.measure(t)
	if err != nil || l.size == nil || l.base != "refs/heads/low" || l.size.Files.Value != 1 {
		t.Fatalf("%v %+v", err, l)
	}
}

func TestLayerSizeReturnsResolverRefusals(t *testing.T) {
	f, _, lower := setupLayer(t, func(r *task.Record) { r.Branch = nil })
	var coded *cli.Error
	if _, err := f.measure(t); !errors.As(err, &coded) || coded.Code != "land_branch_unknown" || !strings.Contains(coded.Message, lower) {
		t.Fatalf("%v", err)
	}
	f, parent, lower := setupLayer(t, nil)
	tasktest.Seed(t, f.repo, task.Record{Title: "x", Brief: "b", Status: task.Created, Parent: &parent, After: []string{lower}, CreatedAt: *task.Now()})
	if _, err := f.measure(t); !errors.As(err, &coded) || coded.Code != "land_stack_fork" {
		t.Fatalf("%v", err)
	}
}

// TestLayerWithoutSizeRefusesNothing: a missing base ref or a failed diff
// leaves the size unset and does not refuse.
func TestLayerWithoutSizeRefusesNothing(t *testing.T) {
	missing, _, _ := setupLayer(t, func(r *task.Record) { r.Branch = tasktest.Ptr("gone") })
	failed, _, _ := setupLayer(t, func(r *task.Record) { r.Branch = tasktest.Ptr("orphan") })
	git(t, failed.repo, "checkout", "-q", "--orphan", "orphan")
	commitFile(t, failed.repo, "other", "other\n")
	git(t, failed.repo, "checkout", "-q", "main")
	for name, f := range map[string]fixture{"missing": missing, "failed": failed} {
		if l, err := f.measure(t); err != nil || l.size != nil {
			t.Fatalf("%s: %v %+v", name, err, l.size)
		}
	}
}

// TestLayerWithoutStackBaseRefusesNothing: when the parent has no stack
// base, the size stays unset and measure does not refuse.
func TestLayerWithoutStackBaseRefusesNothing(t *testing.T) {
	f, _, _ := setupLayer(t, func(r *task.Record) { r.Status = task.Landed })
	git(t, f.repo, "branch", "-m", "main", "trunk")
	if _, err := task.Update(tasktest.Store(t, f.repo), f.id, func(r *task.Record) error {
		r.BaseBranch = tasktest.Ptr("trunk")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if l, err := f.measure(t); err != nil || l.size != nil {
		t.Fatalf("%v %+v", err, l.size)
	}
}

// stack is a fixture with a bare origin holding main, a fake gh, a git that
// logs its arguments, and a verified stack parent in pr mode with three
// layers l1, l2, and l3. Each layer is verified at its local head, and no
// layer branch exists on origin. f.id is the parent.
type stack struct {
	fixture
	gh             fakeGH
	gitLog, origin string
	layers, heads  []string
}

func setupStack(t *testing.T) stack {
	t.Helper()
	f, g := setupPR(t, task.ModePR)
	s := stack{fixture: f, gh: g, origin: git(t, f.repo, "remote", "get-url", "origin")}
	s.id = tasktest.Seed(t, f.repo, task.Record{Title: "p", Brief: "b", Status: task.Verified, Shape: task.ShapeStack, DeliveryMode: task.ModePR,
		Owner: &f.worker.ID, Verifier: &f.boss.ID, VerifiedAt: task.Now(), CreatedAt: *task.Now()})
	below := "main"
	for _, name := range []string{"l1", "l2", "l3"} {
		git(t, f.repo, "checkout", "-qb", name, below)
		head := commitFile(t, f.repo, name, name+"\n")
		r := task.Record{Title: name, Brief: "b", Status: task.Verified, Owner: &f.worker.ID, Verifier: &f.boss.ID, VerifiedAt: task.Now(),
			VerifiedCommit: &head, DeliveryMode: task.ModeLocal, Parent: &s.id, Branch: tasktest.Ptr(name), CreatedAt: *task.Now()}
		if len(s.layers) > 0 {
			r.After = []string{s.layers[len(s.layers)-1]}
		}
		s.layers, s.heads, below = append(s.layers, tasktest.Seed(t, f.repo, r)), append(s.heads, head), name
	}
	git(t, f.repo, "checkout", "-q", "main")
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	s.gitLog = filepath.Join(dir, "log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + s.gitLog + "'\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return s
}

// edit stores mutate's changes to task id.
func (s stack) edit(t *testing.T, id string, mutate func(*task.Record)) {
	t.Helper()
	if _, err := task.Update(tasktest.Store(t, s.repo), id, func(r *task.Record) error { mutate(r); return nil }); err != nil {
		t.Fatal(err)
	}
}

// pushes returns the git push commands that ran since the last call.
func (s stack) pushes(t *testing.T) []string {
	t.Helper()
	b, _ := os.ReadFile(s.gitLog)
	os.Remove(s.gitLog)
	var pushes []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, " push ") {
			pushes = append(pushes, line)
		}
	}
	return pushes
}

// remote returns the commit of branch on origin, or "" when it is missing.
func (s stack) remote(t *testing.T, branch string) string {
	t.Helper()
	out, _ := exec.Command("git", "-C", s.origin, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Output()
	return strings.TrimSpace(string(out))
}

// stackRefused requires out to be a rejected refusal with code at the check
// step, with no layer branch on origin.
func (s stack) stackRefused(t *testing.T, out cli.Outcome, code string) {
	t.Helper()
	if out.Error == nil || out.Error.Code != code || out.Status != "rejected" {
		t.Fatalf("want %s, got %+v", code, out.Error)
	}
	if res, ok := out.Result.(Result); !ok || res.LandStep == nil || *res.LandStep != stepCheck {
		t.Fatalf("%+v", out.Result)
	}
	for _, name := range []string{"l1", "l2", "l3"} {
		if s.remote(t, name) != "" {
			t.Fatalf("%s was pushed", name)
		}
	}
}

func TestLandStackPushesLayersBottomFirst(t *testing.T) {
	s := setupStack(t)
	out := s.land(t, Options{})
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	pushes := s.pushes(t)
	if len(pushes) != 3 {
		t.Fatalf("%q", pushes)
	}
	res := out.Result.(Result)
	text := render(t, out)
	for i, name := range []string{"l1", "l2", "l3"} {
		if !strings.HasSuffix(pushes[i], "push origin "+s.heads[i]+":refs/heads/"+name) {
			t.Fatalf("push %d: %q", i, pushes[i])
		}
		if got := s.remote(t, name); got != s.heads[i] {
			t.Fatalf("origin %s at %q", name, got)
		}
		r := tasktest.Load(t, s.repo, s.layers[i])
		if r.Status != task.Verified || r.LandStep == nil || *r.LandStep != stepPush || r.Branch == nil || *r.Branch != name {
			t.Fatalf("%+v", r)
		}
		if l := res.Layers[i]; l != (Layer{ID: s.layers[i], Branch: name, Head: s.heads[i], LandStep: stepPush}) {
			t.Fatalf("%+v", l)
		}
		if !strings.Contains(text, "Layer "+s.layers[i]+": branch "+name+" at "+s.heads[i]+", land step push.\n") {
			t.Fatalf("%q", text)
		}
	}
	if p := s.load(t); p.Status != task.Verified || p.LandStep == nil || *p.LandStep != stepPush || p.Branch != nil {
		t.Fatalf("%+v", p)
	}
	if len(res.Layers) != 3 || strings.Count(text, "\n") != 3 {
		t.Fatalf("%q", text)
	}
	b, err := json.Marshal(res)
	if err != nil || !strings.Contains(string(b), `"layers":[{"id":"`+s.layers[0]+`","branch":"l1","head":"`+s.heads[0]+`","land_step":"push"}`) {
		t.Fatalf("%s %v", b, err)
	}
}

func TestLandStackRunAgainPushesNothing(t *testing.T) {
	s := setupStack(t)
	if out := s.land(t, Options{}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	s.pushes(t)
	if out := s.land(t, Options{}); out.Error != nil || len(out.Result.(Result).Layers) != 3 {
		t.Fatalf("%+v", out.Error)
	}
	if pushes := s.pushes(t); len(pushes) != 0 {
		t.Fatalf("%q", pushes)
	}
}

// TestLandStackFetchesBranchesOnOrigin: a layer branch on origin is fetched,
// so the stale tracking ref does not cause a push. A missing branch is not
// fetched.
func TestLandStackFetchesBranchesOnOrigin(t *testing.T) {
	s := setupStack(t)
	git(t, s.repo, "push", "-q", "origin", "l1")
	git(t, s.repo, "update-ref", "-d", "refs/remotes/origin/l1")
	s.pushes(t)
	if out := s.land(t, Options{}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	b, _ := os.ReadFile(s.gitLog)
	if !strings.Contains(string(b), "ls-remote --heads origin main l1 l2 l3\n") || !strings.Contains(string(b), "fetch origin main l1\n") {
		t.Fatalf("%s", b)
	}
	if pushes := s.pushes(t); len(pushes) != 2 || strings.Contains(strings.Join(pushes, "\n"), "/l1") {
		t.Fatalf("%q", pushes)
	}
}

func TestLandStackRefusesParentGuards(t *testing.T) {
	s := setupStack(t)
	s.edit(t, s.id, func(r *task.Record) { r.Status = task.Created })
	out := s.land(t, Options{})
	s.stackRefused(t, out, "task_invalid_state")
	if text := render(t, out); !strings.HasPrefix(text, "rejected: task_invalid_state: task "+s.id+" is created") || !strings.Contains(text, " (land)\nLand step reached: check.\n") {
		t.Fatalf("%q", text)
	}
	s = setupStack(t)
	s.edit(t, s.id, func(r *task.Record) { r.DeliveryMode = task.ModeAuto })
	out = s.land(t, Options{})
	s.stackRefused(t, out, "land_stack_mode")
	if text := render(t, out); !strings.HasPrefix(text, "rejected: land_stack_mode: ") || !strings.Contains(text, " (land)\nLand step reached: check.\n") {
		t.Fatalf("%q", text)
	}
}

func TestLandStackRefusesUnverifiedLayer(t *testing.T) {
	for name, mutate := range map[string]func(*task.Record){
		"completed": func(r *task.Record) { r.Status = task.Completed },
		"no commit": func(r *task.Record) { r.VerifiedCommit = nil },
	} {
		t.Run(name, func(t *testing.T) {
			s := setupStack(t)
			s.edit(t, s.layers[1], mutate)
			out := s.land(t, Options{})
			s.stackRefused(t, out, "land_stack_layer_unverified")
			text := render(t, out)
			if !strings.Contains(out.Error.Message, s.layers[1]) || !strings.HasPrefix(text, "rejected: land_stack_layer_unverified: ") ||
				!strings.Contains(text, " (land)\nLand step reached: check.\n") || len(s.pushes(t)) != 0 {
				t.Fatalf("%q", text)
			}
		})
	}
}

// TestLandStackRunsLocalGuardsFirst: a dirty checkout of the top layer stops
// the landing before any push.
func TestLandStackRunsLocalGuardsFirst(t *testing.T) {
	s := setupStack(t)
	git(t, s.repo, "checkout", "-q", "l3")
	writeFile(t, s.repo, "l3", "dirty\n")
	s.stackRefused(t, s.land(t, Options{}), "land_dirty")
	git(t, s.repo, "checkout", "-q", "--", "l3")
	git(t, s.repo, "checkout", "-q", "main")
	// Each layer is checked at its current head.
	git(t, s.repo, "checkout", "-q", "l2")
	commitFile(t, s.repo, "other", "other\n")
	git(t, s.repo, "checkout", "-q", "main")
	s.stackRefused(t, s.land(t, Options{}), "land_head_mismatch")
}

func TestLandStackRefusesBranchAndMode(t *testing.T) {
	s := setupStack(t)
	for _, id := range []string{s.id, s.layers[1]} {
		for _, o := range []Options{{ID: id, Mode: task.ModePR, ModeSet: true}, {ID: id, Branch: "l2", BranchSet: true}} {
			if out := s.land(t, o); out.Error == nil || out.Error.Code != "invalid_input" || out.ExitCode() != 2 {
				t.Fatalf("%+v: %+v", o, out.Error)
			}
		}
	}
	if len(s.pushes(t)) != 0 || s.load(t).DeliveryMode != task.ModePR {
		t.Fatal("changed")
	}
}

func TestLandStackLayerNotOpen(t *testing.T) {
	s := setupStack(t)
	out := s.land(t, Options{ID: s.layers[1]})
	s.stackRefused(t, out, "land_stack_not_open")
	text := render(t, out)
	if !strings.HasPrefix(text, "rejected: land_stack_not_open: ") || !strings.Contains(text, "fledge task land --id "+s.id) ||
		!strings.Contains(text, " (land)\nLand step reached: check.\n") {
		t.Fatalf("%q", text)
	}
	if r := tasktest.Load(t, s.repo, s.layers[1]); r.LandStep != nil {
		t.Fatalf("%+v", r)
	}
}

// TestLandStackDeletesNothing: a stack landing runs no git push --delete, no
// git branch -d, and no DELETE call.
func TestLandStackDeletesNothing(t *testing.T) {
	s := setupStack(t)
	for range 2 {
		if out := s.land(t, Options{}); out.Error != nil {
			t.Fatalf("%+v", out.Error)
		}
	}
	b, _ := os.ReadFile(s.gitLog)
	for _, bad := range []string{"--delete", "push -d", "push origin :", "branch -d", "branch -D"} {
		if strings.Contains(string(b), bad) {
			t.Fatalf("%q in %s", bad, b)
		}
	}
	for _, line := range s.gh.log(t) {
		if strings.Contains(line, "DELETE") {
			t.Fatalf("%q", line)
		}
	}
}
