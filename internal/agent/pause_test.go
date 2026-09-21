package agent

import (
	"context"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"testing"
	"time"
)

func TestPauseMappings(t *testing.T) {
	for _, h := range kinds {
		t.Run(h, func(t *testing.T) {
			keys := []string{"esc"}
			switch h {
			case "amp", "copilot", "opencode", "kilo":
				keys = []string{"esc", "esc"}
			case "droid", "grok", "hermes", "mastracode", "qodercli":
				keys = []string{"ctrl+c"}
			}
			p := liveAgent("working")
			p.Agent = &h
			s := fake(t, call{method: "agent.get", params: map[string]any{"target": "worker"}, result: info(p)}, call{method: "agent.send_keys", params: map[string]any{"target": p.PaneID, "keys": keys}, result: closed()})
			out := s.Pause(context.Background(), PauseOptions{Name: "worker", Timeout: 10 * time.Second, NoWait: true})
			r := out.Result.(PauseResult)
			if out.Status != "success" || !r.Submitted || r.Settled || len(out.Effects) != 1 || out.Effects[0] != (Effect{Action: "submitted", Kind: "interrupt", ID: p.PaneID}) {
				t.Fatalf("%+v %+v", out, r)
			}
		})
	}
}
func TestPauseValidation(t *testing.T) {
	for _, o := range []PauseOptions{{Timeout: time.Second}, {Name: "w", Pane: "p", Timeout: time.Second}, {Name: "w"}, {Name: "w", Timeout: -time.Second}, {Name: " ", Timeout: time.Second}, {Pane: "\t", Timeout: time.Second}} {
		out := fake(t).Pause(context.Background(), o)
		if out.ExitCode() != 2 || out.Error.Phase != "validation" {
			t.Fatalf("%+v", out)
		}
	}
}
func TestPauseGuards(t *testing.T) {
	for _, status := range []string{"idle", "done", "blocked", "unknown", "working"} {
		for _, harness := range []string{"claude", "", "future"} {
			t.Run(status+"/"+harness, func(t *testing.T) {
				p := liveAgent(status)
				p.Agent = pointer(harness)
				a := info(p)
				pending := true
				if status == "working" {
					a.Agent.LaunchPending = &pending
				}
				out := fake(t, call{method: "agent.get", result: a}).Pause(context.Background(), PauseOptions{Pane: p.PaneID, Timeout: time.Second})
				success := (status == "idle" || status == "done") && harness == "claude"
				if (out.Status == "success") != success {
					t.Fatalf("%+v", out)
				}
				r := out.Result.(PauseResult)
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
			p := liveAgent("working")
			now := time.Unix(0, 0)
			s := fake(t, call{method: "agent.get", params: map[string]any{"target": p.PaneID}, result: info(p), before: func() { now = now.Add(time.Second) }}, call{method: "agent.send_keys", result: closed(), before: func() { now = now.Add(2 * time.Second) }}, call{method: "agent.wait", params: map[string]any{"target": p.PaneID, "timeout_ms": 17000}, result: waited(p, status)})
			s.Now = func() time.Time { return now }
			out := s.Pause(context.Background(), PauseOptions{Pane: p.PaneID, Timeout: 20 * time.Second})
			r := out.Result.(PauseResult)
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
			s := fake(t, call{method: "agent.get", result: info(liveAgent("working"))}, call{method: "agent.send_keys", result: tc.result, err: tc.err})
			out := s.Pause(context.Background(), PauseOptions{Name: "worker", Timeout: time.Second})
			if out.Status != tc.status || out.Error.Phase != "agent.send_keys" || out.Result.(PauseResult).Submitted || len(out.Effects) != 0 {
				t.Fatalf("%+v", out)
			}
		})
	}
}
func TestPauseWaitFailures(t *testing.T) {
	for _, name := range []string{"timeout", "disappeared", "lost", "malformed", "terminal", "pane", "blocked", "working", "unknown", "pending"} {
		t.Run(name, func(t *testing.T) {
			p := liveAgent("working")
			w := waited(p, "idle")
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
			s := fake(t, call{method: "agent.get", result: info(p)}, call{method: "agent.send_keys", result: closed()}, call{method: "agent.wait", result: w, err: err})
			out := s.Pause(context.Background(), PauseOptions{Name: "worker", Timeout: time.Second})
			r := out.Result.(PauseResult)
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
			c := call{method: "agent.get", result: info(liveAgent("working"))}
			if !afterSend {
				c.before = advance
			}
			calls := []call{c}
			if afterSend {
				calls = append(calls, call{method: "agent.send_keys", result: closed(), before: advance})
			}
			s := fake(t, calls...)
			s.Now = func() time.Time { return now }
			out := s.Pause(context.Background(), PauseOptions{Name: "worker", Timeout: time.Second})
			if out.Error == nil || out.Error.Code != "timeout" || out.Result.(PauseResult).Submitted != afterSend {
				t.Fatalf("%+v", out)
			}
		})
	}
}
func TestPauseLookupFailures(t *testing.T) {
	for _, c := range []call{{method: "agent.get", result: map[string]any{}}, {method: "agent.get", err: &herdr.Error{Code: "agent_not_found", Message: "gone"}}} {
		out := fake(t, c).Pause(context.Background(), PauseOptions{Name: "worker", Timeout: time.Second})
		if out.Status != "rejected" || out.Error.Phase != "agent.get" {
			t.Fatalf("%+v", out)
		}
	}
}
