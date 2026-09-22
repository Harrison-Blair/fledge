package message

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
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
	out := run(context.Background(), s, Options{Name: "worker", File: "-", FileSet: true}, strings.NewReader("hello\nworld\n"), "m-0a1b2c")
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
	out := Run(context.Background(), s, Options{Name: "worker", Body: "hi", BodySet: true}, strings.NewReader(""))
	if r, ok := out.Result.(Result); !ok || !regexp.MustCompile(`^m-[0-9a-f]{6}$`).MatchString(r.MessageID) {
		t.Fatalf("%+v", out)
	}
}

func TestAttributionFailureDoesNotFailSend(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(p)}, call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: &herdr.Error{Code: "timeout", Message: "slow"}}, call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": "ᛉ fledge message from unknown sender · id m-0a1b2c\nhi"}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: p}}})
	out := run(context.Background(), s, Options{Name: "worker", Body: "hi", BodySet: true}, strings.NewReader(""), "m-0a1b2c")
	r, ok := out.Result.(Result)
	if out.Status != "success" || !ok || r.Sender.Kind != "unknown" || r.Sender.Error == nil || *r.Sender.Error != "timeout: slow" {
		t.Fatalf("%+v", out)
	}
}
func TestInvalidMessageBeforeAPI(t *testing.T) {
	s := fake(t)
	out := Run(context.Background(), s, Options{Name: "a", BodySet: true}, strings.NewReader(""))
	if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Message != "message must be nonempty UTF-8" {
		t.Fatal(out)
	}
}
func TestMessageRequiresExactlyOneOfBodyOrFile(t *testing.T) {
	for _, o := range []Options{
		{Name: "a"},
		{Name: "a", Body: "hi", BodySet: true, File: "-", FileSet: true},
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
	out := Run(context.Background(), s, Options{Name: "worker", File: "/does/not/exist", FileSet: true}, strings.NewReader(""))
	if out.ExitCode() != 1 || !strings.HasPrefix(out.Error.Message, "read message: ") {
		t.Fatalf("%+v", out)
	}
}
func TestMessageBlockedIsRejectedWithoutMutation(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "blocked"
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(p)}, senderCall(), call{Method: "agent.prompt", Err: &herdr.Error{Code: "agent_blocked", Message: "approval"}})
	out := Run(context.Background(), s, Options{Name: "worker", Body: "hello", BodySet: true}, strings.NewReader(""))
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
