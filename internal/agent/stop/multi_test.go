package stop

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

// agentIn is a named claude agent in pane with status and terminal term_x.
func agentIn(pane, name, status string) herdr.Pane {
	p := herdrscript.Pane(pane, "w1", "w1:t2")
	harness := "claude"
	p.AgentStatus, p.Name, p.Agent = status, &name, &harness
	return p
}

// withTerminal gives each pane's agent its own terminal id.
func withTerminal(p herdr.Pane, terminal string) herdr.AgentDetails {
	a := herdrscript.Info(p).Agent
	a.TerminalID = terminal
	return a
}

func getCall(target string, p herdr.Pane) call {
	return call{Method: "agent.get", Params: map[string]any{"target": target}, Result: herdrscript.Info(p)}
}

func closeCall(pane string, err error) call {
	return call{Method: "pane.close", Params: map[string]any{"pane_id": pane}, Result: herdrscript.OK(), Err: err}
}

func listCall(agents ...herdr.AgentDetails) call {
	return call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": agents}}
}

func names(v ...string) Options { return Options{Selection: selector.Selection{Names: v}} }

func noGrace(o Options) Options {
	o.GraceSet = true
	return o
}

func filter(f selector.Filter) Options { return Options{Selection: selector.Selection{Filter: f}} }

// rows summarizes a multi-target result as target=outcome/pane per row.
func rows(t *testing.T, out libagent.Outcome, mode string) string {
	t.Helper()
	r, ok := out.Result.(FanOut)
	if !ok || r.Mode != mode {
		t.Fatalf("not a %s: %+v", mode, out)
	}
	var parts []string
	for _, row := range r.Targets {
		pane := "-"
		if row.Agent != nil {
			pane = libagent.Display(row.Agent.PaneID)
		}
		parts = append(parts, row.Target+"="+row.Outcome+"/"+pane)
	}
	return strings.Join(parts, " ")
}

// recorder notes every Herdr method a command calls.
type recorder struct {
	api     libagent.API
	methods []string
}

func (r *recorder) Call(ctx context.Context, method string, params, result any) error {
	r.methods = append(r.methods, method)
	return r.api.Call(ctx, method, params, result)
}

func record(s libagent.Client) (libagent.Client, *recorder) {
	r := &recorder{api: s.API}
	s.API = r
	return s, r
}

// readOnly fails t when any method outside Herdr's read-only queries was called.
func readOnly(t *testing.T, r *recorder) {
	t.Helper()
	for _, m := range r.methods {
		if !slices.Contains([]string{"agent.get", "agent.list", "pane.list"}, m) {
			t.Fatalf("dry run called mutating %s (all: %v)", m, r.methods)
		}
	}
}

func TestStopSeveralTargetsReportsEachRow(t *testing.T) {
	a, b, c := agentIn("w1:p1", "a", "idle"), agentIn("w1:p2", "b", "working"), agentIn("w1:p3", "c", "idle")
	s := fake(t, getCall("a", a), closeCall("w1:p1", nil), getCall("b", b), getCall("c", c), closeCall("w1:p3", &herdr.Error{Code: "internal_error", Message: "refused"}))
	out := Run(context.Background(), s, noGrace(names("a", "b", "c")))
	if got, want := rows(t, out, "fan-out"), "a=stopped/w1:p1 b=refused/w1:p2 c=failed/w1:p3"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	r := out.Result.(FanOut)
	if out.Status != "partial" || out.ExitCode() != 1 || out.Error == nil || !strings.Contains(out.Error.Message, "2 of 3 targets not stopped cleanly: b (invalid_input), c (internal_error)") {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if r.Targets[0].Error != nil || r.Targets[1].Error.Phase != "guard" || !strings.Contains(r.Targets[1].Error.Message, "--force") || r.Targets[2].Error.Code != "internal_error" {
		t.Fatalf("%+v", r.Targets)
	}
	if len(out.Effects) != 1 || out.Effects[0] != (libagent.Effect{Action: "closed", Kind: "pane", ID: "w1:p1"}) {
		t.Fatalf("effects %+v", out.Effects)
	}
}

func TestStopSeveralTargetsAllStopped(t *testing.T) {
	a, b := agentIn("w1:p1", "a", "idle"), agentIn("w1:p2", "b", "done")
	s := fake(t, getCall("a", a), closeCall("w1:p1", nil), getCall("w1:p2", b), closeCall("w1:p2", nil))
	o := names("a")
	o.Panes = []string{"w1:p2"}
	out := Run(context.Background(), s, o)
	if got, want := rows(t, out, "fan-out"), "a=stopped/w1:p1 w1:p2=stopped/w1:p2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if out.Status != "success" || out.Error != nil || out.ExitCode() != 0 || len(out.Effects) != 2 {
		t.Fatalf("%+v", out)
	}
}

