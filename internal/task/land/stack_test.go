package land

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/landing"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
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
// layer branch exists on origin. f.id is the parent. The fake gh answers for
// pull request i+1 of layer i at its head, with passing checks.
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
	for i, name := range []string{"l1", "l2", "l3"} {
		git(t, f.repo, "checkout", "-qb", name, below)
		s.answer(t, i, commitFile(t, f.repo, name, name+"\n"), git(t, f.repo, "rev-parse", below))
		head := git(t, f.repo, "rev-parse", "HEAD")
		r := task.Record{Title: name, Brief: "b", Status: task.Verified, Owner: &f.worker.ID, Verifier: &f.boss.ID, VerifiedAt: task.Now(),
			VerifiedCommit: &head, DeliveryMode: task.ModeLocal, Parent: &s.id, Branch: tasktest.Ptr(name), CreatedAt: *task.Now()}
		if len(s.layers) > 0 {
			r.After = []string{s.layers[len(s.layers)-1]}
		}
		s.layers, s.heads, below = append(s.layers, tasktest.Seed(t, f.repo, r)), append(s.heads, head), name
	}
	git(t, f.repo, "checkout", "-q", "main")
	s.gh.set(t, apiStacks, `{"id":2120746,"number":11,"base":{"ref":"main"},"open":true,"pull_requests":[]}`)
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

// pullURL is the url of the pull request of layer i.
func pullURL(i int) string { return "https://example.com/pr/" + strconv.Itoa(i+1) }

// apiPull is the fake gh file that answers the REST read of the pull request
// of layer i.
func apiPull(i int) string { return "api-GET-repos_{owner}_{repo}_pulls_" + strconv.Itoa(i+1) }

// apiStacks is the fake gh file that answers the POST of a stack.
const apiStacks = "api-POST-repos_{owner}_{repo}_stacks"

// stackPosts are the commands in the fake gh log that post a stack.
func (s stack) stackPosts(t *testing.T) []string {
	t.Helper()
	return slices.DeleteFunc(s.gh.log(t), func(l string) bool { return !strings.HasPrefix(l, "api --method=POST repos/{owner}/{repo}/stacks") })
}

// inStack makes the REST read of the pull request of layer i answer with
// stack, a JSON object or null.
func (s stack) inStack(t *testing.T, i int, stack string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.gh.dir, apiPull(i)))
	if err != nil || !strings.Contains(string(b), `"stack":null`) {
		t.Fatalf("%v %s", err, b)
	}
	s.gh.set(t, apiPull(i), strings.Replace(string(b), `"stack":null`, `"stack":`+stack, 1))
}

// stackJSON is the stack of a pull request with number, size, and position.
func stackJSON(number, size, position int) string {
	return fmt.Sprintf(`{"id":2120746,"number":%d,"base":{"ref":"main","sha":"cccc"},"size":%d,"position":%d}`, number, size, position)
}

// restPull is the REST pull request of layer i at head on base.
func restPull(i int, head, base string) string {
	below := "main"
	if i > 0 {
		below = "l" + strconv.Itoa(i)
	}
	return `{"number":` + strconv.Itoa(i+1) + `,"state":"open","merged":false,"merge_commit_sha":null,"draft":false,` +
		`"head":{"sha":"` + head + `","ref":"l` + strconv.Itoa(i+1) + `"},"base":{"sha":"` + base + `","ref":"` + below + `"},"stack":null}`
}

