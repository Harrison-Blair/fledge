package spawn

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

// pollClock advances a movable clock by each readiness poll delay.
type pollClock struct {
	movableClock
	delays []time.Duration
}

func (c *pollClock) wait(_ context.Context, d time.Duration) error {
	c.delays = append(c.delays, d)
	c.advance(d)
	return nil
}

// launching is worker in p with the given readiness flags; nil leaves a flag absent.
func launching(p herdr.Pane, status string, ready, pending *bool) herdr.AgentResult {
	r := settled(p, status)
	r.Agent.InteractiveReady, r.Agent.LaunchPending = ready, pending
	return r
}
func flag(v bool) *bool { return &v }
func getCall(r herdr.AgentResult) call {
	return call{Method: "agent.get", Params: map[string]any{"target": "w1:p1"}, Result: r}
}

// gatedSpawn scripts a spawn into w1:p1 whose lifecycle wait returns wait,
// followed by the readiness polls and any later calls in rest.
func gatedSpawn(t *testing.T, wait herdr.AgentResult, rest ...call) (*spawner, *pollClock) {
	t.Helper()
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	calls := append([]call{{Method: "session.snapshot", Result: snapshot()}, {Method: "agent.start", Result: started(p)}, {Method: "agent.wait", Result: wait}}, rest...)
	s := fake(t, calls...)
	clock := &pollClock{movableClock: movableClock{t: time.Unix(0, 0)}}
	s.Now, s.Wait = clock.now, clock.wait
	return s, clock
}

// Lifecycle idle is not prompt readiness: the first prompt waits until
// interactive_ready is true, launch_pending is clear, and the status settled.
func TestSpawnWaitsForPromptReadiness(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	for _, tc := range []struct {
		name  string
		wait  herdr.AgentResult
		polls []herdr.AgentResult
	}{
		{"pending clears by absence", launching(p, "idle", nil, flag(true)), []herdr.AgentResult{launching(p, "idle", flag(true), nil)}},
		{"pending clears by false", launching(p, "idle", nil, flag(true)), []herdr.AgentResult{launching(p, "idle", flag(true), flag(false))}},
		{"ready while pending is not ready", launching(p, "idle", flag(true), flag(true)), []herdr.AgentResult{launching(p, "idle", flag(true), nil)}},
		{"absent ready is not ready", launching(p, "idle", nil, nil), []herdr.AgentResult{launching(p, "idle", flag(true), nil)}},
		{"false ready is not ready", launching(p, "idle", flag(false), nil), []herdr.AgentResult{launching(p, "idle", flag(false), nil), launching(p, "done", flag(true), nil)}},
		{"transient statuses keep polling", launching(p, "idle", nil, flag(true)), []herdr.AgentResult{launching(p, "unknown", flag(true), nil), launching(p, "working", flag(true), nil), launching(p, "idle", flag(true), nil)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := paneOptions()
			o.Prompt, o.PromptSet = "hi", true
			var rest []call
			for _, r := range tc.polls {
				rest = append(rest, getCall(r))
			}
			rest = append(rest, senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": header + "hi"}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: launching(p, "working", flag(true), nil).Agent.Pane}}})
			s, clock := gatedSpawn(t, tc.wait, rest...)
			out := s.run(context.Background(), o, nil)
			r := out.Result.(*Result)
			if out.Status != "success" || !r.Prompted {
				t.Fatalf("%+v %+v", out, out.Error)
			}
			if want := tc.polls[len(tc.polls)-1].Agent.AgentStatus; *r.AgentStatus != want {
				t.Fatalf("status %s, want %s from the last poll", *r.AgentStatus, want)
			}
			for _, d := range clock.delays {
				if d != 100*time.Millisecond {
					t.Fatalf("poll delays %v", clock.delays)
				}
			}
			if len(clock.delays) != len(tc.polls) {
				t.Fatalf("poll delays %v for %d polls", clock.delays, len(tc.polls))
			}
		})
	}
}

// A spawn without a first prompt is also gated, and registers only once ready.
func TestSpawnWithoutPromptRegistersAfterReadiness(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s, _ := gatedSpawn(t, launching(p, "idle", nil, flag(true)), getCall(launching(p, "idle", flag(true), nil)), callerNotAgent())
	s.Cwd = identitytest.Repository(t)
	out := s.run(context.Background(), paneOptions(), nil)
	if r := out.Result.(*Result); out.Status != "success" || !r.Registered || r.Prompted {
		t.Fatalf("%+v %+v", out, r)
	}
}

// Blocked during the gate exits partial like a blocked lifecycle wait: the
// agent is registered and kept, and the requested prompt is never submitted.
func TestSpawnBlockedDuringReadinessIsPartial(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	o := paneOptions()
	o.Prompt, o.PromptSet = "secret brief", true
	s, _ := gatedSpawn(t, launching(p, "idle", nil, flag(true)), getCall(launching(p, "blocked", nil, flag(true))), callerNotAgent())
	s.Cwd = identitytest.Repository(t)
	out := s.run(context.Background(), o, nil)
	r := out.Result.(*Result)
	if out.Status != "partial" || out.Error == nil || out.Error.Code != "agent_blocked" || out.Error.Phase != "agent.wait" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if r.Prompted || !r.PromptRequested || !r.Registered || r.MessageID != nil {
		t.Fatalf("%+v", r)
	}
}

// The gate shares spawn's local budget: it sleeps at most the remaining time,
// then fails partial with timeout, before registering or prompting.
func TestSpawnReadinessBudgetExhaustionIsPartialTimeout(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	o := paneOptions()
	o.Timeout = 3001 * time.Millisecond
	o.Prompt, o.PromptSet = "hi", true
	pending := launching(p, "idle", nil, flag(true))
	clock := &pollClock{movableClock: movableClock{t: time.Unix(0, 0)}}
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "agent.start", Result: started(p), Before: func() { clock.advance(2800 * time.Millisecond) }}, call{Method: "agent.wait", Params: map[string]any{"target": "worker", "timeout_ms": 201}, Result: pending}, getCall(pending), getCall(pending), getCall(pending))
	s.Now, s.Wait = clock.now, clock.wait
	s.Cwd = identitytest.Repository(t)
	out := s.run(context.Background(), o, nil)
	r := out.Result.(*Result)
	if out.Status != "partial" || out.Error == nil || out.Error.Code != "timeout" || out.Error.Phase != "agent.wait" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if r.Prompted || !r.PromptRequested || r.Registered || r.RegistrationError != nil {
		t.Fatalf("%+v", r)
	}
	if want := []time.Duration{100 * time.Millisecond, 100 * time.Millisecond, time.Millisecond}; !reflect.DeepEqual(clock.delays, want) {
		t.Fatalf("delays %v, want %v", clock.delays, want)
	}
}

