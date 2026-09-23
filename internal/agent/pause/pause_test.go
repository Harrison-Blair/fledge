package pause

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

type call = herdrscript.Call

func fake(t *testing.T, calls ...call) *pauser {
	t.Helper()
	return &pauser{Client: herdrscript.Client(t, calls...), Now: func() time.Time { return time.Unix(0, 0) }}
}

func TestPauseMappings(t *testing.T) {
	for _, h := range libagent.Harnesses() {
		t.Run(h, func(t *testing.T) {
			keys := []string{"esc"}
			switch h {
			case "amp", "copilot", "opencode", "kilo":
				keys = []string{"esc", "esc"}
			case "droid", "grok", "hermes", "mastracode", "qodercli":
				keys = []string{"ctrl+c"}
			}
			p := herdrscript.LiveAgent("working")
			p.Agent = &h
			s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: herdrscript.Info(p)}, call{Method: "agent.send_keys", Params: map[string]any{"target": p.PaneID, "keys": keys}, Result: herdrscript.OK()})
			out := s.run(context.Background(), Options{Target: identity.Target{Name: "worker"}, Timeout: 10 * time.Second, NoWait: true})
			r := out.Result.(Result)
			if out.Status != "success" || !r.Submitted || r.Settled || len(out.Effects) != 1 || out.Effects[0] != (libagent.Effect{Action: "submitted", Kind: "interrupt", ID: p.PaneID}) {
				t.Fatalf("%+v %+v", out, r)
			}
		})
	}
}
func TestPauseValidation(t *testing.T) {
	for _, o := range []Options{{Timeout: time.Second}, {Target: identity.Target{Name: "w", Pane: "p"}, Timeout: time.Second}, {Target: identity.Target{Name: "w"}}, {Target: identity.Target{Name: "w"}, Timeout: -time.Second}, {Target: identity.Target{Name: " "}, Timeout: time.Second}, {Target: identity.Target{Pane: "\t"}, Timeout: time.Second}} {
		out := fake(t).run(context.Background(), o)
		if out.ExitCode() != 2 || out.Error.Phase != "validation" {
			t.Fatalf("%+v", out)
		}
	}
}
func TestPauseGuards(t *testing.T) {
	for _, status := range []string{"idle", "done", "blocked", "unknown", "working"} {
		for _, harness := range []string{"claude", "", "future"} {
			t.Run(status+"/"+harness, func(t *testing.T) {
				p := herdrscript.LiveAgent(status)
				p.Agent = libagent.Pointer(harness)
				a := herdrscript.Info(p)
				pending := true
				if status == "working" {
					a.Agent.LaunchPending = &pending
				}
				out := fake(t, call{Method: "agent.get", Result: a}).run(context.Background(), Options{Target: identity.Target{Pane: p.PaneID}, Timeout: time.Second})
				success := (status == "idle" || status == "done") && harness == "claude"
				if (out.Status == "success") != success {
					t.Fatalf("%+v", out)
				}
				r := out.Result.(Result)
				if r.Submitted || r.Settled != success || len(out.Effects) != 0 {
					t.Fatalf("%+v", out)
				}
			})
		}
	}
}
func TestPauseWait(t *testing.T) {
	for _, status := range []string{"idle", "done"} {
		t.Run(status, func(t *testing.T) {
			p := herdrscript.LiveAgent("working")
			now := time.Unix(0, 0)
			s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": p.PaneID}, Result: herdrscript.Info(p), Before: func() { now = now.Add(time.Second) }}, call{Method: "agent.send_keys", Result: herdrscript.OK(), Before: func() { now = now.Add(2 * time.Second) }}, call{Method: "agent.wait", Params: map[string]any{"target": p.PaneID, "timeout_ms": 17000}, Result: herdrscript.Waited(p, status)})
			s.Now = func() time.Time { return now }
			out := s.run(context.Background(), Options{Target: identity.Target{Pane: p.PaneID}, Timeout: 20 * time.Second})
			r := out.Result.(Result)
			if out.Status != "success" || !r.Submitted || !r.Settled || *r.AgentStatus != status {
				t.Fatalf("%+v %+v", out, r)
			}
		})
	}
}
func TestPauseSendFailures(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		result       any
		err          error
	}{
		{"lost", "unknown", nil, &herdr.Error{Code: "transport_error", Message: "lost", Uncertain: true}},
		{"malformed", "unknown", map[string]any{"type": "wrong"}, nil},
		{"missing", "unknown", map[string]any{}, nil},
		{"rejected", "rejected", nil, &herdr.Error{Code: "agent_not_ready", Message: "not ready"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("working"))}, call{Method: "agent.send_keys", Result: tc.result, Err: tc.err})
			out := s.run(context.Background(), Options{Target: identity.Target{Name: "worker"}, Timeout: time.Second})
			if out.Status != tc.status || out.Error.Phase != "agent.send_keys" || out.Result.(Result).Submitted || len(out.Effects) != 0 {
				t.Fatalf("%+v", out)
			}
		})
	}
}
func TestPauseWaitFailures(t *testing.T) {
	for _, name := range []string{"timeout", "disappeared", "lost", "malformed", "terminal", "pane", "blocked", "working", "unknown", "pending"} {
		t.Run(name, func(t *testing.T) {
			p := herdrscript.LiveAgent("working")
			w := herdrscript.Waited(p, "idle")
			var err error
			switch name {
			case "timeout":
				err = &herdr.Error{Code: "timeout", Message: "timed out"}
			case "disappeared":
				err = &herdr.Error{Code: "agent_not_found", Message: "gone"}
			case "lost":
				err = &herdr.Error{Code: "transport_error", Message: "lost", Uncertain: true}
			case "malformed":
				w.Type = "wrong"
			case "terminal":
				w.Agent.TerminalID = "replacement"
			case "pane":
				w.Agent.PaneID = "w1:p99"
			case "blocked", "working", "unknown":
				w.Agent.AgentStatus = name
			case "pending":
				b := true
				w.Agent.LaunchPending = &b
			}
			s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(p)}, call{Method: "agent.send_keys", Result: herdrscript.OK()}, call{Method: "agent.wait", Result: w, Err: err})
			out := s.run(context.Background(), Options{Target: identity.Target{Name: "worker"}, Timeout: time.Second})
			r := out.Result.(Result)
			if out.Status != "partial" || out.Error.Phase != "agent.wait" || !r.Submitted || r.Settled || len(out.Effects) != 1 {
				t.Fatalf("%+v %+v", out, r)
			}
			if name == "blocked" && out.Error.Code != "agent_blocked" {
				t.Fatalf("%+v", out)
			}
		})
	}
}
func TestPauseBudgetExhausted(t *testing.T) {
	for _, afterSend := range []bool{false, true} {
		t.Run(map[bool]string{false: "lookup", true: "send"}[afterSend], func(t *testing.T) {
			now := time.Unix(0, 0)
			advance := func() { now = now.Add(2 * time.Second) }
			c := call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("working"))}
			if !afterSend {
				c.Before = advance
			}
			calls := []call{c}
			if afterSend {
				calls = append(calls, call{Method: "agent.send_keys", Result: herdrscript.OK(), Before: advance})
			}
			s := fake(t, calls...)
			s.Now = func() time.Time { return now }
			out := s.run(context.Background(), Options{Target: identity.Target{Name: "worker"}, Timeout: time.Second})
			if out.Error == nil || out.Error.Code != "timeout" || out.Result.(Result).Submitted != afterSend {
				t.Fatalf("%+v", out)
			}
		})
	}
}
func TestPauseLookupFailures(t *testing.T) {
	for _, c := range []call{{Method: "agent.get", Result: map[string]any{}}, {Method: "agent.get", Err: &herdr.Error{Code: "agent_not_found", Message: "gone"}}} {
		out := fake(t, c).run(context.Background(), Options{Target: identity.Target{Name: "worker"}, Timeout: time.Second})
		if out.Status != "rejected" || out.Error.Phase != "agent.get" {
			t.Fatalf("%+v", out)
		}
	}
}
func TestRender(t *testing.T) {
	row := herdrscript.Row()
	for _, tc := range []struct {
		name string
		out  libagent.Outcome
		want string
	}{
		{"requested", libagent.Outcome{Status: "success", Result: Result{AgentRow: row, Submitted: true}}, "Pause requested: worker (claude) in w1:p1.\n"},
		{"paused", libagent.Outcome{Status: "success", Result: Result{AgentRow: row, Submitted: true, Settled: true}}, "Paused: worker (claude) in w1:p1.\n"},
		{"already settled", libagent.Outcome{Status: "success", Result: Result{AgentRow: row, Settled: true}}, "Already idle or done: worker (claude) in w1:p1.\n"},
		{"failure", libagent.Outcome{Status: "partial", Result: Result{AgentRow: row, Submitted: true}, Error: &libagent.Failure{Message: "timed out", Phase: "agent.wait"}}, "partial: timed out (agent.wait)\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := tc.out.Write(&b, false, Render); err != nil {
				t.Fatal(err)
			}
			if b.String() != tc.want {
				t.Fatalf("got %q, want %q", b.String(), tc.want)
			}
		})
	}
}