// answer makes the fake gh answer for the pull request of layer i at head on
// base: gh pr list, passing checks, and the REST read.
func (s stack) answer(t *testing.T, i int, head, base string) {
	t.Helper()
	n := strconv.Itoa(i + 1)
	s.gh.set(t, "list-l"+n, `[{"number":`+n+`,"url":"`+pullURL(i)+`","headRefOid":"`+head+`","isDraft":false,"mergeable":"MERGEABLE"}]`)
	s.gh.set(t, "checks-"+n, checks(head, "MERGEABLE", ""))
	s.gh.set(t, apiPull(i), restPull(i, head, base))
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

func TestLandStackOpensPullRequestsAndStopsForReview(t *testing.T) {
	s := setupStack(t)
	out := s.land(t, Options{})
	if out.Error != nil || out.ExitCode() != 0 {
		t.Fatalf("%+v", out.Error)
	}
	pushes := s.pushes(t)
	if len(pushes) != 3 {
		t.Fatalf("%q", pushes)
	}
	var creates []string
	for _, line := range s.gh.log(t) {
		if strings.HasPrefix(line, "pr create ") {
			creates = append(creates, line)
		}
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
		base := []string{"main", "l1", "l2"}[i]
		if len(creates) != 3 || !strings.HasPrefix(creates[i], "pr create --base="+base+" --head="+name+" ") {
			t.Fatalf("%q", creates)
		}
		r := tasktest.Load(t, s.repo, s.layers[i])
		if r.Status != task.Verified || r.LandStep == nil || *r.LandStep != stepReview || r.Branch == nil || *r.Branch != name || r.PRURL == nil || *r.PRURL != pullURL(i) {
			t.Fatalf("%+v", r)
		}
		if l := res.Layers[i]; l != (Layer{ID: s.layers[i], Branch: name, Head: s.heads[i], PRURL: pullURL(i), LandStep: stepReview}) {
			t.Fatalf("%+v", l)
		}
		if !strings.Contains(text, "Layer "+s.layers[i]+": branch "+name+" at "+s.heads[i]+", pull request "+pullURL(i)+", land step review.\n") {
			t.Fatalf("%q", text)
		}
	}
	if p := s.load(t); p.Status != task.Verified || p.LandStep == nil || *p.LandStep != stepReview || p.Branch != nil || p.PRURL != nil {
		t.Fatalf("%+v", p)
	}
	if !res.ReviewPending || len(res.Layers) != 3 || strings.Count(text, "\n") != 5 ||
		!strings.HasSuffix(text, "Merge the stack only on the owner's word: fledge task land --id "+s.id+" --merge\n") {
		t.Fatalf("%q", text)
	}
	b, err := json.Marshal(res)
	if err != nil || !strings.Contains(string(b), `"review_pending":true`) ||
		!strings.Contains(string(b), `"layers":[{"id":"`+s.layers[0]+`","branch":"l1","head":"`+s.heads[0]+`","pr_url":"`+pullURL(0)+`","land_step":"review"}`) {
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
	log := s.gh.log(t)
	if creates := slices.DeleteFunc(slices.Clone(log), func(l string) bool { return !strings.HasPrefix(l, "pr create") }); len(creates) != 3 {
		t.Fatalf("%q", log)
	}
	for i := range 3 {
		if !slices.Contains(log, "api --method=GET repos/{owner}/{repo}/pulls/"+strconv.Itoa(i+1)) {
			t.Fatalf("%q", log)
		}
		if r := tasktest.Load(t, s.repo, s.layers[i]); r.PRURL == nil || *r.PRURL != pullURL(i) || *r.LandStep != stepReview {
			t.Fatalf("%+v", r)
		}
	}
}

// rebase lands the stack to review. Then GitHub rebases l2 onto a new commit
// on l1 with content as its l2 file, and the fake gh answers with that head.
// The local l2 does not change. The l2 verification stores its patch id
// against l1.
func (s stack) rebase(t *testing.T, content string) (rebased string) {
	t.Helper()
	if out := s.land(t, Options{}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	id, err := landing.PatchID(context.Background(), s.repo, s.heads[0], s.heads[1])
	if err != nil || id == "" {
		t.Fatal(err)
	}
	s.edit(t, s.layers[1], func(r *task.Record) { r.VerifiedPatchID = &id })
	git(t, s.repo, "checkout", "-qb", "rebased", s.heads[0])
	base := commitFile(t, s.repo, "x", "x\n")
	rebased = commitFile(t, s.repo, "l2", content)
	git(t, s.repo, "checkout", "-q", "main")
	git(t, s.repo, "push", "-qf", "origin", "rebased:refs/heads/l2")
	s.answer(t, 1, rebased, base)
	s.pushes(t)
	return rebased
}

func TestLandStackAcceptsRebasedPullHead(t *testing.T) {
	s := setupStack(t)
	rebased := s.rebase(t, "l2\n")
	out := s.land(t, Options{})
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if pushes := s.pushes(t); len(pushes) != 0 {
		t.Fatalf("%q", pushes)
	}
	if got := git(t, s.repo, "rev-parse", "l2"); got != s.heads[1] || s.remote(t, "l2") != rebased {
		t.Fatalf("local l2 %s, origin l2 %s", got, s.remote(t, "l2"))
	}
	r := tasktest.Load(t, s.repo, s.layers[1])
	if *r.VerifiedCommit != rebased || r.CarriedFrom == nil || *r.CarriedFrom != s.heads[1] || *r.LandStep != stepReview {
		t.Fatalf("%+v", r)
	}
	if l := out.Result.(Result).Layers[1]; l.Head != rebased || !out.Result.(Result).ReviewPending {
		t.Fatalf("%+v", l)
	}
}

// stackRefusedAt requires out to be a rejected refusal with code at step,
// with the two human lines, and a parent with a null branch and pr_url.
func (s stack) stackRefusedAt(t *testing.T, out cli.Outcome, code, step, name string) {
	t.Helper()
	if out.Error == nil || out.Error.Code != code || out.ExitCode() != 1 {
		t.Fatalf("want %s, got %+v", code, out.Error)
	}
	text := render(t, out)
	first, _, _ := strings.Cut(text, "\n")
	if !strings.HasPrefix(first, "rejected: "+code+": ") || !strings.HasSuffix(first, " (land)") || !strings.Contains(first, name) ||
		!strings.HasSuffix(text, " (land)\nLand step reached: "+step+".\n") {
		t.Fatalf("%q", text)
	}
	if p := s.load(t); p.LandStep == nil || *p.LandStep != step || p.Branch != nil || p.PRURL != nil {
		t.Fatalf("%+v", p)
	}
}

func TestLandStackRefusesChangedPullHead(t *testing.T) {
	s := setupStack(t)
	s.rebase(t, "changed\n")
	s.stackRefusedAt(t, s.land(t, Options{}), "land_head_mismatch", stepPR, "l2")
	if r := tasktest.Load(t, s.repo, s.layers[1]); *r.VerifiedCommit != s.heads[1] || r.CarriedFrom != nil {
		t.Fatalf("%+v", r)
	}
	if pushes := s.pushes(t); len(pushes) != 0 {
		t.Fatalf("%q", pushes)
	}
}

func TestLandStackRefusesPullWithoutHead(t *testing.T) {
	s := setupStack(t)
	if out := s.land(t, Options{}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	body := restPull(1, s.heads[1], s.heads[0])
	s.gh.set(t, apiPull(1), strings.Replace(body, `"head":{"sha":"`+s.heads[1]+`","ref":"l2"},`, "", 1))
	s.refusedAtCheck(t, s.land(t, Options{}), "land_gh_output_invalid", "")
}

// TestLandStackRefusesClosedStoredPull: the l2 layer has a stored pull
// request. GitHub closed it without a merge. The reconcile refuses it at the
// check step and creates no pull request.
func TestLandStackRefusesClosedStoredPull(t *testing.T) {
	s := setupStack(t)
	if out := s.land(t, Options{}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	os.Remove(filepath.Join(s.gh.dir, "log"))
	s.gh.set(t, apiPull(1), strings.Replace(restPull(1, s.heads[1], s.heads[0]), `"state":"open"`, `"state":"closed"`, 1))
	out := s.land(t, Options{})
	if slices.ContainsFunc(s.gh.log(t), func(l string) bool { return strings.HasPrefix(l, "pr create") }) {
		t.Fatalf("%q", s.gh.log(t))
	}
	s.refusedAtCheck(t, out, "land_pr_closed", s.layers[1])
	if r := tasktest.Load(t, s.repo, s.layers[1]); r.PRURL == nil || *r.PRURL != pullURL(1) {
		t.Fatalf("%+v", r)
	}
}

// refusedAtCheck requires out to be a refusal with code that names name at
// the check step of the reconcile. The parent stays at review.
func (s stack) refusedAtCheck(t *testing.T, out cli.Outcome, code, name string) {
	t.Helper()
	if out.Error == nil || out.Error.Code != code || out.ExitCode() != 1 || !strings.Contains(out.Error.Message, name) {
		t.Fatalf("want %s, got %+v", code, out.Error)
	}
	if text := render(t, out); !strings.HasSuffix(text, " (land)\nLand step reached: check.\n") {
		t.Fatalf("%q", text)
	}
	if p := s.load(t); cli.Display(p.LandStep) != stepReview {
		t.Fatalf("%+v", p)
	}
}

func TestLandStackRefusesFailedLayerChecks(t *testing.T) {
	s := setupStack(t)
	s.gh.set(t, "checks-3", checks(s.heads[2], "MERGEABLE", run("COMPLETED", "FAILURE")))
	s.stackRefusedAt(t, s.land(t, Options{}), "land_checks_failed", stepChecks, "l3")
}

// TestLandStackPushesBranchThatOriginLacks: origin lacks a layer branch and
// a stale tracking ref has the verified commit. The landing pushes the branch
// again.
func TestLandStackPushesBranchThatOriginLacks(t *testing.T) {
	s := setupStack(t)
	if out := s.land(t, Options{}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	git(t, s.origin, "update-ref", "-d", "refs/heads/l3")
	if out := s.land(t, Options{}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if got := strings.TrimSpace(git(t, s.origin, "for-each-ref", "--format=%(objectname)", "refs/heads/l3")); got != s.heads[2] {
		t.Fatalf("origin lacks l3: got %q, want %s", got, s.heads[2])
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

// TestLandStackFormsTheStack: no layer pull request is in a stack, so the
// landing reads each one and posts one stack of all of them, bottom first,
// with no base. No gh stack command runs.
func TestLandStackFormsTheStack(t *testing.T) {
	s := setupStack(t)
	out := s.land(t, Options{})
	if out.Error != nil || out.ExitCode() != 0 {
		t.Fatalf("%+v", out.Error)
	}
	log := s.gh.log(t)
	for i := range 3 {
		if !slices.Contains(log, "api --method=GET repos/{owner}/{repo}/pulls/"+strconv.Itoa(i+1)) {
			t.Fatalf("pull request %d not read: %q", i+1, log)
		}
	}
	want := "api --method=POST repos/{owner}/{repo}/stacks --field=pull_requests[]=1 --field=pull_requests[]=2 --field=pull_requests[]=3"
	if posts := s.stackPosts(t); len(posts) != 1 || posts[0] != want {
		t.Fatalf("%q", posts)
	}
	for _, line := range log {
		if strings.HasPrefix(line, "stack ") || strings.Contains(line, "base") && strings.Contains(line, "stacks") {
			t.Fatalf("%q", line)
		}
	}
	res := out.Result.(Result)
	text := render(t, out)
	layers, _, _ := strings.Cut(text, "GitHub stack")
	if res.Stack != 11 || strings.Count(layers, "\n") != 3 || !strings.Contains(text, "\nThe layer pull requests form GitHub stack 11.\n") {
		t.Fatalf("%d %q", res.Stack, text)
	}
	if b, err := json.Marshal(res); err != nil || !strings.Contains(string(b), `"stack":11`) {
		t.Fatalf("%s %v", b, err)
	}
}

// TestLandStackKeepsTheStack: every layer pull request is in stack 11 at its
// layer position, so the landing posts nothing.
func TestLandStackKeepsTheStack(t *testing.T) {
	s := setupStack(t)
	for i := range 3 {
		s.inStack(t, i, stackJSON(11, 3, i+1))
	}
	out := s.land(t, Options{})
	if out.Error != nil || out.ExitCode() != 0 {
		t.Fatalf("%+v", out.Error)
	}
	if posts := s.stackPosts(t); len(posts) != 0 {
		t.Fatalf("%q", posts)
	}
	if res := out.Result.(Result); res.Stack != 11 || !strings.Contains(render(t, out), "GitHub stack 11.") {
		t.Fatalf("%+v", res)
	}
}

// TestLandStackRefusesAnotherStack: a partial stack, another order, another
// size, or two stacks is land_stack_mismatch. The message names each pull
// request with its stack number and position. No stack is posted.
func TestLandStackRefusesAnotherStack(t *testing.T) {
	for name, stacks := range map[string][3]string{
		"partial":  {stackJSON(11, 3, 1), stackJSON(11, 3, 2), "null"},
		"order":    {stackJSON(11, 3, 2), stackJSON(11, 3, 1), stackJSON(11, 3, 3)},
		"size":     {stackJSON(11, 4, 1), stackJSON(11, 4, 2), stackJSON(11, 4, 3)},
		"two":      {stackJSON(11, 3, 1), stackJSON(11, 3, 2), stackJSON(12, 3, 3)},
		"top only": {"null", "null", stackJSON(12, 1, 1)},
	} {
		t.Run(name, func(t *testing.T) {
			s := setupStack(t)
			for i, stack := range stacks {
				if stack != "null" {
					s.inStack(t, i, stack)
				}
			}
			out := s.land(t, Options{})
			s.stackRefusedAt(t, out, "land_stack_mismatch", stepChecks, "")
			for i, stack := range stacks {
				want := fmt.Sprintf("#%d in no stack", i+1)
				if stack != "null" {
					var st landing.Stack
					json.Unmarshal([]byte(stack), &st)
					want = fmt.Sprintf("#%d in stack %d at position %d of %d", i+1, st.Number, st.Position, st.Size)
				}
				if !strings.Contains(out.Error.Message, want) {
					t.Fatalf("no %q in %q", want, out.Error.Message)
				}
			}
			if posts := s.stackPosts(t); len(posts) != 0 {
				t.Fatalf("%q", posts)
			}
			for _, id := range s.layers {
				if r := tasktest.Load(t, s.repo, id); *r.LandStep == stepReview {
					t.Fatalf("%+v", r)
				}
			}
		})
	}
}

// TestLandStackRefusesARefusedPost: GitHub answers the POST with HTTP 422.
func TestLandStackRefusesARefusedPost(t *testing.T) {
	s := setupStack(t)
	s.gh.set(t, apiStacks, `{"message":"Pull requests #1, #2 are already part of a stack","status":"422"}`)
	s.gh.set(t, apiStacks+".exit", "1")
	out := s.land(t, Options{})
	s.stackRefusedAt(t, out, "land_stack_mismatch", stepChecks, "Pull requests #1, #2 are already part of a stack")
	if posts := s.stackPosts(t); len(posts) != 1 {
		t.Fatalf("%q", posts)
	}
}

// The merge-async answers of the brief.
const (
	mergeUUID     = "d3a8ff89-475a-464b-9c87-02dc01da37ec"
	putPending    = `{"status":"pending","details":{"message":"Merge request enqueued.","uuid":"` + mergeUUID + `","merge_method":"merge","merge_action":"default","expected_head_sha":"e0954a8e013aae0ce9290709bc0e00735f077617","bypass_rules":false}}`
	getPending    = `{"status":"pending","details":{"message":"Merge request is in progress."}}`
	getMerged     = `{"status":"merged","details":{"message":"Pull request was merged.","sha":"7a4be7028342660f46d0c0869c8c74c436df9cb1"}}`
	putFailed     = `{"status":"failed","details":{"message":"Pull request head branch was modified."}}`
	putAlready    = `{"status":"merged","details":{"message":"Pull request is already merged.","sha":"7a4be7028342660f46d0c0869c8c74c436df9cb1"}}`
	putNoUUID     = `{"status":"pending","details":{"message":"Merge request enqueued."}}`
	mergeUUIDPath = "pulls/2/merge-async/" + mergeUUID
)

// apiMerge is the fake gh file that answers the merge-async PUT of pull
// request n.
func apiMerge(n int) string {
	return "api-PUT-repos_{owner}_{repo}_pulls_" + strconv.Itoa(n) + "_merge-async"
}

// apiPoll is the fake gh file that answers the poll of uuid on pull request n.
func apiPoll(n int, uuid string) string {
	return "api-GET-repos_{owner}_{repo}_pulls_" + strconv.Itoa(n) + "_merge-async_" + uuid
}

// putLine is the fake gh log line of a merge-async PUT on pull request n at
// head.
func putLine(n int, head string) string {
	return "api --method=PUT repos/{owner}/{repo}/pulls/" + strconv.Itoa(n) + "/merge-async --raw-field=merge_method=merge --raw-field=sha=" + head
}

// review lands the stack to review. The poll runs every millisecond for at
// most 20 milliseconds, and the reads after a merge are one millisecond
// apart. The fake gh log starts empty.
func (s stack) review(t *testing.T) {
	t.Helper()
	if out := s.land(t, Options{}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	interval, timeout, confirm := pollInterval, pollTimeout, confirmInterval
	pollInterval, pollTimeout, confirmInterval = time.Millisecond, 20*time.Millisecond, time.Millisecond
	t.Cleanup(func() { pollInterval, pollTimeout, confirmInterval = interval, timeout, confirm })
	os.Remove(filepath.Join(s.gh.dir, "log"))
}

// mergeCalls returns the merge-async commands of the fake gh log and empties
// the log.
func (s stack) mergeCalls(t *testing.T) []string {
	t.Helper()
	calls := slices.DeleteFunc(s.gh.log(t), func(l string) bool { return !strings.Contains(l, "merge-async") })
	os.Remove(filepath.Join(s.gh.dir, "log"))
	return calls
}

// mergeRefused requires out to be exit 1 with code, a rejected line that
// contains text, and the step line. The parent stays at review with a null
// branch and pr_url.
func (s stack) mergeRefused(t *testing.T, out cli.Outcome, code, step, text string) {
	t.Helper()
	if out.Error == nil || out.Error.Code != code || out.ExitCode() != 1 {
		t.Fatalf("want %s, got %+v", code, out.Error)
	}
	got := render(t, out)
	first, _, _ := strings.Cut(got, "\n")
	if !strings.HasPrefix(first, "rejected: "+code+": ") || !strings.HasSuffix(first, " (land)") || !strings.Contains(first, text) ||
		got != first+"\nLand step reached: "+step+".\n" {
		t.Fatalf("%q", got)
	}
	if p := s.load(t); p.LandStep == nil || *p.LandStep != stepReview || p.Branch != nil || p.PRURL != nil {
		t.Fatalf("%+v", p)
	}
}

// mergeRequest is the stored merge_request of layer i, or "".
func (s stack) mergeRequest(t *testing.T, i int) string {
	t.Helper()
	if r := tasktest.Load(t, s.repo, s.layers[i]); r.MergeRequest != nil {
		return *r.MergeRequest
	}
	return ""
}

// TestLandStackMergeRequestsAndPolls: --merge on l2 sends one PUT on pull
// request 2 at its head, then polls the uuid. Pull request 2 still reads
// open after merged, so the run clears the uuid and refuses
// land_unconfirmed.
func TestLandStackMergeRequestsAndPolls(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.gh.set(t, apiMerge(2), putPending)
	s.gh.set(t, apiPoll(2, mergeUUID), getMerged)
	out := s.land(t, Options{ID: s.layers[1], Merge: true})
	if calls := s.mergeCalls(t); !slices.Equal(calls, []string{putLine(2, s.heads[1]), "api --method=GET repos/{owner}/{repo}/" + mergeUUIDPath}) {
		t.Fatalf("%q", calls)
	}
	s.mergeRefused(t, out, "land_unconfirmed", stepMerge, "fledge task land --id "+s.id)
	for _, i := range []int{0, 1, 2} {
		if got := s.mergeRequest(t, i); got != "" {
			t.Fatalf("layer %d: %q", i, got)
		}
	}
}

// TestLandStackParentMergesTopLayer: --merge on the parent merges the top
// layer. An already-merged PUT answer goes to the record step with no poll.
func TestLandStackParentMergesTopLayer(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.gh.set(t, apiMerge(3), putAlready)
	out := s.land(t, Options{Merge: true})
	if calls := s.mergeCalls(t); !slices.Equal(calls, []string{putLine(3, s.heads[2])}) {
		t.Fatalf("%q", calls)
	}
	s.mergeRefused(t, out, "land_unconfirmed", stepMerge, "fledge task land --id "+s.id)
}

// TestLandStackMergePendingContinues: a pending poll at the timeout is
// land_merge_pending, and the uuid stays stored. The next run polls the
// stored uuid and sends no new PUT.
func TestLandStackMergePendingContinues(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.gh.set(t, apiMerge(2), putPending)
	s.gh.set(t, apiPoll(2, mergeUUID), getPending)
	out := s.land(t, Options{ID: s.layers[1], Merge: true})
	s.mergeRefused(t, out, "land_merge_pending", stepMerge, "")
	if calls := s.mergeCalls(t); len(calls) < 3 || calls[0] != putLine(2, s.heads[1]) || slices.Contains(calls[1:], calls[0]) {
		t.Fatalf("%q", calls)
	}
	if b, err := json.Marshal(tasktest.Load(t, s.repo, s.layers[1])); err != nil || !strings.Contains(string(b), `"merge_request":"`+mergeUUID+`"`) {
		t.Fatalf("%s %v", b, err)
	}
	s.gh.set(t, apiPoll(2, mergeUUID), getMerged)
	out = s.land(t, Options{ID: s.layers[1], Merge: true})
	s.mergeRefused(t, out, "land_unconfirmed", stepMerge, "")
	if calls := s.mergeCalls(t); !slices.Equal(calls, []string{"api --method=GET repos/{owner}/{repo}/" + mergeUUIDPath}) {
		t.Fatalf("%q", calls)
	}
}

// TestLandStackContinuesFromAnyRun: after a timeout on l2, a run on the
// parent continues the request of l2. It skips the new-request guards, also
// without --merge. It sends no PUT and polls only pull request 2. The parent
// guards still run.
func TestLandStackContinuesFromAnyRun(t *testing.T) {
	for name, o := range map[string]func(s stack) Options{
		"parent merge": func(s stack) Options { return Options{Merge: true} },
		"parent":       func(s stack) Options { return Options{} },
		"layer 3":      func(s stack) Options { return Options{ID: s.layers[2], Merge: true} },
	} {
		t.Run(name, func(t *testing.T) {
			s := setupStack(t)
			s.review(t)
			s.gh.set(t, apiMerge(2), putPending)
			s.gh.set(t, apiPoll(2, mergeUUID), getPending)
			s.mergeRefused(t, s.land(t, Options{ID: s.layers[1], Merge: true}), "land_merge_pending", stepMerge, "")
			s.mergeCalls(t)
			s.pushes(t)
			s.gh.set(t, apiPoll(2, mergeUUID), getMerged)
			s.gh.set(t, "checks-2", checks(s.heads[1], "MERGEABLE", run("COMPLETED", "FAILURE")))
			s.edit(t, s.layers[2], func(r *task.Record) { r.LandStep, r.PRURL = nil, nil })
			s.mergeRefused(t, s.land(t, o(s)), "land_unconfirmed", stepMerge, "")
			if calls := s.mergeCalls(t); !slices.Equal(calls, []string{"api --method=GET repos/{owner}/{repo}/" + mergeUUIDPath}) {
				t.Fatalf("%q", calls)
			}
			if pushes := s.pushes(t); len(pushes) != 0 {
				t.Fatalf("%q", pushes)
			}
			s.edit(t, s.id, func(r *task.Record) { r.Status = task.Created })
			if out := s.land(t, o(s)); out.Error == nil || out.Error.Code != "task_invalid_state" || len(s.mergeCalls(t)) != 0 {
				t.Fatalf("%+v", out.Error)
			}
		})
	}
}

// TestLandStackMergeFailed: a failed answer is land_merge_failed with the
// message of GitHub, from the PUT or from a poll. A failed poll clears the
// stored uuid.
func TestLandStackMergeFailed(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.gh.set(t, apiMerge(2), putFailed)
	s.gh.set(t, apiMerge(2)+".exit", "1")
	s.mergeRefused(t, s.land(t, Options{ID: s.layers[1], Merge: true}), "land_merge_failed", stepMerge, "Pull request head branch was modified.")
	if got := s.mergeRequest(t, 1); got != "" {
		t.Fatalf("%q", got)
	}
	s.gh.set(t, apiMerge(2), putPending)
	s.gh.set(t, apiMerge(2)+".exit", "0")
	s.gh.set(t, apiPoll(2, mergeUUID), putFailed)
	s.gh.set(t, apiPoll(2, mergeUUID)+".exit", "1")
	s.mergeRefused(t, s.land(t, Options{ID: s.layers[1], Merge: true}), "land_merge_failed", stepMerge, "Pull request head branch was modified.")
	if got := s.mergeRequest(t, 1); got != "" {
		t.Fatalf("%q", got)
	}
}

// TestLandStackMergeRefusesPendingWithoutUUID: a pending PUT answer with no
// uuid is land_gh_output_invalid. The run sends no poll and stores no
// merge_request on a layer.
func TestLandStackMergeRefusesPendingWithoutUUID(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.gh.set(t, apiMerge(2), putNoUUID)
	out := s.land(t, Options{ID: s.layers[1], Merge: true})
	if calls := s.mergeCalls(t); !slices.Equal(calls, []string{putLine(2, s.heads[1])}) {
		t.Fatalf("%q", calls)
	}
	s.mergeRefused(t, out, "land_gh_output_invalid", stepMerge, "")
	for i, id := range s.layers {
		if r := tasktest.Load(t, s.repo, id); r.MergeRequest != nil {
			t.Fatalf("layer %d: %q", i, *r.MergeRequest)
		}
	}
}

// TestLandStackMergesUnlandedSuffix: l1 is landed from an earlier merge. The
// merge of l2 is not checked against l1 again, and l1 keeps its record.
func TestLandStackMergesUnlandedSuffix(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.edit(t, s.layers[0], func(r *task.Record) { r.Status, r.MergeCommit = task.Landed, tasktest.Ptr(mergeHash) })
	s.gh.set(t, "checks-1", checks(s.heads[0], "MERGEABLE", run("COMPLETED", "FAILURE")))
	s.gh.set(t, apiMerge(2), putPending)
	s.gh.set(t, apiPoll(2, mergeUUID), getPending)
	s.mergeRefused(t, s.land(t, Options{ID: s.layers[1], Merge: true}), "land_merge_pending", stepMerge, "")
	if calls := s.mergeCalls(t); len(calls) == 0 || calls[0] != putLine(2, s.heads[1]) {
		t.Fatalf("%q", calls)
	}
	if r := tasktest.Load(t, s.repo, s.layers[0]); r.Status != task.Landed || r.MergeCommit == nil || *r.MergeCommit != mergeHash {
		t.Fatalf("%+v", r)
	}
}

// TestLandStackMergeChecksEachSuffixLayer: each layer of the suffix needs
// the review step, a pull request, and green checks at its current head
// before the PUT.
func TestLandStackMergeChecksEachSuffixLayer(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.edit(t, s.layers[1], func(r *task.Record) { r.PRURL = nil })
	s.mergeRefused(t, s.land(t, Options{ID: s.layers[1], Merge: true}), "land_stack_not_open", stepCheck, s.layers[1])
	s = setupStack(t)
	s.review(t)
	s.edit(t, s.layers[0], func(r *task.Record) { r.LandStep = tasktest.Ptr(stepChecks) })
	s.mergeRefused(t, s.land(t, Options{ID: s.layers[1], Merge: true}), "land_stack_not_open", stepCheck, s.layers[0])
	s = setupStack(t)
	s.review(t)
	s.gh.set(t, "checks-1", checks(s.heads[0], "MERGEABLE", run("COMPLETED", "FAILURE")))
	s.mergeRefused(t, s.land(t, Options{ID: s.layers[1], Merge: true}), "land_checks_failed", stepCheck, "l1")
	if calls := s.mergeCalls(t); len(calls) != 0 {
		t.Fatalf("%q", calls)
	}
}

// TestLandStackMergeSendsTheCurrentHead: GitHub rebased l2 after the
// review stop. The PUT sends the head of the REST pull request, not the
// verified commit that the review stored.
func TestLandStackMergeSendsTheCurrentHead(t *testing.T) {
	s := setupStack(t)
	rebased := s.rebase(t, "l2\n")
	s.gh.set(t, apiMerge(2), putAlready)
	s.mergeRefused(t, s.land(t, Options{ID: s.layers[1], Merge: true}), "land_unconfirmed", stepMerge, "")
	if calls := s.mergeCalls(t); !slices.Equal(calls, []string{putLine(2, rebased)}) {
		t.Fatalf("%q", calls)
	}
}

// TestLandStackMergeRequestGone tests an assumption, not a fact: GitHub
// answers HTTP 404 to a poll of an expired uuid. The probes did not observe
// that answer. The run clears the uuid, reads pull request 2, and sends a new
// PUT for the open pull request. No other test uses an HTTP 404 answer.
func TestLandStackMergeRequestGone(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	old := "00000000-0000-0000-0000-000000000000"
	s.edit(t, s.layers[1], func(r *task.Record) { r.MergeRequest = &old })
	s.gh.set(t, apiPoll(2, old), `{"message":"Not Found","documentation_url":"https://docs.github.com/rest","status":"404"}`)
	s.gh.set(t, apiPoll(2, old)+".exit", "1")
	s.gh.set(t, apiMerge(2), putPending)
	s.gh.set(t, apiPoll(2, mergeUUID), getPending)
	s.mergeRefused(t, s.land(t, Options{ID: s.layers[1], Merge: true}), "land_merge_pending", stepMerge, "")
	calls := s.mergeCalls(t)
	puts := slices.DeleteFunc(slices.Clone(calls), func(l string) bool { return !strings.Contains(l, "PUT") })
	if !slices.Equal(puts, []string{putLine(2, s.heads[1])}) || !strings.HasSuffix(calls[0], "/"+old) {
		t.Fatalf("%q", calls)
	}
	if got := s.mergeRequest(t, 1); got != mergeUUID {
		t.Fatalf("%q", got)
	}
}

// The merge commits of the reconcile cases.
const (
	mergeSHA  = "7a4be7028342660f46d0c0869c8c74c436df9cb1"
	buttonSHA = "ea8b531b9f3403aa20afc22c7ef4549b3b3e7b6c"
)

// mergedPull is the REST pull request of layer i, merged at head on base as
// merge commit sha.
func mergedPull(i int, head, base, sha string) string {
	return strings.Replace(restPull(i, head, base), `"state":"open","merged":false,"merge_commit_sha":null`, `"state":"closed","merged":true,"merge_commit_sha":"`+sha+`"`, 1)
}

// base is the base commit of layer i.
func (s stack) base(t *testing.T, i int) string {
	t.Helper()
	if i == 0 {
		return git(t, s.repo, "rev-parse", "main")
	}
	return s.heads[i-1]
}

// merge makes the REST read of the pull request of layer i answer merged at
// its verified head as merge commit sha. With suffix .after-put, only the
// reads after a PUT answer so.
func (s stack) merge(t *testing.T, i int, sha, suffix string) {
	t.Helper()
	s.gh.set(t, apiPull(i)+suffix, mergedPull(i, s.heads[i], s.base(t, i), sha))
}

// changed commits a change of the file of layer i on its base and returns
// the new head. The verification of layer i stores its patch id against its
// base, so only the changed line stops a carry.
func (s stack) changed(t *testing.T, i int) string {
	t.Helper()
	s.patchID(t, i)
	git(t, s.repo, "checkout", "-qb", "changed", s.base(t, i))
	head := commitFile(t, s.repo, "l"+strconv.Itoa(i+1), "changed\n")
	git(t, s.repo, "checkout", "-q", "main")
	return head
}

// patchID stores the patch id of layer i against its base on its record.
func (s stack) patchID(t *testing.T, i int) {
	t.Helper()
	id, err := landing.PatchID(context.Background(), s.repo, s.base(t, i), s.heads[i])
	if err != nil || id == "" {
		t.Fatal(err)
	}
	s.edit(t, s.layers[i], func(r *task.Record) { r.VerifiedPatchID = &id })
}

// rebaseLow rebases l1 onto a new commit on main with the same change and
// makes its REST read answer merged at that head as mergeSHA. It returns the
// new head.
func (s stack) rebaseLow(t *testing.T) string {
	t.Helper()
	s.patchID(t, 0)
	git(t, s.repo, "checkout", "-qb", "rebased-l1", "main")
	base := commitFile(t, s.repo, "x", "x\n")
	head := commitFile(t, s.repo, "l1", "l1\n")
	git(t, s.repo, "checkout", "-q", "main")
	s.gh.set(t, apiPull(0), mergedPull(0, head, base, mergeSHA))
	return head
}

// snapshot is the JSON of the parent and of each layer record.
func (s stack) snapshot(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, id := range append([]string{s.id}, s.layers...) {
		b, err := json.Marshal(tasktest.Load(t, s.repo, id))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	return out
}

// unchanged requires the records to equal before.
func (s stack) unchanged(t *testing.T, before []string) {
	t.Helper()
	if after := s.snapshot(t); !slices.Equal(after, before) {
		t.Fatalf("records changed:\n%q\n%q", before, after)
	}
}

// apiCalls returns the gh api commands of the fake gh log and empties the
// log.
func (s stack) apiCalls(t *testing.T) []string {
	t.Helper()
	calls := slices.DeleteFunc(s.gh.log(t), func(l string) bool { return !strings.HasPrefix(l, "api ") })
	os.Remove(filepath.Join(s.gh.dir, "log"))
	return calls
}

// pullLine is the fake gh log line of the REST read of pull request n.
func pullLine(n int) string { return "api --method=GET repos/{owner}/{repo}/pulls/" + strconv.Itoa(n) }

// threeReads are the reads of the three layer pull requests.
var threeReads = []string{pullLine(1), pullLine(2), pullLine(3)}

// landedAt requires layer i to be landed by the reconcile at sha.
func (s stack) landedAt(t *testing.T, i int, sha string) task.Record {
	t.Helper()
	r := tasktest.Load(t, s.repo, s.layers[i])
	if r.Status != task.Landed || cli.Display(r.MergeCommit) != sha || cli.Display(r.TargetBranch) != "main" || cli.Display(r.LandStep) != stepConfirm ||
		r.LandedAt == nil || cli.Display(r.LandedBy) != s.boss.ID || r.MergeRequest != nil {
		t.Fatalf("layer %d: %+v", i, r)
	}
	return r
}

// atReview requires task id to be verified at the review step.
func (s stack) atReview(t *testing.T, id string) {
	t.Helper()
	if r := tasktest.Load(t, s.repo, id); r.Status != task.Verified || cli.Display(r.LandStep) != stepReview || r.MergeRequest != nil {
		t.Fatalf("%+v", r)
	}
}

// ok requires out to exit 0 with human output that contains text.
func ok(t *testing.T, out cli.Outcome, text string) {
	t.Helper()
	if out.Error != nil || out.ExitCode() != 0 {
		t.Fatalf("%+v", out.Error)
	}
	if got := render(t, out); !strings.Contains(got, text) {
		t.Fatalf("%q", got)
	}
}

// refusedNaming requires out to exit 1 with code and a message that names
// id.
func refusedNaming(t *testing.T, out cli.Outcome, code, id string) {
	t.Helper()
	if out.Error == nil || out.Error.Code != code || out.ExitCode() != 1 || !strings.Contains(out.Error.Message, id) {
		t.Fatalf("want %s naming %s, got %+v", code, id, out.Error)
	}
}

// TestReconcileReadsNoLayerWithoutPull: a stack at the push step has no
// pr_url, so the reconcile reads no pull request before the pull request
// steps.
func TestReconcileReadsNoLayerWithoutPull(t *testing.T) {
	s := setupStack(t)
	for _, id := range append([]string{s.id}, s.layers...) {
		s.edit(t, id, func(r *task.Record) { r.LandStep = tasktest.Ptr(stepPush) })
	}
	ok(t, s.land(t, Options{}), "")
	log := s.gh.log(t)
	first := slices.IndexFunc(log, func(l string) bool { return strings.HasPrefix(l, "pr ") })
	if first < 0 || slices.ContainsFunc(log[:first], func(l string) bool { return strings.Contains(l, "/pulls/") }) {
		t.Fatalf("%q", log)
	}
}

// TestReconcileRecordsAMergeOfLayersOneToTwo: the merge of l2 records l1 and
// l2 with the one merge commit and clears the uuid. Then a merge of the
// parent records l3 and the parent.
func TestReconcileRecordsAMergeOfLayersOneToTwo(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.gh.set(t, apiMerge(2), putPending)
	s.gh.set(t, apiPoll(2, mergeUUID), getMerged)
	s.merge(t, 0, mergeSHA, ".after-put")
	s.merge(t, 1, mergeSHA, ".after-put")
	out := s.land(t, Options{ID: s.layers[1], Merge: true})
	ok(t, out, "Layers 1 to 2 landed")
	want := "Layer " + s.layers[0] + ": landed at " + mergeSHA + ".\nLayer " + s.layers[1] + ": landed at " + mergeSHA + ".\nLayer " + s.layers[2] +
		": land step review.\nLayers 1 to 2 landed.\n"
	if got := render(t, out); got != want {
		t.Fatalf("%q", got)
	}
	l1, l2 := s.landedAt(t, 0, mergeSHA), s.landedAt(t, 1, mergeSHA)
	s.atReview(t, s.layers[2])
	s.atReview(t, s.id)
	s.gh.set(t, apiPull(2), mergedPull(2, s.heads[2], s.heads[1], mergeSHA))
	ok(t, s.land(t, Options{Merge: true}), "Stack landed")
	s.landedAt(t, 2, mergeSHA)
	if p := s.load(t); p.Status != task.Landed || cli.Display(p.MergeCommit) != mergeSHA || cli.Display(p.LandStep) != stepConfirm {
		t.Fatalf("%+v", p)
	}
	for i, r := range []task.Record{l1, l2} {
		if got := tasktest.Load(t, s.repo, s.layers[i]); !slices.Equal(s.snapshotOf(t, got), s.snapshotOf(t, r)) {
			t.Fatalf("layer %d changed: %+v", i, got)
		}
	}
}

// snapshotOf is the JSON of r.
func (s stack) snapshotOf(t *testing.T, r task.Record) []string {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return []string{string(b)}
}

// TestReconcileRecordsAnAlreadyMergedPut: the PUT answers that pull request
// 2 is already merged. The run reads every pull request again, records l1
// and l2, and polls nothing.
func TestReconcileRecordsAnAlreadyMergedPut(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.gh.set(t, apiMerge(2), putAlready)
	s.merge(t, 0, mergeSHA, ".after-put")
	s.merge(t, 1, mergeSHA, ".after-put")
	ok(t, s.land(t, Options{ID: s.layers[1], Merge: true}), "Layers 1 to 2 landed")
	want := append(append(slices.Clone(threeReads), putLine(2, s.heads[1])), threeReads...)
	if calls := s.apiCalls(t); !slices.Equal(calls, want) {
		t.Fatalf("%q", calls)
	}
	s.landedAt(t, 0, mergeSHA)
	s.landedAt(t, 1, mergeSHA)
	s.atReview(t, s.layers[2])
	s.atReview(t, s.id)
}

// TestReconcileRefusesALaggingRead: the poll answers merged, but pull
// request 2 still reads open after 3 more reads. The run clears the uuid and
// refuses land_unconfirmed. The next run records the merge with no PUT and
// no poll.
func TestReconcileRefusesALaggingRead(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	before := s.snapshot(t)
	s.gh.set(t, apiMerge(2), putPending)
	s.gh.set(t, apiPoll(2, mergeUUID), getMerged)
	s.mergeRefused(t, s.land(t, Options{ID: s.layers[1], Merge: true}), "land_unconfirmed", stepMerge, "fledge task land --id "+s.id)
	calls := s.apiCalls(t)
	poll := slices.Index(calls, "api --method=GET repos/{owner}/{repo}/"+mergeUUIDPath)
	if poll < 0 || !slices.Equal(calls[poll+1:], []string{pullLine(1), pullLine(2), pullLine(2), pullLine(2), pullLine(2), pullLine(3)}) {
		t.Fatalf("%q", calls)
	}
	s.unchanged(t, before)
	s.merge(t, 0, mergeSHA, "")
	s.merge(t, 1, mergeSHA, "")
	ok(t, s.land(t, Options{}), "Layers 1 to 2 landed")
	if calls := s.apiCalls(t); !slices.Equal(calls, threeReads) {
		t.Fatalf("%q", calls)
	}
	s.landedAt(t, 0, mergeSHA)
	s.landedAt(t, 1, mergeSHA)
}

// TestReconcileRefusesAMergeAtAnotherHead: l2 was merged at a head with a
// changed line. No record changes.
func TestReconcileRefusesAMergeAtAnotherHead(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	head := s.changed(t, 1)
	s.gh.set(t, apiPull(1), mergedPull(1, head, s.heads[0], mergeSHA))
	before := s.snapshot(t)
	refusedNaming(t, s.land(t, Options{}), "land_pr_merged_other_head", s.layers[1])
	s.unchanged(t, before)
}

// TestReconcileCarriesARebasedLayer: GitHub rebased l1 to a head with an
// equal patch id before the merge. The landed write of l1 also stores the
// carry.
func TestReconcileCarriesARebasedLayer(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	rebased := s.rebaseLow(t)
	s.merge(t, 1, mergeSHA, "")
	ok(t, s.land(t, Options{}), "Layers 1 to 2 landed")
	if r := s.landedAt(t, 0, mergeSHA); cli.Display(r.VerifiedCommit) != rebased || cli.Display(r.CarriedFrom) != s.heads[0] {
		t.Fatalf("%+v", r)
	}
	if r := s.landedAt(t, 1, mergeSHA); cli.Display(r.VerifiedCommit) != s.heads[1] || r.CarriedFrom != nil {
		t.Fatalf("%+v", r)
	}
}

// TestReconcileRefusesACarryBesideAChangedLayer: l1 has a carry, and l2 was
// merged at a head with a changed line. Neither layer changes, also not the
// carry of l1.
func TestReconcileRefusesACarryBesideAChangedLayer(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.rebaseLow(t)
	head := s.changed(t, 1)
	s.gh.set(t, apiPull(1), mergedPull(1, head, s.heads[0], mergeSHA))
	before := s.snapshot(t)
	refusedNaming(t, s.land(t, Options{}), "land_pr_merged_other_head", s.layers[1])
	s.unchanged(t, before)
	if r := tasktest.Load(t, s.repo, s.layers[0]); cli.Display(r.VerifiedCommit) != s.heads[0] || r.CarriedFrom != nil {
		t.Fatalf("%+v", r)
	}
}

// TestReconcileRefusesAClosedLayer: GitHub closed the pull request of l2
// without a merge. No record changes.
func TestReconcileRefusesAClosedLayer(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.gh.set(t, apiPull(1), strings.Replace(restPull(1, s.heads[1], s.heads[0]), `"state":"open"`, `"state":"closed"`, 1))
	before := s.snapshot(t)
	refusedNaming(t, s.land(t, Options{}), "land_pr_closed", s.layers[1])
	s.unchanged(t, before)
}

// hook makes beforeRecord run fn for this test.
func hook(t *testing.T, fn func()) {
	t.Helper()
	old := beforeRecord
	beforeRecord = fn
	t.Cleanup(func() { beforeRecord = old })
}

// failPut makes the record write of task id fail for this test, until the
// returned function restores it.
func failPut(t *testing.T, id string) (restore func()) {
	t.Helper()
	old := putRecord
	putRecord = func(tx *state.Tx, r task.Record) error {
		if r.ID == id {
			return errors.New("disk full")
		}
		return old(tx, r)
	}
	restore = func() { putRecord = old }
	t.Cleanup(restore)
	return restore
}

// TestReconcileRefusesAChangedLayer: l1 is cancelled after the GitHub read.
// The reconcile refuses task_state_changed and writes nothing.
func TestReconcileRefusesAChangedLayer(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.merge(t, 0, mergeSHA, "")
	s.merge(t, 1, mergeSHA, "")
	var before []string
	hook(t, func() {
		s.edit(t, s.layers[0], func(r *task.Record) { r.Status = task.Cancelled })
		before = s.snapshot(t)
	})
	refusedNaming(t, s.land(t, Options{}), "task_state_changed", s.layers[0])
	s.unchanged(t, before)
}

// TestReconcileKeepsACancelledParent: the parent is cancelled after the
// GitHub read. The layers are recorded, the parent stays cancelled, and the
// parent guard refuses.
func TestReconcileKeepsACancelledParent(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	for i := range s.layers {
		s.merge(t, i, mergeSHA, "")
	}
	hook(t, func() { s.edit(t, s.id, func(r *task.Record) { r.Status = task.Cancelled }) })
	refusedNaming(t, s.land(t, Options{}), "task_invalid_state", s.id)
	for i := range s.layers {
		s.landedAt(t, i, mergeSHA)
	}
	if p := s.load(t); p.Status != task.Cancelled {
		t.Fatalf("%+v", p)
	}
}

// TestReconcileFinishesAFailedLayerWrite: the write of l2 fails after the
// write of l1. The next run writes l2.
func TestReconcileFinishesAFailedLayerWrite(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.merge(t, 0, mergeSHA, "")
	s.merge(t, 1, mergeSHA, "")
	restore := failPut(t, s.layers[1])
	out := s.land(t, Options{ID: s.layers[1]})
	refusedNaming(t, out, "land_record_incomplete", s.layers[0])
	if !strings.Contains(out.Error.Message, "fledge task land --id "+s.id) {
		t.Fatalf("%+v", out.Error)
	}
	l1 := s.landedAt(t, 0, mergeSHA)
	s.atReview(t, s.layers[1])
	restore()
	ok(t, s.land(t, Options{ID: s.layers[1]}), "Layers 1 to 2 landed")
	s.landedAt(t, 1, mergeSHA)
	if got := tasktest.Load(t, s.repo, s.layers[0]); !slices.Equal(s.snapshotOf(t, got), s.snapshotOf(t, l1)) {
		t.Fatalf("%+v", got)
	}
}

// TestReconcileFinishesAFailedParentWrite: the parent write fails after the
// three layer writes. The next run lands the parent with no PUT and no poll.
func TestReconcileFinishesAFailedParentWrite(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	for i := range s.layers {
		s.merge(t, i, mergeSHA, "")
	}
	s.edit(t, s.layers[2], func(r *task.Record) { r.MergeRequest = tasktest.Ptr(mergeUUID) })
	restore := failPut(t, s.id)
	refusedNaming(t, s.land(t, Options{Merge: true}), "land_record_incomplete", s.layers[2])
	for i := range s.layers {
		s.landedAt(t, i, mergeSHA)
	}
	s.atReview(t, s.id)
	restore()
	s.apiCalls(t)
	ok(t, s.land(t, Options{}), "Stack landed")
	if calls := s.apiCalls(t); !slices.Equal(calls, threeReads) {
		t.Fatalf("%q", calls)
	}
	if p := s.load(t); p.Status != task.Landed || cli.Display(p.MergeCommit) != mergeSHA || cli.Display(p.TargetBranch) != "main" {
		t.Fatalf("%+v", p)
	}
}

// TestReconcileClearsTheUUIDOfALandedStack: the stack is landed, and l3
// still holds a uuid. The run clears it and changes nothing else.
func TestReconcileClearsTheUUIDOfALandedStack(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	for i, id := range append(slices.Clone(s.layers), s.id) {
		s.edit(t, id, func(r *task.Record) { r.Status, r.MergeCommit = task.Landed, tasktest.Ptr(mergeSHA) })
		if i < len(s.layers) {
			s.merge(t, i, mergeSHA, "")
		}
	}
	before := s.snapshot(t)
	s.edit(t, s.layers[2], func(r *task.Record) { r.MergeRequest = tasktest.Ptr(mergeUUID) })
	ok(t, s.land(t, Options{}), "Stack "+s.id+" is already landed at "+mergeSHA+".")
	if calls := s.apiCalls(t); !slices.Equal(calls, threeReads) {
		t.Fatalf("%q", calls)
	}
	s.unchanged(t, before)
}

// TestReconcileRecordsAButtonMerge: l1 and l2 were merged with the GitHub
// button, so no layer holds a uuid. The run records both, and two more runs
// change nothing.
func TestReconcileRecordsAButtonMerge(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.pushes(t)
	s.merge(t, 0, buttonSHA, "")
	s.merge(t, 1, buttonSHA, "")
	ok(t, s.land(t, Options{}), "Layers 1 to 2 landed")
	if log := s.gh.log(t); !slices.Equal(log, threeReads) {
		t.Fatalf("%q", log)
	}
	if pushes := s.pushes(t); len(pushes) != 0 {
		t.Fatalf("%q", pushes)
	}
	s.landedAt(t, 0, buttonSHA)
	s.landedAt(t, 1, buttonSHA)
	s.atReview(t, s.layers[2])
	s.atReview(t, s.id)
	for range 2 {
		before := s.snapshot(t)
		os.Remove(filepath.Join(s.gh.dir, "log"))
		ok(t, s.land(t, Options{}), "Layers 1 to 2 landed")
		if log := s.gh.log(t); !slices.Equal(log, threeReads) {
			t.Fatalf("%q", log)
		}
		s.unchanged(t, before)
	}
}

// TestReconcileRefusesALandedParentWithAnOpenLayer: the parent is landed, but
// l1 is not.
func TestReconcileRefusesALandedParentWithAnOpenLayer(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.edit(t, s.id, func(r *task.Record) { r.Status = task.Landed })
	refusedNaming(t, s.land(t, Options{}), "task_invalid_state", s.layers[0])
}

// TestReconcileMergesNoLandedSuffix: l1 and l2 are landed, so --merge of l2
// sends no PUT and prints the layer lines.
func TestReconcileMergesNoLandedSuffix(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.merge(t, 0, mergeSHA, "")
	s.merge(t, 1, mergeSHA, "")
	ok(t, s.land(t, Options{ID: s.layers[1], Merge: true}), "Layers 1 to 2 landed")
	if calls := s.apiCalls(t); !slices.Equal(calls, threeReads) {
		t.Fatalf("%q", calls)
	}
}

// TestReconcileRefusesAChangedUUID: l3 holds another uuid after the GitHub
// read. The reconcile refuses task_state_changed and writes nothing.
func TestReconcileRefusesAChangedUUID(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	for i := range s.layers {
		s.merge(t, i, mergeSHA, "")
	}
	s.edit(t, s.layers[2], func(r *task.Record) { r.MergeRequest = tasktest.Ptr(mergeUUID) })
	var before []string
	hook(t, func() {
		s.edit(t, s.layers[2], func(r *task.Record) { r.MergeRequest = tasktest.Ptr("00000000-0000-0000-0000-000000000000") })
		before = s.snapshot(t)
	})
	refusedNaming(t, s.land(t, Options{}), "task_state_changed", s.layers[2])
	s.unchanged(t, before)
}

// TestLandStackReadsEachPullOneTime: a run on three open layers at the review
// step reads each layer pull request one time.
func TestLandStackReadsEachPullOneTime(t *testing.T) {
	s := setupStack(t)
	for i := range 3 {
		s.inStack(t, i, stackJSON(11, 3, i+1))
	}
	s.review(t)
	ok(t, s.land(t, Options{}), "")
	reads := slices.DeleteFunc(s.apiCalls(t), func(l string) bool { return !slices.Contains(threeReads, l) })
	if !slices.Equal(reads, threeReads) {
		t.Fatalf("%d reads: %q", len(reads), reads)
	}
}

// TestLandStackWithALandedHighLayerPushesNothing: l2 is merged and l1 is
// open. A run without --merge records l2, prints the layer lines, and pushes
// nothing.
func TestLandStackWithALandedHighLayerPushesNothing(t *testing.T) {
	s := setupStack(t)
	s.review(t)
	s.pushes(t)
	s.merge(t, 1, mergeSHA, "")
	out := s.land(t, Options{})
	ok(t, out, "Layer "+s.layers[0]+": land step review.\nLayer "+s.layers[1]+": landed at "+mergeSHA+".\nLayer "+s.layers[2]+": land step review.\n")
	if pushes := s.pushes(t); len(pushes) != 0 {
		t.Fatalf("%q", pushes)
	}
	s.landedAt(t, 1, mergeSHA)
	s.atReview(t, s.layers[0])
}

// originMerge makes a merge commit of head on main in origin outside
// Fledge, as the GitHub button does, and returns it. The local tracking ref
// does not change.
func (s stack) originMerge(t *testing.T, head string) string {
	t.Helper()
	m := git(t, s.origin, "commit-tree", head+"^{tree}", "-p", "main", "-p", head, "-m", "merge "+head)
	git(t, s.origin, "update-ref", "refs/heads/main", m)
	return m
}

// tracked is the commit of refs/remotes/origin/main in the repository.
func (s stack) tracked(t *testing.T) string {
	t.Helper()
	return git(t, s.repo, "rev-parse", "refs/remotes/origin/main")
}

// rebaseOnOrigin makes the GitHub rebase of layer i on origin: a commit on
// onto that writes content to the file of layer i, pushed to the layer
// branch from another clone. The repository does not get the commit. It
// returns the new head.
func (s stack) rebaseOnOrigin(t *testing.T, i int, onto, content string) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "clone")
	git(t, s.repo, "clone", "-q", s.origin, clone)
	git(t, clone, "checkout", "-q", "--detach", onto)
	name := "l" + strconv.Itoa(i+1)
	head := commitFile(t, clone, name, content)
	git(t, clone, "push", "-qf", "origin", "HEAD:refs/heads/"+name)
	return head
}

// buttonMerge makes merge commit M1 of l2 on origin main outside Fledge and
// makes the pull requests of l1 and l2 answer merged at their verified heads
// as M1. l3 stays open. The stack is at the review step. It returns M1.
func (s stack) buttonMerge(t *testing.T) string {
	t.Helper()
	s.review(t)
	m1 := s.originMerge(t, s.heads[1])
	s.merge(t, 0, m1, "")
	s.merge(t, 1, m1, "")
	return m1
}

// TestLandStackRecordsButtonMergesAndFetches: GitHub merged l1 and l2 as M1
// with the button. The run records them, prints the record line, and gets
// M1 into the tracking ref. Then GitHub rebased l3 onto M1 and merged it as
// M2. The next run records l3 and the parent, and gets M2.
func TestLandStackRecordsButtonMergesAndFetches(t *testing.T) {
	s := setupStack(t)
	m1 := s.buttonMerge(t)
	out := s.land(t, Options{})
	want := "Recorded a GitHub merge of layers 1 to 2.\nLayer " + s.layers[0] + ": landed at " + m1 + ".\nLayer " + s.layers[1] + ": landed at " + m1 +
		".\nLayer " + s.layers[2] + ": land step review.\nLayers 1 to 2 landed.\n"
	ok(t, out, want)
	if got := render(t, out); got != want {
		t.Fatalf("%q", got)
	}
	s.landedAt(t, 0, m1)
	s.landedAt(t, 1, m1)
	if got := s.tracked(t); got != m1 {
		t.Fatalf("origin/main is %s, not M1 %s", got, m1)
	}
	s.patchID(t, 2)
	rebased := s.rebaseOnOrigin(t, 2, m1, "l3\n")
	m2 := s.originMerge(t, rebased)
	s.gh.set(t, apiPull(2), mergedPull(2, rebased, m1, m2))
	refs := s.refs(t)
	ok(t, s.land(t, Options{}), "Recorded a GitHub merge of layer 3.\n")
	if got := s.refs(t); !slices.Equal(got, refs) {
		t.Fatalf("refs changed:\n%q\n%q", refs, got)
	}
	if r := s.landedAt(t, 2, m2); cli.Display(r.VerifiedCommit) != rebased || cli.Display(r.CarriedFrom) != s.heads[2] {
		t.Fatalf("%+v", r)
	}
	if p := s.load(t); p.Status != task.Landed || cli.Display(p.MergeCommit) != m2 {
		t.Fatalf("%+v", p)
	}
	if got := s.tracked(t); got != m2 {
		t.Fatalf("origin/main is %s, not M2 %s", got, m2)
	}
}

// TestLandStackRefusesAChangedButtonMerge: GitHub merged l3 as M2 at a
// rebased head with a changed line. No record changes, and the tracking ref
// stays at M1.
func TestLandStackRefusesAChangedButtonMerge(t *testing.T) {
	s := setupStack(t)
	m1 := s.buttonMerge(t)
	ok(t, s.land(t, Options{}), "")
	s.patchID(t, 2)
	rebased := s.rebaseOnOrigin(t, 2, m1, "changed\n")
	m2 := s.originMerge(t, rebased)
	s.gh.set(t, apiPull(2), mergedPull(2, rebased, m1, m2))
	before := s.snapshot(t)
	refusedNaming(t, s.land(t, Options{}), "land_pr_merged_other_head", s.layers[2])
	s.unchanged(t, before)
	if got := s.tracked(t); got != m1 {
		t.Fatalf("origin/main is %s, not M1 %s", got, m1)
	}
}

// TestLandStackRefusesAClosedButtonLayer: l2 is closed without a merge. No
// record changes, and the tracking ref does not change.
func TestLandStackRefusesAClosedButtonLayer(t *testing.T) {
	s := setupStack(t)
	s.buttonMerge(t)
	s.gh.set(t, apiPull(1), strings.Replace(restPull(1, s.heads[1], s.heads[0]), `"state":"open"`, `"state":"closed"`, 1))
	before, tracked := s.snapshot(t), s.tracked(t)
	refusedNaming(t, s.land(t, Options{}), "land_pr_closed", s.layers[1])
	s.unchanged(t, before)
	if got := s.tracked(t); got != tracked {
		t.Fatalf("origin/main moved from %s to %s", tracked, got)
	}
}

// TestLandStackWithNoGitHubChangeFetchesNothing: after the record of M1,
// origin main moves to a new commit. A run with no change on GitHub writes
// nothing and runs no git fetch, so the tracking ref stays at M1.
func TestLandStackWithNoGitHubChangeFetchesNothing(t *testing.T) {
	s := setupStack(t)
	m1 := s.buttonMerge(t)
	ok(t, s.land(t, Options{}), "")
	moved := git(t, s.origin, "commit-tree", m1+"^{tree}", "-p", m1, "-m", "moved")
	git(t, s.origin, "update-ref", "refs/heads/main", moved)
	before := s.snapshot(t)
	out := s.land(t, Options{})
	ok(t, out, "Layers 1 to 2 landed.")
	if strings.Contains(render(t, out), "Recorded") {
		t.Fatalf("%q", render(t, out))
	}
	s.unchanged(t, before)
	if got := s.tracked(t); got != m1 {
		t.Fatalf("origin/main is %s, not M1 %s", got, m1)
	}
}

// TestLandStackWarnsOnAFailedFetch: origin does not exist. The run records
// l1 and l2, exits 0, and warns about the failed git fetch.
func TestLandStackWarnsOnAFailedFetch(t *testing.T) {
	s := setupStack(t)
	m1 := s.buttonMerge(t)
	git(t, s.repo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
	out := s.land(t, Options{})
	ok(t, out, "Recorded a GitHub merge of layers 1 to 2.")
	s.landedAt(t, 0, m1)
	s.landedAt(t, 1, m1)
	if w := out.Result.(Result).Warnings; len(w) != 1 || !strings.HasPrefix(w[0], "git fetch origin main failed: ") {
		t.Fatalf("%q", w)
	}
}

// TestLandStackNamesNonPrefixLayers: the landed layers do not form a prefix,
// so the stack line names each landed layer.
func TestLandStackNamesNonPrefixLayers(t *testing.T) {
	for _, c := range []struct {
		merged []int
		want   string
	}{{[]int{1}, "\nLayer 2 landed.\n"}, {[]int{1, 2}, "\nLayers 2 and 3 landed.\n"}} {
		s := setupStack(t)
		s.review(t)
		for _, i := range c.merged {
			s.merge(t, i, mergeSHA, "")
		}
		ok(t, s.land(t, Options{}), c.want)
	}
}

// refs are the refs of the repository other than refs/remotes/origin/main,
// each with its commit.
func (s stack) refs(t *testing.T) []string {
	t.Helper()
	return slices.DeleteFunc(strings.Split(git(t, s.repo, "for-each-ref", "--format=%(refname) %(objectname)"), "\n"),
		func(l string) bool { return strings.HasPrefix(l, "refs/remotes/origin/main ") })
}

// TestLandStackNamesAFailedPatchFetch: GitHub rebased l3 onto M1 and merged
// it, but origin is missing, so the git fetch of the rebased head fails. The
// run refuses land_pr_merged_other_head with the fetch error, and no record
// changes.
func TestLandStackNamesAFailedPatchFetch(t *testing.T) {
	s := setupStack(t)
	m1 := s.buttonMerge(t)
	ok(t, s.land(t, Options{}), "")
	s.patchID(t, 2)
	rebased := s.rebaseOnOrigin(t, 2, m1, "l3\n")
	s.gh.set(t, apiPull(2), mergedPull(2, rebased, m1, s.originMerge(t, rebased)))
	git(t, s.repo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
	before := s.snapshot(t)
	out := s.land(t, Options{})
	refusedNaming(t, out, "land_pr_merged_other_head", s.layers[2])
	if want := "git fetch origin " + m1 + " " + rebased + " failed: "; !strings.Contains(out.Error.Message, want) {
		t.Fatalf("message lacks %q: %s", want, out.Error.Message)
	}
	s.unchanged(t, before)
}
