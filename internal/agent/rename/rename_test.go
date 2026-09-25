package rename

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
)

type call = herdrscript.Call

func named(s string) *string { return &s }

// agent is the live claude agent in pane with the given name.
func agent(pane string, name *string) herdr.AgentResult {
	p := herdrscript.LiveAgent("idle")
	p.PaneID, p.Name = pane, name
	return herdrscript.Info(p)
}

// labels are the requests that label pane as reviewer, with count panes in
// its tab w1:t2.
func labels(pane string, count int) []call {
	calls := []call{
		{Method: "pane.rename", Params: map[string]any{"pane_id": pane, "label": "reviewer"}, Result: herdr.PaneResult{Type: "pane_info", Pane: herdrscript.Pane(pane, "w1", "w1:t2")}},
		{Method: "tab.get", Params: map[string]any{"tab_id": "w1:t2"}, Result: herdr.TabResult{Type: "tab_info", Tab: herdr.Tab{ID: "w1:t2", WorkspaceID: "w1", Label: "2", PaneCount: &count}}},
	}
	if count == 1 {
		calls = append(calls, call{Method: "tab.rename", Params: map[string]any{"tab_id": "w1:t2", "label": "reviewer"}, Result: herdr.TabResult{Type: "tab_info", Tab: herdr.Tab{ID: "w1:t2", WorkspaceID: "w1", Label: "reviewer", PaneCount: &count}}})
	}
	return calls
}

func TestRenameCallerByDefaultRenamesAndLabels(t *testing.T) {
	calls := append([]call{
		{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: agent("old:p1", named("worker"))},
		{Method: "agent.rename", Params: map[string]any{"target": "old:p1", "name": "reviewer"}, Result: agent("old:p1", named("reviewer"))},
	}, labels("old:p1", 1)...)
	out := Run(context.Background(), herdrscript.Client(t, calls...), Options{To: "reviewer"})
	if out.Status != "success" || out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	want := []libagent.Effect{{Action: "updated", Kind: "agent_name", ID: "old:p1"}, {Action: "updated", Kind: "pane_label", ID: "old:p1"}, {Action: "updated", Kind: "tab", ID: "w1:t2"}}
	if !reflect.DeepEqual(out.Effects, want) {
		t.Fatalf("%+v", out.Effects)
	}
	r := out.Result.(Result)
	if *r.Name != "reviewer" || *r.Previous != "worker" || r.ID != nil {
		t.Fatalf("%+v", r)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || b.String() != "Renamed worker to reviewer (old:p1).\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestRenameInSharedTabKeepsTabLabel(t *testing.T) {
	calls := append([]call{
		{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: agent("w1:p3", named("worker"))},
		{Method: "agent.rename", Result: agent("w1:p3", named("reviewer"))},
	}, labels("w1:p3", 2)...)
	out := Run(context.Background(), herdrscript.Client(t, calls...), Options{Target: identity.Target{Name: "worker"}, To: "reviewer"})
	if out.Error != nil || len(out.Effects) != 2 || out.Effects[1].Kind != "pane_label" {
		t.Fatalf("%+v %+v", out.Error, out.Effects)
	}
}

// An agent that already has the name is not renamed, but its labels are set,
// so rename can repair labels for an agent named some other way.
func TestRenameToCurrentNameOnlyLabels(t *testing.T) {
	calls := append([]call{{Method: "agent.get", Result: agent("w1:p3", named("reviewer"))}}, labels("w1:p3", 1)...)
	out := Run(context.Background(), herdrscript.Client(t, calls...), Options{Target: identity.Target{Pane: "w1:p3"}, To: "reviewer"})
	if out.Error != nil || len(out.Effects) != 2 || out.Effects[0].Kind != "pane_label" {
		t.Fatalf("%+v %+v", out.Error, out.Effects)
	}
}

// A registered agent keeps its record and id; the record takes the new name.
func TestRenameRegisteredAgentKeepsRecord(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	calls := append([]call{
		{Method: "agent.get", Result: agent("w1:p3", named("worker"))},
		{Method: "agent.rename", Result: agent("w1:p3", named("reviewer"))},
	}, labels("w1:p3", 2)...)
	c := herdrscript.Client(t, calls...)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if b, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	c.Cwd = root
	s, err := identity.OpenStore(context.Background(), root, &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := identity.Register(context.Background(), s, libagent.Client{}, agent("w1:p3", named("worker")).Agent, "spawn", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := Run(context.Background(), c, Options{Target: identity.Target{Name: "worker"}, To: "reviewer"})
	if out.Error != nil || !reflect.DeepEqual(out.Effects[1], libagent.Effect{Action: "updated", Kind: "agent_record", ID: rec.ID}) {
		t.Fatalf("%+v %+v", out.Error, out.Effects)
	}
	var got identity.Record
	if err := s.Get(identity.Kind, rec.ID, &got); err != nil || *got.Name != "reviewer" || *out.Result.(Result).ID != rec.ID {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestRenameRejections(t *testing.T) {
	for label, tc := range map[string]struct {
		o      Options
		caller string
		calls  []call
		code   string
	}{
		"missing to":    {Options{}, "old:p1", nil, "invalid_input"},
		"bad name":      {Options{To: "Bad Name"}, "old:p1", nil, "invalid_input"},
		"two selectors": {Options{Target: identity.Target{Name: "a", Pane: "w1:p3"}, To: "reviewer"}, "old:p1", nil, "invalid_input"},
		"no caller":     {Options{To: "reviewer"}, "", nil, "invalid_input"},
		"name taken": {Options{To: "reviewer"}, "old:p1", []call{{Method: "agent.get", Result: agent("old:p1", named("worker"))},
			{Method: "agent.rename", Err: &herdr.Error{Code: "agent_name_taken", Message: "taken"}}}, "agent_name_taken"},
	} {
		t.Run(label, func(t *testing.T) {
			c := herdrscript.Client(t, tc.calls...)
			c.CallerPane = tc.caller
			out := Run(context.Background(), c, tc.o)
			if out.Error == nil || out.Error.Code != tc.code || len(out.Effects) != 0 {
				t.Fatalf("%+v %+v", out.Error, out.Effects)
			}
		})
	}
}
