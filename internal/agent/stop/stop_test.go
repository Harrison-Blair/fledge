package stop

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

type call = herdrscript.Call

var fake = herdrscript.Client

func TestStopIdleClosesResolvedPane(t *testing.T) {
	p := herdrscript.LiveAgent("idle")
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: herdrscript.Info(p)}, call{Method: "pane.close", Params: map[string]any{"pane_id": "w1:p3"}, Result: herdrscript.OK()})
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}})
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
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Panes: []string{"w1:p3"}}})
	if out.Status != "success" || !out.Result.(Result).Stopped {
		t.Fatalf("%+v", out)
	}
}
func TestStopBusyRequiresForce(t *testing.T) {
	for _, status := range []string{"working", "blocked", "unknown"} {
		t.Run(status, func(t *testing.T) {
			calls := []call{{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent(status))}}
			if status == "working" {
				calls = append(calls, call{Method: "agent.wait", Err: &herdr.Error{Code: "timeout", Message: "timed out"}})
			}
			s := fake(t, calls...)
			out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}})
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
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}, Force: true})
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
}
func TestStopUnknownAgentDoesNotClose(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Err: &herdr.Error{Code: "agent_not_found", Message: "no such agent"}})
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"ghost"}}})
	if out.Status != "rejected" || out.ExitCode() != 1 || out.Error.Code != "agent_not_found" || out.Error.Phase != "agent.get" {
		t.Fatalf("%+v", out)
	}
}
func TestStopMalformedAgentInfoDoesNotClose(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.Pane("", "w1", "w1:t2"))})
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}})
	if out.Status != "rejected" || out.Error.Phase != "agent.get" {
		t.Fatalf("%+v", out)
	}
}
func TestStopLostCloseIsUnknown(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("idle"))}, call{Method: "pane.close", Err: &herdr.Error{Code: "transport_error", Message: "lost", Uncertain: true}})
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}})
	if out.Status != "unknown" || out.Error.Code != "transport_error" || out.Error.Phase != "pane.close" || out.ExitCode() != 1 {
		t.Fatalf("%+v", out)
	}
	if r := out.Result.(Result); r.Stopped {
		t.Fatalf("stopped claimed: %+v", r)
	}
}
func TestStopWrongCloseResultIsUnknown(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("idle"))}, call{Method: "pane.close", Result: map[string]any{"type": "pane_info"}})
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}})
	if out.Status != "unknown" || out.Error.Phase != "pane.close" {
		t.Fatalf("%+v", out)
	}
}
func TestStopMissingPaneIsFailure(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("idle"))}, call{Method: "pane.close", Err: &herdr.Error{Code: "pane_not_found", Message: "gone"}})
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}})
	if out.Status == "success" || out.ExitCode() != 1 || out.Error.Code != "pane_not_found" || out.Result.(Result).Stopped {
		t.Fatalf("%+v", out)
	}
}
func TestStopRequiresTargetsOrFilter(t *testing.T) {
	for name, o := range map[string]Options{
		"neither":   {},
		"mixed":     {Selection: selector.Selection{Names: []string{"worker"}, Filter: selector.Filter{States: []string{"idle"}}}},
		"duplicate": {Selection: selector.Selection{Names: []string{"worker", "worker"}}},
	} {
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
	if err := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}}).Write(&b, true, Render); err != nil {
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
	if out := Run(context.Background(), s, Options{Selection: selector.Selection{IDs: []string{rec.ID}}}); out.Status != "success" || !out.Result.(Result).Stopped {
		t.Fatalf("%+v", out)
	}
}

