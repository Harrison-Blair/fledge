package stop

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

type call = herdrscript.Call

var fake = herdrscript.Client

func TestStopIdleClosesResolvedPane(t *testing.T) {
	p := herdrscript.LiveAgent("idle")
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: herdrscript.Info(p)}, call{Method: "pane.close", Params: map[string]any{"pane_id": "w1:p3"}, Result: herdrscript.OK()})
	out := Run(context.Background(), s, Options{Name: "worker"})
	if out.Operation != "agent.stop" || out.Status != "success" || out.Error != nil || out.ExitCode() != 0 {
		t.Fatalf("%+v", out)
	}
	want := Result{AgentRow: libagent.NewAgentRow(p), Stopped: true}
	if !reflect.DeepEqual(out.Result, want) {
		t.Fatalf("result %+v want %+v", out.Result, want)
	}
	if !reflect.DeepEqual(out.Effects, []libagent.Effect{{Action: "closed", Kind: "pane", ID: "w1:p3"}}) {
		t.Fatalf("effects %+v", out.Effects)
	}
}
func TestStopByPaneTargetsThatPane(t *testing.T) {
	p := herdrscript.LiveAgent("done")
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: herdrscript.Info(p)}, call{Method: "pane.close", Params: map[string]any{"pane_id": "w1:p3"}, Result: herdrscript.OK()})
	out := Run(context.Background(), s, Options{Pane: "w1:p3"})
	if out.Status != "success" || !out.Result.(Result).Stopped {
		t.Fatalf("%+v", out)
	}
}
func TestStopBusyRequiresForce(t *testing.T) {
	for _, status := range []string{"working", "blocked", "unknown"} {
		t.Run(status, func(t *testing.T) {
			s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent(status))})
			out := Run(context.Background(), s, Options{Name: "worker"})
			if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Code != "invalid_input" || len(out.Effects) != 0 {
				t.Fatalf("%+v", out)
			}
			if !strings.Contains(out.Error.Message, status) || !strings.Contains(out.Error.Message, "--force") {
				t.Fatalf("message %q", out.Error.Message)
			}
		})
	}
}
func TestStopForceClosesBusyAgent(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("working"))}, call{Method: "pane.close", Params: map[string]any{"pane_id": "w1:p3"}, Result: herdrscript.OK()})
	out := Run(context.Background(), s, Options{Name: "worker", Force: true})
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
}
func TestStopUnknownAgentDoesNotClose(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Err: &herdr.Error{Code: "agent_not_found", Message: "no such agent"}})
	out := Run(context.Background(), s, Options{Name: "ghost"})
	if out.Status != "rejected" || out.ExitCode() != 1 || out.Error.Code != "agent_not_found" || out.Error.Phase != "agent.get" {
		t.Fatalf("%+v", out)
	}
}
func TestStopMalformedAgentInfoDoesNotClose(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.Pane("", "w1", "w1:t2"))})
	out := Run(context.Background(), s, Options{Name: "worker"})
	if out.Status != "rejected" || out.Error.Phase != "agent.get" {
		t.Fatalf("%+v", out)
	}
}
func TestStopLostCloseIsUnknown(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("idle"))}, call{Method: "pane.close", Err: &herdr.Error{Code: "transport_error", Message: "lost", Uncertain: true}})
	out := Run(context.Background(), s, Options{Name: "worker"})
	if out.Status != "unknown" || out.Error.Code != "transport_error" || out.Error.Phase != "pane.close" || out.ExitCode() != 1 {
		t.Fatalf("%+v", out)
	}
	if r := out.Result.(Result); r.Stopped {
		t.Fatalf("stopped claimed: %+v", r)
	}
}
func TestStopWrongCloseResultIsUnknown(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("idle"))}, call{Method: "pane.close", Result: map[string]any{"type": "pane_info"}})
	out := Run(context.Background(), s, Options{Name: "worker"})
	if out.Status != "unknown" || out.Error.Phase != "pane.close" {
		t.Fatalf("%+v", out)
	}
}
func TestStopMissingPaneIsFailure(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("idle"))}, call{Method: "pane.close", Err: &herdr.Error{Code: "pane_not_found", Message: "gone"}})
	out := Run(context.Background(), s, Options{Name: "worker"})
	if out.Status == "success" || out.ExitCode() != 1 || out.Error.Code != "pane_not_found" || out.Result.(Result).Stopped {
		t.Fatalf("%+v", out)
	}
}
func TestStopRequiresExactlyOneTarget(t *testing.T) {
	for name, o := range map[string]Options{"neither": {}, "both": {Name: "worker", Pane: "w1:p3"}} {
		t.Run(name, func(t *testing.T) {
			out := Run(context.Background(), fake(t), o)
			if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Phase != "validation" {
				t.Fatalf("%+v", out)
			}
		})
	}
}
func TestStopJSONEnvelope(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("idle"))}, call{Method: "pane.close", Result: herdrscript.OK()})
	var b bytes.Buffer
	if err := Run(context.Background(), s, Options{Name: "worker"}).Write(&b, true, Render); err != nil {
		t.Fatal(err)
	}
	want := `{"operation":"agent.stop","status":"success","result":{"name":"worker","harness":"claude","agent_status":"idle","workspace_id":"w1","tab_id":"w1:t2","pane_id":"w1:p3","cwd":"/repo","stopped":true},"effects":[{"action":"closed","kind":"pane","id":"w1:p3"}],"error":null}` + "\n"
	if b.String() != want {
		t.Fatalf("got %s", b.String())
	}
}
func TestHumanStop(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  libagent.AgentRow
		want string
	}{
		{"named", libagent.NewAgentRow(herdrscript.LiveAgent("idle")), "Stopped worker (claude) in w1:p3.\n"},
		{"unnamed", libagent.NewAgentRow(herdr.Pane{PaneID: "w1:p3", WorkspaceID: "w1", TabID: "w1:t2", AgentStatus: "idle"}), "Stopped - (-) in w1:p3.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := (libagent.Outcome{Operation: "agent.stop", Status: "success", Result: Result{AgentRow: tc.row, Stopped: true}}).Write(&b, false, Render); err != nil {
				t.Fatal(err)
			}
			if b.String() != tc.want {
				t.Fatalf("%q", b.String())
			}
		})
	}
}
func TestHumanOperationResults(t *testing.T) {
	var b bytes.Buffer
	if err := (libagent.Outcome{Status: "success", Result: Result{AgentRow: herdrscript.Row(), Stopped: true}}).Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(strings.Fields(b.String()), " "), "Stopped worker (claude) in w1:p1."; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
func TestOutputFailuresPropagate(t *testing.T) {
	herdrscript.CheckOutputFailures(t, Render, libagent.Outcome{Result: Result{}})
}

func TestStopByIDClosesVerifiedPane(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live}, call{Method: "pane.close", Params: map[string]any{"pane_id": "w1:p3"}, Result: herdrscript.OK()})
	s.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, s.Cwd, live.Agent)
	if out := Run(context.Background(), s, Options{ID: rec.ID}); out.Status != "success" || !out.Result.(Result).Stopped {
		t.Fatalf("%+v", out)
	}
}

