package land

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/reviewsize"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

var (
	boss   = tasktest.Agent("w1:p1", "term_boss", "boss")
	worker = tasktest.Agent("w1:p3", "term_worker", "worker")
)

// fixture is a repository on main with branch feat one commit ahead, a
// registered boss and worker, and a task the worker owns, verified at feat.
type fixture struct {
	repo, id, head string
	boss, worker   identity.Record
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(gittest.Git(t, dir, args...))
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	writeFile(t, dir, name, content)
	git(t, dir, "add", name)
	git(t, dir, "commit", "-qm", name)
	return git(t, dir, "rev-parse", "HEAD")
}

func setup(t *testing.T, mode string) fixture {
	t.Helper()
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "Test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "t@example.com")
	}
	repo := identitytest.Repository(t)
	git(t, repo, "symbolic-ref", "HEAD", "refs/heads/main")
	commitFile(t, repo, "base", "base\n")
	git(t, repo, "checkout", "-qb", "feat")
	head := commitFile(t, repo, "feature", "feature\n")
	git(t, repo, "checkout", "-q", "main")
	f := fixture{repo: repo, head: head, boss: tasktest.Register(t, repo, boss), worker: tasktest.Register(t, repo, worker)}
	f.id = tasktest.Seed(t, repo, task.Record{Title: "Feature", Brief: "b", Status: task.Verified, Owner: &f.worker.ID, Verifier: &f.boss.ID,
		VerifiedAt: task.Now(), VerifiedCommit: &head, DeliveryMode: mode, CreatedAt: *task.Now()})
	return f
}

// land runs task land as boss.
func (f fixture) land(t *testing.T, o Options) cli.Outcome {
	t.Helper()
	return f.landAs(t, tasktest.Client(t, f.repo, "w1:p1", tasktest.Get("w1:p1", boss)), o)
}

func (f fixture) landAs(t *testing.T, c libagent.Client, o Options) cli.Outcome {
	t.Helper()
	if o.ID == "" {
		o.ID = f.id
	}
	return Run(context.Background(), c, o)
}

func (f fixture) load(t *testing.T) task.Record { return tasktest.Load(t, f.repo, f.id) }

// refused requires out to be a rejected refusal with code that left the
// task verified.
func (f fixture) refused(t *testing.T, out cli.Outcome, code string) task.Record {
	t.Helper()
	if out.Error == nil || out.Error.Code != code || out.Status != "rejected" {
		t.Fatalf("want %s, got %+v", code, out.Error)
	}
	r := f.load(t)
	if r.Status != task.Verified || r.LandedAt != nil || r.MergeCommit != nil {
		t.Fatalf("%+v", r)
	}
	if res, ok := out.Result.(Result); !ok || res.LandStep == nil {
		t.Fatalf("no land step: %+v", out.Result)
	}
	return r
}

func render(t *testing.T, out cli.Outcome) string {
	t.Helper()
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestLandRefusesBeforeGuards(t *testing.T) {
	f := setup(t, task.ModeLocal)
	before := f.load(t)
	// Invalid options are refused before the caller is looked up.
	for _, o := range []Options{{Branch: "-x", BranchSet: true}, {Branch: "", BranchSet: true}, {Mode: "none", ModeSet: true}, {Mode: "merge", ModeSet: true}} {
		if out := f.landAs(t, tasktest.Client(t, f.repo, "w1:p1"), o); out.Error == nil || out.Error.Code != "invalid_input" {
			t.Fatalf("%+v: %+v", o, out.Error)
		}
	}
	for _, mode := range []string{"none", "bogus"} {
		if out := f.landAs(t, tasktest.Client(t, f.repo, "w1:p1"), Options{Mode: mode, ModeSet: true}); out.Error == nil || out.Error.Message != "--mode must be local, pr, or auto" {
			t.Fatalf("%s: %+v", mode, out.Error)
		}
	}
	if out := f.landAs(t, tasktest.Client(t, f.repo, ""), Options{Branch: "feat", BranchSet: true}); out.Error == nil || out.Error.Code != "caller_unregistered" {
		t.Fatalf("%+v", out.Error)
	}
	out := f.landAs(t, tasktest.Client(t, f.repo, "w1:p3", tasktest.Get("w1:p3", worker)), Options{Branch: "feat", BranchSet: true})
	f.refused(t, out, "task_self_landing")
	if !reflect.DeepEqual(f.load(t), before) {
		t.Fatal("record changed")
	}
}

func TestLandRequiresVerifiedTaskAndCommit(t *testing.T) {
	f := setup(t, task.ModeLocal)
	for _, status := range []string{task.Created, task.Assigned, task.Completed, task.Cancelled, task.Landed} {
		id := tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: status, Owner: &f.worker.ID, VerifiedCommit: &f.head, DeliveryMode: task.ModeLocal})
		out := f.land(t, Options{ID: id, Branch: "feat", BranchSet: true})
		if out.Error == nil || out.Error.Code != "task_invalid_state" {
			t.Fatalf("%s: %+v", status, out.Error)
		}
	}
	id := tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &f.worker.ID, DeliveryMode: task.ModeLocal})
	if out := f.land(t, Options{ID: id, Branch: "feat", BranchSet: true}); out.Error == nil || out.Error.Code != "land_unverified_commit" {
		t.Fatalf("%+v", out.Error)
	}
}

