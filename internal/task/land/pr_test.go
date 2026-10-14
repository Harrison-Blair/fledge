package land

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/reviewsize"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// fakeGH is a gh on PATH that logs its arguments and answers from files in
// dir: list (before a create) and listed (after), checks, view (before a
// merge) and merged (after), and the optional merge-fails and
// list-after-create-fails. When list-<head> exists, pr list for that head
// answers [] before a create and list-<head> after it, and pr create prints
// its url. checks-<n> answers for pull request n when it exists. gh api
// answers from api-<method>-<path with _ for />.
type fakeGH struct{ dir string }

const ghScript = `#!/bin/sh
d=%q
printf '%s\n' "$*" >> "$d/log"
h=; for a in "$@"; do case "$a" in --head=*) h=${a#--head=} ;; esac; done
case "$1 $2" in
"pr list") if [ -f "$d/list-$h" ]; then
    if [ -f "$d/created-$h" ]; then cat "$d/list-$h"; else echo '[]'; fi
  elif [ -f "$d/created" ]; then
    if [ -f "$d/list-after-create-fails" ]; then echo "lookup failed" >&2; exit 1; fi
    cat "$d/listed"; else cat "$d/list"; fi ;;
"pr create") touch "$d/created" "$d/created-$h"
  for a in "$@"; do case "$a" in --body-file=*) cp "${a#--body-file=}" "$d/body" ;; esac; done
  if [ -f "$d/list-$h" ]; then sed 's/.*"url":"\([^"]*\)".*/\1/' "$d/list-$h"; else echo https://example.com/pr/7; fi ;;
"pr view") case "$4" in *statusCheckRollup*) if [ -f "$d/checks-$3" ]; then cat "$d/checks-$3"; else cat "$d/checks"; fi ;;
  *) if [ -f "$d/did-merge" ]; then cat "$d/merged"; else cat "$d/view"; fi ;; esac ;;
"pr merge") if [ -f "$d/merge-fails" ]; then echo "merge blocked" >&2; exit 1; fi; touch "$d/did-merge" ;;
"api "*) f="$d/api-${2#--method=}-$(printf '%s' "$3" | tr / _)"; cat "$f"; if [ -f "$f.exit" ]; then exit "$(cat "$f.exit")"; fi ;;
*) echo "unexpected gh $*" >&2; exit 2 ;;
esac
`

const (
	mergeHash = "1111111111111111111111111111111111111111"
	prJSON    = `{"number":7,"url":"https://example.com/pr/7","headRefOid":"%s","isDraft":false,"mergeable":"MERGEABLE"}`
)

func installGH(t *testing.T) fakeGH {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	os.Mkdir(bin, 0o755)
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(strings.Replace(ghScript, "%q", "'"+dir+"'", 1)), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	g := fakeGH{dir: dir}
	g.set(t, "list", "[]")
	return g
}

