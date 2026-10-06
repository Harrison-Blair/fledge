package message

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
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
func rows(t *testing.T, out cli.Outcome) string {
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
			pane = cli.Display(row.Agent.PaneID)
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
	if out.Status != "success" || out.Error != nil || len(out.Effects) != 2 || out.Effects[1] != (cli.Effect{Action: "submitted", Kind: "message", ID: "w1:p2"}) {
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
		getCall("a", a), confirm("a", a, "working", nil), getCall("b", b), confirm("b", b, "working", nil),
		getCall("c", a), confirm("c", a, "", &herdr.Error{Code: "agent_prompt_stalled", Message: "no activity"}))
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

// liveHerdr answers agent.get with each target's status at that moment and
// agent.prompt with working, first running the prompt's effect on the others.
type liveHerdr struct {
	panes, statuses map[string]string
	effects         map[string]func(statuses map[string]string)
}

func (h liveHerdr) Call(_ context.Context, method string, params, result any) error {
	target := params.(map[string]any)["target"].(string)
	if target == "old:p1" {
		b, _ := json.Marshal(senderCall().Result)
		return json.Unmarshal(b, result)
	}
	var r herdr.AgentResult
	switch method {
	case "agent.get":
		r = herdrscript.Info(livePane(h.panes[target], h.statuses[target]))
	case "agent.prompt":
		if effect := h.effects[target]; effect != nil {
			effect(h.statuses)
		}
		h.statuses[target] = "working"
		r = herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: livePane(h.panes[target], "working")}}
	}
	b, _ := json.Marshal(r)
	return json.Unmarshal(b, result)
}

