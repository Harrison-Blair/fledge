package message

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
)

// livePane is an agent in pane with status.
func livePane(pane, status string) herdr.Pane {
	p := herdrscript.Pane(pane, "w1", "w1:t1")
	p.AgentStatus = status
	return p
}

func getCall(target string, p herdr.Pane) call {
	return call{Method: "agent.get", Params: map[string]any{"target": target}, Result: herdrscript.Info(p)}
}

func promptCall(target string, p herdr.Pane, err error) call {
	return call{Method: "agent.prompt", Params: map[string]any{"target": target, "text": header + "hi"}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}, Err: err}
}

func names(v ...string) Options {
	return Options{Selection: selector.Selection{Names: v}, Body: "hi", BodySet: true}
}

// rows summarizes a fan-out as target=outcome/message_id/pane per row.
func rows(t *testing.T, out libagent.Outcome) string {
	t.Helper()
	r, ok := out.Result.(FanOut)
	if !ok || r.Mode != "fan-out" {
		t.Fatalf("not a fan-out: %+v", out)
	}
	var parts []string
	for _, row := range r.Targets {
		id, pane := "-", "-"
		if row.MessageID != nil {
			id = *row.MessageID
		}
		if row.Agent != nil {
			pane = libagent.Display(row.Agent.PaneID)
		}
		parts = append(parts, row.Target+"="+row.Outcome+"/"+id+"/"+pane)
	}
	return strings.Join(parts, " ")
}

func TestFanOutSendsIdenticalHeaderToEachTargetInOrder(t *testing.T) {
	a, b := livePane("w1:p1", "idle"), livePane("w1:p2", "idle")
	s := fake(t, getCall("worker", a), getCall("w1:p2", b), senderCall(), promptCall("worker", a, nil), promptCall("w1:p2", b, nil))
	o := names("worker")
	o.Panes = []string{"w1:p2"}
	out := run(context.Background(), s, o, nil, "m-0a1b2c")
	if got, want := rows(t, out), "worker=submitted/m-0a1b2c/w1:p1 w1:p2=submitted/m-0a1b2c/w1:p2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if out.Status != "success" || out.Error != nil || len(out.Effects) != 2 || out.Effects[1] != (libagent.Effect{Action: "submitted", Kind: "message", ID: "w1:p2"}) {
		t.Fatalf("%+v", out)
	}
}

func TestFanOutFailedDeliveryIsPartialAndKeepsSuccessfulRows(t *testing.T) {
	a, b, c := livePane("w1:p1", "idle"), livePane("w1:p2", "blocked"), livePane("w1:p3", "idle")
	s := fake(t, getCall("a", a), getCall("b", b), getCall("c", c), senderCall(),
		promptCall("a", a, nil), promptCall("b", b, &herdr.Error{Code: "agent_blocked", Message: "approval"}), promptCall("c", c, nil))
	out := run(context.Background(), s, names("a", "b", "c"), nil, "m-0a1b2c")
	if got, want := rows(t, out), "a=submitted/m-0a1b2c/w1:p1 b=rejected/-/w1:p2 c=submitted/m-0a1b2c/w1:p3"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	row := out.Result.(FanOut).Targets[1]
	if out.Status != "partial" || out.ExitCode() != 1 || out.Error.Code != "agent_blocked" || !strings.Contains(out.Error.Message, "1 of 3 targets failed: b (agent_blocked)") ||
		row.Error == nil || row.Error.Code != "agent_blocked" || len(out.Effects) != 2 {
		t.Fatalf("%+v %+v", out, row)
	}
}

func TestFanOutEveryDeliveryRejectedIsPartial(t *testing.T) {
	a, b := livePane("w1:p1", "blocked"), livePane("w1:p2", "idle")
	s := fake(t, getCall("a", a), getCall("b", b), senderCall(),
		promptCall("a", a, &herdr.Error{Code: "agent_blocked", Message: "approval"}), promptCall("b", b, &herdr.Error{Code: "agent_not_ready", Message: "busy"}))
	out := run(context.Background(), s, names("a", "b"), nil, "m-0a1b2c")
	if got, want := rows(t, out), "a=rejected/-/w1:p1 b=rejected/-/w1:p2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if out.Status != "partial" || out.ExitCode() != 1 || out.Error.Code != "operation_failed" || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
}