// Cancellation during a readiness pause ends the spawn without another poll.
func TestSpawnReadinessHonorsCancellation(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "agent.start", Result: started(p)}, call{Method: "agent.wait", Result: launching(p, "idle", nil, flag(true))})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &recordedWaits{cancel: cancel}
	s.Wait = w.wait
	out := s.run(ctx, paneOptions(), nil)
	if out.Status != "partial" || out.Error == nil || out.Error.Phase != "agent.wait" || len(w.delays) != 1 {
		t.Fatalf("%+v %+v %v", out, out.Error, w.delays)
	}
}

// A different terminal, name, harness, or pane answering for the started
// agent fails closed: no prompt, no registration of the replacement.
func TestSpawnReadinessRejectsReplacedAgent(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	ready := func(change func(*herdr.AgentDetails)) herdr.AgentResult {
		r := launching(p, "idle", flag(true), nil)
		change(&r.Agent)
		return r
	}
	other, pi := "other", "pi"
	for _, tc := range []struct {
		name   string
		change func(*herdr.AgentDetails)
	}{
		{"terminal", func(a *herdr.AgentDetails) { a.TerminalID = "term_new" }},
		{"name", func(a *herdr.AgentDetails) { a.Name = &other }},
		{"name lost", func(a *herdr.AgentDetails) { a.Name = nil }},
		{"harness", func(a *herdr.AgentDetails) { a.Agent = &pi }},
		{"pane", func(a *herdr.AgentDetails) { a.PaneID = "w1:p9" }},
	} {
		for _, when := range []string{"wait", "poll"} {
			t.Run(tc.name+" at "+when, func(t *testing.T) {
				o := paneOptions()
				o.Prompt, o.PromptSet = "hi", true
				var s *spawner
				if when == "wait" {
					s, _ = gatedSpawn(t, ready(tc.change))
				} else {
					s, _ = gatedSpawn(t, launching(p, "idle", nil, flag(true)), getCall(ready(tc.change)))
				}
				s.Cwd = identitytest.Repository(t)
				out := s.run(context.Background(), o, nil)
				r := out.Result.(*Result)
				if out.Status != "unknown" || out.Error == nil || out.Error.Phase != "agent.wait" {
					t.Fatalf("%+v %+v", out, out.Error)
				}
				if r.Prompted || r.Registered || r.RegistrationError != nil {
					t.Fatalf("%+v", r)
				}
			})
		}
	}
}