func TestStopByStaleIDDoesNotClose(t *testing.T) {
	for name, get := range map[string]call{
		"other terminal": {Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: herdrscript.Info(herdrscript.LiveAgent("idle"))},
		"no agent":       {Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Err: &herdr.Error{Code: "agent_not_found", Message: "gone"}},
	} {
		t.Run(name, func(t *testing.T) {
			s := fake(t, get, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []any{}}}, call{Method: "pane.list", Result: map[string]any{"type": "pane_list", "panes": []any{}}})
			s.Cwd = identitytest.Repository(t)
			recorded := herdrscript.Info(herdrscript.LiveAgent("idle")).Agent
			recorded.TerminalID = "term_old"
			rec := identitytest.Register(t, s.Cwd, recorded)
			out := Run(context.Background(), s, Options{Selection: selector.Selection{IDs: []string{rec.ID}}, Force: true})
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
		"by name": {call{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: live}, func(string) Options { return Options{Selection: selector.Selection{Names: []string{"worker"}}} }},
		"by id":   {call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live}, func(id string) Options { return Options{Selection: selector.Selection{IDs: []string{id}}} }},
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

// An agent stopping its own pane is hung up by pane.close before control
// returns, so the record must already be ended when the pane closes.
func TestStopEndsRecordBeforeClosingPane(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	var cwd, id string
	endedAtClose := false
	s := fake(t, call{Method: "agent.get", Result: live}, call{Method: "pane.close", Result: herdrscript.OK(), Before: func() { endedAtClose = ended(t, cwd, id) }})
	s.Cwd = identitytest.Repository(t)
	cwd, id = s.Cwd, identitytest.Register(t, s.Cwd, live.Agent).ID
	if out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}}); out.Status != "success" || !endedAtClose || !ended(t, cwd, id) {
		t.Fatalf("ended at close %v: %+v", endedAtClose, out)
	}
}

// A failed close rolls the ended record back to live, reporting no record
// effect.
func TestStopWithoutClosingKeepsRecordLive(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		status string
	}{
		"known":     {&herdr.Error{Code: "internal_error", Message: "refused"}, "rejected"},
		"uncertain": {&herdr.Error{Code: "transport_error", Message: "lost", Uncertain: true}, "unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			live := herdrscript.Info(herdrscript.LiveAgent("idle"))
			var cwd, id string
			endedAtClose := false
			s := fake(t, call{Method: "agent.get", Result: live}, call{Method: "pane.close", Err: tc.err, Before: func() { endedAtClose = ended(t, cwd, id) }})
			s.Cwd = identitytest.Repository(t)
			cwd, id = s.Cwd, identitytest.Register(t, s.Cwd, live.Agent).ID
			out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}})
			if out.Status != tc.status || out.Error.Phase != "pane.close" || len(out.Effects) != 0 || !endedAtClose || ended(t, cwd, id) || !isLive(t, cwd, id) {
				t.Fatalf("ended at close %v: %+v", endedAtClose, out)
			}
		})
	}
}