// An explicit target that cannot be resolved is a failed row; the others are
// still stopped.
func TestStopSeveralTargetsUnresolvedIsFailedRow(t *testing.T) {
	b := agentIn("w1:p2", "b", "idle")
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "a"}, Err: &herdr.Error{Code: "agent_not_found", Message: "no a"}}, getCall("b", b), closeCall("w1:p2", nil))
	out := Run(context.Background(), s, names("a", "b"))
	if got, want := rows(t, out, "fan-out"), "a=failed/- b=stopped/w1:p2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if out.Status != "partial" || out.Error.Code != "agent_not_found" || out.Result.(FanOut).Targets[0].Error.Phase != "agent.get" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

func TestStopForceAppliesToEveryTarget(t *testing.T) {
	a, b := agentIn("w1:p1", "a", "working"), agentIn("w1:p2", "b", "blocked")
	s := fake(t, getCall("a", a), closeCall("w1:p1", nil), getCall("b", b), closeCall("w1:p2", nil))
	o := names("a", "b")
	o.Force = true
	if got, want := rows(t, Run(context.Background(), s, o), "fan-out"), "a=stopped/w1:p1 b=stopped/w1:p2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStopGraceAppliesToEveryTarget(t *testing.T) {
	wait := func(target string, p herdr.Pane) call {
		return call{Method: "agent.wait", Params: map[string]any{"target": target, "until": []string{"idle", "done", "blocked"}, "timeout_ms": 1500}, Result: herdrscript.Waited(p, "idle")}
	}
	a, b := agentIn("w1:p1", "a", "working"), agentIn("w1:p2", "b", "working")
	s := fake(t, getCall("a", a), wait("a", a), closeCall("w1:p1", nil), getCall("b", b), wait("b", b), closeCall("w1:p2", nil))
	o := names("a", "b")
	o.Grace, o.GraceSet = 1500*time.Millisecond, true
	if got, want := rows(t, Run(context.Background(), s, o), "fan-out"), "a=stopped/w1:p1 b=stopped/w1:p2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Every row ends its own record, as a single stop does.
func TestStopSeveralTargetsEndsEachRecord(t *testing.T) {
	a, b := agentIn("w1:p1", "a", "idle"), agentIn("w1:p2", "b", "idle")
	s := fake(t,
		call{Method: "agent.get", Params: map[string]any{"target": "a"}, Result: herdr.AgentResult{Type: "agent_info", Agent: withTerminal(a, "term_a")}}, closeCall("w1:p1", nil),
		call{Method: "agent.get", Params: map[string]any{"target": "b"}, Result: herdr.AgentResult{Type: "agent_info", Agent: withTerminal(b, "term_b")}}, closeCall("w1:p2", nil))
	s.Cwd = identitytest.Repository(t)
	ra, rb := identitytest.Register(t, s.Cwd, withTerminal(a, "term_a")), identitytest.Register(t, s.Cwd, withTerminal(b, "term_b"))
	out := Run(context.Background(), s, names("a", "b"))
	if out.Status != "success" || len(out.Effects) != 4 || !ended(t, s.Cwd, ra.ID) || !ended(t, s.Cwd, rb.ID) {
		t.Fatalf("%+v", out)
	}
}

func TestStopDryRunPlansWithoutMutating(t *testing.T) {
	a, b, c := agentIn("w1:p1", "a", "idle"), agentIn("w1:p2", "b", "working"), agentIn("w1:p3", "c", "blocked")
	s, r := record(fake(t, getCall("a", a), getCall("b", b), getCall("c", c)))
	o := names("a", "b", "c")
	o.DryRun = true
	out := Run(context.Background(), s, o)
	readOnly(t, r)
	if got, want := rows(t, out, "dry-run"), "a=stop/w1:p1 b=refuse/w1:p2 c=refuse/w1:p3"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	refused := out.Result.(FanOut).Targets[1].Error
	if out.Status != "success" || out.Error != nil || out.ExitCode() != 0 || len(out.Effects) != 0 ||
		refused == nil || refused.Phase != "guard" || !strings.Contains(refused.Message, "is working") || !strings.Contains(refused.Message, "--force") {
		t.Fatalf("%+v %+v", out, refused)
	}
}

func TestStopDryRunUnresolvedIsErrorRow(t *testing.T) {
	a := agentIn("w1:p1", "a", "idle")
	s, r := record(fake(t, getCall("a", a), call{Method: "agent.get", Params: map[string]any{"target": "ghost"}, Err: &herdr.Error{Code: "agent_not_found", Message: "no ghost"}}))
	o := names("a", "ghost")
	o.DryRun = true
	out := Run(context.Background(), s, o)
	readOnly(t, r)
	if got, want := rows(t, out, "dry-run"), "a=stop/w1:p1 ghost=error/-"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if out.Status != "rejected" || out.ExitCode() != 1 || out.Error.Code != "agent_not_found" || len(out.Effects) != 0 {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

func TestStopDryRunForcePlansStop(t *testing.T) {
	s, r := record(fake(t, getCall("worker", herdrscript.LiveAgent("working"))))
	o := names("worker")
	o.DryRun, o.Force = true, true
	out := Run(context.Background(), s, o)
	readOnly(t, r)
	if got, want := rows(t, out, "dry-run"), "worker=stop/w1:p3"; got != want || out.Status != "success" {
		t.Fatalf("got %q (%+v), want %q", got, out, want)
	}
}

// A dry run neither ends records nor waits for a working agent to settle.
func TestStopDryRunKeepsRecords(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	s, r := record(fake(t, getCall("worker", herdrscript.LiveAgent("idle"))))
	s.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, s.Cwd, live.Agent)
	o := names("worker")
	o.DryRun = true
	if out := Run(context.Background(), s, o); out.Status != "success" || ended(t, s.Cwd, rec.ID) {
		t.Fatalf("%+v", out)
	}
	readOnly(t, r)
}

func TestStopFilterExcludesCaller(t *testing.T) {
	caller, a, b := agentIn("old:p1", "me", "idle"), agentIn("w1:p1", "a", "idle"), agentIn("w1:p2", "b", "idle")
	s := fake(t, listCall(withTerminal(a, "term_x"), withTerminal(caller, "term_me"), withTerminal(b, "term_x")),
		getCall("w1:p1", a), closeCall("w1:p1", nil), getCall("w1:p2", b), closeCall("w1:p2", nil))
	out := Run(context.Background(), s, filter(selector.Filter{States: []string{"idle"}}))
	if got, want := rows(t, out, "fan-out"), "w1:p1=stopped/w1:p1 w1:p2=stopped/w1:p2"; got != want || out.Status != "success" {
		t.Fatalf("got %q (%+v), want %q", got, out, want)
	}
}

func TestStopFilterWithNoMatchFails(t *testing.T) {
	caller := agentIn("old:p1", "me", "idle")
	s := fake(t, listCall(withTerminal(caller, "term_me"), withTerminal(agentIn("w1:p1", "a", "working"), "term_a")))
	out := Run(context.Background(), s, filter(selector.Filter{States: []string{"idle"}}))
	if out.Status != "rejected" || out.ExitCode() != 1 || out.Error.Code != "no_agents_matched" || out.Error.Phase != "selection" || len(out.Effects) != 0 {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

func TestStopFilterMatchingOneTargetKeepsSingleResult(t *testing.T) {
	a := agentIn("w1:p1", "a", "idle")
	s := fake(t, listCall(withTerminal(a, "term_x")), getCall("w1:p1", a), closeCall("w1:p1", nil))
	out := Run(context.Background(), s, filter(selector.Filter{States: []string{"idle"}}))
	if r, ok := out.Result.(Result); out.Status != "success" || !ok || !r.Stopped || *r.PaneID != "w1:p1" {
		t.Fatalf("%+v", out)
	}
}

// A matched pane that now hosts another agent's terminal is not stopped: the
// filter never matched that agent.
func TestStopFilterMatchReplacedIsNotStopped(t *testing.T) {
	a, b := agentIn("w1:p1", "a", "idle"), agentIn("w1:p2", "b", "idle")
	s := fake(t, listCall(withTerminal(a, "term_a"), withTerminal(b, "term_x")),
		getCall("w1:p1", a), getCall("w1:p2", b), closeCall("w1:p2", nil))
	out := Run(context.Background(), s, filter(selector.Filter{States: []string{"idle"}}))
	if got, want := rows(t, out, "fan-out"), "w1:p1=failed/w1:p1 w1:p2=stopped/w1:p2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if row := out.Result.(FanOut).Targets[0]; out.Status != "partial" || row.Error.Code != "agent_identity_stale" || len(out.Effects) != 1 {
		t.Fatalf("%+v %+v", out, row.Error)
	}
}

// Registered matches are labelled by record id, followed by it, and each
// record is ended.
func TestStopFilterRegisteredMatches(t *testing.T) {
	a, b := agentIn("w1:p1", "a", "idle"), agentIn("w1:p2", "b", "idle")
	da, db := withTerminal(a, "term_a"), withTerminal(b, "term_b")
	s := fake(t, listCall(da, db),
		call{Method: "agent.get", Params: map[string]any{"target": "w1:p1"}, Result: herdr.AgentResult{Type: "agent_info", Agent: da}}, closeCall("w1:p1", nil),
		call{Method: "agent.get", Params: map[string]any{"target": "w1:p2"}, Result: herdr.AgentResult{Type: "agent_info", Agent: db}}, closeCall("w1:p2", nil))
	s.Cwd = identitytest.Repository(t)
	ra, rb := identitytest.Register(t, s.Cwd, da), identitytest.Register(t, s.Cwd, db)
	out := Run(context.Background(), s, filter(selector.Filter{Registered: true}))
	if got, want := rows(t, out, "fan-out"), ra.ID+"=stopped/w1:p1 "+rb.ID+"=stopped/w1:p2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if out.Status != "success" || !ended(t, s.Cwd, ra.ID) || !ended(t, s.Cwd, rb.ID) {
		t.Fatalf("%+v", out)
	}
}

// A filtered dry run plans from the listing without further lookups.
func TestStopFilterDryRunUsesListing(t *testing.T) {
	a, b := agentIn("w1:p1", "a", "idle"), agentIn("w1:p2", "b", "unknown")
	s, r := record(fake(t, listCall(withTerminal(a, "term_a"), withTerminal(b, "term_b"))))
	o := filter(selector.Filter{Harnesses: []string{"claude"}})
	o.DryRun = true
	out := Run(context.Background(), s, o)
	readOnly(t, r)
	if got, want := rows(t, out, "dry-run"), "w1:p1=stop/w1:p1 w1:p2=refuse/w1:p2"; got != want || out.Status != "success" {
		t.Fatalf("got %q (%+v), want %q", got, out, want)
	}
}

func TestStopFanOutRendering(t *testing.T) {
	agent := func(pane, name, status string) *libagent.AgentRow {
		r := libagent.NewAgentRow(agentIn(pane, name, status))
		return &r
	}
	refused := &libagent.Failure{Code: "invalid_input", Message: "agent b is working; pass --force to stop it anyway", Phase: "guard"}
	missing := &libagent.Failure{Code: "agent_not_found", Message: "agent_not_found: no c", Phase: "agent.get"}
	for _, tc := range []struct {
		name string
		out  libagent.Outcome
		want string
	}{
		{"fan-out", libagent.Outcome{Status: "partial", Result: FanOut{Mode: "fan-out", Targets: []Row{
			{Target: "a", Outcome: "stopped", Agent: agent("w1:p1", "a", "idle")},
			{Target: "b", Outcome: "refused", Agent: agent("w1:p2", "b", "working"), Error: refused},
			{Target: "c", Outcome: "failed", Error: missing},
		}}, Error: &libagent.Failure{Code: "operation_failed", Message: "2 of 3 targets not stopped cleanly: b (invalid_input), c (agent_not_found)", Phase: "agent.stop"}},
			`partial: 2 of 3 targets not stopped cleanly: b (invalid_input), c (agent_not_found) (agent.stop)
Stopped 1 of 3 agents.
  stopped  a (w1:p1)
  refused  b (w1:p2): agent b is working; pass --force to stop it anyway
  failed   c: agent_not_found: no c
`},
		{"dry-run", libagent.Outcome{Status: "success", Result: FanOut{Mode: "dry-run", Targets: []Row{
			{Target: "a", Outcome: "stop", Agent: agent("w1:p1", "a", "idle")},
			{Target: "b", Outcome: "refuse", Agent: agent("w1:p2", "b", "working"), Error: refused},
		}}},
			`Dry run: would stop 1 of 2 agents.
  stop    a (w1:p1, idle)
  refuse  b (w1:p2, working): agent b is working; pass --force to stop it anyway
`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := tc.out.Write(&b, false, Render); err != nil {
				t.Fatal(err)
			}
			if b.String() != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", b.String(), tc.want)
			}
			b.Reset()
			if err := tc.out.Write(&b, true, Render); err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Result struct {
					Mode    string           `json:"mode"`
					Targets []map[string]any `json:"targets"`
				} `json:"result"`
			}
			if err := json.Unmarshal(b.Bytes(), &decoded); err != nil || decoded.Result.Mode != tc.name {
				t.Fatalf("%v %s", err, b.String())
			}
			for _, key := range []string{"target", "outcome", "agent", "error"} {
				if _, ok := decoded.Result.Targets[1][key]; !ok || len(decoded.Result.Targets[1]) != 4 {
					t.Fatalf("row keys %v lack %s", decoded.Result.Targets[1], key)
				}
			}
			herdrscript.CheckOutputFailures(t, Render, tc.out)
		})
	}
}

// A registered match is followed by its record, so a terminal that moved to
// another pane after the listing is still stopped where it now runs.
func TestStopFilterRegisteredMatchFollowsMovedTerminal(t *testing.T) {
	a := withTerminal(agentIn("w1:p1", "a", "idle"), "term_a")
	moved := withTerminal(agentIn("w1:p5", "a", "idle"), "term_a")
	other := withTerminal(agentIn("w1:p1", "z", "idle"), "term_z")
	s := fake(t, listCall(a),
		call{Method: "agent.get", Params: map[string]any{"target": "w1:p1"}, Result: herdr.AgentResult{Type: "agent_info", Agent: other}},
		listCall(other, moved), closeCall("w1:p5", nil))
	s.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, s.Cwd, a)
	out := Run(context.Background(), s, filter(selector.Filter{Registered: true}))
	if r, ok := out.Result.(Result); out.Status != "success" || !ok || !r.Stopped || *r.PaneID != "w1:p5" || !ended(t, s.Cwd, rec.ID) {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}