// Each target's already-working decision uses its status read just before
// its own message, not the status from the lookup before any was sent.
func TestFanOutConfirmReadsEachStatusBeforeSending(t *testing.T) {
	for _, tc := range []struct{ name, before, during, want string }{
		{"became busy", "idle", "working", "already_working"},
		{"went idle", "working", "idle", "confirmed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := liveHerdr{panes: map[string]string{"a": "w1:p1", "b": "w1:p2"}, statuses: map[string]string{"a": "idle", "b": tc.before},
				effects: map[string]func(map[string]string){"a": func(s map[string]string) { s["b"] = tc.during }}}
			o := names("a", "b")
			o.Confirm, o.Timeout = true, 5*time.Second
			out := run(context.Background(), libagent.Client{API: h, CallerPane: "old:p1", Cwd: t.TempDir()}, o, nil, "m-0a1b2c")
			if got, want := rows(t, out), "a=confirmed/m-0a1b2c/w1:p1 b="+tc.want+"/m-0a1b2c/w1:p2"; got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

// A target whose status cannot be reread before its confirmed message is not
// messaged; the others still are.
func TestFanOutConfirmRereadFailureRejectsTarget(t *testing.T) {
	a, b := livePane("w1:p1", "idle"), livePane("w1:p2", "idle")
	confirm := promptCall("b", livePane("w1:p2", "working"), nil)
	confirm.Params["wait"] = map[string]any{"until": []string{"working", "done", "idle", "blocked"}, "timeout_ms": 5000}
	s := fake(t, getCall("a", a), getCall("b", b), senderCall(),
		call{Method: "agent.get", Params: map[string]any{"target": "a"}, Err: &herdr.Error{Code: "agent_not_found", Message: "gone"}}, getCall("b", b), confirm)
	o := names("a", "b")
	o.Confirm, o.Timeout = true, 5*time.Second
	out := run(context.Background(), s, o, nil, "m-0a1b2c")
	if got, want := rows(t, out), "a=rejected/-/w1:p1 b=confirmed/m-0a1b2c/w1:p2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if row := out.Result.(FanOut).Targets[0]; out.Status != "partial" || row.Error.Code != "agent_not_found" || row.Error.Phase != "agent.get" || len(out.Effects) != 1 {
		t.Fatalf("%+v %+v", out, row.Error)
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

// failure is the row failure that Fail records for err.
func failure(err error, phase string) *cli.Failure {
	o := cli.NewOutcome("")
	o.Fail(err, phase, false)
	return o.Error
}

func TestFanOutRendering(t *testing.T) {
	id := "m-0a1b2c"
	agent := func(pane, status string) *libagent.AgentRow {
		r := libagent.NewAgentRow(livePane(pane, status))
		return &r
	}
	blocked := failure(&herdr.Error{Code: "agent_blocked", Message: "approval"}, "agent.prompt")
	stalled := failure(&herdr.Error{Code: "agent_prompt_stalled", Message: "no activity"}, "agent.prompt")
	uncertain := failure(&herdr.Error{Code: "transport_error", Message: "EOF", Uncertain: true}, "agent.prompt")
	result := FanOut{Mode: "fan-out", Targets: []Row{
		{Target: "a", Outcome: "submitted", MessageID: &id, Agent: agent("w1:p1", "idle")},
		{Target: "b", Outcome: "confirmed", MessageID: &id, Agent: agent("w1:p2", "working")},
		{Target: "c", Outcome: "already_working", MessageID: &id, Agent: agent("w1:p3", "working")},
		{Target: "d", Outcome: "unconfirmed", MessageID: &id, Agent: agent("w1:p4", "idle"), Error: stalled},
		{Target: "e", Outcome: "unknown", MessageID: &id, Agent: agent("w1:p5", "idle"), Error: uncertain},
		{Target: "f", Outcome: "rejected", Agent: agent("w1:p6", "blocked"), Error: blocked},
	}}
	out := cli.Outcome{Status: "partial", Result: result, Error: &cli.Failure{Code: "operation_failed", Message: "3 of 6 targets failed: d (agent_prompt_stalled), e (transport_error), f (agent_blocked)", Phase: "agent.prompt"}}
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

// shiftingHerdr is a stateful Herdr: agents maps each pane to the agent it
// hosts, listed in pane order, shift reshapes them once the first message is submitted, and prompts
// records every pane prompted, in order.
type shiftingHerdr struct {
	agents  map[string]herdr.AgentDetails
	shift   func(agents map[string]herdr.AgentDetails)
	prompts *[]string
}

func (h shiftingHerdr) Call(_ context.Context, method string, params, result any) error {
	var r any
	switch method {
	case "agent.list", "pane.list":
		list := []herdr.AgentDetails{}
		for _, pane := range slices.Sorted(maps.Keys(h.agents)) {
			list = append(list, h.agents[pane])
		}
		key := "agents"
		if method == "pane.list" {
			key = "panes"
		}
		r = map[string]any{"type": strings.ReplaceAll(method, ".", "_"), key: list}
	case "agent.get":
		target := params.(map[string]any)["target"].(string)
		if target == "old:p1" {
			r = senderCall().Result
			break
		}
		a, ok := h.agents[target]
		if !ok {
			return &herdr.Error{Code: "agent_not_found", Message: "no agent in " + target}
		}
		r = herdr.AgentResult{Type: "agent_info", Agent: a}
	case "agent.prompt":
		target := params.(map[string]any)["target"].(string)
		a, ok := h.agents[target]
		if !ok {
			return &herdr.Error{Code: "agent_not_found", Message: "no agent in " + target}
		}
		*h.prompts = append(*h.prompts, target)
		if len(*h.prompts) == 1 {
			h.shift(h.agents)
		}
		a.AgentStatus = "working"
		r = herdr.AgentResult{Type: "agent_prompted", Agent: a}
	}
	b, _ := json.Marshal(r)
	return json.Unmarshal(b, result)
}

// hosted is an idle agent in pane running harness in terminal.
func hosted(pane, terminal, harness string) herdr.AgentDetails {
	p := livePane(pane, "idle")
	p.Agent, p.Name = &harness, &terminal
	a := herdrscript.Info(p).Agent
	a.TerminalID = terminal
	return a
}

// After the first recipient's message, the second registered recipient's
// terminal moves, is replaced, or runs another harness. Its message follows
// the moved terminal and is never typed into the agent now in its old pane.
func TestFanOutReresolvesRegisteredTargetsBeforeEachDelivery(t *testing.T) {
	for _, tc := range []struct {
		name  string
		shift func(map[string]herdr.AgentDetails)
		// want is b's row outcome/pane for unconfirmed and confirmed delivery.
		want, wantConfirm, prompted string
	}{
		{"moved", func(m map[string]herdr.AgentDetails) {
			moved := hosted("w1:p9", "term_b", "claude")
			moved.AgentStatus = "working"
			m["w1:p9"], m["w1:p2"] = moved, hosted("w1:p2", "term_c", "claude")
		}, "submitted/m-0a1b2c/w1:p9", "already_working/m-0a1b2c/w1:p9", "w1:p1 w1:p9"},
		{"terminal replaced", func(m map[string]herdr.AgentDetails) {
			m["w1:p2"] = hosted("w1:p2", "term_c", "claude")
		}, "rejected/-/w1:p2", "rejected/-/w1:p2", "w1:p1"},
		{"harness changed", func(m map[string]herdr.AgentDetails) {
			m["w1:p2"] = hosted("w1:p2", "term_b", "codex")
		}, "rejected/-/w1:p2", "rejected/-/w1:p2", "w1:p1"},
	} {
		for _, confirm := range []bool{false, true} {
			for _, by := range []string{"id", "filter"} {
				t.Run(fmt.Sprintf("%s/confirm=%v/%s", tc.name, confirm, by), func(t *testing.T) {
					var prompts []string
					h := shiftingHerdr{agents: map[string]herdr.AgentDetails{"w1:p1": hosted("w1:p1", "term_a", "claude"), "w1:p2": hosted("w1:p2", "term_b", "claude")}, shift: tc.shift, prompts: &prompts}
					c := libagent.Client{API: h, CallerPane: "old:p1", Cwd: identitytest.Repository(t)}
					a, b := identitytest.Register(t, c.Cwd, h.agents["w1:p1"]), identitytest.Register(t, c.Cwd, h.agents["w1:p2"])
					o := Options{Selection: selector.Selection{IDs: []string{a.ID, b.ID}}, Body: "hi", BodySet: true}
					if by == "filter" {
						o.Selection = selector.Selection{Filter: selector.Filter{States: []string{"idle"}}}
					}
					o.Confirm, o.Timeout = confirm, 5*time.Second
					want, first := tc.want, "submitted"
					if confirm {
						want, first = tc.wantConfirm, "confirmed"
					}
					out := run(context.Background(), c, o, nil, "m-0a1b2c")
					if got, want := rows(t, out), a.ID+"="+first+"/m-0a1b2c/w1:p1 "+b.ID+"="+want; got != want {
						t.Fatalf("got %q, want %q", got, want)
					}
					if got := strings.Join(prompts, " "); got != tc.prompted {
						t.Fatalf("prompted %q, want %q", got, tc.prompted)
					}
					if len(out.Effects) != len(prompts) || out.Effects[len(prompts)-1].ID != prompts[len(prompts)-1] {
						t.Fatalf("effects %+v for prompts %v", out.Effects, prompts)
					}
					if row := out.Result.(FanOut).Targets[1]; row.Outcome == "rejected" &&
						(out.Status != "partial" || out.ExitCode() != 1 || row.Error.Code != "agent_identity_stale" || row.Error.Phase != "identity") {
						t.Fatalf("%+v %+v", out, row.Error)
					}
				})
			}
		}
	}
}

// A fan-out to registered targets finds the repository root once: target
// selection and every reread share one store.
func TestFanOutRereadsReuseOneStore(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	h := shiftingHerdr{agents: map[string]herdr.AgentDetails{"w1:p1": hosted("w1:p1", "term_a", "claude"), "w1:p2": hosted("w1:p2", "term_b", "claude"), "w1:p3": hosted("w1:p3", "term_c", "claude")}, shift: func(map[string]herdr.AgentDetails) {}, prompts: new([]string)}
	c := libagent.Client{API: h, CallerPane: "old:p1", Cwd: identitytest.Repository(t)}
	var ids []string
	for _, pane := range []string{"w1:p1", "w1:p2", "w1:p3"} {
		ids = append(ids, identitytest.Register(t, c.Cwd, h.agents[pane]).ID)
	}
	bin, log := t.TempDir(), filepath.Join(t.TempDir(), "roots")
	script := fmt.Sprintf("#!/bin/sh\ncase \"$*\" in *--is-bare-repository*) echo >>%q;; esac\nexec %q \"$@\"\n", log, git)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out := run(context.Background(), c, Options{Selection: selector.Selection{IDs: ids}, Body: "hi", BodySet: true}, nil, "m-0a1b2c")
	if out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	b, _ := os.ReadFile(log)
	if got, want := strings.Count(string(b), "\n"), 1; got != want {
		t.Fatalf("resolved the repository root %d times, want %d", got, want)
	}
}

// A store that cannot be opened fails an id fan-out at phase identity
// before any target receives the message.
func TestFanOutStoreOpenFailureRejectsIDs(t *testing.T) {
	var prompts []string
	h := shiftingHerdr{agents: map[string]herdr.AgentDetails{"w1:p1": hosted("w1:p1", "term_a", "claude"), "w1:p2": hosted("w1:p2", "term_b", "claude")}, shift: func(map[string]herdr.AgentDetails) {}, prompts: &prompts}
	c := libagent.Client{API: h, CallerPane: "old:p1", Cwd: identitytest.Repository(t)}
	var ids []string
	for _, pane := range []string{"w1:p1", "w1:p2"} {
		ids = append(ids, identitytest.Register(t, c.Cwd, h.agents[pane]).ID)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexit 128\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out := run(context.Background(), c, Options{Selection: selector.Selection{IDs: ids}, Body: "hi", BodySet: true}, nil, "m-0a1b2c")
	if out.Error == nil || out.Error.Phase != "identity" || out.Status != "rejected" || out.ExitCode() != 1 || len(prompts) != 0 {
		t.Fatalf("%+v %+v prompted %v", out, out.Error, prompts)
	}
}
