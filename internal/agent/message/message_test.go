package message

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

type call = herdrscript.Call

var fake = herdrscript.Client

func TestMessagePreservesContentAndDoesNotWait(t *testing.T) {
	lookupCwd, promptCwd := "/before-prompt", "/after-prompt"
	lookupPane := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	lookupPane.AgentStatus, lookupPane.Cwd = "working", &lookupCwd
	promptPane := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	promptPane.AgentStatus, promptPane.Cwd = "working", &promptCwd
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: herdrscript.Info(lookupPane)}, senderCall(), call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": header + "hello\nworld\n"}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: promptPane}}})
	out := run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}, File: "-", FileSet: true}, strings.NewReader("hello\nworld\n"), "m-0a1b2c")
	result, ok := out.Result.(Result)
	if out.Status != "success" || !ok || !result.Submitted || result.Cwd == nil || *result.Cwd != promptCwd {
		t.Fatalf("%+v", out)
	}
	if result.MessageID != "m-0a1b2c" || result.Sender == nil || result.Sender.Kind != "named" || *result.Sender.Name != "orchestrator" || *result.Sender.Pane != "old:p1" {
		t.Fatalf("%+v", result)
	}
}

const header = "ᛉ fledge message from orchestrator (old:p1) · id m-0a1b2c · reply: fledge agent message --name orchestrator\n"

// senderCall resolves the scripted caller pane old:p1 to the named agent orchestrator.
func senderCall() call {
	p := herdrscript.Pane("old:p1", "old", "old:t1")
	p.AgentStatus = "working"
	name := "orchestrator"
	p.Name = &name
	return call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: herdrscript.Info(p)}
}

func TestRunGeneratesMessageID(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(p)}, senderCall(), call{Method: "agent.prompt", Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}, Body: "hi", BodySet: true}, strings.NewReader(""))
	if r, ok := out.Result.(Result); !ok || !regexp.MustCompile(`^m-[0-9a-f]{6}$`).MatchString(r.MessageID) {
		t.Fatalf("%+v", out)
	}
}

