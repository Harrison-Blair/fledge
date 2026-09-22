package list

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

type call = herdrscript.Call

func TestListIncludesUnnamed(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []herdr.Pane{p}}}), Options{})
	if out.Status != "success" || len(out.Result.(Result).Agents) != 1 {
		t.Fatal(out)
	}
}
func TestRuntimeErrorExit(t *testing.T) {
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "agent.list", Err: errors.New("offline")}), Options{})
	if out.ExitCode() != 1 {
		t.Fatal(out)
	}
}

// TestFailedListSkipsRecords uses a git on PATH that leaves a marker, so a
// record lookup after a failed agent.list is observable.
func TestFailedListSkipsRecords(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "called")
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\n: >"+marker+"\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	for _, tc := range []struct {
		name   string
		result any
		err    error
		looked bool
	}{
		{"success", map[string]any{"type": "agent_list", "agents": []herdr.Pane{}}, nil, true},
		{"runtime", nil, errors.New("offline"), false},
		{"protocol", map[string]any{"type": "wrong"}, nil, false},
		{"incomplete agent", map[string]any{"type": "agent_list", "agents": []herdr.Pane{{PaneID: "w1:p1"}}}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			os.Remove(marker)
			c := herdrscript.Client(t, call{Method: "agent.list", Result: tc.result, Err: tc.err})
			c.Cwd = t.TempDir()
			out := Run(context.Background(), c, Options{})
			_, err := os.Stat(marker)
			if (out.Error == nil) != tc.looked || (err == nil) != tc.looked {
				t.Fatalf("%+v looked=%v", out, err == nil)
			}
		})
	}
}
func TestHumanOperationResults(t *testing.T) {
	row := herdrscript.Row()
	id, parent := "0000beef", "0000cafe"
	for _, tc := range []struct {
		name   string
		result any
		want   string
	}{
		{"empty list", Result{}, "No live agents."},
		{"list", Result{Agents: []Row{{ID: &id, Parent: &parent, AgentRow: row}, {}}}, "ID PARENT NAME HARNESS STATUS WORKSPACE TAB PANE CWD 0000beef 0000cafe worker claude idle w1 w1:t1 w1:p1 /repo - - - - - - - - -"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := (libagent.Outcome{Status: "success", Result: tc.result}).Write(&b, false, Render); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(strings.Fields(b.String()), " "); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
func TestOutputFailuresPropagate(t *testing.T) {
	name := "worker"
	herdrscript.CheckOutputFailures(t, Render,
		libagent.Outcome{Result: Result{}},
		libagent.Outcome{Result: Result{Agents: []Row{{AgentRow: libagent.AgentRow{Name: &name}}}}},
	)
}

func TestListMatchesRecordsByTerminal(t *testing.T) {
	registered := herdrscript.Info(herdrscript.LiveAgent("idle")).Agent
	other := herdrscript.Info(herdrscript.Pane("w1:p4", "w1", "w1:t2")).Agent
	other.AgentStatus, other.TerminalID = "idle", "term_other"
	c := herdrscript.Client(t, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []herdr.AgentDetails{registered, other}}})
	c.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, c.Cwd, registered)
	out := Run(context.Background(), c, Options{})
	rows := out.Result.(Result).Agents
	if out.Error != nil || len(rows) != 2 || rows[0].ID == nil || *rows[0].ID != rec.ID || rows[1].ID != nil {
		t.Fatalf("%+v", out)
	}
	var b bytes.Buffer
	if err := out.Write(&b, true, Render); err != nil || !strings.Contains(b.String(), `"id":"`+rec.ID+`"`) || !strings.Contains(b.String(), `"id":null`) {
		t.Fatalf("%s %v", b.String(), err)
	}
}

// lineage is a live parent in w1:p3, its child in w1:p5, and an unregistered
// agent in w1:p6, all registered in a fresh repository.
type lineage struct {
	cwd                   string
	parent, child, orphan herdr.AgentDetails
	parentID, childID     string
}

func newLineage(t *testing.T) lineage {
	l := lineage{cwd: identitytest.Repository(t)}
	l.parent = herdrscript.Info(herdrscript.LiveAgent("working")).Agent
	l.parent.TerminalID = "term_parent"
	l.child = herdrscript.Info(herdrscript.Pane("w1:p5", "w1", "w1:t2")).Agent
	l.child.AgentStatus, l.child.TerminalID = "idle", "term_child"
	l.orphan = herdrscript.Info(herdrscript.Pane("w1:p6", "w1", "w1:t2")).Agent
	l.orphan.AgentStatus, l.orphan.TerminalID = "idle", "term_orphan"
	l.parentID = identitytest.Register(t, l.cwd, l.parent).ID
	l.childID = identitytest.RegisterChild(t, l.cwd, l.child, l.parentID).ID
	return l
}
func (l lineage) listCall() call {
	return call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []herdr.AgentDetails{l.parent, l.child, l.orphan}}}
}
func ids(rows []Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, libagent.Display(r.ID)+"<"+libagent.Display(r.Parent))
	}
	return out
}

