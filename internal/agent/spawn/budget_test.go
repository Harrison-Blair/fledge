package spawn

import (
	"context"
	"strings"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

// budgetOptions is a pane spawn with a first prompt and a short --timeout.
func budgetOptions() Options {
	o := paneOptions()
	o.Timeout = 3001 * time.Millisecond
	o.Prompt, o.PromptSet = "hi", true
	return o
}

// lateSpawn scripts snapshot then calls, returning a clock that calls can move
// past the budget with late, modelling a reply that arrives after the deadline.
func lateSpawn(t *testing.T, calls func(late func()) []call) *spawner {
	t.Helper()
	clock := &pollClock{movableClock: movableClock{t: time.Unix(0, 0)}}
	late := func() { clock.advance(4 * time.Second) }
	s := fake(t, append([]call{{Method: "session.snapshot", Result: snapshot()}}, calls(late)...)...)
	s.Now, s.Wait = clock.now, clock.wait
	return s
}
func render(t *testing.T, out libagent.Outcome) string {
	t.Helper()
	var b strings.Builder
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// cutOff is the transport failure of a request the budget's deadline closed.
func cutOff() error {
	return &herdr.Error{Code: "transport_error", Message: "read: i/o timeout", Uncertain: true}
}

// A reply that arrives after --timeout ends the spawn with a partial timeout:
// nothing later runs, the first prompt is never submitted, and effects that
// happened (the launch, a written record) stay reported.
func TestSpawnBudgetRejectsLateReplies(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	ready := launching(p, "idle", flag(true), nil)
	for _, tc := range []struct {
		name, phase string
		repo        bool
		registered  bool
		calls       func(late func()) []call
	}{
		{"start ack", "agent.wait", false, false, func(late func()) []call {
			return []call{labeled(p), {Method: "agent.start", Result: started(p), Before: late}}
		}},
		{"lifecycle wait", "agent.wait", false, false, func(late func()) []call {
			return []call{labeled(p), {Method: "agent.start", Result: started(p)}, {Method: "agent.wait", Result: ready, Before: late}}
		}},
		{"readiness poll", "agent.wait", false, false, func(late func()) []call {
			poll := getCall(ready)
			poll.Before = late
			return []call{labeled(p), {Method: "agent.start", Result: started(p)}, {Method: "agent.wait", Result: launching(p, "idle", nil, flag(true))}, poll}
		}},
		{"lifecycle wait cut off", "agent.wait", false, false, func(late func()) []call {
			return []call{labeled(p), {Method: "agent.start", Result: started(p)}, {Method: "agent.wait", Err: cutOff(), Before: late}}
		}},
		{"readiness poll cut off", "agent.wait", false, false, func(late func()) []call {
			return []call{labeled(p), {Method: "agent.start", Result: started(p)}, {Method: "agent.wait", Result: launching(p, "idle", nil, flag(true))}, {Method: "agent.get", Err: cutOff(), Before: late}}
		}},
		{"registration", "agent.prompt", true, true, func(late func()) []call {
			lookup := callerNotAgent()
			lookup.Before = late
			return []call{labeled(p), {Method: "agent.start", Result: started(p)}, {Method: "agent.wait", Result: ready}, lookup}
		}},
		{"sender lookup", "agent.prompt", false, false, func(late func()) []call {
			sender := senderCall()
			sender.Before = late
			return []call{labeled(p), {Method: "agent.start", Result: started(p)}, {Method: "agent.wait", Result: ready}, sender}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := lateSpawn(t, tc.calls)
			if tc.repo {
				s.Cwd = identitytest.Repository(t)
			}
			out := s.run(context.Background(), budgetOptions(), nil)
			r := out.Result.(*Result)
			if out.Status != "partial" || out.Error == nil || out.Error.Code != "timeout" || out.Error.Phase != tc.phase {
				t.Fatalf("%+v %+v", out, out.Error)
			}
			if r.Prompted || !r.PromptRequested || r.MessageID != nil || r.Registered != tc.registered {
				t.Fatalf("%+v", r)
			}
			started := false
			for _, e := range out.Effects {
				started = started || e.Action == "started"
			}
			if !started {
				t.Fatalf("launch effect dropped: %+v", out.Effects)
			}
			if h := render(t, out); !strings.Contains(h, "The first prompt was not submitted") || strings.Contains(h, "may have been submitted") {
				t.Fatalf("%q", h)
			}
		})
	}
}

// A spawn without a first prompt that became ready in time succeeds even when
// registration finishes late; the record it wrote is kept.
func TestSpawnBudgetKeepsLateRegistrationWithoutPrompt(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := lateSpawn(t, func(late func()) []call {
		lookup := callerNotAgent()
		lookup.Before = late
		return []call{labeled(p), {Method: "agent.start", Result: started(p)}, {Method: "agent.wait", Result: launching(p, "idle", flag(true), nil)}, lookup}
	})
	s.Cwd = identitytest.Repository(t)
	o := budgetOptions()
	o.Prompt, o.PromptSet = "", false
	if out := s.run(context.Background(), o, nil); out.Status != "success" || !out.Result.(*Result).Registered {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

// A first prompt whose acknowledgement is lost when the budget runs out may
// have been submitted: the outcome is unknown and never says it was unsent.
func TestSpawnBudgetLostPromptAckIsUnknown(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := lateSpawn(t, func(late func()) []call {
		return []call{labeled(p), {Method: "agent.start", Result: started(p)}, {Method: "agent.wait", Result: launching(p, "idle", flag(true), nil)}, senderCall(),
			{Method: "agent.prompt", Err: cutOff(), Before: late}}
	})
	out := s.run(context.Background(), budgetOptions(), nil)
	r := out.Result.(*Result)
	if out.Status != "unknown" || out.Error == nil || out.Error.Phase != "agent.prompt" || r.Prompted || r.MessageID == nil {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if h := render(t, out); !strings.Contains(h, "may have been submitted") || !strings.Contains(h, "fledge agent read --pane w1:p1") || strings.Contains(h, "not submitted") {
		t.Fatalf("%q", h)
	}
}

// deadlines records the context deadline each request carried.
type deadlines struct {
	inner libagent.API
	seen  map[string][]time.Time
}

func (d *deadlines) Call(ctx context.Context, method string, params, result any) error {
	deadline, _ := ctx.Deadline()
	d.seen[method] = append(d.seen[method], deadline)
	return d.inner.Call(ctx, method, params, result)
}

// Every request from agent.start through the first prompt carries the spawn's
// --timeout deadline, so a late reply is cut off instead of awaited.
func TestSpawnBudgetBoundsEveryRequestFromStart(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, labeled(p), call{Method: "agent.start", Result: started(p)}, call{Method: "agent.wait", Result: launching(p, "idle", nil, flag(true))}, getCall(launching(p, "idle", flag(true), nil)), senderCall(), call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: launching(p, "working", flag(true), nil).Agent.Pane}}})
	s.Wait = func(context.Context, time.Duration) error { return nil }
	d := &deadlines{inner: s.API, seen: map[string][]time.Time{}}
	s.API = d
	before := time.Now()
	out := s.run(context.Background(), budgetOptions(), nil)
	if out.Status != "success" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	for _, method := range []string{"agent.start", "agent.wait", "agent.get", "agent.prompt"} {
		if len(d.seen[method]) == 0 {
			t.Fatalf("no %s request", method)
		}
		for _, deadline := range d.seen[method] {
			if deadline.IsZero() || deadline.Before(before) || deadline.After(time.Now().Add(3001*time.Millisecond)) {
				t.Fatalf("%s deadline %v, want within 3001ms of the spawn", method, deadline)
			}
		}
	}
}
