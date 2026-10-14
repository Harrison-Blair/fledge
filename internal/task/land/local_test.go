package land

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/reviewsize"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestLandLocalMergesVerifiedCommit(t *testing.T) {
	f := setup(t, task.ModeLocal)
	before := git(t, f.repo, "rev-parse", "main")
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	if out.Error != nil || out.Status != "success" || out.Operation != "task.land" {
		t.Fatalf("%+v", out.Error)
	}
	merge := git(t, f.repo, "rev-parse", "main")
	if parents := git(t, f.repo, "rev-list", "--parents", "-n1", merge); parents != merge+" "+before+" "+f.head {
		t.Fatalf("%s", parents)
	}
	if msg := git(t, f.repo, "log", "-1", "--format=%s", merge); msg != "Merge branch 'feat' (verified task "+f.id+")" {
		t.Fatalf("%q", msg)
	}
	r := f.load(t)
	if r.Status != task.Landed || *r.MergeCommit != merge || *r.TargetBranch != "main" || *r.Branch != "feat" || *r.LandedBy != f.boss.ID || r.LandedAt == nil || *r.LandStep != "merge" || r.PRURL != nil {
		t.Fatalf("%+v", r)
	}
	if res := out.Result.(Result); res.Mode != task.ModeLocal || !reflect.DeepEqual(res.Record, r) {
		t.Fatalf("%+v", res)
	}
	want := "Landed task " + f.id + ": merged feat into main (local) as " + merge + ".\n" + sizeBlock(reviewsize.KindTask, "refs/heads/main", "", small)
	if got := render(t, out); got != want {
		t.Fatalf("%q\nwant %q", got, want)
	}
	// Landing again is refused: the task is landed.
	if out := f.land(t, Options{}); out.Error == nil || out.Error.Code != "task_invalid_state" {
		t.Fatalf("%+v", out.Error)
	}
}

func TestLandHeadMismatchAfterVerification(t *testing.T) {
	f := setup(t, task.ModeLocal)
	git(t, f.repo, "checkout", "-q", "feat")
	commitFile(t, f.repo, "late", "late\n")
	git(t, f.repo, "checkout", "-q", "main")
	f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_head_mismatch")
	f.refused(t, f.land(t, Options{Branch: "nosuch", BranchSet: true}), "land_head_mismatch")
}

