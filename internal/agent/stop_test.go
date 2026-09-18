package agent

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

func liveAgent(status string) herdr.Pane {
	p := pane("w1:p3", "w1", "w1:t2")
	p.AgentStatus = status
	name, harness, cwd := "worker", "claude", "/repo"
	p.Name, p.Agent, p.Cwd = &name, &harness, &cwd
	return p
}
func info(p herdr.Pane) herdr.AgentResult { return herdr.AgentResult{Type: "agent_info", Agent: p} }
func closed() map[string]any              { return map[string]any{"type": "ok"} }

func TestStopIdleClosesResolvedPane(t *testing.T) {
	p := liveAgent("idle")
	s := fake(t, call{method: "agent.get", params: map[string]any{"target": "worker"}, result: info(p)}, call{method: "pane.close", params: map[string]any{"pane_id": "w1:p3"}, result: closed()})
	out := s.Stop(context.Background(), StopOptions{Name: "worker"})
	if out.Operation != "agent.stop" || out.Status != "success" || out.Error != nil || out.ExitCode() != 0 {
		t.Fatalf("%+v", out)
	}
	want := StopResult{AgentRow: row(p), Stopped: true}
	if !reflect.DeepEqual(out.Result, want) {
		t.Fatalf("result %+v want %+v", out.Result, want)
	}
	if !reflect.DeepEqual(out.Effects, []Effect{{Action: "closed", Kind: "pane", ID: "w1:p3"}}) {
		t.Fatalf("effects %+v", out.Effects)
	}
}
func TestStopByPaneTargetsThatPane(t *testing.T) {
	p := liveAgent("done")
	s := fake(t, call{method: "agent.get", params: map[string]any{"target": "w1:p3"}, result: info(p)}, call{method: "pane.close", params: map[string]any{"pane_id": "w1:p3"}, result: closed()})
	out := s.Stop(context.Background(), StopOptions{Pane: "w1:p3"})
	if out.Status != "success" || !out.Result.(StopResult).Stopped {
		t.Fatalf("%+v", out)
	}
}
func TestStopBusyRequiresForce(t *testing.T) {
	for _, status := range []string{"working", "blocked", "unknown"} {
		t.Run(status, func(t *testing.T) {
			s := fake(t, call{method: "agent.get", result: info(liveAgent(status))})
			out := s.Stop(context.Background(), StopOptions{Name: "worker"})
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
	s := fake(t, call{method: "agent.get", result: info(liveAgent("working"))}, call{method: "pane.close", params: map[string]any{"pane_id": "w1:p3"}, result: closed()})
	out := s.Stop(context.Background(), StopOptions{Name: "worker", Force: true})
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
}
func TestStopUnknownAgentDoesNotClose(t *testing.T) {
	s := fake(t, call{method: "agent.get", err: &herdr.Error{Code: "agent_not_found", Message: "no such agent"}})
	out := s.Stop(context.Background(), StopOptions{Name: "ghost"})
	if out.Status != "rejected" || out.ExitCode() != 1 || out.Error.Code != "agent_not_found" || out.Error.Phase != "agent.get" {
		t.Fatalf("%+v", out)
	}
}
func TestStopMalformedAgentInfoDoesNotClose(t *testing.T) {
	s := fake(t, call{method: "agent.get", result: herdr.AgentResult{Type: "agent_info", Agent: pane("", "w1", "w1:t2")}})
	out := s.Stop(context.Background(), StopOptions{Name: "worker"})
	if out.Status != "rejected" || out.Error.Phase != "agent.get" {
		t.Fatalf("%+v", out)
	}
}
func TestStopLostCloseIsUnknown(t *testing.T) {
	s := fake(t, call{method: "agent.get", result: info(liveAgent("idle"))}, call{method: "pane.close", err: &herdr.Error{Code: "transport_error", Message: "lost", Uncertain: true}})
	out := s.Stop(context.Background(), StopOptions{Name: "worker"})
	if out.Status != "unknown" || out.Error.Code != "transport_error" || out.Error.Phase != "pane.close" || out.ExitCode() != 1 {
		t.Fatalf("%+v", out)
	}
	if r := out.Result.(StopResult); r.Stopped {
		t.Fatalf("stopped claimed: %+v", r)
	}
}
func TestStopWrongCloseResultIsUnknown(t *testing.T) {
	s := fake(t, call{method: "agent.get", result: info(liveAgent("idle"))}, call{method: "pane.close", result: map[string]any{"type": "pane_info"}})
	out := s.Stop(context.Background(), StopOptions{Name: "worker"})
	if out.Status != "unknown" || out.Error.Phase != "pane.close" {
		t.Fatalf("%+v", out)
	}
}
func TestStopMissingPaneIsFailure(t *testing.T) {
	s := fake(t, call{method: "agent.get", result: info(liveAgent("idle"))}, call{method: "pane.close", err: &herdr.Error{Code: "pane_not_found", Message: "gone"}})
	out := s.Stop(context.Background(), StopOptions{Name: "worker"})
	if out.Status == "success" || out.ExitCode() != 1 || out.Error.Code != "pane_not_found" || out.Result.(StopResult).Stopped {
		t.Fatalf("%+v", out)
	}
}
func TestStopRequiresExactlyOneTarget(t *testing.T) {
	for name, o := range map[string]StopOptions{"neither": {}, "both": {Name: "worker", Pane: "w1:p3"}} {
		t.Run(name, func(t *testing.T) {
			out := fake(t).Stop(context.Background(), o)
			if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Phase != "validation" {
				t.Fatalf("%+v", out)
			}
		})
	}
}
func TestStopJSONEnvelope(t *testing.T) {
	s := fake(t, call{method: "agent.get", result: info(liveAgent("idle"))}, call{method: "pane.close", result: closed()})
	var b bytes.Buffer
	if err := s.Stop(context.Background(), StopOptions{Name: "worker"}).Write(&b, true); err != nil {
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
		row  AgentRow
		want string
	}{
		{"named", row(liveAgent("idle")), "Stopped worker (claude) in w1:p3.\n"},
		{"unnamed", row(herdr.Pane{PaneID: "w1:p3", WorkspaceID: "w1", TabID: "w1:t2", AgentStatus: "idle"}), "Stopped - (-) in w1:p3.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := (Outcome{Operation: "agent.stop", Status: "success", Result: StopResult{AgentRow: tc.row, Stopped: true}}).Write(&b, false); err != nil {
				t.Fatal(err)
			}
			if b.String() != tc.want {
				t.Fatalf("%q", b.String())
			}
		})
	}
}