// isLive reports whether record id is among the live records scans find.
func isLive(t *testing.T, cwd, id string) bool {
	t.Helper()
	s, err := identity.Existing(context.Background(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	records, err := identity.LiveByTerminal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range records {
		if rec.ID == id {
			return true
		}
	}
	return false
}

// A record another actor ended after stop looked it up is not stop's to
// reopen when the close fails.
func TestStopFailedCloseKeepsOthersEnd(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	var cwd, id string
	endOthers := func() {
		s, err := identity.Existing(context.Background(), cwd)
		if err != nil {
			t.Fatal(err)
		}
		if err := identity.End(s, id); err != nil {
			t.Fatal(err)
		}
	}
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live, Before: endOthers},
		call{Method: "pane.close", Err: &herdr.Error{Code: "internal_error", Message: "refused"}})
	s.Cwd = identitytest.Repository(t)
	cwd, id = s.Cwd, identitytest.Register(t, s.Cwd, live.Agent).ID
	out := Run(context.Background(), s, Options{Selection: selector.Selection{IDs: []string{id}}})
	if out.Error == nil || out.Error.Code != "internal_error" || out.Error.Phase != "pane.close" || !ended(t, cwd, id) || isLive(t, cwd, id) {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

// A record another actor ended after stop looked it up was not updated by
// stop, so a successful close reports only the closed pane.
func TestStopReportsOnlyItsOwnRecordEnd(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	var cwd, id string
	endOthers := func() {
		s, err := identity.Existing(context.Background(), cwd)
		if err != nil {
			t.Fatal(err)
		}
		if err := identity.End(s, id); err != nil {
			t.Fatal(err)
		}
	}
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live, Before: endOthers},
		call{Method: "pane.close", Params: map[string]any{"pane_id": "w1:p3"}, Result: herdrscript.OK()})
	s.Cwd = identitytest.Repository(t)
	cwd, id = s.Cwd, identitytest.Register(t, s.Cwd, live.Agent).ID
	out := Run(context.Background(), s, Options{Selection: selector.Selection{IDs: []string{id}}})
	want := []libagent.Effect{{Action: "closed", Kind: "pane", ID: "w1:p3"}}
	if out.Status != "success" || out.Error != nil || !reflect.DeepEqual(out.Effects, want) || !ended(t, cwd, id) {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

// When the terminal was registered anew before the close failed, the refused
// reopen is reported alongside the close error, whose code is kept.
func TestStopFailedCloseReportsRefusedReopen(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	var cwd, newID string
	reregister := func() { newID = identitytest.Register(t, cwd, live.Agent).ID }
	s := fake(t, call{Method: "agent.get", Result: live}, call{Method: "pane.close", Err: &herdr.Error{Code: "internal_error", Message: "refused"}, Before: reregister})
	s.Cwd = identitytest.Repository(t)
	cwd = s.Cwd
	id := identitytest.Register(t, s.Cwd, live.Agent).ID
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}})
	if out.Error == nil || out.Error.Code != "internal_error" || out.Error.Phase != "pane.close" || !strings.Contains(out.Error.Message, "refused") ||
		!strings.Contains(out.Error.Message, "agent_already_registered") || !strings.Contains(out.Error.Message, newID) {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if !ended(t, cwd, id) || isLive(t, cwd, id) || !isLive(t, cwd, newID) {
		t.Fatal("reopen must not displace the new registration")
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
		"lookup by name": {call{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: live}, func(string) Options { return Options{Selection: selector.Selection{Names: []string{"worker"}}} },
			func(t *testing.T, agents string) {
				if err := os.WriteFile(filepath.Join(agents, "0000beef.json"), []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			}},
		// End cannot write into a read-only record directory.
		"end by id": {call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live}, func(id string) Options { return Options{Selection: selector.Selection{IDs: []string{id}}} },
			func(t *testing.T, agents string) {
				if os.Geteuid() == 0 {
					t.Skip("root ignores directory permissions")
				}
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

// Stopping the agent in a terminal ends a record left there by a different
// harness without reporting it as the stopped agent's record.
func TestStopDoesNotAttributeRecordOfDifferentHarness(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: live}, call{Method: "pane.close", Params: map[string]any{"pane_id": "w1:p3"}, Result: herdrscript.OK()})
	s.Cwd = identitytest.Repository(t)
	recorded, codex := live.Agent, "codex"
	recorded.Agent = &codex
	rec := identitytest.Register(t, s.Cwd, recorded)
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}})
	if out.Status != "success" || !reflect.DeepEqual(out.Effects, []libagent.Effect{{Action: "closed", Kind: "pane", ID: "w1:p3"}}) {
		t.Fatalf("%+v", out)
	}
	if !ended(t, s.Cwd, rec.ID) {
		t.Fatal("record not ended")
	}
}

// A working agent often reports before its turn ends; stop gives it the
// default grace to settle and closes it with the settled row.
func TestStopWorkingSettlesWithinGrace(t *testing.T) {
	s := fake(t,
		call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("working"))},
		call{Method: "agent.wait", Params: map[string]any{"target": "worker", "until": []string{"idle", "done", "blocked"}, "timeout_ms": 5000}, Result: herdrscript.Waited(herdrscript.LiveAgent("working"), "done")},
		call{Method: "pane.close", Params: map[string]any{"pane_id": "w1:p3"}, Result: herdrscript.OK()})
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}})
	if out.Status != "success" || !out.Result.(Result).Stopped || *out.Result.(Result).AgentStatus != "done" {
		t.Fatalf("%+v", out)
	}
}

// Any failed settle wait, including an agent still working at grace expiry,
// is today's refusal.
func TestStopWorkingUnsettledRefuses(t *testing.T) {
	for _, code := range []string{"timeout", "agent_not_running", "transport_error"} {
		t.Run(code, func(t *testing.T) {
			s := fake(t,
				call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("working"))},
				call{Method: "agent.wait", Err: &herdr.Error{Code: code, Message: "failed", Uncertain: code == "transport_error"}})
			out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}})
			if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Phase != "guard" || out.Error.Message != "agent worker is working; pass --force to stop it anyway" || len(out.Effects) != 0 {
				t.Fatalf("%+v", out)
			}
		})
	}
}