func (g fakeGH) set(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(g.dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (g fakeGH) log(t *testing.T) []string {
	b, _ := os.ReadFile(filepath.Join(g.dir, "log"))
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func checks(head, mergeable, rollup string) string {
	return `{"isDraft":false,"mergeable":"` + mergeable + `","headRefOid":"` + head + `","statusCheckRollup":[` + rollup + `]}`
}

func run(status, conclusion string) string {
	return `{"__typename":"CheckRun","name":"ci","status":"` + status + `","conclusion":"` + conclusion + `"}`
}

func context_(state string) string {
	return `{"__typename":"StatusContext","context":"ci","state":"` + state + `"}`
}

// setupPR is setup in delivery mode mode with a bare origin holding main,
// and a fake gh.
func setupPR(t *testing.T, mode string) (fixture, fakeGH) {
	t.Helper()
	f := setup(t, mode)
	origin := filepath.Join(t.TempDir(), "origin.git")
	git(t, f.repo, "init", "-q", "--bare", origin)
	git(t, f.repo, "remote", "add", "origin", origin)
	git(t, f.repo, "push", "-q", "origin", "main")
	git(t, f.repo, "fetch", "-q", "origin")
	g := installGH(t)
	g.set(t, "listed", "["+strings.Replace(prJSON, "%s", f.head, 1)+"]")
	g.set(t, "checks", checks(f.head, "MERGEABLE", ""))
	g.set(t, "view", view("OPEN", "", f.head))
	g.set(t, "merged", view("MERGED", mergeHash, f.head))
	return f, g
}

// view is gh pr view output with state, merge commit, and head.
func view(state, merge, head string) string {
	commit := "null"
	if merge != "" {
		commit = `{"oid":"` + merge + `"}`
	}
	return `{"state":"` + state + `","mergeCommit":` + commit + `,"headRefOid":"` + head + `"}`
}

// TestLandPRPushesOpensAndMerges lands an auto task in one command: push,
// pull request, checks, merge, confirm.
func TestLandPRPushesOpensAndMerges(t *testing.T) {
	for _, mode := range []string{task.ModeAuto} {
		t.Run(mode, func(t *testing.T) {
			f, g := setupPR(t, mode)
			out := f.land(t, Options{Branch: "feat", BranchSet: true})
			if out.Error != nil {
				t.Fatalf("%+v", out.Error)
			}
			origin := git(t, f.repo, "remote", "get-url", "origin")
			if pushed := git(t, origin, "rev-parse", "refs/heads/feat"); pushed != f.head {
				t.Fatalf("pushed %s", pushed)
			}
			log := g.log(t)
			wantCreate := "pr create --base=main --head=feat --title=Feature --body-file="
			wantMerge := "pr merge 7 --merge --match-head-commit=" + f.head + " --subject=Merge branch 'feat' (verified task " + f.id + ")"
			if !slices.ContainsFunc(log, func(l string) bool { return strings.HasPrefix(l, wantCreate) }) || !slices.Contains(log, wantMerge) ||
				!slices.Contains(log, "pr list --head=feat --base=main --state=open --json=number,url,headRefOid,isDraft,mergeable") ||
				!slices.Contains(log, "pr view 7 --json=isDraft,mergeable,headRefOid,statusCheckRollup") || !slices.Contains(log, "pr view 7 --json=state,mergeCommit,headRefOid") {
				t.Fatalf("%q", log)
			}
			r := f.load(t)
			if r.Status != task.Landed || *r.MergeCommit != mergeHash || *r.PRURL != "https://example.com/pr/7" || *r.TargetBranch != "main" || *r.LandStep != "confirm" || *r.LandedBy != f.boss.ID {
				t.Fatalf("%+v", r)
			}
			body, _ := os.ReadFile(filepath.Join(g.dir, "body"))
			for _, part := range []string{f.id, f.head, f.boss.ID, f.worker.ID} {
				if !strings.Contains(string(body), part) {
					t.Fatalf("body lacks %s: %s", part, body)
				}
			}
			want := "Landed task " + f.id + ": merged feat into main (" + mode + ") as " + mergeHash + ".\nPull request: https://example.com/pr/7\n" + sizeBlock(reviewsize.KindPR, "refs/remotes/origin/main", "", small)
			if got := render(t, out); got != want {
				t.Fatalf("%q", got)
			}
		})
	}
}

func TestLandPRRefusesUnreadyPullRequests(t *testing.T) {
	for _, tc := range []struct{ name, checks, code string }{
		{"draft", strings.Replace(checks("HEAD", "MERGEABLE", ""), `"isDraft":false`, `"isDraft":true`, 1), "land_pr_draft"},
		{"conflicting", checks("HEAD", "CONFLICTING", ""), "land_pr_not_mergeable"},
		{"unknown", checks("HEAD", "UNKNOWN", ""), "land_mergeable_unknown"},
		{"moved head", checks("2222222222222222222222222222222222222222", "MERGEABLE", ""), "land_head_mismatch"},
		{"run pending", checks("HEAD", "MERGEABLE", run("IN_PROGRESS", "")), "land_checks_pending"},
		{"context pending", checks("HEAD", "MERGEABLE", context_("PENDING")), "land_checks_pending"},
		{"context expected", checks("HEAD", "MERGEABLE", context_("EXPECTED")), "land_checks_pending"},
		{"run failed", checks("HEAD", "MERGEABLE", run("COMPLETED", "SUCCESS")+","+run("COMPLETED", "FAILURE")), "land_checks_failed"},
		{"context failed", checks("HEAD", "MERGEABLE", context_("ERROR")), "land_checks_failed"},
		{"missing field", `{"isDraft":false,"mergeable":"MERGEABLE","headRefOid":"HEAD"}`, "land_gh_output_invalid"},
		{"run missing conclusion", checks("HEAD", "MERGEABLE", `{"__typename":"CheckRun","status":"COMPLETED"}`), "land_gh_output_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, g := setupPR(t, task.ModePR)
			g.set(t, "checks", strings.ReplaceAll(tc.checks, `"HEAD"`, `"`+f.head+`"`))
			out := f.land(t, Options{Branch: "feat", BranchSet: true})
			r := f.refused(t, out, tc.code)
			// The push, the pull request, and the step reached remain.
			if *r.LandStep != "checks" || *r.PRURL != "https://example.com/pr/7" || *out.Result.(Result).LandStep != "checks" {
				t.Fatalf("%+v", r)
			}
			if _, err := os.Stat(filepath.Join(g.dir, "did-merge")); err == nil {
				t.Fatal("merged")
			}
		})
	}
}

func TestLandPRAcceptsSkippedNeutralAndRerunsIdempotently(t *testing.T) {
	f, g := setupPR(t, task.ModeAuto)
	g.set(t, "checks", checks(f.head, "UNKNOWN", ""))
	f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_mergeable_unknown")
	// The rerun finds the pushed branch and the open pull request.
	g.set(t, "list", "["+strings.Replace(prJSON, "%s", f.head, 1)+"]")
	os.Remove(filepath.Join(g.dir, "log"))
	g.set(t, "checks", checks(f.head, "MERGEABLE", run("COMPLETED", "SKIPPED")+","+run("COMPLETED", "NEUTRAL")+","+run("COMPLETED", "SUCCESS")+","+context_("SUCCESS")))
	if out := f.land(t, Options{}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if log := g.log(t); slices.ContainsFunc(log, func(l string) bool { return strings.HasPrefix(l, "pr create") }) {
		t.Fatalf("%q", log)
	}
	if r := f.load(t); r.Status != task.Landed {
		t.Fatalf("%+v", r)
	}
}

func TestLandPRRefusesAmbiguousMergeFailureAndUnconfirmed(t *testing.T) {
	f, g := setupPR(t, task.ModeAuto)
	pr := strings.Replace(prJSON, "%s", f.head, 1)
	g.set(t, "list", "["+pr+","+strings.Replace(pr, `"number":7`, `"number":8`, 1)+"]")
	if r := f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_pr_ambiguous"); *r.LandStep != "pr" {
		t.Fatalf("%+v", r)
	}

	f, g = setupPR(t, task.ModeAuto)
	g.set(t, "merge-fails", "")
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	if r := f.refused(t, out, "land_merge_failed"); *r.LandStep != "merge" || !strings.Contains(out.Error.Message, "merge blocked") {
		t.Fatalf("%+v %s", r, out.Error.Message)
	}

	f, g = setupPR(t, task.ModeAuto)
	g.set(t, "merged", view("OPEN", "", f.head))
	if r := f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_unconfirmed"); *r.LandStep != "confirm" {
		t.Fatalf("%+v", r)
	}
}

func TestLandPRPushRejected(t *testing.T) {
	f, _ := setupPR(t, task.ModePR)
	origin := git(t, f.repo, "remote", "get-url", "origin")
	hook := filepath.Join(origin, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho denied >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if r := f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_push_rejected"); *r.LandStep != "push" || r.PRURL != nil {
		t.Fatalf("%+v", r)
	}
}

func TestLandPRGuards(t *testing.T) {
	f, _ := setupPR(t, task.ModePR)
	// gh missing: PATH holds only git.
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	os.Symlink(gitPath, filepath.Join(bin, "git"))
	path := os.Getenv("PATH")
	t.Setenv("PATH", bin)
	before := f.load(t)
	f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_gh_missing")
	t.Setenv("PATH", path)
	// Only --branch is stored by a refusal before the push.
	before.Branch = tasktest.Ptr("feat")
	if got := f.load(t); got.LandStep != nil || got.PRURL != nil || *got.Branch != "feat" {
		t.Fatalf("%+v", got)
	}
	// A target that exists only as origin/<name> is accepted; one missing
	// on origin is refused.
	git(t, f.repo, "push", "-q", "origin", "main:dev")
	git(t, f.repo, "fetch", "-q", "origin")
	id := tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &f.worker.ID, VerifiedCommit: &f.head, DeliveryMode: task.ModeAuto, BaseBranch: tasktest.Ptr("refs/remotes/origin/dev")})
	out := f.land(t, Options{ID: id, Branch: "feat", BranchSet: true})
	if r := tasktest.Load(t, f.repo, id); out.Error != nil || *r.TargetBranch != "dev" {
		t.Fatalf("%+v %+v", out.Error, r)
	}
	id = tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &f.worker.ID, VerifiedCommit: &f.head, DeliveryMode: task.ModePR, BaseBranch: tasktest.Ptr("nosuch")})
	if out := f.land(t, Options{ID: id, Branch: "feat", BranchSet: true}); out.Error == nil || out.Error.Code != "land_target_missing" {
		t.Fatalf("%+v", out.Error)
	}
}

