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
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
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

// byMethod answers each Herdr method the same way however often it is called.
type byMethod map[string]call

func (b byMethod) Call(_ context.Context, method string, _, result any) error {
	c, ok := b[method]
	if !ok {
		return &herdr.Error{Code: "unexpected", Message: method}
	}
	if c.Err != nil {
		return c.Err
	}
	data, _ := json.Marshal(c.Result)
	return json.Unmarshal(data, result)
}

// A dry run by record id neither ends a record whose terminal is gone nor
// moves one whose terminal now runs in another pane.
func TestStopDryRunByIDLeavesRecordUnchanged(t *testing.T) {
	recorded := withTerminal(agentIn("w1:p3", "worker", "idle"), "term_old")
	other := withTerminal(agentIn("w1:p3", "other", "idle"), "term_new")
	moved := withTerminal(agentIn("w1:p5", "worker", "idle"), "term_old")
	for _, tc := range []struct {
		name string
		api  byMethod
		want string
	}{
		{"gone", byMethod{"agent.get": {Err: &herdr.Error{Code: "agent_not_found", Message: "gone"}}, "agent.list": listCall([]herdr.AgentDetails{}...), "pane.list": {Result: map[string]any{"type": "pane_list", "panes": []any{}}}}, "error/-"},
		{"moved", byMethod{"agent.get": {Result: herdr.AgentResult{Type: "agent_info", Agent: other}}, "agent.list": listCall(other, moved)}, "stop/w1:p5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := libagent.Client{API: tc.api, CallerPane: "old:p1", Cwd: identitytest.Repository(t)}
			rec := identitytest.Register(t, s.Cwd, recorded)
			o := Options{Selection: selector.Selection{IDs: []string{rec.ID}}, DryRun: true}
			if got, want := rows(t, Run(context.Background(), s, o), "dry-run"), rec.ID+"="+tc.want; got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
			store, err := identity.Existing(context.Background(), s.Cwd)
			if err != nil {
				t.Fatal(err)
			}
			var after identity.Record
			if err := store.Get(identity.Kind, rec.ID, &after); err != nil {
				t.Fatal(err)
			}
			if after.EndedAt != nil || after.Pane != rec.Pane {
				t.Fatalf("dry run changed the record: ended %v, pane %s (was %s)", after.EndedAt, after.Pane, rec.Pane)
			}
		})
	}
}

// A dry run by record id fails with the code a real stop would: an unknown
// id is agent_record_not_found, an ended record agent_identity_stale.
func TestStopDryRunByIDReportsRealStopCodes(t *testing.T) {
	recorded := withTerminal(agentIn("w1:p3", "worker", "idle"), "term_old")
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) (cwd, id string)
		code  string
	}{
		{"no store", func(t *testing.T) (string, string) { return identitytest.Repository(t), "deadbeef" }, "agent_record_not_found"},
		{"unknown id", func(t *testing.T) (string, string) {
			cwd := identitytest.Repository(t)
			identitytest.Register(t, cwd, recorded)
			return cwd, "deadbeef"
		}, "agent_record_not_found"},
		{"ended", func(t *testing.T) (string, string) {
			cwd := identitytest.Repository(t)
			rec := identitytest.Register(t, cwd, recorded)
			store, err := identity.Existing(context.Background(), cwd)
			if err != nil || identity.End(store, rec.ID) != nil {
				t.Fatal(err)
			}
			return cwd, rec.ID
		}, "agent_identity_stale"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cwd, id := tc.setup(t)
			s := libagent.Client{API: byMethod{"agent.list": listCall([]herdr.AgentDetails{}...)}, CallerPane: "old:p1", Cwd: cwd}
			out := Run(context.Background(), s, Options{Selection: selector.Selection{IDs: []string{id}}, DryRun: true})
			if got := rows(t, out, "dry-run"); got != id+"=error/-" {
				t.Fatalf("got %q", got)
			}
			if e := out.Result.(FanOut).Targets[0].Error; e.Code != tc.code || e.Phase != "identity" {
				t.Fatalf("got %+v, want code %s at phase identity", e, tc.code)
			}
		})
	}
}

// closes serves agent.get from agents by target and notes each closed pane.
type closes struct {
	agents map[string]herdr.Pane
	panes  []string
}

func (c *closes) Call(_ context.Context, method string, params, result any) error {
	p := params.(map[string]any)
	var r any = herdrscript.OK()
	if method == "agent.get" {
		r = herdrscript.Info(c.agents[p["target"].(string)])
	} else {
		c.panes = append(c.panes, p["pane_id"].(string))
	}
	data, _ := json.Marshal(r)
	return json.Unmarshal(data, result)
}

// The caller's own pane, however it is named, is stopped after every other
// target: closing it ends this process. Rows keep target order.
func TestStopCallerTargetIsStoppedLast(t *testing.T) {
	me, a := agentIn("old:p1", "me", "idle"), agentIn("w1:p1", "a", "idle")
	for _, tc := range []struct {
		name string
		o    Options
		rows string
	}{
		{"by name", names("me", "a"), "me=stopped/old:p1 a=stopped/w1:p1"},
		{"by pane", Options{Selection: selector.Selection{Panes: []string{"old:p1", "w1:p1"}}}, "old:p1=stopped/old:p1 w1:p1=stopped/w1:p1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &closes{agents: map[string]herdr.Pane{"me": me, "a": a, "old:p1": me, "w1:p1": a}}
			out := Run(context.Background(), libagent.Client{API: api, CallerPane: "old:p1", Cwd: t.TempDir()}, tc.o)
			if got := strings.Join(api.panes, " "); got != "w1:p1 old:p1" {
				t.Fatalf("closed %q, want the caller's pane last", got)
			}
			if got := rows(t, out, "fan-out"); got != tc.rows || out.Status != "success" {
				t.Fatalf("got %q (%+v), want %q", got, out, tc.rows)
			}
		})
	}
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
