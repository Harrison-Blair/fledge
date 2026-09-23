package spawn

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func callerNotAgent() call {
	return call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: &herdr.Error{Code: "agent_not_found", Message: "not an agent"}}
}

func stored(t *testing.T, cwd, id string) identity.Record {
	t.Helper()
	s, err := identity.Existing(context.Background(), cwd)
	if err != nil || s == nil {
		t.Fatalf("%v", err)
	}
	var rec identity.Record
	if err := s.Get(identity.Kind, id, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestSpawnRegistersAfterWait(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), callerNotAgent())
	s.Cwd = identitytest.Repository(t)
	out := s.run(context.Background(), o, nil)
	r := out.Result.(*Result)
	if out.Status != "success" || !r.Registered || r.ID == nil || r.RegistrationError != nil {
		t.Fatalf("%+v %+v", out, r)
	}
	rec := stored(t, s.Cwd, *r.ID)
	if rec.TerminalID != "term_x" || rec.Pane != "w1:p1" || rec.RegisteredBy != "spawn" || rec.Parent != nil || rec.WorktreePath != nil {
		t.Fatalf("%+v", rec)
	}
	if last := out.Effects[len(out.Effects)-1]; last != (libagent.Effect{Action: "created", Kind: "agent_record", ID: *r.ID}) {
		t.Fatalf("%+v", out.Effects)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || !strings.Contains(b.String(), "  id: "+*r.ID+"\n") {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestSpawnRecordsCallerAsParent(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	caller := herdrscript.Info(herdrscript.Pane("old:p1", "old", "old:t1"))
	caller.Agent.AgentStatus, caller.Agent.TerminalID = "working", "term_parent"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"),
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: caller})
	s.Cwd = identitytest.Repository(t)
	parent := identitytest.Register(t, s.Cwd, caller.Agent)
	out := s.run(context.Background(), o, nil)
	r := out.Result.(*Result)
	if rec := stored(t, s.Cwd, *r.ID); rec.Parent == nil || *rec.Parent != parent.ID {
		t.Fatalf("%+v", rec)
	}
}

func TestSpawnNoWaitRegistersFromStart(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.NoWait = true
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	start := started(p)
	start.Agent.TerminalID = "term_start"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "agent.start", Result: start}, callerNotAgent())
	s.Cwd = identitytest.Repository(t)
	out := s.run(context.Background(), o, nil)
	r := out.Result.(*Result)
	if out.Status != "success" || !r.Registered || stored(t, s.Cwd, *r.ID).TerminalID != "term_start" {
		t.Fatalf("%+v %+v", out, r)
	}
}

func TestSpawnNoWaitRecordsRequestedHarness(t *testing.T) {
	// agent.start reports no harness until Herdr detects one; the record must
	// still carry the requested harness so a later different harness in the
	// same terminal does not inherit it.
	o := validOptions()
	o.Pane = "w1:p1"
	o.NoWait = true
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	start := started(p)
	start.Agent.Agent, start.Agent.TerminalID = nil, "term_start"
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "agent.start", Result: start}, callerNotAgent())
	s.Cwd = identitytest.Repository(t)
	out := s.run(context.Background(), o, nil)
	r := out.Result.(*Result)
	if out.Status != "success" || !r.Registered {
		t.Fatalf("%+v %+v", out, r)
	}
	if rec := stored(t, s.Cwd, *r.ID); rec.Harness == nil || *rec.Harness != "claude" {
		t.Fatalf("harness = %v, want claude", rec.Harness)
	}
	if r.DetectedHarness != nil {
		t.Fatalf("detected harness = %q, want none", *r.DetectedHarness)
	}
}

func TestSpawnWaitRecordsRequestedHarnessWhenUnclassified(t *testing.T) {
	// agent.wait can settle before Herdr classifies the agent; the record must
	// still carry the requested harness.
	o := validOptions()
	o.Pane = "w1:p1"
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	wait := waitCall("worker", p, "idle")
	if wait.Result.(herdr.AgentResult).Agent.Agent != nil {
		t.Fatal("fixture must model an unclassified agent")
	}
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "agent.start", Result: started(p)}, wait, callerNotAgent())
	s.Cwd = identitytest.Repository(t)
	out := s.run(context.Background(), o, nil)
	r := out.Result.(*Result)
	if out.Status != "success" || !r.Registered {
		t.Fatalf("%+v %+v", out, r)
	}
	if rec := stored(t, s.Cwd, *r.ID); rec.Harness == nil || *rec.Harness != "claude" {
		t.Fatalf("harness = %v, want claude", rec.Harness)
	}
	if r.DetectedHarness != nil {
		t.Fatalf("detected harness = %q, want none", *r.DetectedHarness)
	}
}

func TestSpawnRegistersBeforeFirstPrompt(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	o.Prompt, o.PromptSet = "go", true
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"), callerNotAgent(), callerNotAgent(),
		call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: herdrscript.Waited(p, "working").Agent.Pane}}})
	s.Cwd = identitytest.Repository(t)
	out := s.run(context.Background(), o, nil)
	if r := out.Result.(*Result); out.Status != "success" || !r.Registered || !r.Prompted {
		t.Fatalf("%+v %+v", out, r)
	}
}

func TestSpawnWithoutStoreStillSucceeds(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "agent.start", Result: started(p)}, waitCall("worker", p, "idle"))
	out := s.run(context.Background(), o, nil)
	r := out.Result.(*Result)
	if out.Status != "success" || out.Error != nil || r.Registered || r.ID != nil || r.RegistrationError == nil {
		t.Fatalf("%+v %+v", out, r)
	}
	if !reflect.DeepEqual(out.Effects, []libagent.Effect{{Action: "reused", Kind: "pane", ID: "w1:p1"}, {Action: "started", Kind: "agent", ID: "w1:p1"}}) {
		t.Fatalf("%+v", out.Effects)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || !strings.Contains(b.String(), "  id: - (not registered: ") {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestSpawnDoesNotRegisterUnconfirmedStartup(t *testing.T) {
	o := validOptions()
	o.Pane = "w1:p1"
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	s := fake(t, call{Method: "session.snapshot", Result: snapshot()}, call{Method: "agent.start", Result: started(p)}, call{Method: "agent.wait", Err: &herdr.Error{Code: "timeout", Message: "slow"}})
	s.Cwd = identitytest.Repository(t)
	out := s.run(context.Background(), o, nil)
	if r := out.Result.(*Result); r.Registered || r.ID != nil {
		t.Fatalf("%+v", r)
	}
}
