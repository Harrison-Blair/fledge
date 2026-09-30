package board

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestLoadEmptyDoesNotCreateState(t *testing.T) {
	cwd := identitytest.Repository(t)
	got := Load(context.Background(), libagent.Client{Cwd: cwd}, Tasks)
	if got.Err != nil || len(got.Snapshot.Records) != 0 {
		t.Fatalf("%+v", got)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".fledge")); !os.IsNotExist(err) {
		t.Fatalf("created state: %v", err)
	}
}

func TestProjectionHierarchyDependenciesAndHistory(t *testing.T) {
	rs := []task.Record{
		{ID: "11111111", Status: task.Cancelled, Title: "parent"},
		{ID: "22222222", Status: task.Created, Parent: tasktest.Ptr("11111111"), After: []string{"44444444"}},
		{ID: "33333333", Status: task.Created, Parent: tasktest.Ptr("22222222"), After: []string{"missing"}},
		{ID: "44444444", Status: task.Cancelled},
		{ID: "55555555", Status: task.Completed, Parent: tasktest.Ptr("missing")},
		{ID: "66666666", Status: task.Verified},
	}
	s, err := project(rs)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Active["11111111"] || s.Active["44444444"] {
		t.Fatalf("active: %v", s.Active)
	}
	if s.Nodes["22222222"].State != "created" || s.Nodes["33333333"].State != "waiting" {
		t.Fatalf("states: %+v", s.Nodes)
	}
	if !s.Nodes["55555555"].MissingParent || !strings.Contains(s.Nodes["55555555"].State, "awaiting verification") {
		t.Fatal("missing parent or completed label")
	}
	if s.Nodes["11111111"].Progress != "0/1 verified" {
		t.Fatal(s.Nodes["11111111"])
	}
	rs[0].Parent = tasktest.Ptr("33333333")
	if _, err := project(rs); err == nil {
		t.Fatal("cycle accepted")
	}
}