func TestFanOutResolutionFailureRejectsBeforeDelivery(t *testing.T) {
	a := livePane("w1:p1", "idle")
	s := fake(t, getCall("a", a), call{Method: "agent.get", Params: map[string]any{"target": "b"}, Err: &herdr.Error{Code: "agent_not_found", Message: "no b"}})
	out := run(context.Background(), s, names("a", "b"), nil, "m-0a1b2c")
	if out.Status != "rejected" || out.ExitCode() != 1 || out.Error.Code != "agent_not_found" || out.Error.Phase != "agent.get" || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
}

func TestFanOutConfirmAppliesPerTarget(t *testing.T) {
	a, b := livePane("w1:p1", "idle"), livePane("w1:p2", "working")
	wait := map[string]any{"until": []string{"working", "done", "idle", "blocked"}, "timeout_ms": 5000}
	confirm := func(target string, before herdr.Pane, after string, err error) call {
		c := promptCall(target, livePane(before.PaneID, after), err)
		c.Params["wait"] = wait
		return c
	}
	s := fake(t, getCall("a", a), getCall("b", b), getCall("c", a), senderCall(),
		confirm("a", a, "working", nil), confirm("b", b, "working", nil), confirm("c", a, "", &herdr.Error{Code: "agent_prompt_stalled", Message: "no activity"}))
	o := names("a", "b", "c")
	o.Confirm, o.Timeout = true, 5*time.Second
	out := run(context.Background(), s, o, nil, "m-0a1b2c")
	if got, want := rows(t, out), "a=confirmed/m-0a1b2c/w1:p1 b=already_working/m-0a1b2c/w1:p2 c=unconfirmed/m-0a1b2c/w1:p1"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if out.Status != "partial" || out.Error.Code != "agent_prompt_stalled" || len(out.Effects) != 3 {
		t.Fatalf("%+v", out)
	}
}

func TestFanOutUncertainDeliveryKeepsMessageID(t *testing.T) {
	a, b := livePane("w1:p1", "idle"), livePane("w1:p2", "idle")
	s := fake(t, getCall("a", a), getCall("b", b), senderCall(),
		promptCall("a", a, &herdr.Error{Code: "transport_error", Message: "EOF", Uncertain: true}), promptCall("b", b, nil))
	out := run(context.Background(), s, names("a", "b"), nil, "m-0a1b2c")
	if got, want := rows(t, out), "a=unknown/m-0a1b2c/w1:p1 b=submitted/m-0a1b2c/w1:p2"; got != want || out.Status != "partial" {
		t.Fatalf("got %q (%s), want %q", got, out.Status, want)
	}
}

func listAgents(panes ...herdr.Pane) call {
	agents := make([]herdr.AgentDetails, len(panes))
	for i, p := range panes {
		agents[i] = herdrscript.Info(p).Agent
	}
	return call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": agents}}
}

func TestFanOutFilterExcludesCaller(t *testing.T) {
	caller, a, b := livePane("old:p1", "idle"), livePane("w1:p1", "idle"), livePane("w1:p2", "idle")
	s := fake(t, listAgents(a, caller, b), senderCall(), promptCall("w1:p1", a, nil), promptCall("w1:p2", b, nil))
	out := run(context.Background(), s, Options{Selection: selector.Selection{Filter: selector.Filter{States: []string{"idle"}}}, Body: "hi", BodySet: true}, nil, "m-0a1b2c")
	if got, want := rows(t, out), "w1:p1=submitted/m-0a1b2c/w1:p1 w1:p2=submitted/m-0a1b2c/w1:p2"; got != want || out.Status != "success" {
		t.Fatalf("got %q (%+v), want %q", got, out, want)
	}
}

func TestFilterMatchingOneTargetKeepsSingleResult(t *testing.T) {
	caller, a := livePane("old:p1", "idle"), livePane("w1:p1", "idle")
	s := fake(t, listAgents(caller, a), senderCall(), promptCall("w1:p1", a, nil))
	out := run(context.Background(), s, Options{Selection: selector.Selection{Filter: selector.Filter{States: []string{"idle"}}}, Body: "hi", BodySet: true}, nil, "m-0a1b2c")
	if r, ok := out.Result.(Result); out.Status != "success" || !ok || !r.Submitted || *r.PaneID != "w1:p1" {
		t.Fatalf("%+v", out)
	}
}

