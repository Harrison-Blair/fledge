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
