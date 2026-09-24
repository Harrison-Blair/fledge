package taskimport

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// taskTOML is one [[tasks]] entry with a template brief.
func taskTOML(key, title string, after ...string) string {
	quoted := []string{}
	for _, a := range after {
		quoted = append(quoted, `"`+a+`"`)
	}
	return "\n[[tasks]]\nkey = \"" + key + "\"\ntitle = \"" + title + "\"\nafter = [" + strings.Join(quoted, ", ") + "]\nbrief = '''\n" + tasktest.Brief() + "'''\n"
}

const parentTOML = "\n[parent]\ntitle = \"Templates\"\nbrief = '''\n"

// write stores a proposal file in a fresh directory and returns its path.
func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func count(t *testing.T, repo string) int {
	t.Helper()
	s, err := task.Existing(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := task.List(s)
	if err != nil {
		t.Fatal(err)
	}
	return len(rs)
}

func render(t *testing.T, out libagent.Outcome) string {
	t.Helper()
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestImportDryRunCreatesNothing(t *testing.T) {
	repo := identitytest.Repository(t)
	goal := tasktest.Seed(t, repo, task.Record{Title: "goal", Status: task.Assigned})
	research := tasktest.Seed(t, repo, task.Record{Title: "research", Status: task.Completed})
	path := write(t, "schema_version = 1\n"+taskTOML("import", "Add task import", "brief-lib", research)+taskTOML("brief-lib", "Add brief template validation"))
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: path, FileSet: true, Parent: goal, DryRun: true}, strings.NewReader(""))
	if out.Error != nil || out.Status != "success" || out.Operation != "task.import" || len(out.Effects) != 0 {
		t.Fatalf("%+v %+v", out.Error, out.Effects)
	}
	if count(t, repo) != 2 {
		t.Fatal("dry run created tasks")
	}
	want := "Would create 2 tasks under " + goal + ":\n" +
		"  brief-lib  Add brief template validation\n" +
		"  import     Add task import  (after: brief-lib, " + research + ")\n"
	if got := render(t, out); got != want {
		t.Fatalf("%q", got)
	}
	b, err := json.Marshal(out.Result)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON := `{"parent":"` + goal + `","tasks":[{"key":"brief-lib","id":null,"title":"Add brief template validation","after":[]},{"key":"import","id":null,"title":"Add task import","after":["brief-lib","` + research + `"]}],"dry_run":true}`
	if string(b) != wantJSON {
		t.Fatalf("%s", b)
	}
}

func TestImportDryRunWithNewParent(t *testing.T) {
	repo := identitytest.Repository(t)
	path := write(t, "schema_version = 1\n"+parentTOML+tasktest.Brief()+"'''\n"+taskTOML("only", "Only task"))
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: path, FileSet: true, DryRun: true}, strings.NewReader(""))
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if got := render(t, out); got != "Would create parent task: Templates\nWould create 1 task under it:\n  only  Only task\n" {
		t.Fatalf("%q", got)
	}
	if r := out.Result.(Result); r.Parent != nil || !r.DryRun {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(filepath.Join(repo, ".fledge")); err == nil {
		t.Fatal("dry run created state")
	}
}

func TestImportCreatesParentAndTasksInOrder(t *testing.T) {
	repo := identitytest.Repository(t)
	boss := tasktest.Agent("w1:p1", "term_boss", "boss")
	rec := tasktest.Register(t, repo, boss)
	research := tasktest.Seed(t, repo, task.Record{Title: "research", Status: task.Assigned})
	path := write(t, "schema_version = 1\n"+parentTOML+tasktest.Brief()+"'''\n"+
		taskTOML("import", "Add task import", "brief-lib", research)+taskTOML("brief-lib", "Add brief template validation"))
	c := tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss))
	out := Run(context.Background(), c, Options{File: path, FileSet: true}, strings.NewReader(""))
	if out.Error != nil || out.Status != "success" {
		t.Fatalf("%+v", out.Error)
	}
	r := out.Result.(Result)
	if r.Parent == nil || r.DryRun || len(r.Tasks) != 2 || r.Tasks[0].Key != "brief-lib" || r.Tasks[1].Key != "import" || r.Tasks[0].ID == nil || r.Tasks[1].ID == nil {
		t.Fatalf("%+v", r)
	}
	parent, lib, imp := *r.Parent, *r.Tasks[0].ID, *r.Tasks[1].ID
	if !reflect.DeepEqual(r.Tasks[1].After, []string{lib, research}) {
		t.Fatalf("%+v", r.Tasks[1].After)
	}
	var effects []string
	for _, e := range out.Effects {
		if e.Kind == "task" {
			effects = append(effects, e.Action+" "+e.ID)
		}
	}
	if !reflect.DeepEqual(effects, []string{"created " + parent, "created " + lib, "created " + imp}) {
		t.Fatalf("%+v", out.Effects)
	}
	p := tasktest.Load(t, repo, parent)
	if p.Title != "Templates" || p.Brief != tasktest.Brief() || p.Parent != nil || p.Status != task.Created || p.CreatedBy == nil || *p.CreatedBy != rec.ID {
		t.Fatalf("%+v", p)
	}
	for id, after := range map[string][]string{lib: nil, imp: {lib, research}} {
		got := tasktest.Load(t, repo, id)
		if got.Parent == nil || *got.Parent != parent || !reflect.DeepEqual(got.After, after) || got.Status != task.Created || got.CreatedBy == nil || *got.CreatedBy != rec.ID || got.Brief != tasktest.Brief() {
			t.Fatalf("%s: %+v", id, got)
		}
	}
	want := "Created task " + parent + ": Templates\n" +
		"Created task " + lib + " (brief-lib): Add brief template validation\n" +
		"Created task " + imp + " (import): Add task import\n" +
		"Created 2 tasks under parent " + parent + "\n"
	if got := render(t, out); got != want {
		t.Fatalf("%q", got)
	}
}

