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
func TestPartialHintDistinguishesBlockedFromTimeout(t *testing.T) {
	for _, tc := range []struct {
		name, code, status, want string
	}{
		{"blocked", "agent_blocked", "partial", "Agent is waiting on a startup prompt"},
		{"timeout", "timeout", "partial", "Startup was not confirmed"},
		{"unknown", "transport_error", "unknown", "Startup was not confirmed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := Outcome{Status: tc.status, Result: &SpawnResult{Name: "worker"}, Error: &Failure{Code: tc.code, Message: "failure", Phase: "agent.wait"}}
			var b bytes.Buffer
			if err := out.Write(&b, false); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(b.String(), tc.want) {
				t.Fatalf("%q missing %q", b.String(), tc.want)
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
