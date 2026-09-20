package agent

import (
	"bytes"
	"errors"
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

func TestHumanOperationResults(t *testing.T) {
	row := AgentRow{Name: pointer("worker"), Harness: pointer("claude"), AgentStatus: pointer("idle"), WorkspaceID: pointer("w1"), TabID: pointer("w1:t1"), PaneID: pointer("w1:p1"), Cwd: pointer("/repo")}
	for _, tc := range []struct {
		name   string
		result any
		want   string
	}{
		{"empty list", ListResult{}, "No live agents."},
		{"list", ListResult{Agents: []AgentRow{row, {}}}, "NAME HARNESS STATUS WORKSPACE TAB PANE CWD worker claude idle w1 w1:t1 w1:p1 /repo - - - - - - -"},
		{"message", MessageResult{AgentRow: row, Submitted: true}, "Message submitted to w1:p1."},
		{"stop", StopResult{AgentRow: row, Stopped: true}, "Stopped worker (claude) in w1:p1."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := (Outcome{Status: "success", Result: tc.result}).Write(&b, false); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(strings.Fields(b.String()), " "); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHumanFailureEffects(t *testing.T) {
	out := Outcome{Status: "partial", Error: &Failure{Message: "rename failed", Phase: "tab.rename"}, Effects: []Effect{
		{Action: "created", Kind: "pane", ID: "w2:p1"},
		{Action: "created", Kind: "worktree", Path: "/repo/topic"},
	}}
	var b bytes.Buffer
	if err := out.Write(&b, false); err != nil {
		t.Fatal(err)
	}
	want := "partial: rename failed (tab.rename)\n  created pane w2:p1\n  created worktree /repo/topic\n"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
}

// failAfterWriter checks errors on later writes as well as the initial write.
type failAfterWriter struct {
	remaining int
	err       error
	failed    bool
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		w.failed = true
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}

func TestOutputFailuresPropagate(t *testing.T) {
	outcomes := []Outcome{
		{Result: &SpawnResult{Name: "worker", Prompted: true}},
		{Result: ListResult{}},
		{Result: ListResult{Agents: []AgentRow{{Name: pointer("worker")}}}},
		{Result: GetResult{}},
		{Result: MessageResult{}},
		{Result: StopResult{}},
		{Result: ModelsResult{}},
		{Result: ModelsResult{Models: []ModelRow{{Harness: "claude", Model: "sonnet"}}}},
		{Status: "partial", Error: &Failure{Message: "failed"}, Effects: []Effect{{Action: "created", Kind: "pane", ID: "w1:p1"}}},
		{Status: "partial", Result: &SpawnResult{Name: "worker"}, Error: &Failure{Message: "blocked", Code: "agent_blocked", Phase: "agent.wait"}},
	}
	for i, out := range outcomes {
		for _, asJSON := range []bool{false, true} {
			var rendered bytes.Buffer
			if err := out.Write(&rendered, asJSON); err != nil {
				t.Fatal(err)
			}
			// Exercise each write boundary without depending on the number of writes.
			for after := 0; ; after++ {
				sentinel := errors.New("output unavailable")
				w := &failAfterWriter{remaining: after, err: sentinel}
				err := Finish(out, w, asJSON)
				var outputErr *OutputError
				if !w.failed {
					break
				}
				if !errors.As(err, &outputErr) {
					t.Fatalf("output failure lost: %v", err)
				}
				if !errors.Is(err, sentinel) || outputErr.ExitCode() != 1 || outputErr.Error() != sentinel.Error() {
					t.Fatalf("case %d json=%v write=%d: %v", i, asJSON, after, err)
				}
				if after > 100 {
					t.Fatalf("unbounded writes for case %d", i)
				}
			}
		}
	}
}