// An unclassified harness is not a different one: the record keeps the
// requested harness, as for an unclassified lifecycle wait.
func TestSpawnReadinessAcceptsUnclassifiedHarness(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	poll := launching(p, "idle", flag(true), nil)
	poll.Agent.Agent = nil
	s, _ := gatedSpawn(t, launching(p, "idle", nil, flag(true)), getCall(poll))
	if out := s.run(context.Background(), paneOptions(), nil); out.Status != "success" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

// A failed readiness poll stops the spawn at the startup wait's phase, so the
// pane-addressed recovery hints still apply.
func TestSpawnReadinessPollFailure(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	for _, tc := range []struct {
		name, status, code string
		poll               call
	}{
		{"not found", "partial", "agent_not_found", call{Method: "agent.get", Err: &herdr.Error{Code: "agent_not_found", Message: "gone"}}},
		{"lost", "unknown", "transport_error", call{Method: "agent.get", Err: &herdr.Error{Code: "transport_error", Message: "lost", Uncertain: true}}},
		{"malformed", "unknown", "protocol_error", call{Method: "agent.get", Result: herdr.AgentResult{Type: "wrong"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := paneOptions()
			o.Prompt, o.PromptSet = "hi", true
			s, _ := gatedSpawn(t, launching(p, "idle", nil, flag(true)), tc.poll)
			out := s.run(context.Background(), o, nil)
			if out.Status != tc.status || out.Error == nil || out.Error.Code != tc.code || out.Error.Phase != "agent.wait" || out.Result.(*Result).Prompted {
				t.Fatalf("%+v %+v", out, out.Error)
			}
			var b strings.Builder
			if err := Render(&b, out); err != nil || !strings.Contains(b.String(), "fledge agent read --pane w1:p1") || !strings.Contains(b.String(), "The first prompt was not submitted.") {
				t.Fatalf("%q %v", b.String(), err)
			}
		})
	}
}

// Every first-prompt source is submitted exactly once, after the gate, with
// one sender header.
func TestSpawnFirstPromptSourcesSubmitOnceAfterReadiness(t *testing.T) {
	reviewer := builtinProfile(t, "reviewer").Role
	for _, tc := range []struct {
		name, in, text string
		set            func(*Options)
	}{
		{"inline", "", "task", func(o *Options) { o.Prompt, o.PromptSet = "task", true }},
		{"file", "from file\n", "from file\n", func(o *Options) { o.File, o.FileSet = "-", true }},
		{"role only", "", reviewer, func(o *Options) { o.Harness, o.Profile = "", "reviewer" }},
		{"role and task", "", reviewer + "\n\ntask", func(o *Options) { o.Harness, o.Profile, o.Prompt, o.PromptSet = "", "reviewer", "task", true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
			o := paneOptions()
			tc.set(&o)
			wait, poll := launching(p, "idle", nil, flag(true)), launching(p, "idle", flag(true), nil)
			if o.Profile != "" {
				pi := "pi"
				wait.Agent.Agent, poll.Agent.Agent = &pi, &pi
			}
			s, _ := gatedSpawn(t, wait, getCall(poll), senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": header + tc.text}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: poll.Agent.Pane}}})
			out := s.run(context.Background(), o, strings.NewReader(tc.in))
			if r := out.Result.(*Result); out.Status != "success" || !r.Prompted || !r.PromptRequested {
				t.Fatalf("%+v %+v", out, out.Error)
			}
		})
	}
}