// TestLandAutoUsesPRGuards checks that an auto task, like a pr task, needs gh.
func TestLandAutoUsesPRGuards(t *testing.T) {
	f, _ := setupPR(t, task.ModeAuto)
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	os.Symlink(gitPath, filepath.Join(bin, "git"))
	t.Setenv("PATH", bin)
	f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_gh_missing")
}

func TestLandPRRefusesInvalidTargetBeforeGH(t *testing.T) {
	f, g := setupPR(t, task.ModePR)
	git(t, f.repo, "update-ref", "refs/remotes/origin/-foo", "main")
	check := func(id string) {
		t.Helper()
		out := f.land(t, Options{ID: id, Branch: "feat", BranchSet: true})
		if out.Error == nil || out.Error.Code != "land_target_invalid" || *out.Result.(Result).LandStep != "check" {
			t.Fatalf("%+v %+v", out.Error, out.Result)
		}
		if _, err := os.Stat(filepath.Join(g.dir, "log")); err == nil {
			t.Fatalf("gh ran: %q", g.log(t))
		}
	}
	// A stored base_branch, short or as a remote ref.
	for _, base := range []string{"-foo", "refs/remotes/origin/-foo", "refs/heads/-foo"} {
		check(tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &f.worker.ID, VerifiedCommit: &f.head, DeliveryMode: task.ModePR, BaseBranch: tasktest.Ptr(base)}))
	}
	// The integration branch derived from origin/HEAD.
	git(t, f.repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/-foo")
	check(f.id)
}

