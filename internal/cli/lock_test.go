package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// If the status rewrite fails after the lock file is created, the lock is
// rolled back so lock and frontmatter never desync.
func TestLockRollsBackOnStatusWriteFailure(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	tasksDir := filepath.Join(root, "spec", "tasks")
	for _, d := range []string{filepath.Join(root, ".fledge"), tasksDir, filepath.Join(root, "spec", "requirements")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	reqFile := `---
id: REQ-001
title: r
status: approved
priority: P1
authored: 2026-07-06T12:00:00Z
agent: t
fledge_version: 0.1.0
---
## Functional Criteria
x
## Acceptance Criteria
x
`
	taskFile := `---
id: TASK-001
title: t
requirement: REQ-001
status: ready
priority: P1
depends_on: []
authored: 2026-07-06T12:00:00Z
agent: t
fledge_version: 0.1.0
---
## Description
x
## Tests
- x
## Acceptance Criteria
x
`
	os.WriteFile(filepath.Join(root, "spec", "requirements", "REQ-001-r.md"), []byte(reqFile), 0o644)
	os.WriteFile(filepath.Join(tasksDir, "TASK-001-t.md"), []byte(taskFile), 0o644)

	// Read-only tasks dir: the atomic rewrite's temp file creation fails.
	if err := os.Chmod(tasksDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(tasksDir, 0o755) })

	t.Chdir(root)
	if code := Run([]string{"lock", "TASK-001", "--owner", "tester"}); code != ExitFail {
		t.Fatalf("lock exit = %d, want %d", code, ExitFail)
	}
	if _, err := os.Stat(filepath.Join(root, ".fledge", "locks", "TASK-001.lock")); !os.IsNotExist(err) {
		t.Error("lock file was not rolled back")
	}
}
