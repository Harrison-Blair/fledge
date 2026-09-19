package agent

import (
	"bytes"
	"strings"
	"testing"
)

func TestHumanSpawnIncludesDirectories(t *testing.T) {
	cwd := "/repo/.fledge/worktrees/topic"
	out := Outcome{Status: "success", Result: &SpawnResult{Name: "worker", Harness: "claude", Cwd: &cwd, WorktreePath: &cwd}}
	var b bytes.Buffer
	if err := out.Write(&b, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "cwd: "+cwd) || !strings.Contains(b.String(), "worktree: "+cwd) {
		t.Fatal(b.String())
	}
}
func TestHumanSpawnIncludesPromptSubmission(t *testing.T) {
	pid := "w1:p1"
	out := Outcome{Status: "success", Result: &SpawnResult{Name: "worker", Harness: "claude", PaneID: &pid, Prompted: true}}
	var b bytes.Buffer
	if err := out.Write(&b, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "Message submitted to w1:p1.") {
		t.Fatal(b.String())
	}
}
func TestHumanSpawnOmitsPromptLineWithoutSubmission(t *testing.T) {
	out := Outcome{Status: "success", Result: &SpawnResult{Name: "worker", Harness: "claude"}}
	var b bytes.Buffer
	if err := out.Write(&b, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "Message submitted") {
		t.Fatal(b.String())
	}
}
func TestSpawnJSONIncludesPromptedField(t *testing.T) {
	pid := "w1:p1"
	out := Outcome{Operation: "agent.spawn", Status: "success", Effects: []Effect{}, Result: &SpawnResult{Name: "worker", Harness: "claude", PaneID: &pid, Prompted: true}}
	var b bytes.Buffer
	if err := out.Write(&b, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"prompted":true`) {
		t.Fatalf("%q", b.String())
	}
}
func TestHumanModelsTable(t *testing.T) {
	label := "Opus 5"
	out := Outcome{Status: "success", Result: ModelsResult{Models: []ModelRow{{Harness: "claude", Model: "claude-opus-5", Name: &label}, {Harness: "opencode", Model: "opencode/big-pickle"}}}}
	var b bytes.Buffer
	if err := out.Write(&b, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) != 3 || strings.Fields(lines[0])[0] != "HARNESS" || strings.Join(strings.Fields(lines[0]), " ") != "HARNESS MODEL NAME" {
		t.Fatal(b.String())
	}
	if strings.Join(strings.Fields(lines[1]), " ") != "claude claude-opus-5 Opus 5" || strings.Join(strings.Fields(lines[2]), " ") != "opencode opencode/big-pickle -" {
		t.Fatal(b.String())
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := Outcome{Status: tc.status, Result: &SpawnResult{Name: "worker"}, Error: &Failure{Code: tc.code, Message: "failure", Phase: tc.phase}}
			var b bytes.Buffer
			if err := out.Write(&b, false); err != nil {
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
func TestHumanModelsEmpty(t *testing.T) {
	out := Outcome{Status: "success", Result: ModelsResult{Models: []ModelRow{}}}
	var b bytes.Buffer
	if err := out.Write(&b, false); err != nil {
		t.Fatal(err)
	}
	if b.String() != "No models discovered.\n" {
		t.Fatalf("%q", b.String())
	}
}
