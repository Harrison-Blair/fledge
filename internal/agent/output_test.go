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