func TestStopByStaleIDDoesNotClose(t *testing.T) {
	for name, get := range map[string]call{
		"other terminal": {Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: herdrscript.Info(herdrscript.LiveAgent("idle"))},
		"no agent":       {Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Err: &herdr.Error{Code: "agent_not_found", Message: "gone"}},
	} {
		t.Run(name, func(t *testing.T) {
			s := fake(t, get, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []any{}}})
			s.Cwd = identitytest.Repository(t)
			recorded := herdrscript.Info(herdrscript.LiveAgent("idle")).Agent
			recorded.TerminalID = "term_old"
			rec := identitytest.Register(t, s.Cwd, recorded)
			out := Run(context.Background(), s, Options{ID: rec.ID, Force: true})
			if out.Error == nil || out.Error.Code != "agent_identity_stale" || out.Error.Phase != "identity" || len(out.Effects) != 0 {
				t.Fatalf("%+v", out)
			}
		})
	}
}

// ended reports whether record id has ended_at set.
func ended(t *testing.T, cwd, id string) bool {
	t.Helper()
	s, err := identity.Existing(context.Background(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	var rec identity.Record
	if err := s.Get(identity.Kind, id, &rec); err != nil {
		t.Fatal(err)
	}
	return rec.EndedAt != nil
}

func TestStopEndsAgentRecord(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	for name, tc := range map[string]struct {
		get call
		o   func(id string) Options
	}{
		"by name": {call{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: live}, func(string) Options { return Options{Name: "worker"} }},
		"by id":   {call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live}, func(id string) Options { return Options{ID: id} }},
	} {
		t.Run(name, func(t *testing.T) {
			s := fake(t, tc.get, call{Method: "pane.close", Params: map[string]any{"pane_id": "w1:p3"}, Result: herdrscript.OK()})
			s.Cwd = identitytest.Repository(t)
			rec := identitytest.Register(t, s.Cwd, live.Agent)
			out := Run(context.Background(), s, tc.o(rec.ID))
			if out.Status != "success" || !out.Result.(Result).Stopped {
				t.Fatalf("%+v", out)
			}
			want := []libagent.Effect{{Action: "closed", Kind: "pane", ID: "w1:p3"}, {Action: "updated", Kind: "agent_record", ID: rec.ID}}
			if !reflect.DeepEqual(out.Effects, want) {
				t.Fatalf("effects %+v", out.Effects)
			}
			if !ended(t, s.Cwd, rec.ID) {
				t.Fatal("record not ended")
			}
		})
	}
}

func TestStopWithoutClosingKeepsRecordLive(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	s := fake(t, call{Method: "agent.get", Result: live}, call{Method: "pane.close", Err: &herdr.Error{Code: "transport_error", Message: "lost", Uncertain: true}})
	s.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, s.Cwd, live.Agent)
	if out := Run(context.Background(), s, Options{Name: "worker"}); out.Status != "unknown" || ended(t, s.Cwd, rec.ID) {
		t.Fatalf("%+v", out)
	}
}