func TestFilterWithNoMatchFails(t *testing.T) {
	caller := livePane("old:p1", "idle")
	s := fake(t, listAgents(caller, livePane("w1:p1", "working")))
	out := run(context.Background(), s, Options{Selection: selector.Selection{Filter: selector.Filter{States: []string{"idle"}}}, Body: "hi", BodySet: true}, nil, "m-0a1b2c")
	if out.Status != "rejected" || out.ExitCode() != 1 || out.Error.Code != "no_agents_matched" || out.Error.Phase != "selection" {
		t.Fatalf("%+v", out)
	}
}

func TestFanOutExplicitCallerIsMessaged(t *testing.T) {
	caller, a := livePane("old:p1", "working"), livePane("w1:p1", "idle")
	s := fake(t, getCall("old:p1", caller), getCall("w1:p1", a), senderCall(), promptCall("old:p1", caller, nil), promptCall("w1:p1", a, nil))
	out := run(context.Background(), s, Options{Selection: selector.Selection{Panes: []string{"old:p1", "w1:p1"}}, Body: "hi", BodySet: true}, nil, "m-0a1b2c")
	if got, want := rows(t, out), "old:p1=submitted/m-0a1b2c/old:p1 w1:p1=submitted/m-0a1b2c/w1:p1"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFanOutRendering(t *testing.T) {
	id := "m-0a1b2c"
	agent := func(pane, status string) *libagent.AgentRow {
		r := libagent.NewAgentRow(livePane(pane, status))
		return &r
	}
	blocked := &libagent.Failure{Code: "agent_blocked", Message: "agent_blocked: approval", Phase: "agent.prompt"}
	stalled := &libagent.Failure{Code: "agent_prompt_stalled", Message: "agent_prompt_stalled: no activity", Phase: "agent.prompt"}
	uncertain := &libagent.Failure{Code: "transport_error", Message: "transport_error: EOF", Phase: "agent.prompt"}
	result := FanOut{Mode: "fan-out", Targets: []Row{
		{Target: "a", Outcome: "submitted", MessageID: &id, Agent: agent("w1:p1", "idle")},
		{Target: "b", Outcome: "confirmed", MessageID: &id, Agent: agent("w1:p2", "working")},
		{Target: "c", Outcome: "already_working", MessageID: &id, Agent: agent("w1:p3", "working")},
		{Target: "d", Outcome: "unconfirmed", MessageID: &id, Agent: agent("w1:p4", "idle"), Error: stalled},
		{Target: "e", Outcome: "unknown", MessageID: &id, Agent: agent("w1:p5", "idle"), Error: uncertain},
		{Target: "f", Outcome: "rejected", Agent: agent("w1:p6", "blocked"), Error: blocked},
	}}
	out := libagent.Outcome{Status: "partial", Result: result, Error: &libagent.Failure{Code: "operation_failed", Message: "3 of 6 targets failed: d (agent_prompt_stalled), e (transport_error), f (agent_blocked)", Phase: "agent.prompt"}}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	want := `partial: 3 of 6 targets failed: d (agent_prompt_stalled), e (transport_error), f (agent_blocked) (agent.prompt)
Message m-0a1b2c submitted to a (w1:p1).
Message m-0a1b2c submitted to b (w1:p2); activity confirmed (working).
Message m-0a1b2c submitted to c (w1:p3) while the agent was already observed working; this prompt's start is not confirmed.
Message m-0a1b2c submitted to d (w1:p4), but activity was not confirmed; do not resend it: agent_prompt_stalled: no activity.
Message m-0a1b2c may have been submitted to e (w1:p5); do not resend it: transport_error: EOF.
Message not submitted to f (w1:p6): agent_blocked: approval.
`
	if b.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b.String(), want)
	}
	b.Reset()
	if err := out.Write(&b, true, Render); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Result struct {
			Mode    string           `json:"mode"`
			Targets []map[string]any `json:"targets"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b.Bytes(), &decoded); err != nil || decoded.Result.Mode != "fan-out" || len(decoded.Result.Targets) != 6 {
		t.Fatalf("%v %s", err, b.String())
	}
	for _, key := range []string{"target", "outcome", "message_id", "agent", "error"} {
		if _, ok := decoded.Result.Targets[5][key]; !ok || len(decoded.Result.Targets[5]) != 5 {
			t.Fatalf("row keys %v lack %s", decoded.Result.Targets[5], key)
		}
	}
	herdrscript.CheckOutputFailures(t, Render, out)
}
