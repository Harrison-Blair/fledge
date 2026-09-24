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
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

type call = herdrscript.Call

func TestListIncludesUnnamed(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []herdr.AgentDetails{herdrscript.Info(p).Agent}}}), Options{})
	if out.Status != "success" || len(out.Result.(Result).Agents) != 1 {
		t.Fatal(out)
	}
}

// TestListRejectsIncompleteAgentInfo holds agent.list entries to the full
// AgentInfo shape: pane ids and a status are not enough without a terminal.
func TestListRejectsIncompleteAgentInfo(t *testing.T) {
	good, bad := herdrscript.Info(herdrscript.LiveAgent("idle")).Agent, herdrscript.Info(herdrscript.LiveAgent("idle")).Agent
	bad.PaneID, bad.TerminalID = "w1:p4", ""
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []herdr.AgentDetails{good, bad}}}), Options{})
	if out.Status != "rejected" || out.Result != nil || out.Error == nil || *out.Error != (libagent.Failure{Code: "protocol_error", Message: "protocol_error: incomplete agent.list result", Phase: "agent.list"}) {
		t.Fatalf("%+v %+v", out, out.Error)
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
	id, parent, profile := "0000beef", "0000cafe", "reviewer"
	for _, tc := range []struct {
		name   string
		result any
		want   string
	}{
		{"empty list", Result{}, "No live agents."},
		{"list", Result{Agents: []Row{{ID: &id, Parent: &parent, Profile: &profile, AgentRow: row}, {}}}, "ID PARENT NAME HARNESS PROFILE STATUS WORKSPACE TAB PANE CWD 0000beef 0000cafe worker claude reviewer idle w1 w1:t1 w1:p1 /repo - - - - - - - - - -"},
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
	rec := identitytest.RegisterProfile(t, c.Cwd, registered, "reviewer")
	out := Run(context.Background(), c, Options{})
	rows := out.Result.(Result).Agents
	if out.Error != nil || len(rows) != 2 || rows[0].ID == nil || *rows[0].ID != rec.ID || rows[1].ID != nil {
		t.Fatalf("%+v", out)
	}
	var b bytes.Buffer
	if err := out.Write(&b, true, Render); err != nil || !strings.Contains(b.String(), `"id":"`+rec.ID+`"`) || !strings.Contains(b.String(), `"id":null`) ||
		!strings.Contains(b.String(), `"profile":"reviewer"`) || !strings.Contains(b.String(), `"profile":null`) {
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
		out := Run(context.Background(), c, Options{Filter: selector.Filter{Parent: tc.parent}})
		if got := ids(out.Result.(Result).Agents); out.Error != nil || strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Fatalf("--parent %s: got %v want %v (%+v)", tc.parent, got, tc.want, out.Error)
		}
	}
}

func TestListParentFilterWithoutState(t *testing.T) {
	a := herdrscript.Info(herdrscript.LiveAgent("idle")).Agent
	c := herdrscript.Client(t, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []herdr.AgentDetails{a}}})
	c.Cwd = identitytest.Repository(t)
	out := Run(context.Background(), c, Options{Filter: selector.Filter{Parent: "0000beef"}})
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
	out := Run(context.Background(), c, Options{Filter: selector.Filter{Mine: true}})
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
			out := Run(context.Background(), c, Options{Filter: selector.Filter{Mine: true}})
			if out.ExitCode() != 1 || out.Error.Code != "caller_unregistered" || out.Error.Phase != "identity" || !strings.Contains(out.Error.Message, "fledge agent adopt") {
				t.Fatalf("%+v", out.Error)
			}
		})
	}
}

func TestListRejectsInvalidLineageFilters(t *testing.T) {
	for _, o := range []Options{{Filter: selector.Filter{Parent: "BEEF"}}, {Filter: selector.Filter{Parent: "0000beef", Mine: true}}} {
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
	for _, o := range []Options{{}, {Filter: selector.Filter{Parent: l.parentID}}} {
		c := herdrscript.Client(t, l.listCall())
		c.Cwd = l.cwd
		out := Run(context.Background(), c, o)
		if filtered := o.Parent != ""; filtered != (out.Error != nil) || filtered && (out.Error.Phase != "state" || out.ExitCode() != 1) {
			t.Fatalf("%+v: %+v", o, out)
		}
	}
}

func TestFilteredEmptyListSaysNoMatch(t *testing.T) {
	l := newLineage(t)
	c := herdrscript.Client(t, l.listCall())
	c.Cwd = l.cwd
	var b bytes.Buffer
	if err := Run(context.Background(), c, Options{Filter: selector.Filter{Parent: l.childID}}).Write(&b, false, Render); err != nil || b.String() != "No agents match.\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

// A record left in the terminal by a different harness is not the live agent's.
func TestListSkipsRecordOfDifferentHarness(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle")).Agent
	recorded, codex := live, "codex"
	recorded.Agent = &codex
	c := herdrscript.Client(t, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []herdr.AgentDetails{live}}})
	c.Cwd = identitytest.Repository(t)
	identitytest.Register(t, c.Cwd, recorded)
	out := Run(context.Background(), c, Options{})
	if rows := out.Result.(Result).Agents; out.Error != nil || len(rows) != 1 || rows[0].ID != nil {
		t.Fatalf("%+v", out)
	}
}

// fleet is a repository with a registered lead (idle claude, profile lead,
// worktree /wt/lead) in w1:p3, its registered child (working codex, profile
// reviewer, owner of task) in w1:p4, and an unregistered stray (idle claude)
// in w1:p5.
type fleet struct {
	cwd             string
	agents          []herdr.AgentDetails
	leadID, childID string
	task            string
}