// A settled agent that turns out to be blocked is refused as blocked.
func TestStopWorkingSettledBlockedRefuses(t *testing.T) {
	s := fake(t,
		call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("working"))},
		call{Method: "agent.wait", Result: herdrscript.Waited(herdrscript.LiveAgent("working"), "blocked")})
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}})
	if out.Status != "rejected" || out.Error.Phase != "guard" || !strings.Contains(out.Error.Message, "is blocked") {
		t.Fatalf("%+v", out)
	}
}

// A settled row from another terminal is not the agent stop inspected.
func TestStopWorkingSettledOtherTerminalRefuses(t *testing.T) {
	other := herdrscript.Waited(herdrscript.LiveAgent("working"), "idle")
	other.Agent.TerminalID = "term_other"
	s := fake(t,
		call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("working"))},
		call{Method: "agent.wait", Result: other})
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}})
	if out.Status != "rejected" || out.Error.Phase != "guard" || !strings.Contains(out.Error.Message, "is working") {
		t.Fatalf("%+v", out)
	}
}

// An --id target's settled row must still match its record.
func TestStopByIDSettledRowIsVerified(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("working"))
	settled := herdrscript.Waited(herdrscript.LiveAgent("working"), "idle")
	codex := "codex"
	settled.Agent.Agent = &codex
	s := fake(t,
		call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live},
		call{Method: "agent.wait", Params: map[string]any{"target": "w1:p3", "until": []string{"idle", "done", "blocked"}, "timeout_ms": 5000}, Result: settled})
	s.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, s.Cwd, live.Agent)
	out := Run(context.Background(), s, Options{Selection: selector.Selection{IDs: []string{rec.ID}}})
	if out.Status != "rejected" || out.Error.Phase != "guard" || !strings.Contains(out.Error.Message, "is working") || ended(t, s.Cwd, rec.ID) {
		t.Fatalf("%+v", out)
	}
}

// --grace tunes the settle wait; --grace 0 refuses at once with no agent.wait.
func TestStopGraceFlag(t *testing.T) {
	s := fake(t,
		call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("working"))},
		call{Method: "agent.wait", Params: map[string]any{"target": "worker", "until": []string{"idle", "done", "blocked"}, "timeout_ms": 1500}, Result: herdrscript.Waited(herdrscript.LiveAgent("working"), "idle")},
		call{Method: "pane.close", Params: map[string]any{"pane_id": "w1:p3"}, Result: herdrscript.OK()})
	if out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}, Grace: 1500 * time.Millisecond, GraceSet: true}); out.Status != "success" {
		t.Fatalf("%+v", out)
	}
	s = fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("working"))})
	if out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}, GraceSet: true}); out.Status != "rejected" || out.Error.Phase != "guard" {
		t.Fatalf("%+v", out)
	}
}

func TestStopGraceValidation(t *testing.T) {
	for name, o := range map[string]Options{
		"negative":   {Grace: -time.Second, GraceSet: true},
		"over max":   {Grace: time.Minute + time.Millisecond, GraceSet: true},
		"with force": {Grace: time.Second, GraceSet: true, Force: true},
		"zero force": {GraceSet: true, Force: true},
	} {
		t.Run(name, func(t *testing.T) {
			o.Selection = selector.Selection{Names: []string{"worker"}}
			out := Run(context.Background(), fake(t), o)
			if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Phase != "validation" || !strings.Contains(out.Error.Message, "--grace") {
				t.Fatalf("%+v", out)
			}
		})
	}
	s := fake(t,
		call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("working"))},
		call{Method: "agent.wait", Params: map[string]any{"target": "worker", "until": []string{"idle", "done", "blocked"}, "timeout_ms": 60000}, Result: herdrscript.Waited(herdrscript.LiveAgent("working"), "idle")},
		call{Method: "pane.close", Result: herdrscript.OK()})
	if out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}, Grace: time.Minute, GraceSet: true}); out.Status != "success" {
		t.Fatalf("max grace: %+v", out)
	}
}