func TestLandPRRecordsCreatedURLWhenLookupFails(t *testing.T) {
	f, g := setupPR(t, task.ModeAuto)
	g.set(t, "list-after-create-fails", "")
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	if r := f.refused(t, out, "land_gh_failed"); *r.LandStep != "pr" || r.PRURL == nil || *r.PRURL != "https://example.com/pr/7" {
		t.Fatalf("%+v", r)
	}
	// The rerun finds the pull request and does not open a second one.
	os.Remove(filepath.Join(g.dir, "list-after-create-fails"))
	if out := f.land(t, Options{}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	creates := 0
	for _, l := range g.log(t) {
		if strings.HasPrefix(l, "pr create") {
			creates++
		}
	}
	if r := f.load(t); creates != 1 || r.Status != task.Landed {
		t.Fatalf("%d creates, %+v", creates, r)
	}
}

func TestLandEarlyRefusalsNameCheckStep(t *testing.T) {
	f, g := setupPR(t, task.ModePR)
	for _, tc := range []struct {
		name string
		pane string
		o    Options
		code string
	}{
		{"invalid branch", "w1:p1", Options{Branch: "-foo", BranchSet: true}, "invalid_input"},
		{"invalid mode", "w1:p1", Options{Mode: "merge", ModeSet: true}, "invalid_input"},
		{"unregistered caller", "", Options{Branch: "feat", BranchSet: true}, "caller_unregistered"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := f.landAs(t, tasktest.Client(t, f.repo, tc.pane), tc.o)
			res, ok := out.Result.(Result)
			if out.Error == nil || out.Error.Code != tc.code || !ok || res.LandStep == nil || *res.LandStep != "check" {
				t.Fatalf("%+v %+v", out.Error, out.Result)
			}
			if got := render(t, out); !strings.Contains(got, "Land step reached: check.") {
				t.Fatalf("%q", got)
			}
			if _, err := os.Stat(filepath.Join(g.dir, "log")); err == nil {
				t.Fatalf("gh ran: %q", g.log(t))
			}
		})
	}
}

