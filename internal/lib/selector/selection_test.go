package selector

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
)

func TestSelectionValidateRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    Selection
		want string
	}{
		{"nothing selected", Selection{}, "required"},
		{"explicit with filter", Selection{Names: []string{"w"}, Filter: Filter{Registered: true}}, "mutually exclusive"},
		{"id with filter", Selection{IDs: []string{"0000beef"}, Filter: Filter{States: []string{"idle"}}}, "mutually exclusive"},
		{"empty name", Selection{Names: []string{"a", " "}}, "nonempty"},
		{"empty id", Selection{IDs: []string{""}}, "nonempty"},
		{"duplicate name", Selection{Names: []string{"a", "a"}}, "duplicate"},
		{"duplicate across flags", Selection{Names: []string{"w1:p3"}, Panes: []string{"w1:p3"}}, "duplicate"},
		{"duplicate id", Selection{IDs: []string{"0000beef", "0000beef"}}, "duplicate"},
		{"malformed id", Selection{IDs: []string{"BEEF"}}, "--id must be an 8 lowercase hexadecimal agent id"},
		{"invalid filter", Selection{Filter: Filter{States: []string{"asleep"}}}, "--state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.s.Validate()
			var input *libagent.InputError
			if !errors.As(err, &input) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an input error containing %q", err, tc.want)
			}
		})
	}
	for _, s := range []Selection{{Names: []string{"a"}, Panes: []string{"w1:p1"}, IDs: []string{"0000beef"}}, {Filter: Filter{Registered: true}}} {
		if err := s.Validate(); err != nil {
			t.Fatalf("%+v: %v", s, err)
		}
	}
}

func get(target string, a herdr.AgentDetails) call {
	return call{Method: "agent.get", Params: map[string]any{"target": target}, Result: herdr.AgentResult{Type: "agent_info", Agent: a}}
}

// labels names each target as label@pane=record id or "-".
func labels(ts []Target) string {
	var out []string
	for _, t := range ts {
		id := "-"
		if t.Record != nil {
			id = t.Record.ID
		}
		out = append(out, t.Label+"@"+t.Pane+"="+id)
	}
	return strings.Join(out, " ")
}

func TestTargetsExplicitInFlagOrder(t *testing.T) {
	f := newFleet(t)
	c := herdrscript.Client(t, get("worker", f.stray), get("w1:p3", f.lead), get(f.child.PaneID, f.child))
	c.Cwd = f.cwd
	ts, err := Selection{Names: []string{"worker"}, Panes: []string{"w1:p3"}, IDs: []string{f.childID}}.Targets(context.Background(), c, identity.OpenOnce(context.Background(), c.Cwd))
	want := "worker@worker=- w1:p3@w1:p3=- " + f.childID + "@w1:p4=" + f.childID
	if got := labels(ts); err != nil || got != want {
		t.Fatalf("got %q (%v), want %q", got, err, want)
	}
	if ts[0].Agent.TerminalID != "term_stray" || ts[2].Agent.TerminalID != "term_child" {
		t.Fatalf("%+v", ts)
	}
}

func TestTargetsExplicitStaleIDs(t *testing.T) {
	f := newFleet(t)
	gone := agent(f.child.PaneID, "term_other", "idle", "codex")
	for _, tc := range []struct {
		name, id string
		calls    []call
		code     string
	}{
		{"unknown record", "0000dead", nil, "agent_record_not_found"},
		// The recorded pane hosts another terminal and the terminal is in no
		// pane: the record is stale.
		{"stale record", f.childID, []call{get(f.child.PaneID, gone), listCall(f.lead), {Method: "pane.list", Result: map[string]any{"type": "pane_list", "panes": []herdr.AgentDetails{f.lead}}}}, "agent_identity_stale"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := herdrscript.Client(t, tc.calls...)
			c.Cwd = f.cwd
			_, err := Selection{IDs: []string{tc.id}}.Targets(context.Background(), c, identity.OpenOnce(context.Background(), c.Cwd))
			if code, phase := failure(err, "x"); code != tc.code || phase != "identity" {
				t.Fatalf("%v: %s at %s", err, code, phase)
			}
		})
	}
}

