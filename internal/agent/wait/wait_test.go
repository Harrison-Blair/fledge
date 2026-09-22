package wait

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

// reply scripts one target's agent.wait: after delay it returns err or a
// matched agent in status. block holds the call until its context ends.
type reply struct {
	delay  time.Duration
	status string
	err    error
	block  bool
}

// fanFake serves concurrent agent.wait calls by target and, like the socket
// client, fails a call whose context ends with an uncertain transport error.
type fanFake struct {
	t       *testing.T
	replies map[string]reply
	mu      sync.Mutex
	params  map[string]map[string]any
}

func newFake(t *testing.T, replies map[string]reply) (*fanFake, libagent.Client) {
	f := &fanFake{t: t, replies: replies, params: map[string]map[string]any{}}
	t.Cleanup(func() {
		if len(f.params) != len(replies) {
			t.Errorf("called %d of %d targets", len(f.params), len(replies))
		}
	})
	return f, libagent.Client{API: f}
}

func (f *fanFake) Call(ctx context.Context, method string, params any, result any) error {
	data, _ := json.Marshal(params)
	var p map[string]any
	json.Unmarshal(data, &p)
	target, _ := p["target"].(string)
	f.mu.Lock()
	if _, seen := f.params[target]; seen || method != "agent.wait" {
		f.mu.Unlock()
		f.t.Errorf("unexpected %s %v", method, p)
		return &herdr.Error{Code: "test", Message: "unexpected"}
	}
	f.params[target] = p
	f.mu.Unlock()
	r, ok := f.replies[target]
	if !ok {
		f.t.Errorf("unscripted target %q", target)
		return &herdr.Error{Code: "test", Message: "unscripted"}
	}
	var ready <-chan time.Time
	if !r.block {
		ready = time.After(r.delay)
	}
	select {
	case <-ctx.Done():
		return &herdr.Error{Code: "transport_error", Message: "use of closed network connection", Uncertain: true}
	case <-ready:
	}
	if r.err != nil {
		return r.err
	}
	a := herdrscript.LiveAgent(r.status)
	name, pane := target, "w1:p-"+target
	a.Name, a.PaneID = &name, pane
	b, _ := json.Marshal(herdrscript.Waited(a, r.status))
	return json.Unmarshal(b, result)
}

func herr(code string) error { return &herdr.Error{Code: code, Message: code + " message"} }

func TestWaitSingleTarget(t *testing.T) {
	for _, tc := range []struct {
		o    Options
		want map[string]any
	}{
		{Options{Names: []string{"worker"}}, map[string]any{"target": "worker"}},
		{Options{Panes: []string{"w1:p3"}, Until: []string{"working", "unknown"}, Timeout: 2 * time.Second, Any: true}, map[string]any{"target": "w1:p3", "until": []any{"working", "unknown"}, "timeout_ms": float64(2000)}},
	} {
		status := "idle"
		if len(tc.o.Until) > 0 {
			status = "working"
		}
		f, c := newFake(t, map[string]reply{tc.want["target"].(string): {status: status}})
		out := Run(context.Background(), c, tc.o)
		if out.Operation != "agent.wait" || out.Status != "success" || out.ExitCode() != 0 || len(out.Effects) != 0 {
			t.Fatalf("%+v", out)
		}
		row := out.Result.(libagent.AgentRow)
		if *row.AgentStatus != status || !reflect.DeepEqual(f.params[tc.want["target"].(string)], tc.want) {
			t.Fatalf("%+v %v", row, f.params)
		}
	}
}

func TestWaitSingleTargetFailures(t *testing.T) {
	for _, code := range []string{"timeout", "agent_not_found", "agent_not_running"} {
		_, c := newFake(t, map[string]reply{"worker": {err: herr(code)}})
		out := Run(context.Background(), c, Options{Names: []string{"worker"}, Timeout: time.Second})
		if out.ExitCode() != 1 || out.Status != "rejected" || out.Error.Code != code || out.Error.Phase != "agent.wait" || out.Result != nil {
			t.Fatalf("%s: %+v", code, out)
		}
	}
}

