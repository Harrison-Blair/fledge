package size

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/reviewsize"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// fixture is a repository whose origin/main is one commit behind main
// (main.go, 3 lines), and branch feat one commit ahead of main (a.go,
// 2 lines). Against refs/heads/main feat changes 2 lines in 1 file;
// against refs/remotes/origin/main, 5 lines in 2 files.
type fixture struct{ repo, head string }

func commit(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gittest.Git(t, dir, "add", name)
	gittest.Git(t, dir, "commit", "-qm", name)
	return strings.TrimSpace(gittest.Git(t, dir, "rev-parse", "HEAD"))
}

func setup(t *testing.T) fixture {
	t.Helper()
	repo := identitytest.Repository(t)
	gittest.Git(t, repo, "symbolic-ref", "HEAD", "refs/heads/main")
	commit(t, repo, "README.md", "r\n")
	gittest.Git(t, repo, "update-ref", "refs/remotes/origin/main", "main")
	commit(t, repo, "main.go", "package main\n\nfunc main() {}\n")
	gittest.Git(t, repo, "checkout", "-qb", "feat")
	head := commit(t, repo, "a.go", "package main\nvar a = 1\n")
	gittest.Git(t, repo, "checkout", "-q", "main")
	return fixture{repo: repo, head: head}
}

func (f fixture) seed(t *testing.T, r task.Record) string {
	t.Helper()
	if r.Title == "" {
		r.Title, r.Brief, r.Status, r.CreatedAt = "t", "b", task.Completed, "2026-01-01T00:00:00Z"
	}
	return tasktest.Seed(t, f.repo, r)
}

func (f fixture) run(t *testing.T, id string) cli.Outcome {
	t.Helper()
	return Run(context.Background(), tasktest.Client(t, f.repo, ""), Options{ID: id})
}