// Every explicit id, and a filter, reads the store open supplies, so one
// opener serves the whole selection.
func TestTargetsUseGivenStore(t *testing.T) {
	f := newFleet(t)
	s, err := identity.Existing(context.Background(), f.cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		sel   Selection
		calls []call
		want  string
		opens int
	}{
		{"ids", Selection{IDs: []string{f.leadID, f.childID}}, []call{get(f.lead.PaneID, f.lead), get(f.child.PaneID, f.child)}, f.leadID + "@w1:p3=" + f.leadID + " " + f.childID + "@w1:p4=" + f.childID, 2},
		{"filter", Selection{Filter: Filter{Profiles: []string{"reviewer"}}}, []call{listCall(f.lead, f.child, f.stray)}, f.childID + "@w1:p4=" + f.childID, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := herdrscript.Client(t, tc.calls...)
			c.Cwd = t.TempDir() // outside any repository: only open finds the store
			opens := 0
			ts, err := tc.sel.Targets(context.Background(), c, func() (*state.Store, error) { opens++; return s, nil })
			if got := labels(ts); err != nil || got != tc.want || opens != tc.opens {
				t.Fatalf("got %q (%v) opens=%d, want %q", got, err, opens, tc.want)
			}
		})
	}
}

func TestTargetsFilterExcludesCaller(t *testing.T) {
	f := newFleet(t)
	c := f.client(t)
	c.CallerPane = f.lead.PaneID
	ts, err := Selection{Filter: Filter{Harnesses: []string{"claude", "codex"}}}.Targets(context.Background(), c, identity.OpenOnce(context.Background(), c.Cwd))
	want := f.childID + "@w1:p4=" + f.childID + " w1:p5@w1:p5=-"
	if got := labels(ts); err != nil || got != want {
		t.Fatalf("got %q (%v), want %q", got, err, want)
	}
}

func TestTargetsFilterNoMatch(t *testing.T) {
	f := newFleet(t)
	for _, tc := range []struct {
		name   string
		caller string
		flt    Filter
	}{
		{"nothing matched", "", Filter{Profiles: []string{"nobody"}}},
		{"only the caller matched", f.child.PaneID, Filter{Profiles: []string{"reviewer"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := f.client(t)
			c.CallerPane = tc.caller
			ts, err := Selection{Filter: tc.flt}.Targets(context.Background(), c, identity.OpenOnce(context.Background(), c.Cwd))
			var out libagent.Outcome
			out.Fail(err, "x", false)
			if ts != nil || out.Error.Code != "no_agents_matched" || out.Error.Phase != "selection" || out.Status != "rejected" || out.ExitCode() != 1 {
				t.Fatalf("%v %+v %s", ts, out.Error, out.Status)
			}
		})
	}
}

func TestTargetsRejectsInvalidSelection(t *testing.T) {
	_, err := Selection{Names: []string{"a", "a"}}.Targets(context.Background(), herdrscript.Client(t), nil)
	if code, phase := failure(err, "x"); code != "invalid_input" || phase != "validation" {
		t.Fatalf("%v: %s at %s", err, code, phase)
	}
}

func TestSelectionExplicitOrdersNamesPanesIDs(t *testing.T) {
	s := Selection{IDs: []string{"0000beef"}, Panes: []string{"w1:p3", "w1:p4"}, Names: []string{"worker"}}
	want := []identity.Target{{Name: "worker"}, {Pane: "w1:p3"}, {Pane: "w1:p4"}, {ID: "0000beef"}}
	if got := s.Explicit(); !slices.Equal(got, want) {
		t.Fatalf("Explicit() = %v, want %v", got, want)
	}
	if got := (Selection{Filter: Filter{Registered: true}}).Explicit(); got != nil {
		t.Fatalf("filter Explicit() = %v, want nil", got)
	}
}