func TestWaitCancellation(t *testing.T) {
	for _, o := range []Options{{Names: []string{"a"}}, {Names: []string{"a", "b"}, All: true}, {Names: []string{"a", "b"}, Any: true}} {
		replies := map[string]reply{}
		for _, n := range o.Names {
			replies[n] = reply{block: true}
		}
		_, c := newFake(t, replies)
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(20*time.Millisecond, cancel)
		out := Run(ctx, c, o)
		if out.ExitCode() != 1 || out.Error.Code != "cancelled" || out.Error.Phase != "agent.wait" {
			t.Fatalf("%+v: %+v", o, out)
		}
		if f, ok := out.Result.(FanOut); ok {
			for _, row := range f.Targets {
				if row.Outcome != "cancelled" {
					t.Fatalf("%+v", f)
				}
			}
		}
	}
}

func TestWaitValidation(t *testing.T) {
	for _, o := range []Options{
		{},
		{Names: []string{""}},
		{Panes: []string{" "}},
		{Names: []string{"a", "b"}},
		{Names: []string{"a"}, Panes: []string{"w1:p1"}},
		{Names: []string{"a", "b"}, All: true, Any: true},
		{Names: []string{"a"}, All: true, Any: true},
		{Names: []string{"a", "a"}, All: true},
		{Names: []string{"a"}, Panes: []string{"a"}, Any: true},
		{Names: []string{"a"}, Until: []string{"settled"}},
		{Names: []string{"a"}, Timeout: -time.Second},
		{Names: []string{"a"}, Timeout: time.Microsecond},
		{Names: []string{"a"}, Timeout: maxTimeout + 1},
		{Names: []string{"a"}, Timeout: math.MaxInt64},
	} {
		_, c := newFake(t, nil)
		out := Run(context.Background(), c, o)
		if out.ExitCode() != 2 || out.Status != "rejected" || out.Error.Phase != "validation" || out.Error.Code != "invalid_input" {
			t.Fatalf("%+v: %+v", o, out)
		}
	}
}

// TestWaitLargestTimeout accepts the largest timeout whose transport margin
// cannot overflow; the next value is rejected in TestWaitValidation.
func TestWaitLargestTimeout(t *testing.T) {
	if maxTimeout != time.Duration(math.MaxInt64)-15*time.Second {
		t.Fatalf("bound %d", maxTimeout)
	}
	f, c := newFake(t, map[string]reply{"a": {status: "idle"}})
	out := Run(context.Background(), c, Options{Names: []string{"a"}, Timeout: maxTimeout})
	if out.Error != nil || f.params["a"]["timeout_ms"] != float64(maxTimeout.Milliseconds()) {
		t.Fatalf("%+v %v", out, f.params)
	}
}

func rowsByTarget(t *testing.T, out libagent.Outcome) map[string]Row {
	t.Helper()
	f, ok := out.Result.(FanOut)
	if !ok {
		t.Fatalf("%+v", out)
	}
	m := map[string]Row{}
	for _, r := range f.Targets {
		m[r.Target] = r
	}
	return m
}

// TestWaitFanOutErrorThenMatch covers one target exiting (agent_not_running)
// before another matches: --any records the error and succeeds on the later
// match; --all waits for both and fails.
func TestWaitFanOutErrorThenMatch(t *testing.T) {
	_, c := newFake(t, map[string]reply{"a": {err: herr("agent_not_running")}, "w1:p9": {delay: 30 * time.Millisecond, status: "done"}})
	out := Run(context.Background(), c, Options{Names: []string{"a"}, Panes: []string{"w1:p9"}, Any: true})
	rows := rowsByTarget(t, out)
	if rows["a"].Outcome != "errored" || rows["a"].Error.Code != "agent_not_running" || rows["a"].Agent != nil ||
		rows["w1:p9"].Outcome != "matched" || *rows["w1:p9"].Agent.AgentStatus != "done" || rows["w1:p9"].Error != nil {
		t.Fatalf("%+v", rows)
	}
	f := out.Result.(FanOut)
	if out.Status != "success" || out.ExitCode() != 0 || f.Mode != "any" || f.Winner == nil || *f.Winner != "w1:p9" {
		t.Fatalf("%+v", out)
	}
}

