package land

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// fakeGH is a gh on PATH that logs its arguments and answers from files in
// dir: list (before a create) and listed (after), checks, merged, and the
// optional merge-fails and list-after-create-fails.
type fakeGH struct{ dir string }

const ghScript = `#!/bin/sh
d=%q
printf '%s\n' "$*" >> "$d/log"
case "$1 $2" in
"pr list") if [ -f "$d/created" ]; then
    if [ -f "$d/list-after-create-fails" ]; then echo "lookup failed" >&2; exit 1; fi
    cat "$d/listed"; else cat "$d/list"; fi ;;
"pr create") touch "$d/created"
  for a in "$@"; do case "$a" in --body-file=*) cp "${a#--body-file=}" "$d/body" ;; esac; done
  echo https://example.com/pr/7 ;;
"pr view") case "$4" in *statusCheckRollup*) cat "$d/checks" ;; *) cat "$d/merged" ;; esac ;;
"pr merge") if [ -f "$d/merge-fails" ]; then echo "merge blocked" >&2; exit 1; fi; touch "$d/did-merge" ;;
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
	g.set(t, "merged", `{"state":"MERGED","mergeCommit":{"oid":"`+mergeHash+`"}}`)
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

// setupPR is setup in pr mode with a bare origin holding main, and a fake gh.
func setupPR(t *testing.T) (fixture, fakeGH) {
	t.Helper()
	f := setup(t, task.ModePR)
	origin := filepath.Join(t.TempDir(), "origin.git")
	git(t, f.repo, "init", "-q", "--bare", origin)
	git(t, f.repo, "remote", "add", "origin", origin)
	git(t, f.repo, "push", "-q", "origin", "main")
	git(t, f.repo, "fetch", "-q", "origin")
	g := installGH(t)
	g.set(t, "listed", "["+strings.Replace(prJSON, "%s", f.head, 1)+"]")
	g.set(t, "checks", checks(f.head, "MERGEABLE", ""))
	return f, g
}

func TestLandPRPushesOpensAndMerges(t *testing.T) {
	f, g := setupPR(t)
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
		!slices.Contains(log, "pr view 7 --json=isDraft,mergeable,headRefOid,statusCheckRollup") || !slices.Contains(log, "pr view 7 --json=state,mergeCommit") {
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
	want := "Landed task " + f.id + ": merged feat into main (pr) as " + mergeHash + ".\nPull request: https://example.com/pr/7\n"
	if got := render(t, out); got != want {
		t.Fatalf("%q", got)
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
			f, g := setupPR(t)
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
	f, g := setupPR(t)
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
	f, g := setupPR(t)
	pr := strings.Replace(prJSON, "%s", f.head, 1)
	g.set(t, "list", "["+pr+","+strings.Replace(pr, `"number":7`, `"number":8`, 1)+"]")
	if r := f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_pr_ambiguous"); *r.LandStep != "pr" {
		t.Fatalf("%+v", r)
	}

	f, g = setupPR(t)
	g.set(t, "merge-fails", "")
	out := f.land(t, Options{Branch: "feat", BranchSet: true})
	if r := f.refused(t, out, "land_merge_failed"); *r.LandStep != "merge" || !strings.Contains(out.Error.Message, "merge blocked") {
		t.Fatalf("%+v %s", r, out.Error.Message)
	}

	f, g = setupPR(t)
	g.set(t, "merged", `{"state":"OPEN","mergeCommit":null}`)
	if r := f.refused(t, f.land(t, Options{Branch: "feat", BranchSet: true}), "land_unconfirmed"); *r.LandStep != "confirm" {
		t.Fatalf("%+v", r)
	}
}

func TestLandPRPushRejected(t *testing.T) {
	f, _ := setupPR(t)
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
	f, _ := setupPR(t)
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
	id := tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &f.worker.ID, VerifiedCommit: &f.head, DeliveryMode: task.ModePR, BaseBranch: tasktest.Ptr("refs/remotes/origin/dev")})
	out := f.land(t, Options{ID: id, Branch: "feat", BranchSet: true})
	if r := tasktest.Load(t, f.repo, id); out.Error != nil || *r.TargetBranch != "dev" {
		t.Fatalf("%+v %+v", out.Error, r)
	}
	id = tasktest.Seed(t, f.repo, task.Record{Title: "t", Status: task.Verified, Owner: &f.worker.ID, VerifiedCommit: &f.head, DeliveryMode: task.ModePR, BaseBranch: tasktest.Ptr("nosuch")})
	if out := f.land(t, Options{ID: id, Branch: "feat", BranchSet: true}); out.Error == nil || out.Error.Code != "land_target_missing" {
		t.Fatalf("%+v", out.Error)
	}
}

func TestLandPRRefusesInvalidTargetBeforeGH(t *testing.T) {
	f, g := setupPR(t)
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
	f, g := setupPR(t)
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
	f, g := setupPR(t)
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