func human(t *testing.T, out cli.Outcome) string {
	t.Helper()
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestSizeLocalModesUseLocalBaseAndTaskLimits(t *testing.T) {
	f := setup(t)
	branch := "feat"
	for _, mode := range []string{task.ModeLocal, task.ModeNone} {
		out := f.run(t, f.seed(t, task.Record{DeliveryMode: mode, Branch: &branch}))
		r, ok := out.Result.(Result)
		if out.Error != nil || !ok || out.Operation != "task.size" {
			t.Fatalf("%s: %+v", mode, out)
		}
		if r.Base != "refs/heads/main" || r.Size.Kind != reviewsize.KindTask || r.Size.SourceLines.Value != 2 || r.Size.Files.Value != 1 {
			t.Fatalf("%s: %+v", mode, r)
		}
		want := "branch: feat\ntarget: main\nmode: " + mode + " (task limits)\nbase: refs/heads/main\nhead: " + f.head + "\n" +
			"task size\n" +
			"  source lines     2  limit 200                within\n" +
			"  files            1  limit 20                 within\n" +
			"  code domains     1  limit 3                  within\n" +
			"  total read       2  limit 750                within\n" +
			"  domains: module root\n"
		if got := human(t, out); got != want {
			t.Fatalf("%s:\n%s\nwant:\n%s", mode, got, want)
		}
	}
}

func TestSizePRModesUseOriginBaseAndPRLimits(t *testing.T) {
	f := setup(t)
	branch := "feat"
	for _, mode := range []string{task.ModePR, task.ModeAuto} {
		out := f.run(t, f.seed(t, task.Record{DeliveryMode: mode, Branch: &branch}))
		r, _ := out.Result.(Result)
		if out.Error != nil || r.Base != "refs/remotes/origin/main" || r.Size.Kind != reviewsize.KindPR || r.Size.SourceLines.Value != 5 || r.Size.Files.Value != 2 {
			t.Fatalf("%s: %+v %+v", mode, out.Error, r)
		}
		got := human(t, out)
		if !strings.Contains(got, "mode: "+mode+" (pull request target and ceiling)\nbase: refs/remotes/origin/main\n") ||
			!strings.Contains(got, "  source lines     5  target 200, ceiling 300  within\n") {
			t.Fatalf("%s:\n%s", mode, got)
		}
	}
}

func TestSizeNotesBaseFallback(t *testing.T) {
	f := setup(t)
	gittest.Git(t, f.repo, "update-ref", "-d", "refs/remotes/origin/main")
	branch := "feat"
	out := f.run(t, f.seed(t, task.Record{DeliveryMode: task.ModePR, Branch: &branch}))
	r, _ := out.Result.(Result)
	if out.Error != nil || r.Base != "refs/heads/main" || r.BaseMissing == nil || *r.BaseMissing != "refs/remotes/origin/main" || r.Size.SourceLines.Value != 2 {
		t.Fatalf("%+v %+v", out.Error, r)
	}
	if got := human(t, out); !strings.Contains(got, "\nbase: refs/heads/main (refs/remotes/origin/main is missing)\n") {
		t.Fatal(got)
	}
}

func TestSizeUsesVerifiedCommitAndTaskBase(t *testing.T) {
	f := setup(t)
	gittest.Git(t, f.repo, "checkout", "-q", "feat")
	commit(t, f.repo, "b.go", "package main\n")
	gittest.Git(t, f.repo, "checkout", "-q", "main")
	branch, base := "feat", "refs/heads/main"
	// The verified commit, not the newer branch head, is measured.
	out := f.run(t, f.seed(t, task.Record{DeliveryMode: task.ModeLocal, Branch: &branch, BaseBranch: &base, VerifiedCommit: &f.head}))
	r, _ := out.Result.(Result)
	if out.Error != nil || r.Head != f.head || r.Target != "main" || r.Size.Files.Value != 1 {
		t.Fatalf("%+v %+v", out.Error, r)
	}
	// Without one, the branch head is measured.
	out = f.run(t, f.seed(t, task.Record{DeliveryMode: task.ModeLocal, Branch: &branch}))
	r, _ = out.Result.(Result)
	if out.Error != nil || r.Head == f.head || r.Size.Files.Value != 2 {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}

func TestSizeOverLimitStillSucceeds(t *testing.T) {
	f := setup(t)
	gittest.Git(t, f.repo, "checkout", "-q", "feat")
	commit(t, f.repo, "big.go", strings.Repeat("// x\n", 800))
	gittest.Git(t, f.repo, "checkout", "-q", "main")
	branch := "feat"
	out := f.run(t, f.seed(t, task.Record{DeliveryMode: task.ModeLocal, Branch: &branch}))
	r, _ := out.Result.(Result)
	if out.Error != nil || out.Status != "success" || r.Size.SourceLines.Status != reviewsize.OverCeiling || r.Size.TotalLines.Status != reviewsize.OverCeiling {
		t.Fatalf("%+v %+v", out, r)
	}
}

func TestSizeJSON(t *testing.T) {
	f := setup(t)
	branch := "feat"
	out := f.run(t, f.seed(t, task.Record{DeliveryMode: task.ModeAuto, Branch: &branch}))
	var b bytes.Buffer
	if err := out.Write(&b, true, Render); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Operation string `json:"operation"`
		Status    string `json:"status"`
		Result    struct {
			Branch, Target, Base, Head, Mode string
			BaseMissing                      *string `json:"base_missing"`
			Size                             reviewsize.Report
		} `json:"result"`
	}
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	r := got.Result
	if got.Operation != "task.size" || got.Status != "success" || r.Branch != "feat" || r.Target != "main" || r.Head != f.head || r.Mode != task.ModeAuto ||
		r.Base != "refs/remotes/origin/main" || r.BaseMissing != nil || r.Size.Kind != reviewsize.KindPR || r.Size.SourceLines.Value != 5 {
		t.Fatalf("%s", b.String())
	}
}

func TestSizeRefusals(t *testing.T) {
	f := setup(t)
	bad, missing := "-dev", "nope"
	for _, c := range []struct {
		id, code string
	}{
		{"0123abcd", "task_not_found"},
		{f.seed(t, task.Record{DeliveryMode: task.ModeLocal}), "size_branch_unknown"},
		{f.seed(t, task.Record{DeliveryMode: task.ModeLocal, Branch: tasktest.Ptr("feat"), BaseBranch: &bad}), "size_target_invalid"},
		{f.seed(t, task.Record{DeliveryMode: task.ModeLocal, Branch: tasktest.Ptr("feat"), BaseBranch: &missing}), "size_target_missing"},
	} {
		if out := f.run(t, c.id); out.Error == nil || out.Error.Code != c.code {
			t.Fatalf("%s: %+v", c.code, out.Error)
		}
	}
	if out := f.run(t, ""); out.Error == nil || out.Error.Code != "invalid_input" {
		t.Fatalf("%+v", out.Error)
	}
}

func TestSizeIsReadOnlyWithoutHerdr(t *testing.T) {
	t.Setenv("HERDR_SOCKET_PATH", "")
	t.Setenv("HERDR_ENV", "")
	f := setup(t)
	id := f.seed(t, task.Record{DeliveryMode: task.ModeLocal, Branch: tasktest.Ptr("feat")})
	path := filepath.Join(f.repo, ".fledge", "state", "tasks", id+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if out := Run(context.Background(), libagent.Client{Cwd: f.repo}, Options{ID: id}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("record changed: %v\n%s\n%s", err, before, after)
	}
}
