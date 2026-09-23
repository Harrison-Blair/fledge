package spawn

import (
	"bytes"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
)

func TestHumanSpawnIncludesDirectories(t *testing.T) {
	cwd := "/repo/.fledge/worktrees/topic"
	out := libagent.Outcome{Status: "success", Result: &Result{Name: "worker", Harness: "claude", Cwd: &cwd, WorktreePath: &cwd}}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "cwd: "+cwd) || !strings.Contains(b.String(), "worktree: "+cwd) {
		t.Fatal(b.String())
	}
}
func TestHumanSpawnIncludesPromptSubmission(t *testing.T) {
	pid := "w1:p1"
	out := libagent.Outcome{Status: "success", Result: &Result{Name: "worker", Harness: "claude", PaneID: &pid, Prompted: true}}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "Message submitted to w1:p1.") {
		t.Fatal(b.String())
	}
}
func TestHumanSpawnOmitsPromptLineWithoutSubmission(t *testing.T) {
	out := libagent.Outcome{Status: "success", Result: &Result{Name: "worker", Harness: "claude"}}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "Message submitted") {
		t.Fatal(b.String())
	}
}
func TestSpawnJSONIncludesPromptedField(t *testing.T) {
	pid := "w1:p1"
	out := libagent.Outcome{Operation: "agent.spawn", Status: "success", Effects: []libagent.Effect{}, Result: &Result{Name: "worker", Harness: "claude", PaneID: &pid, Prompted: true}}
	var b bytes.Buffer
	if err := out.Write(&b, true, Render); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"prompted":true,"prompt_requested":false`) || !strings.Contains(b.String(), `"message_id":null,"sender":null`) {
		t.Fatalf("%q", b.String())
	}
}

// The startup hints are keyed on the failing phase, not the error code: a
// failure in phase agent.prompt means startup WAS confirmed, so neither hint
// applies there even for an agent_blocked code (the agent is blocked on the
// prompt's own approval dialog, not a startup dialog).
func TestPartialHintKeyedOnPhaseNotCode(t *testing.T) {
	for _, tc := range []struct {
		name, code, status, phase, want, mustNotContain string
	}{
		{"wait blocked", "agent_blocked", "partial", "agent.wait", "Agent is waiting on a startup prompt", "Startup was not confirmed"},
		{"wait timeout", "timeout", "partial", "agent.wait", "Startup was not confirmed", "Agent is waiting on a startup prompt"},
		{"wait malformed result", "transport_error", "unknown", "agent.wait", "Startup was not confirmed", "Agent is waiting on a startup prompt"},
		{"start timeout", "timeout", "partial", "agent.start", "Startup was not confirmed", "Agent is waiting on a startup prompt"},
		{"prompt blocked hides both hints", "agent_blocked", "partial", "agent.prompt", "", "Startup was not confirmed"},
		{"prompt malformed result hides both hints", "transport_error", "unknown", "agent.prompt", "", "Startup was not confirmed"},
		{"rejected start hides both hints", "agent_pane_not_found", "rejected", "agent.start", "", "Startup was not confirmed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := libagent.Outcome{Status: tc.status, Result: &Result{Name: "worker"}, Error: &libagent.Failure{Code: tc.code, Message: "failure", Phase: tc.phase}}
			var b bytes.Buffer
			if err := out.Write(&b, false, Render); err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && !strings.Contains(b.String(), tc.want) {
				t.Fatalf("%q missing %q", b.String(), tc.want)
			}
			if strings.Contains(b.String(), tc.mustNotContain) {
				t.Fatalf("%q should not contain %q", b.String(), tc.mustNotContain)
			}
			if tc.phase == "agent.prompt" && strings.Contains(b.String(), "Agent is waiting on a startup prompt") {
				t.Fatalf("%q should not contain the startup-prompt hint for phase agent.prompt", b.String())
			}
		})
	}
}

func TestHumanFailureEffects(t *testing.T) {
	out := libagent.Outcome{Status: "partial", Error: &libagent.Failure{Message: "rename failed", Phase: "tab.rename"}, Effects: []libagent.Effect{
		{Action: "created", Kind: "pane", ID: "w2:p1"},
		{Action: "created", Kind: "worktree", Path: "/repo/topic"},
	}}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	want := "partial: rename failed (tab.rename)\n  created pane w2:p1\n  created worktree /repo/topic\n"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
}

func TestOutputFailuresPropagate(t *testing.T) {
	herdrscript.CheckOutputFailures(t, Render,
		libagent.Outcome{Result: &Result{Name: "worker", Prompted: true}},
		libagent.Outcome{Status: "partial", Error: &libagent.Failure{Message: "failed"}, Effects: []libagent.Effect{{Action: "created", Kind: "pane", ID: "w1:p1"}}},
		libagent.Outcome{Status: "partial", Result: &Result{Name: "worker"}, Error: &libagent.Failure{Message: "blocked", Code: "agent_blocked", Phase: "agent.wait"}},
		libagent.Outcome{Status: "partial", Result: &Result{Name: "worker", PromptRequested: true}, Error: &libagent.Failure{Message: "blocked", Code: "agent_blocked", Phase: "agent.wait"}},
		libagent.Outcome{Status: "partial", Result: &Result{Name: "worker", PromptRequested: true}, Error: &libagent.Failure{Message: "timeout", Code: "timeout", Phase: "agent.start"}},
	)
}

func renderHuman(t *testing.T, out libagent.Outcome) string {
	t.Helper()
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// Recovery hints name Fledge commands addressed by pane, since the agent's
// name may not resolve, and never echo the prompt or raw Herdr commands.
func TestBlockedWaitHintsUseFledgeByPane(t *testing.T) {
	pid := "w1:p4"
	for _, requested := range []bool{true, false} {
		out := libagent.Outcome{Status: "partial", Result: &Result{Name: "worker", PaneID: &pid, PromptRequested: requested}, Error: &libagent.Failure{Code: "agent_blocked", Message: "agent worker is waiting on a startup prompt", Phase: "agent.wait"}}
		s := renderHuman(t, out)
		for _, want := range []string{"fledge agent read --pane w1:p4", "fledge agent send --pane w1:p4 --key <key>"} {
			if !strings.Contains(s, want) {
				t.Fatalf("requested=%v: %q missing %q", requested, s, want)
			}
		}
		if strings.Contains(s, "herdr ") {
			t.Fatalf("raw herdr hint: %q", s)
		}
		const notSubmitted = "The first prompt was not submitted"
		const resend = "fledge agent message --pane w1:p4 --file <brief>"
		if requested != strings.Contains(s, notSubmitted) || requested != strings.Contains(s, resend) {
			t.Fatalf("requested=%v: %q", requested, s)
		}
	}
}

func TestStartupNotConfirmedHintsUseFledgeByPane(t *testing.T) {
	pid := "w1:p4"
	for _, phase := range []string{"agent.wait", "agent.start"} {
		for _, requested := range []bool{true, false} {
			out := libagent.Outcome{Status: "partial", Result: &Result{Name: "worker", PaneID: &pid, PromptRequested: requested}, Error: &libagent.Failure{Code: "timeout", Message: "timed out", Phase: phase}}
			s := renderHuman(t, out)
			if !strings.Contains(s, "fledge agent get --pane w1:p4") || !strings.Contains(s, "fledge agent read --pane w1:p4") || strings.Contains(s, "herdr ") {
				t.Fatalf("%s requested=%v: %q", phase, requested, s)
			}
			if requested != strings.Contains(s, "The first prompt was not submitted") {
				t.Fatalf("%s requested=%v: %q", phase, requested, s)
			}
		}
	}
}

// An agent.prompt failure may have reached the agent, so it is never called unsubmitted.
func TestPromptFailureNotCalledUnsubmitted(t *testing.T) {
	pid := "w1:p4"
	for _, status := range []string{"unknown", "partial"} {
		out := libagent.Outcome{Status: status, Result: &Result{Name: "worker", PaneID: &pid, PromptRequested: true}, Error: &libagent.Failure{Code: "transport_error", Message: "lost", Phase: "agent.prompt"}}
		if s := renderHuman(t, out); strings.Contains(s, "not submitted") || strings.Contains(s, "not delivered") {
			t.Fatalf("%q", s)
		}
	}
}
