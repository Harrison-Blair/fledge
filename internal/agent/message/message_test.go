package message

import (
	"bytes"
	"context"
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
	s := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: herdrscript.Info(lookupPane)}, call{Method: "agent.prompt", Params: map[string]any{"target": "worker", "text": "hello\nworld\n"}, Result: herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: promptPane}}})
	out := Run(context.Background(), s, Options{Name: "worker", File: "-", FileSet: true}, strings.NewReader("hello\nworld\n"))
	result, ok := out.Result.(Result)
	if out.Status != "success" || !ok || !result.Submitted || result.Cwd == nil || *result.Cwd != promptCwd {
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
	s := fake(t, call{Method: "agent.get", Result: herdrscript.Info(p)}, call{Method: "agent.prompt", Err: &herdr.Error{Code: "agent_blocked", Message: "approval"}})
	out := Run(context.Background(), s, Options{Name: "worker", Body: "hello", BodySet: true}, strings.NewReader(""))
	if out.Status != "rejected" || out.Error.Code != "agent_blocked" {
		t.Fatal(out)
	}
}
func TestHumanOperationResults(t *testing.T) {
	var b bytes.Buffer
	if err := (libagent.Outcome{Status: "success", Result: Result{AgentRow: herdrscript.Row(), Submitted: true}}).Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(strings.Fields(b.String()), " "), "Message submitted to w1:p1."; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
func TestOutputFailuresPropagate(t *testing.T) {
	herdrscript.CheckOutputFailures(t, Render, libagent.Outcome{Result: Result{}})
}