func TestAttributionFailureDoesNotFailSend(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(p)}, call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: &herdr.Error{Code: "timeout", Message: "slow"}}, call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": "ᛉ fledge message from unknown sender · id m-0a1b2c\nhi"}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	out := run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}, Body: "hi", BodySet: true}, strings.NewReader(""), "m-0a1b2c")
	r, ok := out.Result.(Result)
	if out.Status != "success" || !ok || r.Sender.Kind != "unknown" || r.Sender.Error == nil || *r.Sender.Error != "timeout: slow" {
		t.Fatalf("%+v", out)
	}
}
func TestInvalidMessageBeforeAPI(t *testing.T) {
	s := fake(t)
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"a"}}, BodySet: true}, strings.NewReader(""))
	if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Message != "message must be nonempty UTF-8" {
		t.Fatal(out)
	}
}
func TestMessageRequiresExactlyOneOfBodyOrFile(t *testing.T) {
	for _, o := range []Options{
		{Selection: selector.Selection{Names: []string{"a"}}},
		{Selection: selector.Selection{Names: []string{"a"}}, Body: "hi", BodySet: true, File: "-", FileSet: true},
	} {
		s := fake(t)
		out := Run(context.Background(), s, o, strings.NewReader(""))
		if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Message != "exactly one of --body or --file is required" {
			t.Fatalf("%+v", out)
		}
	}
}
func TestUnreadableMessageIsRuntimeFailure(t *testing.T) {
	s := fake(t)
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}, File: "/does/not/exist", FileSet: true}, strings.NewReader(""))
	if out.ExitCode() != 1 || !strings.HasPrefix(out.Error.Message, "read message: ") {
		t.Fatalf("%+v", out)
	}
}
func TestMessageBlockedIsRejectedWithoutMutation(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "blocked"
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(p)}, senderCall(), call{Method: "agent.prompt", Err: &herdr.Error{Code: "agent_blocked", Message: "approval"}})
	out := Run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}, Body: "hello", BodySet: true}, strings.NewReader(""))
	if out.Status != "rejected" || out.Error.Code != "agent_blocked" {
		t.Fatal(out)
	}
}
func TestHumanOperationResults(t *testing.T) {
	var b bytes.Buffer
	name, pane := "orchestrator", "wA:p1"
	result := Result{AgentRow: herdrscript.Row(), Submitted: true, MessageID: "m-0a1b2c", Sender: &libagent.Sender{Name: &name, Pane: &pane, Kind: "named"}}
	if err := (libagent.Outcome{Status: "success", Result: result}).Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(strings.Fields(b.String()), " "), "Message m-0a1b2c submitted to w1:p1 from orchestrator (wA:p1)."; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	b.Reset()
	if err := (libagent.Outcome{Status: "success", Result: result}).Write(&b, true, Render); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"message_id":"m-0a1b2c","sender":{"name":"orchestrator","pane":"wA:p1","kind":"named","error":null}`) {
		t.Fatal(b.String())
	}
}
func TestOutputFailuresPropagate(t *testing.T) {
	herdrscript.CheckOutputFailures(t, Render, libagent.Outcome{Result: Result{}})
}

func TestMessageByIDPromptsVerifiedPane(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	prompted := herdr.AgentResult{Type: "agent_prompted", Agent: live.Agent}
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live}, senderCall(),
		call{Method: "agent.prompt", Params: map[string]any{"target": "w1:p3", "text": header + "hi"}, Result: prompted})
	s.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, s.Cwd, live.Agent)
	out := run(context.Background(), s, Options{Selection: selector.Selection{IDs: []string{rec.ID}}, Body: "hi", BodySet: true}, nil, "m-0a1b2c")
	if out.Status != "success" || !out.Result.(Result).Submitted {
		t.Fatalf("%+v", out)
	}
}

func TestMessageByStaleIDDoesNotPrompt(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live}, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []any{}}}, call{Method: "pane.list", Result: map[string]any{"type": "pane_list", "panes": []any{}}})
	s.Cwd = identitytest.Repository(t)
	recorded := live.Agent
	recorded.TerminalID = "term_old"
	rec := identitytest.Register(t, s.Cwd, recorded)
	out := run(context.Background(), s, Options{Selection: selector.Selection{IDs: []string{rec.ID}}, Body: "hi", BodySet: true}, nil, "m-0a1b2c")
	if out.Error == nil || out.Error.Code != "agent_identity_stale" || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
}

func TestMessageExplicitTargetsExcludeFilters(t *testing.T) {
	out := run(context.Background(), fake(t), Options{Selection: selector.Selection{Names: []string{"worker"}, Filter: selector.Filter{States: []string{"idle"}}}, Body: "hi", BodySet: true}, nil, "m-0a1b2c")
	if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Phase != "validation" {
		t.Fatalf("%+v", out)
	}
}

// confirmRun messages worker, observed before sending in status before, with
// --confirm; the scripted client fails the test on any second agent.prompt.
func confirmRun(t *testing.T, before string, result any, err error) libagent.Outcome {
	t.Helper()
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = before
	wait := map[string]any{"until": []string{"working", "done", "idle", "blocked"}, "timeout_ms": 10000}
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(p)}, senderCall(),
		call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": header + "hi", "wait": wait}, Result: result, Err: err})
	return run(context.Background(), s, Options{Selection: selector.Selection{Names: []string{"worker"}}, Body: "hi", BodySet: true, Confirm: true, Timeout: 10 * time.Second}, nil, "m-0a1b2c")
}

func prompted(status string) herdr.AgentResult {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = status
	return herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}
}

func TestConfirmReportsActivity(t *testing.T) {
	for _, tc := range []struct{ before, after string }{{"idle", "working"}, {"idle", "done"}, {"done", "idle"}, {"done", "working"}} {
		out := confirmRun(t, tc.before, prompted(tc.after), nil)
		r, ok := out.Result.(Result)
		if out.Status != "success" || !ok || !r.Submitted || r.Confirmed == nil || !*r.Confirmed || r.AlreadyWorking || *r.AgentStatus != tc.after || len(out.Effects) != 1 {
			t.Fatalf("%+v: %+v", tc, out)
		}
	}
}

func TestConfirmAlreadyWorkingIsNotConfirmed(t *testing.T) {
	out := confirmRun(t, "working", prompted("working"), nil)
	r, ok := out.Result.(Result)
	if out.Status != "success" || !ok || !r.Submitted || r.Confirmed == nil || *r.Confirmed || !r.AlreadyWorking || out.ExitCode() != 0 {
		t.Fatalf("%+v", out)
	}
}

func TestConfirmReturnedBlockedIsPartialSubmission(t *testing.T) {
	out := confirmRun(t, "idle", prompted("blocked"), nil)
	r, ok := out.Result.(Result)
	if out.Status != "partial" || !ok || !r.Submitted || r.Confirmed == nil || *r.Confirmed || out.ExitCode() != 1 || out.Error.Code != "agent_prompt_blocked" || len(out.Effects) != 1 || out.Effects[0].Action != "submitted" {
		t.Fatalf("%+v", out)
	}
}

func TestConfirmStalledIsPartialSubmission(t *testing.T) {
	out := confirmRun(t, "idle", nil, &herdr.Error{Code: "agent_prompt_stalled", Message: "no activity"})
	r, ok := out.Result.(Result)
	if out.Status != "partial" || !ok || !r.Submitted || r.Confirmed == nil || *r.Confirmed || out.ExitCode() != 1 || out.Error.Code != "agent_prompt_stalled" ||
		len(out.Effects) != 1 || out.Effects[0] != (libagent.Effect{Action: "submitted", Kind: "message", ID: "w1:p1"}) {
		t.Fatalf("%+v", out)
	}
}

func TestConfirmUncertainFailuresAreUnknown(t *testing.T) {
	for _, err := range []error{
		&herdr.Error{Code: "timeout", Message: "wait timed out"},
		&herdr.Error{Code: "transport_error", Message: "EOF", Uncertain: true},
		&herdr.Error{Code: "protocol_error", Message: "garbled", Uncertain: true},
	} {
		out := confirmRun(t, "idle", nil, err)
		r, ok := out.Result.(Result)
		if out.Status != "unknown" || !ok || r.Submitted || r.Confirmed == nil || *r.Confirmed || out.ExitCode() != 1 || len(out.Effects) != 0 {
			t.Fatalf("%v: %+v", err, out)
		}
	}
}

func TestConfirmPreInputRefusalsAreRejected(t *testing.T) {
	for _, code := range []string{"agent_blocked", "agent_not_ready", "agent_not_found", "empty_agent_prompt"} {
		out := confirmRun(t, "idle", nil, &herdr.Error{Code: code, Message: "refused"})
		r, ok := out.Result.(Result)
		if out.Status != "rejected" || !ok || r.Submitted || out.Error.Code != code || len(out.Effects) != 0 {
			t.Fatalf("%s: %+v", code, out)
		}
	}
}

func TestConfirmTimeoutValidation(t *testing.T) {
	for _, o := range []Options{
		{Selection: selector.Selection{Names: []string{"worker"}}, Body: "hi", BodySet: true, Confirm: true, Timeout: 0, TimeoutSet: true},
		{Selection: selector.Selection{Names: []string{"worker"}}, Body: "hi", BodySet: true, Confirm: true, Timeout: 500 * time.Microsecond, TimeoutSet: true},
		{Selection: selector.Selection{Names: []string{"worker"}}, Body: "hi", BodySet: true, Timeout: time.Second, TimeoutSet: true},
	} {
		out := Run(context.Background(), fake(t), o, nil)
		if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Phase != "validation" {
			t.Fatalf("%+v: %+v", o, out)
		}
	}
}

func TestConfirmHumanOutput(t *testing.T) {
	yes, no := true, false
	name, pane := "orchestrator", "wA:p1"
	sender := &libagent.Sender{Name: &name, Pane: &pane, Kind: "named"}
	row := herdrscript.Row()
	working := "working"
	row.AgentStatus = &working
	for _, tc := range []struct {
		out  libagent.Outcome
		want string
	}{
		{libagent.Outcome{Status: "success", Result: Result{AgentRow: row, Submitted: true, Confirmed: &yes, MessageID: "m-0a1b2c", Sender: sender}},
			"Message m-0a1b2c submitted to w1:p1 from orchestrator (wA:p1); activity confirmed (working)."},
		{libagent.Outcome{Status: "success", Result: Result{AgentRow: row, Submitted: true, Confirmed: &no, AlreadyWorking: true, MessageID: "m-0a1b2c", Sender: sender}},
			"Message m-0a1b2c submitted to w1:p1 from orchestrator (wA:p1) while the agent was already observed working; this prompt's start is not confirmed."},
		{libagent.Outcome{Status: "partial", Error: &libagent.Failure{Code: "agent_prompt_stalled", Message: "agent_prompt_stalled: no activity", Phase: "agent.prompt"}, Result: Result{AgentRow: row, Submitted: true, Confirmed: &no, MessageID: "m-0a1b2c", Sender: sender}},
			"partial: agent_prompt_stalled: no activity (agent.prompt) The message was submitted, but activity was not confirmed; do not resend it. Inspect: fledge agent read --pane w1:p1"},
		{libagent.Outcome{Status: "unknown", Error: &libagent.Failure{Code: "timeout", Message: "timeout: slow", Phase: "agent.prompt"}, Result: Result{AgentRow: row, Confirmed: &no, MessageID: "m-0a1b2c", Sender: sender}},
			"unknown: timeout: slow (agent.prompt) The message may have been submitted; do not resend it. Inspect: fledge agent read --pane w1:p1"},
		{libagent.Outcome{Status: "rejected", Error: &libagent.Failure{Code: "agent_blocked", Message: "agent_blocked: approval", Phase: "agent.prompt"}, Result: Result{AgentRow: row, Confirmed: &no, MessageID: "m-0a1b2c", Sender: sender}},
			"rejected: agent_blocked: approval (agent.prompt)"},
	} {
		var b bytes.Buffer
		if err := tc.out.Write(&b, false, Render); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(strings.Fields(b.String()), " "); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
}

func TestConfirmJSONFieldsOnlyWithConfirm(t *testing.T) {
	yes := true
	for _, tc := range []struct {
		result Result
		want   string
		absent bool
	}{
		{Result{Submitted: true, MessageID: "m-0a1b2c"}, `"confirmed"`, true},
		{Result{Submitted: true, MessageID: "m-0a1b2c", Confirmed: &yes}, `"confirmed":true`, false},
	} {
		var b bytes.Buffer
		if err := (libagent.Outcome{Status: "success", Result: tc.result}).Write(&b, true, Render); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(b.String(), tc.want) == tc.absent || strings.Contains(b.String(), "already_working") {
			t.Fatal(b.String())
		}
	}
}