// merged reports whether the fake gh ran gh pr merge.
func (g fakeGH) merged() bool {
	_, err := os.Stat(filepath.Join(g.dir, "did-merge"))
	return err == nil
}

// reviewStop lands a pr task up to the review stop and checks the result.
func reviewStop(t *testing.T, f fixture, g fakeGH, o Options) {
	t.Helper()
	out := f.land(t, o)
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	r := f.load(t)
	if r.Status != task.Verified || *r.LandStep != "review" || r.PRURL == nil || *r.PRURL != "https://example.com/pr/7" || r.MergeCommit != nil || r.LandedAt != nil {
		t.Fatalf("%+v", r)
	}
	if res := out.Result.(Result); !res.ReviewPending {
		t.Fatalf("%+v", res)
	}
	if g.merged() || slices.ContainsFunc(g.log(t), func(l string) bool { return strings.HasPrefix(l, "pr merge") }) {
		t.Fatalf("merged: %q", g.log(t))
	}
	var b bytes.Buffer
	if err := out.Write(&b, true, Render); err != nil || !strings.Contains(b.String(), `"review_pending":true`) {
		t.Fatalf("%v %s", err, b.String())
	}
	want := "Task " + f.id + " is ready for review: pull request https://example.com/pr/7 merges feat into main (pr), and its checks passed.\n" +
		"Merge it only on the owner's word: fledge task land --id " + f.id + " --merge\n" + sizeBlock(reviewsize.KindPR, "refs/remotes/origin/main", "", small)
	if got := render(t, out); got != want {
		t.Fatalf("%q", got)
	}
}

func TestLandPRStopsForReviewIdempotently(t *testing.T) {
	f, g := setupPR(t, task.ModePR)
	reviewStop(t, f, g, Options{Branch: "feat", BranchSet: true})
	g.set(t, "list", "["+strings.Replace(prJSON, "%s", f.head, 1)+"]")
	reviewStop(t, f, g, Options{})
}

func TestLandPRMergeAfterReview(t *testing.T) {
	f, g := setupPR(t, task.ModePR)
	reviewStop(t, f, g, Options{Branch: "feat", BranchSet: true})
	g.set(t, "list", "["+strings.Replace(prJSON, "%s", f.head, 1)+"]")
	out := f.land(t, Options{Merge: true})
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	log := g.log(t)
	wantMerge := "pr merge 7 --merge --match-head-commit=" + f.head + " --subject=Merge branch 'feat' (verified task " + f.id + ")"
	if !slices.Contains(log, wantMerge) || !slices.Contains(log, "pr view 7 --json=state,mergeCommit,headRefOid") {
		t.Fatalf("%q", log)
	}
	r := f.load(t)
	if r.Status != task.Landed || *r.MergeCommit != mergeHash || *r.LandStep != "confirm" || *r.TargetBranch != "main" || out.Result.(Result).ReviewPending {
		t.Fatalf("%+v", r)
	}
	want := "Landed task " + f.id + ": merged feat into main (pr) as " + mergeHash + ".\nPull request: https://example.com/pr/7\n" + sizeBlock(reviewsize.KindPR, "refs/remotes/origin/main", "", small)
	if got := render(t, out); got != want {
		t.Fatalf("%q", got)
	}
}