func TestLandRefusesDirtyBranchCheckout(t *testing.T) {
	f := setup(t, task.ModeLocal)
	path := filepath.Join(t.TempDir(), "feat")
	git(t, f.repo, "worktree", "add", "-q", path, "feat")
	writeFile(t, path, "untracked", "x")
	// Untracked files do not make the checkout dirty.
	if out := f.land(t, Options{Branch: "feat", BranchSet: true}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	f = setup(t, task.ModeLocal)
	path = filepath.Join(t.TempDir(), "feat")
	git(t, f.repo, "worktree", "add", "-q", path, "feat")
	writeFile(t, path, "feature", "changed\n")
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	f.refused(t, out, "land_dirty")
	if !strings.Contains(out.Error.Message, path) {
		t.Fatalf("%s", out.Error.Message)
	}
}

func TestLandAlreadyMergedRecordsNothing(t *testing.T) {
	f := setup(t, task.ModeLocal)
	git(t, f.repo, "merge", "-q", "--no-ff", "-m", "by hand", "feat")
	before := git(t, f.repo, "rev-parse", "main")
	want := f.load(t)
	f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_already_merged")
	// Only the --branch given is stored.
	want.Branch = tasktest.Ptr("feat")
	if got := f.load(t); !reflect.DeepEqual(got, want) || git(t, f.repo, "rev-parse", "main") != before {
		t.Fatalf("%+v", got)
	}
}

func TestLandRefusesPrimaryNotReady(t *testing.T) {
	f := setup(t, task.ModeLocal)
	git(t, f.repo, "checkout", "-qb", "other")
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	f.refused(t, out, "land_target_checkout_missing")
	if !strings.Contains(out.Error.Message, "main") {
		t.Fatalf("%s", out.Error.Message)
	}
	git(t, f.repo, "checkout", "-q", "main")
	writeFile(t, f.repo, "base", "edited\n")
	out = f.land(t, Options{Branch: "feat", BranchSet: true})
	f.refused(t, out, "land_target_checkout_dirty")
	if !strings.Contains(out.Error.Message, "base") {
		t.Fatalf("%s", out.Error.Message)
	}
	git(t, f.repo, "checkout", "--", "base")
	// Untracked files in the primary checkout are allowed.
	writeFile(t, f.repo, "notes", "x")
	if out := f.land(t, Options{Branch: "feat", BranchSet: true}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if _, err := os.Stat(filepath.Join(f.repo, "notes")); err != nil {
		t.Fatal(err)
	}
}

// snapshot is the primary checkout's HEAD and full porcelain status.
func snapshot(t *testing.T, repo string) string {
	return git(t, repo, "rev-parse", "HEAD") + "\n" + gitRaw(t, repo, "status", "--porcelain=v1", "-z", "--untracked-files=all")
}

func gitRaw(t *testing.T, dir string, args ...string) string {
	t.Helper()
	b, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLandAbortsConflict(t *testing.T) {
	f := setup(t, task.ModeLocal)
	commitFile(t, f.repo, "feature", "conflicting\n")
	writeFile(t, f.repo, "notes", "untracked\n")
	before := snapshot(t, f.repo)
	f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_merge_conflict")
	if after := snapshot(t, f.repo); after != before {
		t.Fatalf("%q != %q", after, before)
	}
	if _, err := os.Stat(filepath.Join(f.repo, ".git", "MERGE_HEAD")); !os.IsNotExist(err) {
		t.Fatal("merge left in progress")
	}
}

func TestLandRefusesUntrackedOverwrite(t *testing.T) {
	f := setup(t, task.ModeLocal)
	writeFile(t, f.repo, "feature", "mine\n")
	before := snapshot(t, f.repo)
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	f.refused(t, out, "land_merge_refused")
	if !strings.Contains(out.Error.Message, "untracked") {
		t.Fatalf("%s", out.Error.Message)
	}
	if after := snapshot(t, f.repo); after != before {
		t.Fatalf("%q != %q", after, before)
	}
	if b, _ := os.ReadFile(filepath.Join(f.repo, "feature")); string(b) != "mine\n" {
		t.Fatalf("%q", b)
	}
}

func TestLandRefusesMissingTarget(t *testing.T) {
	f := setup(t, task.ModeLocal)
	id := tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &f.worker.ID, VerifiedCommit: &f.head, DeliveryMode: task.ModeLocal, BaseBranch: tasktest.Ptr("nosuch")})
	if out := f.land(t, Options{ID: id, Branch: "feat", BranchSet: true}); out.Error == nil || out.Error.Code != "land_target_missing" {
		t.Fatalf("%+v", out.Error)
	}
	// A target that exists only on origin cannot be merged into locally.
	git(t, f.repo, "update-ref", "refs/remotes/origin/dev", "main")
	id = tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &f.worker.ID, VerifiedCommit: &f.head, DeliveryMode: task.ModeLocal, BaseBranch: tasktest.Ptr("dev")})
	if out := f.land(t, Options{ID: id, Branch: "feat", BranchSet: true}); out.Error == nil || out.Error.Code != "land_target_missing" {
		t.Fatalf("%+v", out.Error)
	}
}

func TestConcurrentLandsLandOnce(t *testing.T) {
	f := setup(t, task.ModeLocal)
	clients := []libagent.Client{
		tasktest.Client(t, f.repo, "w1:p1", tasktest.Get("w1:p1", boss)),
		tasktest.Client(t, f.repo, "w1:p1", tasktest.Get("w1:p1", boss)),
	}
	codes := make([]string, 2)
	var wg sync.WaitGroup
	for i := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := Run(context.Background(), clients[i], Options{ID: f.id, Branch: "feat", BranchSet: true})
			if out.Error != nil {
				codes[i] = out.Error.Code
			}
		}()
	}
	wg.Wait()
	if !(codes[0] == "" && codes[1] == "task_invalid_state" || codes[1] == "" && codes[0] == "task_invalid_state") {
		t.Fatalf("%v", codes)
	}
	if n := git(t, f.repo, "rev-list", "--count", "--merges", "main"); n != "1" {
		t.Fatalf("%s merges", n)
	}
}

// A task changed during the merge is not recorded as landed: here a
// pre-merge-commit hook cancels it, as a concurrent task cancel would.
func TestLandRechecksTaskBeforeRecording(t *testing.T) {
	f := setup(t, task.ModeLocal)
	record := filepath.Join(f.repo, ".fledge", "state", "tasks", f.id+".json")
	hook := filepath.Join(f.repo, ".git", "hooks", "pre-merge-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nsed -i 's/\"status\": \"verified\"/\"status\": \"cancelled\"/' '"+record+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	if out.Error == nil || out.Error.Code != "task_state_changed" {
		t.Fatalf("%+v", out.Error)
	}
	if r := f.load(t); r.Status != task.Cancelled || r.LandedAt != nil || r.MergeCommit != nil {
		t.Fatalf("%+v", r)
	}
}
