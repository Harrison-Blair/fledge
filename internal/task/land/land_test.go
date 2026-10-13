package land

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
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