func TestLandPRMergeRefusesMovedLocalHead(t *testing.T) {
	f, g := setupPR(t, task.ModePR)
	reviewStop(t, f, g, Options{Branch: "feat", BranchSet: true})
	git(t, f.repo, "checkout", "-q", "feat")
	commitFile(t, f.repo, "later", "later\n")
	git(t, f.repo, "checkout", "-q", "main")
	f.refused(t, f.land(t, Options{Merge: true}), "land_head_mismatch")
	if g.merged() {
		t.Fatal("merged")
	}
}

func TestLandPRMergeRefusesMovedPRHead(t *testing.T) {
	f, g := setupPR(t, task.ModePR)
	reviewStop(t, f, g, Options{Branch: "feat", BranchSet: true})
	g.set(t, "list", "["+strings.Replace(prJSON, "%s", f.head, 1)+"]")
	g.set(t, "checks", checks("2222222222222222222222222222222222222222", "MERGEABLE", ""))
	out := f.land(t, Options{Merge: true})
	if r := f.refused(t, out, "land_head_mismatch"); *r.LandStep != "checks" || !strings.Contains(out.Error.Message, "has head 2222") {
		t.Fatalf("%+v %s", r, out.Error.Message)
	}
	if g.merged() {
		t.Fatal("merged")
	}
}

// TestLandPRMergeNeedsOpenPullRequest gates --merge on pr_url, not on
// land_step: a merge refused at checks after the review stop can run again.
func TestLandPRMergeNeedsOpenPullRequest(t *testing.T) {
	f, g := setupPR(t, task.ModePR)
	if r := f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true, Merge: true}), "land_review_not_open"); r.PRURL != nil {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(filepath.Join(g.dir, "log")); err == nil {
		t.Fatalf("gh ran: %q", g.log(t))
	}
	reviewStop(t, f, g, Options{Branch: "feat", BranchSet: true})
	g.set(t, "list", "["+strings.Replace(prJSON, "%s", f.head, 1)+"]")
	g.set(t, "checks", checks(f.head, "MERGEABLE", run("IN_PROGRESS", "")))
	if r := f.refused(t, f.land(t, Options{Merge: true}), "land_checks_pending"); *r.LandStep != "checks" || g.merged() {
		t.Fatalf("%+v", r)
	}
	g.set(t, "checks", checks(f.head, "MERGEABLE", run("COMPLETED", "SUCCESS")))
	if out := f.land(t, Options{Merge: true}); out.Error != nil || f.load(t).Status != task.Landed || !g.merged() {
		t.Fatalf("%+v", out.Error)
	}
}

func TestLandMergeAppliesOnlyToPRMode(t *testing.T) {
	f, g := setupPR(t, task.ModePR)
	record := filepath.Join(f.repo, ".fledge", "state", "tasks", f.id+".json")
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{task.ModeLocal, task.ModeAuto} {
		// Refused at validation, before the caller is looked up.
		out := f.landAs(t, tasktest.Client(t, f.repo, "w1:p1"), Options{Mode: mode, ModeSet: true, Merge: true, Branch: "feat", BranchSet: true})
		if out.Error == nil || out.Error.Code != "invalid_input" || out.Error.Message != "--merge applies only to pr mode" {
			t.Fatalf("%s: %+v", mode, out.Error)
		}
	}
	if after, _ := os.ReadFile(record); !bytes.Equal(after, before) {
		t.Fatalf("record changed: %s", after)
	}
	// Without --mode, the stored mode decides.
	for _, mode := range []string{task.ModeLocal, task.ModeAuto} {
		id := tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &f.worker.ID, VerifiedCommit: &f.head, DeliveryMode: mode, PRURL: tasktest.Ptr("https://example.com/pr/7")})
		out := f.land(t, Options{ID: id, Branch: "feat", BranchSet: true, Merge: true})
		if out.Error == nil || out.Error.Code != "invalid_input" || out.Error.Message != "--merge applies only to pr mode" {
			t.Fatalf("%s: %+v", mode, out.Error)
		}
		if r := tasktest.Load(t, f.repo, id); r.Status != task.Verified || r.LandStep != nil {
			t.Fatalf("%+v", r)
		}
	}
	if _, err := os.Stat(filepath.Join(g.dir, "log")); err == nil {
		t.Fatalf("gh ran: %q", g.log(t))
	}
}