func TestListShowsParent(t *testing.T) {
	l := newLineage(t)
	c := herdrscript.Client(t, l.listCall())
	c.Cwd = l.cwd
	out := Run(context.Background(), c, Options{})
	want := []string{l.parentID + "<-", l.childID + "<" + l.parentID, "-<-"}
	if got := ids(out.Result.(Result).Agents); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v want %v", got, want)
	}
	var b bytes.Buffer
	if err := out.Write(&b, true, Render); err != nil || !strings.Contains(b.String(), `"parent":"`+l.parentID+`"`) || !strings.Contains(b.String(), `"parent":null`) {
		t.Fatalf("%s %v", b.String(), err)
	}
}

func TestListParentFilter(t *testing.T) {
	l := newLineage(t)
	for _, tc := range []struct {
		parent string
		want   []string
	}{
		{l.parentID, []string{l.childID + "<" + l.parentID}},
		{l.childID, nil},
	} {
		c := herdrscript.Client(t, l.listCall())
		c.Cwd = l.cwd
		out := Run(context.Background(), c, Options{Parent: tc.parent})
		if got := ids(out.Result.(Result).Agents); out.Error != nil || strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Fatalf("--parent %s: got %v want %v (%+v)", tc.parent, got, tc.want, out.Error)
		}
	}
}

func TestListParentFilterWithoutState(t *testing.T) {
	a := herdrscript.Info(herdrscript.LiveAgent("idle")).Agent
	c := herdrscript.Client(t, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []herdr.AgentDetails{a}}})
	c.Cwd = identitytest.Repository(t)
	out := Run(context.Background(), c, Options{Parent: "0000beef"})
	if out.Error != nil || len(out.Result.(Result).Agents) != 0 {
		t.Fatalf("%+v", out)
	}
	if _, err := os.Stat(filepath.Join(c.Cwd, ".fledge")); !os.IsNotExist(err) {
		t.Fatalf("lookup created .fledge: %v", err)
	}
}

func TestListMine(t *testing.T) {
	l := newLineage(t)
	caller := herdrscript.Info(herdrscript.Pane("old:p1", "w1", "w1:t2"))
	caller.Agent.AgentStatus, caller.Agent.TerminalID = "working", "term_parent"
	c := herdrscript.Client(t, call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: caller}, l.listCall())
	c.Cwd = l.cwd
	out := Run(context.Background(), c, Options{Mine: true})
	want := []string{l.childID + "<" + l.parentID}
	if got := ids(out.Result.(Result).Agents); out.Error != nil || strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v want %v (%+v)", got, want, out.Error)
	}
}

func TestListMineRequiresRegisteredCaller(t *testing.T) {
	l := newLineage(t)
	unregistered := herdrscript.Info(herdrscript.Pane("old:p1", "w1", "w1:t2"))
	unregistered.Agent.AgentStatus, unregistered.Agent.TerminalID = "working", "term_orphan"
	for _, tc := range []struct {
		name  string
		cwd   string
		pane  string
		calls []call
	}{
		{"unregistered caller", l.cwd, "old:p1", []call{{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: unregistered}}},
		{"no agent in caller pane", l.cwd, "old:p1", []call{{Method: "agent.get", Err: &herdr.Error{Code: "agent_not_found", Message: "none"}}}},
		{"outside herdr pane", l.cwd, "", nil},
		{"no state", identitytest.Repository(t), "old:p1", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := herdrscript.Client(t, tc.calls...)
			c.Cwd, c.CallerPane = tc.cwd, tc.pane
			out := Run(context.Background(), c, Options{Mine: true})
			if out.ExitCode() != 1 || out.Error.Code != "caller_unregistered" || out.Error.Phase != "identity" || !strings.Contains(out.Error.Message, "fledge agent adopt") {
				t.Fatalf("%+v", out.Error)
			}
		})
	}
}

func TestListRejectsInvalidLineageFilters(t *testing.T) {
	for _, o := range []Options{{Parent: "BEEF"}, {Parent: "0000beef", Mine: true}} {
		out := Run(context.Background(), herdrscript.Client(t), o)
		if out.ExitCode() != 2 || out.Error.Phase != "validation" {
			t.Fatalf("%+v: %+v", o, out)
		}
	}
}

func TestListFilterFailsOnUnreadableStore(t *testing.T) {
	l := newLineage(t)
	if err := os.WriteFile(filepath.Join(l.cwd, ".fledge", "state", "agents", l.childID+".json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, o := range []Options{{}, {Parent: l.parentID}} {
		c := herdrscript.Client(t, l.listCall())
		c.Cwd = l.cwd
		out := Run(context.Background(), c, o)
		if filtered := o.Parent != ""; filtered != (out.Error != nil) || filtered && (out.Error.Phase != "state" || out.ExitCode() != 1) {
			t.Fatalf("%+v: %+v", o, out)
		}
	}
}

func TestFilteredEmptyListNamesChildren(t *testing.T) {
	l := newLineage(t)
	c := herdrscript.Client(t, l.listCall())
	c.Cwd = l.cwd
	var b bytes.Buffer
	if err := Run(context.Background(), c, Options{Parent: l.childID}).Write(&b, false, Render); err != nil || b.String() != "No child agents.\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}