func TestPauseByIDFailsClosedOnStaleTerminal(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("working"))
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live}, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []any{}}}, call{Method: "pane.list", Result: map[string]any{"type": "pane_list", "panes": []any{}}})
	s.Cwd = identitytest.Repository(t)
	recorded := live.Agent
	recorded.TerminalID = "term_old"
	rec := identitytest.Register(t, s.Cwd, recorded)
	out := s.run(context.Background(), Options{Target: identity.Target{ID: rec.ID}, Timeout: time.Second})
	if out.Error == nil || out.Error.Code != "agent_identity_stale" || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
}

func TestPauseByIDInterruptsVerifiedPane(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("working"))
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live},
		call{Method: "agent.send_keys", Params: map[string]any{"target": "w1:p3", "keys": []string{"esc"}}, Result: herdrscript.OK()})
	s.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, s.Cwd, live.Agent)
	if out := s.run(context.Background(), Options{Target: identity.Target{ID: rec.ID}, Timeout: time.Second, NoWait: true}); out.Error != nil {
		t.Fatalf("%+v", out)
	}
}

// serve answers each socket request with reply(method, params), which
// returns a result or a Herdr error object.
func serve(t *testing.T, reply func(method string, params map[string]any) (result, err any)) {
	t.Helper()
	dir, err := os.MkdirTemp("", "fp-")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "s")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close(); os.RemoveAll(dir) })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				var req struct {
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				line, _ := bufio.NewReader(c).ReadBytes('\n')
				if json.Unmarshal(line, &req) != nil {
					return
				}
				result, failure := reply(req.Method, req.Params)
				json.NewEncoder(c).Encode(map[string]any{"id": "fledge", "result": result, "error": failure})
			}()
		}
	}()
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_SOCKET_PATH", path)
}

