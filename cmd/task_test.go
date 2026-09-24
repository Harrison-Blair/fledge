package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestTaskHelp(t *testing.T) {
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"task", "--help"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"create", "depend", "assign", "complete", "verify", "cancel", "list", "get"} {
		if !strings.Contains(out.String(), "\n  "+name+" ") {
			t.Fatalf("%s: %s", name, out.String())
		}
		var sub bytes.Buffer
		if err := ExecuteWithArgs([]string{"task", name, "--help"}, &sub); err != nil || !strings.Contains(sub.String(), "Usage:") || !strings.Contains(sub.String(), "--json") {
			t.Fatalf("%s: %v %s", name, err, sub.String())
		}
	}
	if !strings.Contains(out.String(), "never derived from Herdr") {
		t.Fatal(out.String())
	}
}

func TestTaskJSONValidation(t *testing.T) {
	for _, args := range [][]string{
		{"task", "create", "--body", "b", "--json"},
		{"task", "create", "--title", "t", "--json"},
		{"task", "create", "extra", "--json"},
		{"task", "assign", "--id", "0123abcd", "--json"},
		{"task", "assign", "--id", "0123abcd", "--name", "a", "--agent-id", "0000beef", "--json"},
		{"task", "complete", "--id", "0123abcd", "--json"},
		{"task", "verify", "--id", "zz", "--json"},
		{"task", "verify", "--id", "0123abcd", "--bad", "--json"},
		{"task", "cancel", "--json"},
		{"task", "list", "--status", "done", "--json"},
		{"task", "list", "--parent", "goal", "--json"},
		{"task", "create", "--title", "t", "--body", "b", "--parent", "goal", "--json"},
		{"task", "get", "--json"},
		{"task", "depend", "--id", "0123abcd", "--json"},
		{"task", "depend", "--id", "0123abcd", "--after", "nope", "--json"},
		{"task", "create", "--title", "t", "--body", "b", "--after", "nope", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			err := ExecuteWithArgs(args, &out)
			var status interface{ ExitCode() int }
			if !errors.As(err, &status) || status.ExitCode() != 2 {
				t.Fatalf("wrong exit: %v", err)
			}
			var envelope libagent.Outcome
			if err = json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("not one JSON object: %q: %v", out.String(), err)
			}
			if envelope.Status != "rejected" || !strings.HasPrefix(envelope.Operation, "task.") {
				t.Fatal(out.String())
			}
		})
	}
}

// A grouping parent nobody worked on is verified once its subtasks are done.
func TestTaskVerifyFinishedCreatedParentCLI(t *testing.T) {
	l := newSocket(t)
	gitRepo(t)
	repo, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	boss := tasktest.Agent("w1:p1", "term_boss", "boss")
	rec := tasktest.Register(t, repo, boss)
	id := tasktest.Seed(t, repo, task.Record{Title: "goal", Status: task.Created})
	tasktest.Seed(t, repo, task.Record{Title: "a", Status: task.Verified, Parent: &id})
	tasktest.Seed(t, repo, task.Record{Title: "b", Status: task.Cancelled, Parent: &id})
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	done := serveRPCs(l, boss)
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"task", "verify", "--id", id, "--summary", "grouping done"}, &out); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if calls := waitCalls(t, l, done, 1); calls[0].Method != "agent.get" {
		t.Fatalf("%+v", calls)
	}
	if out.String() != "Verified task "+id+" as "+rec.ID+".\n" {
		t.Fatalf("%q", out.String())
	}
	if r := tasktest.Load(t, repo, id); r.Status != task.Verified || r.Forced || *r.VerificationNote != "grouping done" {
		t.Fatalf("%+v", r)
	}
}

// task create rejects a brief off the template unless --freeform.
func TestTaskCreateBriefTemplateCLI(t *testing.T) {
	t.Chdir(t.TempDir())
	gitRepo(t)
	t.Setenv("HERDR_PANE_ID", "")
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"task", "create", "--title", "T", "--body", "one line", "--json"}, &out)
	var status interface{ ExitCode() int }
	if !errors.As(err, &status) || status.ExitCode() != 1 {
		t.Fatalf("wrong exit: %v %s", err, out.String())
	}
	var envelope libagent.Outcome
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || envelope.Error == nil || envelope.Error.Code != "task_brief_incomplete" || envelope.Error.Phase != "validation" ||
		!strings.Contains(envelope.Error.Message, "missing sections: Objective, Acceptance criteria, Scope, Known facts, Deliverables, Constraints") {
		t.Fatalf("%v %s", err, out.String())
	}
	out.Reset()
	if err := ExecuteWithArgs([]string{"task", "create", "--title", "T", "--body", "one line", "--freeform"}, &out); err != nil || !strings.HasPrefix(out.String(), "Created task ") {
		t.Fatalf("%v %s", err, out.String())
	}
	out.Reset()
	if err := ExecuteWithArgs([]string{"task", "create", "--help"}, &out); err != nil || !strings.Contains(out.String(), "--freeform") || !strings.Contains(out.String(), "fledge task template") {
		t.Fatalf("%v %s", err, out.String())
	}
}