func TestWaitAllFailsFast(t *testing.T) {
	_, c := newFake(t, map[string]reply{"ghost": {err: herr("agent_not_found")}, "busy": {block: true}, "w1:p9": {status: "idle"}})
	start := time.Now()
	out := Run(context.Background(), c, Options{Names: []string{"ghost", "busy"}, Panes: []string{"w1:p9"}, All: true})
	if time.Since(start) > time.Second {
		t.Fatal("remaining waits were not cancelled")
	}
	rows := rowsByTarget(t, out)
	f := out.Result.(FanOut)
	if out.ExitCode() != 1 || out.Status != "rejected" || out.Error.Code != "agent_not_found" || out.Error.Phase != "agent.wait" || f.Mode != "all" || f.Winner != nil ||
		rows["ghost"].Outcome != "errored" || rows["ghost"].Error.Code != "agent_not_found" ||
		rows["busy"].Outcome != "cancelled" || rows["busy"].Error != nil || rows["busy"].Agent != nil {
		t.Fatalf("%+v %+v", out, rows)
	}
	if got := f.Targets; got[0].Target != "ghost" || got[1].Target != "busy" || got[2].Target != "w1:p9" {
		t.Fatalf("rows not in target order: %+v", got)
	}
}

func TestWaitFanOutTimeout(t *testing.T) {
	_, c := newFake(t, map[string]reply{"a": {status: "idle"}, "b": {delay: 10 * time.Millisecond, err: herr("timeout")}})
	out := Run(context.Background(), c, Options{Names: []string{"a", "b"}, All: true, Timeout: 10 * time.Millisecond})
	rows := rowsByTarget(t, out)
	if out.ExitCode() != 1 || out.Error.Code != "timeout" || rows["a"].Outcome != "matched" || rows["b"].Outcome != "errored" || rows["b"].Error.Code != "timeout" {
		t.Fatalf("%+v %+v", out, rows)
	}
	_, c = newFake(t, map[string]reply{"a": {delay: 30 * time.Millisecond, status: "idle"}, "b": {err: herr("timeout")}})
	out = Run(context.Background(), c, Options{Names: []string{"a", "b"}, Any: true, Timeout: time.Second})
	if rows = rowsByTarget(t, out); out.Error != nil || rows["b"].Error.Code != "timeout" || *out.Result.(FanOut).Winner != "a" {
		t.Fatalf("%+v %+v", out, rows)
	}
}

func TestWaitAnyCancelsRemaining(t *testing.T) {
	_, c := newFake(t, map[string]reply{"a": {delay: 10 * time.Millisecond, status: "idle"}, "b": {block: true}, "c": {block: true}})
	start := time.Now()
	out := Run(context.Background(), c, Options{Names: []string{"a", "b", "c"}, Any: true})
	rows := rowsByTarget(t, out)
	if out.Error != nil || rows["a"].Outcome != "matched" || rows["b"].Outcome != "cancelled" || rows["c"].Outcome != "cancelled" || rows["b"].Error != nil {
		t.Fatalf("%+v %+v", out, rows)
	}
	if time.Since(start) > time.Second {
		t.Fatal("remaining waits were not cancelled")
	}
	if got := out.Result.(FanOut).Targets; got[0].Target != "a" || got[1].Target != "b" || got[2].Target != "c" {
		t.Fatalf("rows not in target order: %+v", got)
	}
}