// TestPauseReportsHerdrSettleTimeout answers agent.wait with Herdr's timeout
// just after timeout_ms, so a local deadline equal to timeout_ms would surface
// a transport error instead.
func TestPauseReportsHerdrSettleTimeout(t *testing.T) {
	serve(t, func(method string, params map[string]any) (any, any) {
		switch method {
		case "agent.get":
			return herdrscript.Info(herdrscript.LiveAgent("working")), nil
		case "agent.send_keys":
			return herdrscript.OK(), nil
		}
		ms, _ := params["timeout_ms"].(float64)
		time.Sleep(time.Duration(ms)*time.Millisecond + 2*time.Millisecond)
		return nil, map[string]any{"code": "timeout", "message": "wait timed out"}
	})
	for range 5 {
		out := Run(context.Background(), libagent.FromEnvironment(200*time.Millisecond), Options{Target: identity.Target{Name: "worker"}, Timeout: 200 * time.Millisecond})
		if out.Error == nil || out.Error.Code != "timeout" || out.Error.Phase != "agent.wait" || !out.Result.(Result).Submitted {
			t.Fatalf("%+v %+v", out, out.Error)
		}
	}
}

// TestPauseTimeoutBoundsStalledRequests keeps --timeout as the wall-clock
// bound before agent.wait, although the transport limit is longer.
func TestPauseTimeoutBoundsStalledRequests(t *testing.T) {
	for _, stall := range []string{"agent.get", "agent.send_keys"} {
		t.Run(stall, func(t *testing.T) {
			stalled := make(chan struct{})
			t.Cleanup(func() { close(stalled) })
			serve(t, func(method string, _ map[string]any) (any, any) {
				if method == stall {
					<-stalled
				}
				return herdrscript.Info(herdrscript.LiveAgent("working")), nil
			})
			start := time.Now()
			out := Run(context.Background(), libagent.FromEnvironment(200*time.Millisecond), Options{Target: identity.Target{Name: "worker"}, Timeout: 200 * time.Millisecond})
			if elapsed := time.Since(start); elapsed > 5*time.Second || out.Error == nil || out.Error.Phase != stall {
				t.Fatalf("%s %+v %+v", elapsed, out, out.Error)
			}
		})
	}
}