func TestLoadMalformedAndAgentStorageErrors(t *testing.T) {
	cwd := identitytest.Repository(t)
	id := tasktest.Seed(t, cwd, task.Record{Title: "task", Status: task.Created})
	if err := os.WriteFile(filepath.Join(cwd, ".fledge/state/tasks", id+".json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := Load(context.Background(), libagent.Client{Cwd: cwd}, Tasks); got.Err == nil {
		t.Fatal("accepted malformed task")
	}
	a := tasktest.Agent("w1:p1", "term", "live")
	rec := tasktest.Register(t, cwd, a)
	if err := os.WriteFile(filepath.Join(cwd, ".fledge/state/agents", rec.ID+".json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	c := tasktest.Client(t, cwd, "", herdrscript.Call{Method: "agent.list", Result: herdr.AgentListResult{Type: "agent_list", Agents: []herdr.AgentDetails{a.Agent}}})
	if got := Load(context.Background(), c, Workers); got.Err == nil {
		t.Fatal("agent storage error swallowed")
	}
}

func TestLoadWorkersUsesCurrentAttributionReadOnly(t *testing.T) {
	cwd := identitytest.Repository(t)
	a := tasktest.Agent("w1:p1", "term", "old")
	rec := tasktest.Register(t, cwd, a)
	check := identitytest.ReadOnly(t, cwd, rec.ID)
	defer check()
	a.Agent.PaneID = "w2:p9"
	a.Agent.Name = tasktest.Ptr("current")
	c := tasktest.Client(t, cwd, "", herdrscript.Call{Method: "agent.list", Result: herdr.AgentListResult{Type: "agent_list", Agents: []herdr.AgentDetails{a.Agent}}})
	got := Load(context.Background(), c, Workers)
	if got.Err != nil || got.Workers[rec.ID].Name != "current" {
		t.Fatalf("%+v", got)
	}
	a.Agent.Agent = tasktest.Ptr("codex")
	c = tasktest.Client(t, cwd, "", herdrscript.Call{Method: "agent.list", Result: herdr.AgentListResult{Type: "agent_list", Agents: []herdr.AgentDetails{a.Agent}}})
	if got = Load(context.Background(), c, Workers); got.Err != nil || len(got.Workers) != 0 {
		t.Fatalf("mismatch: %+v", got)
	}
}

type apiFunc func(context.Context, string, any, any) error

func (f apiFunc) Call(ctx context.Context, m string, p, r any) error { return f(ctx, m, p, r) }
func TestRefreshHasTotalDeadline(t *testing.T) {
	c := libagent.Client{Cwd: identitytest.Repository(t), API: apiFunc(func(ctx context.Context, _ string, _, _ any) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Fatal("missing five-second total deadline")
		}
		return context.DeadlineExceeded
	})}
	if got := Load(context.Background(), c, Workers); got.Err == nil {
		t.Fatal("lost timeout")
	}
}

func TestFocusOwnerRevalidatesAndChecksResponse(t *testing.T) {
	for _, mode := range []string{"ok", "terminal", "pane", "type", "incomplete", "reused", "missing", "changed", "harness"} {
		t.Run(mode, func(t *testing.T) {
			cwd := identitytest.Repository(t)
			a := tasktest.Agent("w1:p1", "term", "worker")
			rec := tasktest.Register(t, cwd, a)
			id := tasktest.Seed(t, cwd, task.Record{Status: task.Assigned, Owner: &rec.ID})
			check := identitytest.ReadOnly(t, cwd, rec.ID)
			defer check()
			listed := a.Agent
			listed.PaneID = "w2:p2"
			if mode == "reused" {
				listed.Agent = tasktest.Ptr("codex")
			}
			if mode == "missing" {
				listed.TerminalID = "other"
			}
			focused := herdr.AgentResult{Type: "agent_info", Agent: listed}
			switch mode {
			case "terminal":
				focused.Agent.TerminalID = "other"
			case "pane":
				focused.Agent.PaneID = "w9:p9"
			case "type":
				focused.Type = "wrong"
			case "incomplete":
				focused.Agent.Revision = nil
			case "harness":
				focused.Agent.Agent = tasktest.Ptr("codex")
			}
			calls := []herdrscript.Call{{Method: "agent.list", Result: herdr.AgentListResult{Type: "agent_list", Agents: []herdr.AgentDetails{listed}}}}
			if mode != "reused" && mode != "missing" && mode != "changed" {
				calls = append(calls, herdrscript.Call{Method: "agent.focus", Params: map[string]any{"target": "w2:p2"}, Result: focused})
			}
			expected := rec.ID
			if mode == "changed" {
				expected = "ffffffff"
				calls = nil
			}
			c := tasktest.Client(t, cwd, "", calls...)
			err := FocusOwner(context.Background(), c, id, expected)
			if (err == nil) != (mode == "ok") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestFocusBudgetShared(t *testing.T) {
	cwd := identitytest.Repository(t)
	a := tasktest.Agent("w1:p1", "term", "worker")
	rec := tasktest.Register(t, cwd, a)
	id := tasktest.Seed(t, cwd, task.Record{Status: task.Assigned, Owner: &rec.ID})
	var first time.Time
	c := libagent.Client{Cwd: cwd, API: apiFunc(func(ctx context.Context, m string, _, r any) error {
		d, ok := ctx.Deadline()
		if !ok || time.Until(d) > 5*time.Second {
			t.Fatal("deadline")
		}
		if m == "agent.list" {
			first = d
			*(r.(*herdr.AgentListResult)) = herdr.AgentListResult{Type: "agent_list", Agents: []herdr.AgentDetails{a.Agent}}
		} else {
			if !d.Equal(first) {
				t.Fatal("budget restarted")
			}
			*(r.(*herdr.AgentResult)) = a
		}
		return nil
	})}
	if err := FocusOwner(context.Background(), c, id, rec.ID); err != nil {
		t.Fatal(err)
	}
}

func TestProjectionRejectsMalformedRecordIdentity(t *testing.T) {
	for _, rs := range [][]task.Record{
		{{Status: task.Created}},
		{{ID: "11111111", Status: "invented"}},
		{{ID: "11111111", Status: task.Created}, {ID: "11111111", Status: task.Assigned}},
	} {
		if _, err := project(rs); err == nil {
			t.Fatalf("accepted malformed records %+v", rs)
		}
	}
}