func newFleet(t *testing.T) fleet {
	t.Helper()
	agent := func(pane, terminal, status, harness string) herdr.AgentDetails {
		a := herdrscript.Info(herdrscript.Pane(pane, "w1", "w1:t1")).Agent
		a.TerminalID, a.AgentStatus, a.Agent = terminal, status, &harness
		return a
	}
	f := fleet{cwd: identitytest.Repository(t)}
	lead, child := agent("w1:p3", "term_lead", "idle", "claude"), agent("w1:p4", "term_child", "working", "codex")
	f.agents = []herdr.AgentDetails{lead, child, agent("w1:p5", "term_stray", "idle", "claude")}
	f.leadID = identitytest.RegisterProfile(t, f.cwd, lead, "lead").ID
	f.childID = identitytest.RegisterProfile(t, f.cwd, child, "reviewer").ID
	s, err := identity.Existing(context.Background(), f.cwd)
	if err != nil {
		t.Fatal(err)
	}
	var rec identity.Record
	if err := s.Update(identity.Kind, f.leadID, &rec, func() error { wt := "/wt/lead"; rec.WorktreePath = &wt; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(identity.Kind, f.childID, &rec, func() error { rec.Parent = &f.leadID; return nil }); err != nil {
		t.Fatal(err)
	}
	f.task = tasktest.Seed(t, f.cwd, task.Record{Title: "t", Status: task.Assigned, Owner: &f.childID})
	return f
}

func (f fleet) run(t *testing.T, o Options) libagent.Outcome {
	c := herdrscript.Client(t, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": f.agents}})
	c.Cwd = f.cwd
	return Run(context.Background(), c, o)
}

func panes(rows []Row) string {
	var out []string
	for _, r := range rows {
		out = append(out, libagent.Display(r.PaneID))
	}
	return strings.Join(out, " ")
}

// TestListFilters ANDs flags together and ORs repeated values of one flag.
func TestListFilters(t *testing.T) {
	f := newFleet(t)
	for _, tc := range []struct {
		name   string
		filter selector.Filter
		want   string
	}{
		{"state", selector.Filter{States: []string{"idle"}}, "w1:p3 w1:p5"},
		{"states or", selector.Filter{States: []string{"working", "idle"}}, "w1:p3 w1:p4 w1:p5"},
		{"harness", selector.Filter{Harnesses: []string{"codex"}}, "w1:p4"},
		{"harnesses or", selector.Filter{Harnesses: []string{"codex", "claude"}}, "w1:p3 w1:p4 w1:p5"},
		{"profile", selector.Filter{Profiles: []string{"reviewer"}}, "w1:p4"},
		{"profiles or", selector.Filter{Profiles: []string{"lead", "reviewer"}}, "w1:p3 w1:p4"},
		{"task", selector.Filter{Tasks: []string{f.task}}, "w1:p4"},
		{"worktree", selector.Filter{Worktrees: []string{"/wt/lead"}}, "w1:p3"},
		{"registered", selector.Filter{Registered: true}, "w1:p3 w1:p4"},
		{"registered and state", selector.Filter{Registered: true, States: []string{"idle"}}, "w1:p3"},
		{"harness and state", selector.Filter{Harnesses: []string{"claude"}, States: []string{"working"}}, ""},
		{"parent and profile", selector.Filter{Parent: f.leadID, Profiles: []string{"reviewer"}}, "w1:p4"},
		{"parent and other profile", selector.Filter{Parent: f.leadID, Profiles: []string{"lead"}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := f.run(t, Options{Filter: tc.filter})
			if out.Error != nil {
				t.Fatalf("%+v", out.Error)
			}
			if got := panes(out.Result.(Result).Agents); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestListRejectsInvalidFilters(t *testing.T) {
	for _, o := range []Options{
		{Filter: selector.Filter{States: []string{"asleep"}}},
		{Filter: selector.Filter{Harnesses: []string{"nope"}}},
		{Filter: selector.Filter{Tasks: []string{"BEEF"}}},
		{Filter: selector.Filter{Profiles: []string{" "}}},
		{IDs: true, JSON: true},
	} {
		out := Run(context.Background(), herdrscript.Client(t), o)
		if out.ExitCode() != 2 || out.Error.Phase != "validation" {
			t.Fatalf("%+v: %+v", o, out)
		}
	}
}

func TestListFilterEmptyWording(t *testing.T) {
	f := newFleet(t)
	for _, tc := range []struct {
		o    Options
		want string
	}{
		{Options{Filter: selector.Filter{States: []string{"blocked"}}}, "No agents match.\n"},
		{Options{Filter: selector.Filter{Harnesses: []string{"pi"}}}, "No agents match.\n"},
	} {
		var b bytes.Buffer
		if err := f.run(t, tc.o).Write(&b, false, Render); err != nil || b.String() != tc.want {
			t.Fatalf("%+v: %q %v", tc.o, b.String(), err)
		}
	}
}

// TestListIDs prints one record id per registered match and skips the rest.
func TestListIDs(t *testing.T) {
	f := newFleet(t)
	for _, tc := range []struct {
		name   string
		filter selector.Filter
		want   string
	}{
		{"all", selector.Filter{}, f.leadID + "\n" + f.childID + "\n"},
		{"filtered", selector.Filter{States: []string{"idle"}}, f.leadID + "\n"},
		{"none", selector.Filter{States: []string{"blocked"}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := f.run(t, Options{Filter: tc.filter, IDs: true}).Write(&b, false, Render); err != nil || b.String() != tc.want {
				t.Fatalf("got %q want %q (%v)", b.String(), tc.want, err)
			}
		})
	}
}
