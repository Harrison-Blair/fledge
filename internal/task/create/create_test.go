package create

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestCreateByRegisteredCaller(t *testing.T) {
	repo := identitytest.Repository(t)
	boss := tasktest.Agent("w1:p1", "term_boss", "boss")
	rec := tasktest.Register(t, repo, boss)
	c := tasktest.Client(t, repo, "w1:p1", tasktest.Get("w1:p1", boss))
	out := Run(context.Background(), c, Options{Title: "Fix it", Body: "do the thing", BodySet: true}, strings.NewReader(""))
	if out.Error != nil || out.Status != "success" || out.Operation != "task.create" {
		t.Fatalf("%+v", out.Error)
	}
	r := out.Result.(task.Record)
	stored := tasktest.Load(t, repo, r.ID)
	if !reflect.DeepEqual(stored, r) {
		t.Fatalf("%+v != %+v", stored, r)
	}
	if r.Title != "Fix it" || r.Brief != "do the thing" || r.Status != task.Created || r.CreatedBy == nil || *r.CreatedBy != rec.ID || r.CreatedAt == "" ||
		r.Owner != nil || r.AssignedAt != nil || r.Delivery != nil || r.Result != nil || r.Verifier != nil || r.Forced {
		t.Fatalf("%+v", r)
	}
	last := out.Effects[len(out.Effects)-1]
	if last.Action != "created" || last.Kind != "task" || last.ID != r.ID {
		t.Fatalf("%+v", out.Effects)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || b.String() != "Created task "+r.ID+": Fix it\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestCreateByUnregisteredCallerFromFile(t *testing.T) {
	repo := identitytest.Repository(t)
	path := filepath.Join(t.TempDir(), "brief.md")
	os.WriteFile(path, []byte("from file"), 0o600)
	c := tasktest.Client(t, repo, "w1:p9", herdrscript.Call{Method: "agent.get", Err: &herdr.Error{Code: "agent_not_found", Message: "none"}})
	out := Run(context.Background(), c, Options{Title: "T", File: path, FileSet: true}, strings.NewReader(""))
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if r := out.Result.(task.Record); r.CreatedBy != nil || r.Brief != "from file" {
		t.Fatalf("%+v", r)
	}
	stdin := Run(context.Background(), tasktest.Client(t, repo, ""), Options{Title: "T", File: "-", FileSet: true}, strings.NewReader("piped"))
	if stdin.Error != nil || stdin.Result.(task.Record).Brief != "piped" {
		t.Fatalf("%+v", stdin)
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	for label, o := range map[string]Options{
		"no title":      {Body: "b", BodySet: true},
		"multiline":     {Title: "a\nb", Body: "b", BodySet: true},
		"no brief":      {Title: "t"},
		"empty brief":   {Title: "t", BodySet: true},
		"body and file": {Title: "t", Body: "b", BodySet: true, File: "-", FileSet: true},
	} {
		repo := identitytest.Repository(t)
		out := Run(context.Background(), tasktest.Client(t, repo, ""), o, strings.NewReader(""))
		if out.Error == nil || out.Error.Code != "invalid_input" || out.ExitCode() != 2 {
			t.Fatalf("%s: %+v", label, out.Error)
		}
		if _, err := os.Stat(filepath.Join(repo, ".fledge")); err == nil {
			t.Fatalf("%s: state created", label)
		}
	}
}