// githubMerge stops a pr task for review, then merges on origin as GitHub
// would: it pushes a commit with parents to main on origin and deletes the
// branch there. The local view of origin/main stays stale unless fetched.
func githubMerge(t *testing.T, fetched bool, parents ...string) (fixture, fakeGH, string) {
	t.Helper()
	f, g := setupPR(t, task.ModePR)
	reviewStop(t, f, g, Options{Branch: "feat", BranchSet: true})
	g.set(t, "list", "["+strings.Replace(prJSON, "%s", f.head, 1)+"]")
	os.Remove(filepath.Join(g.dir, "log"))
	stale := git(t, f.repo, "rev-parse", "refs/remotes/origin/main")
	args := []string{"commit-tree", f.head + "^{tree}", "-m", "merged on GitHub"}
	for _, p := range parents {
		args = append(args, "-p", p)
	}
	merge := git(t, f.repo, args...)
	git(t, f.repo, "push", "-q", "origin", merge+":refs/heads/main", ":refs/heads/feat")
	if !fetched {
		git(t, f.repo, "update-ref", "refs/remotes/origin/main", stale)
	}
	return f, g, merge
}

// noPushOrPR checks that land pushed nothing and ran no gh pr create, list,
// or merge.
func noPushOrPR(t *testing.T, f fixture, g fakeGH) {
	t.Helper()
	origin := git(t, f.repo, "remote", "get-url", "origin")
	if out, err := exec.Command("git", "-C", origin, "rev-parse", "--verify", "-q", "refs/heads/feat").Output(); err == nil {
		t.Fatalf("pushed feat: %s", out)
	}
	for _, l := range g.log(t) {
		for _, p := range []string{"pr create", "pr list", "pr merge"} {
			if strings.HasPrefix(l, p) {
				t.Fatalf("ran gh %s: %q", p, g.log(t))
			}
		}
	}
}

// TestLandPRRecordsGitHubMerge records a review-stage pull request merged
// with the GitHub button, whether or not origin/main was fetched.
func TestLandPRRecordsGitHubMerge(t *testing.T) {
	for _, fetched := range []bool{false, true} {
		t.Run(fmt.Sprint("fetched=", fetched), func(t *testing.T) {
			f, g, _ := githubMerge(t, fetched, "main", "feat")
			merge := git(t, git(t, f.repo, "remote", "get-url", "origin"), "rev-parse", "main")
			g.set(t, "view", view("MERGED", merge, f.head))
			out := f.land(t, Options{})
			if out.Error != nil {
				t.Fatalf("%+v", out.Error)
			}
			r := f.load(t)
			if r.Status != task.Landed || *r.MergeCommit != merge || *r.TargetBranch != "main" || r.LandedAt == nil || *r.LandedBy != f.boss.ID ||
				*r.PRURL != "https://example.com/pr/7" || *r.LandStep != "confirm" {
				t.Fatalf("%+v", r)
			}
			if got := git(t, f.repo, "rev-parse", "refs/remotes/origin/main"); got != merge {
				t.Fatalf("origin/main not fetched: %s", got)
			}
			if !slices.Contains(g.log(t), "pr view https://example.com/pr/7 --json=state,mergeCommit,headRefOid") {
				t.Fatalf("%q", g.log(t))
			}
			noPushOrPR(t, f, g)
			// Fetched, origin/main already contains the verified commit.
			d := small
			if fetched {
				d = reviewsize.Diff{}
			}
			want := "Landed task " + f.id + ": merged feat into main (pr) as " + merge + ".\nPull request: https://example.com/pr/7\n" + sizeBlock(reviewsize.KindPR, "refs/remotes/origin/main", "", d)
			if got := render(t, out); got != want || len(out.Result.(Result).Warnings) != 0 {
				t.Fatalf("%q", got)
			}
		})
	}
}

