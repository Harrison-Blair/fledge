package selector

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

type call = herdrscript.Call

// agent is a live agent in pane with terminal, status, and harness.
func agent(pane, terminal, status, harness string) herdr.AgentDetails {
	a := herdrscript.Info(herdrscript.Pane(pane, "w1", "w1:t1")).Agent
	a.TerminalID, a.AgentStatus, a.Agent = terminal, status, &harness
	return a
}

// fleet is a repository with a registered lead (idle claude, profile lead,
// worktree /wt/lead), its registered child (working codex, profile
// reviewer), and an unregistered stray (idle claude), listed in that order.
type fleet struct {
	cwd                 string
	lead, child, stray  herdr.AgentDetails
	leadID, childID     string
	leadTask, strayTask string
}

func newFleet(t *testing.T) fleet {
	t.Helper()
	f := fleet{cwd: identitytest.Repository(t)}
	f.lead = agent("w1:p3", "term_lead", "idle", "claude")
	f.child = agent("w1:p4", "term_child", "working", "codex")
	f.stray = agent("w1:p5", "term_stray", "idle", "claude")
	lead := identitytest.RegisterProfile(t, f.cwd, f.lead, "lead")
	edit(t, f.cwd, lead.ID, func(r *identity.Record) { wt := "/wt/lead"; r.WorktreePath = &wt })
	f.leadID = lead.ID
	f.childID = identitytest.RegisterProfile(t, f.cwd, f.child, "reviewer").ID
	edit(t, f.cwd, f.childID, func(r *identity.Record) { r.Parent = &f.leadID })
	f.leadTask = tasktest.Seed(t, f.cwd, task.Record{Title: "t", Status: task.Assigned, Owner: &f.childID})
	f.strayTask = tasktest.Seed(t, f.cwd, task.Record{Title: "u", Status: task.Created})
	return f
}

