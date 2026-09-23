package current

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
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

type call = herdrscript.Call

func TestCurrentShowsRecordParentAndAssignedTasks(t *testing.T) {
	cwd := identitytest.Repository(t)
	parent := tasktest.Register(t, cwd, tasktest.Agent("w1:p1", "term_parent", "orchestrator"))
	me := tasktest.Agent("old:p1", "term_me", "worker")
	rec := identitytest.RegisterChild(t, cwd, me.Agent, parent.ID)
	other := "0000beef"
	for _, r := range []task.Record{
		{Title: "second", Owner: &rec.ID, Status: task.Assigned, CreatedAt: "2026-01-02T00:00:00Z"},
		{Title: "first", Owner: &rec.ID, Status: task.Assigned, CreatedAt: "2026-01-01T00:00:00Z"},
		{Title: "done", Owner: &rec.ID, Status: task.Completed, CreatedAt: "2026-01-01T00:00:00Z"},
		{Title: "theirs", Owner: &other, Status: task.Assigned, CreatedAt: "2026-01-01T00:00:00Z"},
		{Title: "unowned", Status: task.Created, CreatedAt: "2026-01-01T00:00:00Z"},
	} {
		tasktest.Seed(t, cwd, r)
	}
	out := Run(context.Background(), tasktest.Client(t, cwd, "old:p1", tasktest.Get("old:p1", me)))
	if out.Operation != "agent.current" || out.Error != nil {
		t.Fatalf("%+v", out)
	}
	r := out.Result.(Result)
	var titles []string
	for _, tk := range r.Tasks {
		titles = append(titles, tk.Title)
	}
	if r.ID != rec.ID || r.Parent == nil || *r.Parent != parent.ID || r.ParentName == nil || *r.ParentName != "orchestrator" || !reflect.DeepEqual(titles, []string{"first", "second"}) {
		t.Fatalf("%+v", r)
	}
	var b bytes.Buffer
	if err := out.Write(&b, true, Render); err != nil {
		t.Fatal(err)
	}
	var envelope struct{ Result map[string]any }
	if err := json.Unmarshal(b.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]any{"id": rec.ID, "name": "worker", "pane": "old:p1", "workspace_id": "w1", "harness": "claude", "worktree_path": nil, "parent": parent.ID, "parent_name": "orchestrator"} {
		if got := envelope.Result[field]; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %v want %v in %s", field, got, want, b.String())
		}
	}
	if tasks := envelope.Result["tasks"].([]any); len(tasks) != 2 || tasks[0].(map[string]any)["title"] != "first" || tasks[0].(map[string]any)["id"] != r.Tasks[0].ID {
		t.Fatalf("%s", b.String())
	}
	b.Reset()
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"Fledge ID: " + rec.ID, "Name: worker", "Pane: old:p1", "Workspace ID: w1", "Harness: claude", "Worktree: -", "Parent: " + parent.ID + " (orchestrator)", "Assigned tasks:", "  " + r.Tasks[0].ID + "  first", "  " + r.Tasks[1].ID + "  second"} {
		if !strings.Contains(b.String(), line+"\n") {
			t.Fatalf("missing %q in\n%s", line, b.String())
		}
	}
}

func TestCurrentParentWithoutRecordAndNoTasks(t *testing.T) {
	cwd := identitytest.Repository(t)
	me := tasktest.Agent("old:p1", "term_me", "worker")
	identitytest.RegisterChild(t, cwd, me.Agent, "0000dead")
	out := Run(context.Background(), tasktest.Client(t, cwd, "old:p1", tasktest.Get("old:p1", me)))
	r := out.Result.(Result)
	if out.Error != nil || r.ParentName != nil || r.Tasks == nil || len(r.Tasks) != 0 {
		t.Fatalf("%+v", out)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || !strings.Contains(b.String(), "Parent: 0000dead\n") || !strings.Contains(b.String(), "Assigned tasks: none\n") {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestCurrentWithoutParent(t *testing.T) {
	cwd := identitytest.Repository(t)
	me := tasktest.Agent("old:p1", "term_me", "worker")
	tasktest.Register(t, cwd, me)
	out := Run(context.Background(), tasktest.Client(t, cwd, "old:p1", tasktest.Get("old:p1", me)))
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || !strings.Contains(b.String(), "Parent: -\n") {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestCurrentRequiresRegisteredCaller(t *testing.T) {
	registered := identitytest.Repository(t)
	tasktest.Register(t, registered, tasktest.Agent("w1:p1", "term_other", "other"))
	empty := identitytest.Repository(t)
	for _, tc := range []struct {
		name, cwd, pane string
		calls           []call
	}{
		{"unregistered caller", registered, "old:p1", []call{tasktest.Get("old:p1", tasktest.Agent("old:p1", "term_me", "worker"))}},
		{"no agent in caller pane", registered, "old:p1", []call{{Method: "agent.get", Err: &herdr.Error{Code: "agent_not_found", Message: "none"}}}},
		{"outside herdr pane", registered, "", nil},
		{"no state", empty, "old:p1", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := Run(context.Background(), tasktest.Client(t, tc.cwd, tc.pane, tc.calls...))
			if out.ExitCode() != 1 || out.Error.Code != "caller_unregistered" || out.Error.Phase != "identity" || !strings.Contains(out.Error.Message, "fledge agent adopt") {
				t.Fatalf("%+v", out.Error)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(empty, ".fledge")); !os.IsNotExist(err) {
		t.Fatalf("lookup created .fledge: %v", err)
	}
}

func TestOutputFailuresPropagate(t *testing.T) {
	name, parent := "worker", "0000cafe"
	herdrscript.CheckOutputFailures(t, Render,
		libagent.Outcome{Result: Result{Tasks: []Task{}}},
		libagent.Outcome{Result: Result{Record: identityRecord(&name, &parent), ParentName: &name, Tasks: []Task{{ID: "0000beef", Title: "t"}}}},
	)
}

func identityRecord(name, parent *string) identity.Record {
	return identity.Record{ID: "0000abcd", Name: name, Parent: parent}
}