// A closed pane whose record cannot be ended is a partial stop.
func TestStopRecordFailureAfterCloseIsPartial(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	for name, tc := range map[string]struct {
		get    call
		o      func(id string) Options
		break_ func(t *testing.T, agents string)
	}{
		// Live cannot read an undecodable record.
		"lookup by name": {call{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: live}, func(string) Options { return Options{Name: "worker"} },
			func(t *testing.T, agents string) {
				if err := os.WriteFile(filepath.Join(agents, "0000beef.json"), []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			}},
		// End cannot write into a read-only record directory.
		"end by id": {call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live}, func(id string) Options { return Options{ID: id} },
			func(t *testing.T, agents string) {
				if err := os.Chmod(agents, 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(agents, 0o700) })
			}},
	} {
		t.Run(name, func(t *testing.T) {
			s := fake(t, tc.get, call{Method: "pane.close", Params: map[string]any{"pane_id": "w1:p3"}, Result: herdrscript.OK()})
			s.Cwd = identitytest.Repository(t)
			rec := identitytest.Register(t, s.Cwd, live.Agent)
			tc.break_(t, filepath.Join(s.Cwd, ".fledge", "state", identity.Kind))
			out := Run(context.Background(), s, tc.o(rec.ID))
			if out.Status != "partial" || out.Error == nil || out.Error.Phase != "state" ||
				!reflect.DeepEqual(out.Effects, []libagent.Effect{{Action: "closed", Kind: "pane", ID: "w1:p3"}}) {
				t.Fatalf("%+v %+v", out, out.Error)
			}
		})
	}
}