func TestImportUnderExistingParent(t *testing.T) {
	repo := identitytest.Repository(t)
	goal := tasktest.Seed(t, repo, task.Record{Title: "goal", Status: task.Completed})
	path := write(t, "schema_version = 1\n"+taskTOML("only", "Only task"))
	out := Run(context.Background(), tasktest.Client(t, repo, ""), Options{File: path, FileSet: true, Parent: goal}, strings.NewReader(""))
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	r := out.Result.(Result)
	got := tasktest.Load(t, repo, *r.Tasks[0].ID)
	if *r.Parent != goal || got.Parent == nil || *got.Parent != goal || got.CreatedBy != nil || len(out.Effects) == 0 {
		t.Fatalf("%+v %+v", r, got)
	}
	if text := render(t, out); text != "Created task "+got.ID+" (only): Only task\n" {
		t.Fatalf("%q", text)
	}
}

func TestImportRejectsStoreProblemsUnchanged(t *testing.T) {
	repo := identitytest.Repository(t)
	verified := tasktest.Seed(t, repo, task.Record{Title: "v", Status: task.Verified})
	cancelled := tasktest.Seed(t, repo, task.Record{Title: "c", Status: task.Cancelled})
	flat := write(t, "schema_version = 1\n"+taskTOML("a", "A")+taskTOML("b", "B", "a"))
	missingAfter := write(t, "schema_version = 1\n"+taskTOML("a", "A")+taskTOML("b", "B", "a", "0123abcd"))
	withParent := write(t, "schema_version = 1\n"+parentTOML+tasktest.Brief()+"'''\n"+taskTOML("a", "A"))
	for label, tc := range map[string]struct {
		o    Options
		code string
	}{
		"verified parent":  {Options{File: flat, Parent: verified}, "task_invalid_state"},
		"cancelled parent": {Options{File: flat, Parent: cancelled}, "task_invalid_state"},
		"unknown parent":   {Options{File: flat, Parent: "0123abcd"}, "task_not_found"},
		"unknown after":    {Options{File: missingAfter}, "task_not_found"},
		"both parents":     {Options{File: withParent, Parent: verified}, "invalid_input"},
		"bad parent id":    {Options{File: flat, Parent: "BAD"}, "invalid_input"},
	} {
		for _, dry := range []bool{false, true} {
			tc.o.FileSet, tc.o.DryRun = true, dry
			out := Run(context.Background(), tasktest.Client(t, repo, ""), tc.o, strings.NewReader(""))
			if out.Error == nil || out.Error.Code != tc.code || out.Status != "rejected" || len(out.Effects) != 0 || count(t, repo) != 2 {
				t.Fatalf("%s dry=%v: %+v %+v", label, dry, out.Error, out.Effects)
			}
			if phase := map[bool]string{true: "validation", false: "task"}[tc.code == "invalid_input"]; out.Error.Phase != phase {
				t.Fatalf("%s dry=%v: %+v", label, dry, out.Error)
			}
		}
	}
}

// Validation needs no repository: a plain directory proves the store is never
// opened.
func TestImportValidatesBeforeStore(t *testing.T) {
	dir := t.TempDir()
	for label, tc := range map[string]struct {
		o       Options
		code    string
		exit    int
		message string
	}{
		"no file":        {Options{}, "invalid_input", 2, "--file is required"},
		"bad toml":       {Options{File: write(t, "schema_version = "), FileSet: true}, "invalid_input", 2, ""},
		"unknown key":    {Options{File: write(t, "schema_version = 1\nextra = 1\n"+taskTOML("a", "A")), FileSet: true}, "invalid_input", 2, `unknown key "extra"`},
		"cycle":          {Options{File: write(t, "schema_version = 1\n"+taskTOML("a", "A", "b")+taskTOML("b", "B", "a")), FileSet: true}, "invalid_input", 2, "dependency cycle"},
		"no version":     {Options{File: write(t, taskTOML("a", "A")), FileSet: true}, "invalid_input", 2, "schema_version is required"},
		"brief template": {Options{File: write(t, "schema_version = 1\n"+taskTOML("a", "A")+"\n[[tasks]]\nkey = \"import\"\ntitle = \"I\"\nbrief = \"one line\"\n"), FileSet: true}, "task_brief_incomplete", 1, `task "import": `},
	} {
		out := Run(context.Background(), libagent.Client{Cwd: dir}, tc.o, strings.NewReader(""))
		if out.Error == nil || out.Error.Code != tc.code || out.Error.Phase != "validation" || out.ExitCode() != tc.exit || !strings.Contains(out.Error.Message, tc.message) {
			t.Fatalf("%s: %+v", label, out.Error)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".fledge")); err == nil {
		t.Fatal("state created")
	}
}

func TestImportReadsStdin(t *testing.T) {
	out := Run(context.Background(), libagent.Client{Cwd: t.TempDir()}, Options{File: "-", FileSet: true, DryRun: true}, strings.NewReader("schema_version = 1\n"+taskTOML("a", "A")))
	if out.Error != nil || len(out.Result.(Result).Tasks) != 1 {
		t.Fatalf("%+v", out.Error)
	}
}