func edit(t *testing.T, cwd, id string, mutate func(*identity.Record)) {
	t.Helper()
	s, err := identity.Existing(context.Background(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	var rec identity.Record
	if err := s.Update(identity.Kind, id, &rec, func() error { mutate(&rec); return nil }); err != nil {
		t.Fatal(err)
	}
}

func listCall(agents ...herdr.AgentDetails) call {
	return call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": agents}}
}

func (f fleet) client(t *testing.T) libagent.Client {
	c := herdrscript.Client(t, listCall(f.lead, f.child, f.stray))
	c.Cwd = f.cwd
	return c
}

// panes names each match by pane, with its record id or "-".
func panes(ms []Match) string {
	var out []string
	for _, m := range ms {
		id := "-"
		if m.Record != nil {
			id = m.Record.ID
		}
		out = append(out, m.Agent.PaneID+"="+id)
	}
	return strings.Join(out, " ")
}

// countScans counts reads of the agent records until the test ends.
func countScans(t *testing.T) *int {
	t.Helper()
	n, real := 0, liveByTerminal
	liveByTerminal = func(s *state.Store) (map[string]identity.Record, error) { n++; return real(s) }
	t.Cleanup(func() { liveByTerminal = real })
	return &n
}

func TestResolveMatches(t *testing.T) {
	f := newFleet(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative := filepath.Join(cwd, "rel", "wt")
	edit(t, f.cwd, f.childID, func(r *identity.Record) { r.WorktreePath = &relative })
	lead, child, stray := "w1:p3="+f.leadID, "w1:p4="+f.childID, "w1:p5=-"
	for _, tc := range []struct {
		name string
		f    Filter
		want []string
	}{
		{"empty keeps all in herdr order", Filter{}, []string{lead, child, stray}},
		{"state", Filter{States: []string{"idle"}}, []string{lead, stray}},
		{"states OR within", Filter{States: []string{"working", "idle"}}, []string{lead, child, stray}},
		{"state AND harness", Filter{States: []string{"idle"}, Harnesses: []string{"codex"}}, nil},
		{"state AND registered", Filter{States: []string{"idle"}, Registered: true}, []string{lead}},
		{"harness", Filter{Harnesses: []string{"codex"}}, []string{child}},
		{"registered", Filter{Registered: true}, []string{lead, child}},
		{"parent", Filter{Parent: f.leadID}, []string{child}},
		{"parent of none", Filter{Parent: f.childID}, nil},
		{"profile", Filter{Profiles: []string{"reviewer"}}, []string{child}},
		{"profiles OR", Filter{Profiles: []string{"lead", "reviewer"}}, []string{lead, child}},
		{"profile AND state", Filter{Profiles: []string{"lead", "reviewer"}, States: []string{"working"}}, []string{child}},
		{"worktree", Filter{Worktrees: []string{"/wt/lead/"}}, []string{lead}},
		{"relative worktree", Filter{Worktrees: []string{"rel/./wt"}}, []string{child}},
		{"task owner", Filter{Tasks: []string{f.leadTask}}, []string{child}},
		{"unowned task", Filter{Tasks: []string{f.strayTask}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ms, err := Resolve(context.Background(), f.client(t), tc.f)
			if got := panes(ms); err != nil || got != strings.Join(tc.want, " ") {
				t.Fatalf("got %q (%v), want %q", got, err, strings.Join(tc.want, " "))
			}
		})
	}
}

// TestResolveHarnessUsesLiveValue matches the live agent field even when the
// record's harness snapshot says otherwise or is unknown.
func TestResolveHarnessUsesLiveValue(t *testing.T) {
	f := newFleet(t)
	edit(t, f.cwd, f.leadID, func(r *identity.Record) { r.Harness = nil })
	f.lead.Agent = new(string)
	*f.lead.Agent = "pi"
	ms, err := Resolve(context.Background(), f.client(t), Filter{Harnesses: []string{"pi"}})
	if got := panes(ms); err != nil || got != "w1:p3="+f.leadID {
		t.Fatalf("%q %v", got, err)
	}
}

func TestResolveMine(t *testing.T) {
	f := newFleet(t)
	c := f.client(t)
	c.CallerPane = f.lead.PaneID
	ms, err := Resolve(context.Background(), c, Filter{Mine: true})
	if got := panes(ms); err != nil || got != "w1:p4="+f.childID {
		t.Fatalf("%q %v", got, err)
	}
}

func TestResolveMineRequiresRegisteredCaller(t *testing.T) {
	f := newFleet(t)
	for _, tc := range []struct{ name, cwd, pane string }{
		{"unregistered caller", f.cwd, f.stray.PaneID},
		{"caller pane hosts no agent", f.cwd, "w9:p9"},
		{"outside herdr", f.cwd, ""},
		{"no state", identitytest.Repository(t), f.lead.PaneID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := f.client(t)
			c.Cwd, c.CallerPane = tc.cwd, tc.pane
			_, err := Resolve(context.Background(), c, Filter{Mine: true})
			if code, phase := failure(err, "x"); code != "caller_unregistered" || phase != "identity" {
				t.Fatalf("%v: %s at %s", err, code, phase)
			}
		})
	}
}

func TestResolveMissingTask(t *testing.T) {
	f := newFleet(t)
	_, err := Resolve(context.Background(), f.client(t), Filter{Tasks: []string{f.leadTask, "0000dead"}})
	if code, _ := failure(err, "x"); code != "task_not_found" {
		t.Fatalf("%v: %s", err, code)
	}
}

func TestResolveListsAndScansOnce(t *testing.T) {
	f := newFleet(t)
	n := countScans(t)
	c := f.client(t) // herdrscript fails the test on any call beyond one agent.list
	c.CallerPane = f.lead.PaneID
	ms, err := Resolve(context.Background(), c, Filter{Mine: true, States: []string{"working"}, Harnesses: []string{"codex"},
		Profiles: []string{"reviewer"}, Tasks: []string{f.leadTask}, Registered: true})
	if err != nil || len(ms) != 1 || *n != 1 {
		t.Fatalf("%q %v; scanned %d times, want 1", panes(ms), err, *n)
	}
}

func TestResolveListFailureSkipsRecords(t *testing.T) {
	f := newFleet(t)
	n := countScans(t)
	c := herdrscript.Client(t, call{Method: "agent.list", Err: errors.New("offline")})
	c.Cwd = f.cwd
	if _, err := Resolve(context.Background(), c, Filter{Registered: true}); err == nil || *n != 0 {
		t.Fatalf("%v; scanned %d times", err, *n)
	}
}

func TestResolveRejectsInvalidFilter(t *testing.T) {
	_, err := Resolve(context.Background(), herdrscript.Client(t), Filter{States: []string{"asleep"}})
	if code, phase := failure(err, "x"); code != "invalid_input" || phase != "validation" {
		t.Fatalf("%v: %s at %s", err, code, phase)
	}
}

func TestResolveWithoutStore(t *testing.T) {
	a := agent("w1:p3", "term_a", "idle", "claude")
	for _, tc := range []struct {
		name string
		f    Filter
		want string
	}{
		{"live-only keeps unregistered", Filter{States: []string{"idle"}}, "w1:p3=-"},
		{"record-backed matches nothing", Filter{Registered: true}, ""},
		{"parent matches nothing", Filter{Parent: "0000beef"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := herdrscript.Client(t, listCall(a))
			c.Cwd = identitytest.Repository(t)
			ms, err := Resolve(context.Background(), c, tc.f)
			if got := panes(ms); err != nil || got != tc.want {
				t.Fatalf("%q %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(c.Cwd, ".fledge")); !os.IsNotExist(err) {
				t.Fatalf("resolve created .fledge: %v", err)
			}
		})
	}
	c := herdrscript.Client(t, listCall(a))
	c.Cwd = identitytest.Repository(t)
	if _, err := Resolve(context.Background(), c, Filter{Tasks: []string{"0000beef"}}); err == nil {
		t.Fatal("a task in a repository without state was found")
	}
}

func TestResolveUnavailableStore(t *testing.T) {
	f := newFleet(t)
	if err := os.WriteFile(filepath.Join(f.cwd, ".fledge", "state", "agents", f.childID+".json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	ms, err := Resolve(context.Background(), f.client(t), Filter{States: []string{"working", "idle"}})
	if got := panes(ms); err != nil || got != "w1:p3=- w1:p4=- w1:p5=-" {
		t.Fatalf("live-only: %q %v", got, err)
	}
	for _, flt := range []Filter{{Registered: true}, {Parent: f.leadID}, {Profiles: []string{"lead"}}, {Worktrees: []string{"/wt/lead"}}, {Tasks: []string{f.leadTask}}} {
		_, err := Resolve(context.Background(), f.client(t), flt)
		if _, phase := failure(err, "x"); err == nil || phase != "state" {
			t.Fatalf("%+v: %v at %s", flt, err, phase)
		}
	}
	c := herdrscript.Client(t, listCall(f.lead))
	c.Cwd = t.TempDir() // outside any repository
	if _, err := Resolve(context.Background(), c, Filter{Registered: true}); err == nil {
		t.Fatal("record-backed filter outside a repository succeeded")
	}
	c = herdrscript.Client(t, listCall(f.lead))
	c.Cwd = t.TempDir()
	if ms, err := Resolve(context.Background(), c, Filter{Harnesses: []string{"claude"}}); err != nil || len(ms) != 1 {
		t.Fatalf("live-only outside a repository: %v %v", ms, err)
	}
}

// failure classifies err as an outcome would, returning its code and phase.
func failure(err error, phase string) (string, string) {
	if err == nil {
		return "", ""
	}
	var out libagent.Outcome
	out.Fail(err, phase, false)
	return out.Error.Code, out.Error.Phase
}