func TestLandModeAndBranchResolution(t *testing.T) {
	f := setup(t, task.ModeNone)
	out := f.land(t, Options{})
	f.refused(t, out, "task_no_delivery_mode")
	if !strings.Contains(out.Error.Message, "--mode local, --mode pr, or --mode auto") {
		t.Fatalf("%+v", out.Error)
	}
	// The worker has no checkout recorded, so the branch is unknown; --mode
	// given on a refused run is still stored.
	r := f.refused(t, f.land(t, Options{Mode: "auto", ModeSet: true}), "land_branch_unknown")
	if r.DeliveryMode != task.ModeAuto {
		t.Fatalf("%+v", r)
	}
	r = f.refused(t, f.land(t, Options{Mode: "local", ModeSet: true}), "land_branch_unknown")
	if r.DeliveryMode != task.ModeLocal || r.Branch != nil {
		t.Fatalf("%+v", r)
	}
	r = f.refused(t, f.land(t, Options{Branch: "main", BranchSet: true}), "land_same_branch")
	if r.Branch == nil || *r.Branch != "main" {
		t.Fatalf("%+v", r)
	}
	// A stored branch is used again when --branch is not given.
	f.refused(t, f.land(t, Options{}), "land_same_branch")
}

func TestLandResolvesBranchFromOwnerCheckout(t *testing.T) {
	f := setup(t, task.ModeLocal)
	path := filepath.Join(t.TempDir(), "feat")
	git(t, f.repo, "worktree", "add", "-q", path, "feat")
	s := tasktest.Store(t, f.repo)
	// An owner spawned with --worktree <existing path> has only a path.
	owner, err := identity.Register(context.Background(), s, libagent.Client{}, tasktest.Agent("w1:p4", "term_owner", "owner").Agent, "spawn", &identity.Checkout{Path: path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &owner.ID, VerifiedCommit: &f.head, DeliveryMode: task.ModeLocal})
	out := f.land(t, Options{ID: id})
	if r := tasktest.Load(t, f.repo, id); out.Error != nil || r.Status != task.Landed || *r.Branch != "feat" {
		t.Fatalf("%+v %+v", out.Error, r)
	}
	// An owner whose checkout is gone falls back to its recorded branch.
	git(t, f.repo, "checkout", "-qb", "feat2", "feat")
	head2 := commitFile(t, f.repo, "second", "x\n")
	git(t, f.repo, "checkout", "-q", "main")
	branch := "feat2"
	gone, err := identity.Register(context.Background(), s, libagent.Client{}, tasktest.Agent("w1:p5", "term_gone", "gone").Agent, "spawn", &identity.Checkout{Path: filepath.Join(t.TempDir(), "missing"), Branch: &branch, Created: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id2 := tasktest.Seed(t, f.repo, task.Record{Title: "t2", Status: task.Verified, Owner: &gone.ID, VerifiedCommit: &head2, DeliveryMode: task.ModeLocal})
	out = f.land(t, Options{ID: id2})
	if r := tasktest.Load(t, f.repo, id2); out.Error != nil || r.Status != task.Landed || *r.Branch != "feat2" {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}

// small is the fixture's diff: one non-source file with one line.
var small = reviewsize.Diff{Files: 1, TotalLines: 1}

// sizeBlock is the report land prints for d against base, judged as kind,
// with missing the ref of the mode that did not exist.
func sizeBlock(kind reviewsize.Kind, base, missing string, d reviewsize.Diff) string {
	var b strings.Builder
	b.WriteString("Review size against " + base)
	if missing != "" {
		b.WriteString(" (" + missing + " is missing)")
	}
	b.WriteString(":\n")
	reviewsize.Render(&b, reviewsize.Compare(d, kind))
	return b.String()
}

// sizeJSON returns the JSON size field of out.
func sizeJSON(t *testing.T, out cli.Outcome) json.RawMessage {
	t.Helper()
	var b bytes.Buffer
	if err := out.Write(&b, true, Render); err != nil {
		t.Fatal(err)
	}
	var v struct {
		Result struct {
			Size json.RawMessage `json:"size"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b.Bytes(), &v); err != nil {
		t.Fatalf("%v: %s", err, b.String())
	}
	return v.Result.Size
}

// big moves feat to a new commit adding a 350-line Go file and seeds a task
// in mode verified there.
func (f *fixture) big(t *testing.T, mode string) {
	t.Helper()
	git(t, f.repo, "checkout", "-q", "feat")
	f.head = commitFile(t, f.repo, "big.go", strings.Repeat("// x\n", 350))
	git(t, f.repo, "checkout", "-q", "main")
	f.id = tasktest.Seed(t, f.repo, task.Record{Title: "Feature", Brief: "b", Status: task.Verified, Owner: &f.worker.ID, Verifier: &f.boss.ID,
		VerifiedAt: task.Now(), VerifiedCommit: &f.head, DeliveryMode: mode, CreatedAt: *task.Now()})
}

func TestLandSizeOnLocalLanding(t *testing.T) {
	f := setup(t, task.ModeLocal)
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	merge := git(t, f.repo, "rev-parse", "main")
	want := "Landed task " + f.id + ": merged feat into main (local) as " + merge + ".\n" + sizeBlock(reviewsize.KindTask, "refs/heads/main", "", small)
	if got := render(t, out); got != want {
		t.Fatalf("%q\nwant %q", got, want)
	}
	res := out.Result.(Result)
	if res.Size == nil || res.Size.Kind != reviewsize.KindTask || res.Size.Files.Value != 1 || len(res.Warnings) != 0 {
		t.Fatalf("%+v", res)
	}
	if s := sizeJSON(t, out); !strings.Contains(string(s), `"kind":"task"`) {
		t.Fatalf("%s", s)
	}
}

// TestLandSizeWarnsOnlyOnLanding: an over-limit branch lands with one
// warning per over-limit measure; refused, it has the size and no warning.
func TestLandSizeWarnsOnlyOnLanding(t *testing.T) {
	f := setup(t, task.ModeLocal)
	f.big(t, task.ModeLocal)
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	const warning = "review size: source lines 350 is over limit 200"
	if w := out.Result.(Result).Warnings; !slices.Equal(w, []string{warning}) {
		t.Fatalf("%q", w)
	}
	if got := render(t, out); !strings.Contains(got, "Warning: "+warning+".\n") {
		t.Fatalf("%q", got)
	}

	f, g := setupPR(t, task.ModeAuto)
	f.big(t, task.ModeAuto)
	g.set(t, "listed", "["+strings.Replace(prJSON, "%s", f.head, 1)+"]")
	g.set(t, "checks", checks(f.head, "MERGEABLE", run("IN_PROGRESS", "")))
	out = f.land(t, Options{Branch: "feat", BranchSet: true})
	f.refused(t, out, "land_checks_pending")
	res := out.Result.(Result)
	if res.Size == nil || res.Size.SourceLines.Status != reviewsize.OverCeiling || len(res.Warnings) != 0 {
		t.Fatalf("%+v", res)
	}
	// An over-limit pull request is landed with a target warning.
	g.set(t, "checks", checks(f.head, "MERGEABLE", ""))
	out = f.land(t, Options{})
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if w := out.Result.(Result).Warnings; !slices.Equal(w, []string{"review size: source lines 350 is over ceiling 300"}) {
		t.Fatalf("%q", w)
	}
	// A review stop has the size and no warning.
	f, g = setupPR(t, task.ModePR)
	f.big(t, task.ModePR)
	g.set(t, "listed", "["+strings.Replace(prJSON, "%s", f.head, 1)+"]")
	g.set(t, "checks", checks(f.head, "MERGEABLE", ""))
	out = f.land(t, Options{Branch: "feat", BranchSet: true})
	if res := out.Result.(Result); out.Error != nil || !res.ReviewPending || res.Size == nil || res.Size.SourceLines.Status != reviewsize.OverCeiling || len(res.Warnings) != 0 {
		t.Fatalf("%+v %+v", out.Error, res)
	}
}

// TestLandSizeOnRefusalsAfterHeadCheck: every refusal after the head check
// prints the report after the step reached and has size in JSON.
func TestLandSizeOnRefusalsAfterHeadCheck(t *testing.T) {
	check := func(t *testing.T, f fixture, out cli.Outcome, code, step, block string) {
		t.Helper()
		f.refused(t, out, code)
		if got, want := render(t, out), "Land step reached: "+step+".\n"+block; !strings.HasSuffix(got, want) {
			t.Fatalf("%q\nwant suffix %q", got, want)
		}
		if res := out.Result.(Result); res.Size == nil || len(res.Warnings) != 0 {
			t.Fatalf("%+v", res)
		}
		if s := sizeJSON(t, out); string(s) == "null" {
			t.Fatal("size is null")
		}
	}
	local := sizeBlock(reviewsize.KindTask, "refs/heads/main", "", small)
	pr := sizeBlock(reviewsize.KindPR, "refs/remotes/origin/main", "", small)
	t.Run("dirty", func(t *testing.T) {
		f := setup(t, task.ModeLocal)
		path := filepath.Join(t.TempDir(), "feat")
		git(t, f.repo, "worktree", "add", "-q", path, "feat")
		writeFile(t, path, "feature", "changed\n")
		check(t, f, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_dirty", "check", local)
	})
	t.Run("already merged", func(t *testing.T) {
		f := setup(t, task.ModeLocal)
		git(t, f.repo, "merge", "-q", "--no-ff", "-m", "by hand", "feat")
		check(t, f, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_already_merged", "check", sizeBlock(reviewsize.KindTask, "refs/heads/main", "", reviewsize.Diff{}))
	})
	t.Run("target missing", func(t *testing.T) {
		// A local landing into a target only on origin is compared with it.
		f := setup(t, task.ModeLocal)
		git(t, f.repo, "update-ref", "refs/remotes/origin/dev", "main")
		f.id = tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &f.worker.ID, VerifiedCommit: &f.head, DeliveryMode: task.ModeLocal, BaseBranch: tasktest.Ptr("dev")})
		check(t, f, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_target_missing", "check", sizeBlock(reviewsize.KindTask, "refs/remotes/origin/dev", "refs/heads/dev", small))
	})
	t.Run("gh missing", func(t *testing.T) {
		f, _ := setupPR(t, task.ModePR)
		gitPath, err := exec.LookPath("git")
		if err != nil {
			t.Fatal(err)
		}
		bin := t.TempDir()
		os.Symlink(gitPath, filepath.Join(bin, "git"))
		t.Setenv("PATH", bin)
		check(t, f, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_gh_missing", "check", pr)
	})
	t.Run("checks pending", func(t *testing.T) {
		f, g := setupPR(t, task.ModePR)
		g.set(t, "checks", checks(f.head, "MERGEABLE", run("IN_PROGRESS", "")))
		check(t, f, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_checks_pending", "checks", pr)
	})
}

// TestLandNoSizeBeforeHeadCheck: a refusal before the head check, or a
// target that exists as neither ref, has no size and prints no report.
func TestLandNoSizeBeforeHeadCheck(t *testing.T) {
	none := func(t *testing.T, out cli.Outcome) {
		t.Helper()
		if res := out.Result.(Result); res.Size != nil {
			t.Fatalf("%+v", res.Size)
		}
		if s := sizeJSON(t, out); string(s) != "null" {
			t.Fatalf("%s", s)
		}
		if got := render(t, out); strings.Contains(got, "size") {
			t.Fatalf("%q", got)
		}
	}
	f := setup(t, task.ModeLocal)
	git(t, f.repo, "checkout", "-q", "feat")
	commitFile(t, f.repo, "late", "late\n")
	git(t, f.repo, "checkout", "-q", "main")
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	f.refused(t, out, "land_head_mismatch")
	none(t, out)
	id := tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &f.worker.ID, DeliveryMode: task.ModeLocal})
	out = f.land(t, Options{ID: id, Branch: "feat", BranchSet: true})
	if out.Error == nil || out.Error.Code != "land_unverified_commit" {
		t.Fatalf("%+v", out.Error)
	}
	none(t, out)
	f = setup(t, task.ModeLocal)
	f.id = tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &f.worker.ID, VerifiedCommit: &f.head, DeliveryMode: task.ModeLocal, BaseBranch: tasktest.Ptr("nosuch")})
	out = f.land(t, Options{Branch: "feat", BranchSet: true})
	f.refused(t, out, "land_target_missing")
	none(t, out)
}

func TestLandBranchUnknownMessage(t *testing.T) {
	f := setup(t, task.ModeLocal)
	out := f.land(t, Options{})
	f.refused(t, out, "land_branch_unknown")
	if want := "cannot tell which branch task " + f.id + " is on: its owner has no recorded checkout branch; pass --branch"; out.Error.Message != want {
		t.Fatalf("%q", out.Error.Message)
	}
}
