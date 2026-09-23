package send

import (
	"bytes"
	"context"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

type call = herdrscript.Call

var fake = herdrscript.Client

func TestSendDeliversTextAndKeysVerbatimInOneCall(t *testing.T) {
	for _, tc := range []struct {
		name   string
		o      Options
		params map[string]any
	}{
		{"text", Options{Target: identity.Target{Name: "worker"}, Text: "/model x\nsecond"}, map[string]any{"pane_id": "w1:p3", "text": "/model x\nsecond"}},
		{"keys", Options{Target: identity.Target{Name: "worker"}, Keys: []string{"down", "enter"}}, map[string]any{"pane_id": "w1:p3", "keys": []any{"down", "enter"}}},
		{"both", Options{Target: identity.Target{Name: "worker"}, Text: "/model x", Keys: []string{"enter", "esc"}}, map[string]any{"pane_id": "w1:p3", "text": "/model x", "keys": []any{"enter", "esc"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := herdrscript.LiveAgent("working")
			s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: herdrscript.Info(p)}, call{Method: "pane.send_input", Params: tc.params, Result: herdrscript.OK()})
			out := Run(context.Background(), s, tc.o)
			r, ok := out.Result.(Result)
			if out.Status != "success" || !ok || !r.Submitted || len(out.Effects) != 1 || out.Effects[0] != (libagent.Effect{Action: "submitted", Kind: "input", ID: "w1:p3"}) {
				t.Fatalf("%+v", out)
			}
		})
	}
}

func TestSendReportsStatusObservedBeforeSending(t *testing.T) {
	for _, status := range []string{"idle", "blocked", "working", "unknown", "done"} {
		t.Run(status, func(t *testing.T) {
			p := herdrscript.LiveAgent(status)
			s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(p)}, call{Method: "pane.send_input", Result: herdrscript.OK()})
			out := Run(context.Background(), s, Options{Target: identity.Target{Pane: "w1:p3"}, Keys: []string{"enter"}})
			r := out.Result.(Result)
			if out.Status != "success" || r.AgentStatus == nil || *r.AgentStatus != status {
				t.Fatalf("%+v", out)
			}
		})
	}
}

func TestSendRequiresTextOrKeysBeforeAPI(t *testing.T) {
	for _, o := range []Options{{Target: identity.Target{Name: "worker"}}, {Target: identity.Target{Name: "worker"}, Keys: []string{}}} {
		out := Run(context.Background(), fake(t), o)
		if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Phase != "validation" || out.Error.Message != "at least one of --text or --key is required" {
			t.Fatalf("%+v", out)
		}
	}
}

func TestSendTargetValidation(t *testing.T) {
	for _, o := range []Options{{Keys: []string{"enter"}}, {Target: identity.Target{Name: "a", Pane: "p"}, Keys: []string{"enter"}}, {Target: identity.Target{Name: "a", ID: "0000beef"}, Keys: []string{"enter"}}} {
		out := Run(context.Background(), fake(t), o)
		if out.ExitCode() != 2 || out.Error.Phase != "validation" {
			t.Fatalf("%+v", out)
		}
	}
}

func TestSendInvalidKeyIsRejected(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("idle"))}, call{Method: "pane.send_input", Err: &herdr.Error{Code: "invalid_key", Message: "unsupported key bogus"}})
	out := Run(context.Background(), s, Options{Target: identity.Target{Name: "worker"}, Text: "hi", Keys: []string{"bogus"}})
	if out.Status != "rejected" || out.Error.Code != "invalid_key" || out.Error.Phase != "pane.send_input" || out.Result.(Result).Submitted || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
}

func TestSendDeliveryFailures(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		result       any
		err          error
	}{
		{"lost", "unknown", nil, &herdr.Error{Code: "transport_error", Message: "lost", Uncertain: true}},
		{"malformed", "unknown", map[string]any{"type": "wrong"}, nil},
		{"gone", "rejected", nil, &herdr.Error{Code: "pane_not_found", Message: "gone"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(herdrscript.LiveAgent("idle"))}, call{Method: "pane.send_input", Result: tc.result, Err: tc.err})
			out := Run(context.Background(), s, Options{Target: identity.Target{Name: "worker"}, Keys: []string{"enter"}})
			if out.Status != tc.status || out.Error.Phase != "pane.send_input" || out.Result.(Result).Submitted {
				t.Fatalf("%+v", out)
			}
		})
	}
}

func TestSendLookupFailureDoesNotSend(t *testing.T) {
	s := fake(t, call{Method: "agent.get", Err: &herdr.Error{Code: "agent_not_found", Message: "no agent"}})
	out := Run(context.Background(), s, Options{Target: identity.Target{Name: "worker"}, Keys: []string{"enter"}})
	if out.Status != "rejected" || out.Error.Phase != "agent.get" || out.Error.Code != "agent_not_found" {
		t.Fatalf("%+v", out)
	}
}

func TestSendByIDUsesVerifiedPane(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("blocked"))
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live},
		call{Method: "pane.send_input", Params: map[string]any{"pane_id": "w1:p3", "keys": []any{"down", "enter"}}, Result: herdrscript.OK()})
	s.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, s.Cwd, live.Agent)
	out := Run(context.Background(), s, Options{Target: identity.Target{ID: rec.ID}, Keys: []string{"down", "enter"}})
	if out.Status != "success" || !out.Result.(Result).Submitted {
		t.Fatalf("%+v", out)
	}
}

func TestSendByStaleIDDoesNotSend(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live}, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []any{}}}, call{Method: "pane.list", Result: map[string]any{"type": "pane_list", "panes": []any{}}})
	s.Cwd = identitytest.Repository(t)
	recorded := live.Agent
	recorded.TerminalID = "term_old"
	rec := identitytest.Register(t, s.Cwd, recorded)
	out := Run(context.Background(), s, Options{Target: identity.Target{ID: rec.ID}, Keys: []string{"enter"}})
	if out.Error == nil || out.Error.Code != "agent_identity_stale" || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
}

func TestHumanOperationResults(t *testing.T) {
	var b bytes.Buffer
	row := herdrscript.Row()
	blocked := "blocked"
	row.AgentStatus = &blocked
	if err := (libagent.Outcome{Status: "success", Result: Result{AgentRow: row, Submitted: true}}).Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(b.String()), "Sent input to worker (claude) in w1:p1; it was blocked before sending."; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	b.Reset()
	if err := (libagent.Outcome{Status: "success", Result: Result{AgentRow: row, Submitted: true}}).Write(&b, true, Render); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"agent_status":"blocked"`) || !strings.Contains(b.String(), `"submitted":true`) {
		t.Fatal(b.String())
	}
}

func TestOutputFailuresPropagate(t *testing.T) {
	herdrscript.CheckOutputFailures(t, Render, libagent.Outcome{Result: Result{}})
}