// TestLandPRRefusesGitHubMergeOrCloseElsewhere refuses a pull request merged
// at another head or closed unmerged, and records nothing.
func TestLandPRRefusesGitHubMergeOrCloseElsewhere(t *testing.T) {
	const other = "2222222222222222222222222222222222222222"
	for _, tc := range []struct{ name, code, view string }{
		{"other head", "land_pr_merged_other_head", view("MERGED", mergeHash, other)},
		{"closed", "land_pr_closed", view("CLOSED", "", "HEAD")},
		{"missing head", "land_gh_output_invalid", `{"state":"MERGED","mergeCommit":{"oid":"` + mergeHash + `"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, g, _ := githubMerge(t, false, "main")
			g.set(t, "view", strings.ReplaceAll(tc.view, `"HEAD"`, `"`+f.head+`"`))
			out := f.land(t, Options{})
			r := f.refused(t, out, tc.code)
			if r.Status != task.Verified || r.MergeCommit != nil || r.LandedAt != nil || *r.LandStep != "review" {
				t.Fatalf("%+v", r)
			}
			if tc.code == "land_pr_merged_other_head" && (!strings.Contains(out.Error.Message, other) || !strings.Contains(out.Error.Message, f.head)) {
				t.Fatal(out.Error.Message)
			}
			noPushOrPR(t, f, g)
		})
	}
}

// TestLandPRWarnsOnSquashMerge records a GitHub squash or rebase merge, whose
// merge commit does not contain the verified commit, with a warning.
func TestLandPRWarnsOnSquashMerge(t *testing.T) {
	f, g, merge := githubMerge(t, false, "main")
	g.set(t, "view", view("MERGED", merge, f.head))
	out := f.land(t, Options{})
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if r := f.load(t); r.Status != task.Landed || *r.MergeCommit != merge {
		t.Fatalf("%+v", r)
	}
	const warning = "verified commit is not in the target history; agent cleanup will hold this checkout"
	if w := out.Result.(Result).Warnings; !slices.Equal(w, []string{warning}) {
		t.Fatalf("%q", w)
	}
	if got := render(t, out); !strings.Contains(got, "Warning: "+warning+".\n") {
		t.Fatalf("%q", got)
	}
}

// TestLandPRWithoutPRKeepsAlreadyMerged refuses a pr task with no pull request
// whose commit the target already contains.
func TestLandPRWithoutPRKeepsAlreadyMerged(t *testing.T) {
	f, g := setupPR(t, task.ModePR)
	git(t, f.repo, "push", "-q", "origin", "feat:main")
	f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_already_merged")
	if _, err := os.Stat(filepath.Join(g.dir, "log")); err == nil {
		t.Fatalf("gh ran: %q", g.log(t))
	}
}

// TestLandPRBodyHasSize: the pull request body has the size report after
// the verification note.
func TestLandPRBodyHasSize(t *testing.T) {
	f, g := setupPR(t, task.ModeAuto)
	f.id = tasktest.Seed(t, f.repo, task.Record{Title: "Feature", Brief: "b", Status: task.Verified, Owner: &f.worker.ID, Verifier: &f.boss.ID,
		VerifiedAt: task.Now(), VerifiedCommit: &f.head, VerificationNote: tasktest.Ptr("looks good"), DeliveryMode: task.ModeAuto, CreatedAt: *task.Now()})
	if out := f.land(t, Options{Branch: "feat", BranchSet: true}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	body, _ := os.ReadFile(filepath.Join(g.dir, "body"))
	var b strings.Builder
	reviewsize.Render(&b, reviewsize.Compare(small, reviewsize.KindPR))
	want := "Verification note:\n\nlooks good\n\nReview size against refs/remotes/origin/main:\n\n```\n" + b.String() + "```\n"
	if !strings.HasSuffix(string(body), want) {
		t.Fatalf("%q\nwant suffix %q", body, want)
	}
}