func TestWaitAnyFailsWhenEveryTargetErrors(t *testing.T) {
	_, c := newFake(t, map[string]reply{"a": {err: herr("agent_not_running")}, "b": {delay: 5 * time.Millisecond, err: herr("agent_not_found")}})
	out := Run(context.Background(), c, Options{Names: []string{"a", "b"}, Any: true})
	rows := rowsByTarget(t, out)
	if out.ExitCode() != 1 || out.Status != "rejected" || out.Error.Code != "operation_failed" || out.Error.Phase != "agent.wait" ||
		rows["a"].Outcome != "errored" || rows["b"].Outcome != "errored" || out.Result.(FanOut).Winner != nil {
		t.Fatalf("%+v %+v", out, rows)
	}
}

func TestRender(t *testing.T) {
	row := herdrscript.Row()
	unnamed := row
	unnamed.Name = nil
	winner := "a"
	idle, blocked := herdrscript.Row(), herdrscript.Row()
	s := "blocked"
	blocked.AgentStatus = &s
	fan := FanOut{Mode: "any", Winner: &winner, Targets: []Row{
		{Target: "a", Outcome: "matched", Agent: &idle},
		{Target: "w1:p2", Outcome: "matched", Agent: &blocked},
		{Target: "b", Outcome: "errored", Error: &libagent.Failure{Code: "agent_not_running", Message: "agent_not_running: gone", Phase: "agent.wait"}},
		{Target: "c", Outcome: "cancelled"},
	}}
	for _, tc := range []struct {
		out  libagent.Outcome
		want string
	}{
		{libagent.Outcome{Result: row}, "worker is idle.\n"},
		{libagent.Outcome{Result: unnamed}, "w1:p1 is idle.\n"},
		{libagent.Outcome{Result: fan}, "a is idle (first match).\nw1:p2 is blocked.\nb failed: agent_not_running: gone.\nc was cancelled.\n"},
		{libagent.Outcome{Result: FanOut{Mode: "all", Targets: fan.Targets[2:]}, Error: &libagent.Failure{}}, "b failed: agent_not_running: gone.\nc was cancelled.\n"},
		{libagent.Outcome{Result: row, Error: &libagent.Failure{}}, ""},
	} {
		var b bytes.Buffer
		if err := Render(&b, tc.out); err != nil || b.String() != tc.want {
			t.Fatalf("%v %q", err, b.String())
		}
	}
	herdrscript.CheckOutputFailures(t, Render,
		libagent.Outcome{Operation: "agent.wait", Status: "success", Result: row, Effects: []libagent.Effect{}},
		libagent.Outcome{Operation: "agent.wait", Status: "success", Result: fan, Effects: []libagent.Effect{}})
}

func TestWaitByIDWaitsOnVerifiedPane(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("working"))
	c := herdrscript.Client(t, herdrscript.Call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live},
		herdrscript.Call{Method: "agent.wait", Params: map[string]any{"target": "w1:p3"}, Result: herdrscript.Waited(live.Agent.Pane, "idle")})
	c.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, c.Cwd, live.Agent)
	out := Run(context.Background(), c, Options{ID: rec.ID})
	if out.Error != nil || *out.Result.(libagent.AgentRow).AgentStatus != "idle" {
		t.Fatalf("%+v", out)
	}
}

func TestWaitByIDFailsClosedWhenTerminalChanges(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("working"))
	replaced := herdrscript.Waited(live.Agent.Pane, "idle")
	replaced.Agent.TerminalID = "term_new"
	c := herdrscript.Client(t, herdrscript.Call{Method: "agent.get", Result: live}, herdrscript.Call{Method: "agent.wait", Result: replaced})
	c.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, c.Cwd, live.Agent)
	if out := Run(context.Background(), c, Options{ID: rec.ID}); out.Error == nil || out.Error.Code != "agent_identity_stale" {
		t.Fatalf("%+v", out)
	}
}

func TestWaitIDIsSingleTarget(t *testing.T) {
	for _, o := range []Options{{ID: "0000beef", Names: []string{"a"}}, {ID: "0000beef", Panes: []string{"p"}, All: true}} {
		if out := Run(context.Background(), libagent.Client{}, o); out.ExitCode() != 2 {
			t.Fatalf("%+v", out)
		}
	}
}
