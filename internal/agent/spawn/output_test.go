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
	if !strings.Contains(b.String(), `"prompted":true`) || !strings.Contains(b.String(), `"message_id":null,"sender":null`) {
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
	)
}
