package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSpecs(t *testing.T, root string, files map[string]string) (reqDir, taskDir string) {
	t.Helper()
	reqDir = filepath.Join(root, "spec", "requirements")
	taskDir = filepath.Join(root, "spec", "tasks")
	for _, d := range []string{reqDir, taskDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return reqDir, taskDir
}

const validReq = `---
id: REQ-001
title: A requirement
status: approved
priority: P1
authored: 2026-07-06T12:00:00Z
agent: t
fledge_version: 0.1.0
---
## Context
`

const validTask = `---
id: TASK-001
title: A task
requirement: REQ-001
status: ready
priority: P1
depends_on: []
authored: 2026-07-06T12:00:00Z
agent: t
fledge_version: 0.1.0
---
## Tests
- something
`

func TestLoad(t *testing.T) {
	root := t.TempDir()
	reqDir, taskDir := writeSpecs(t, root, map[string]string{
		"spec/requirements/REQ-001-a-requirement.md": validReq,
		"spec/tasks/TASK-001-a-task.md":              validTask,
		"spec/tasks/TASK-002-broken.md":              "not frontmatter at all\n",
	})
	set, err := Load(reqDir, taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Reqs) != 1 || len(set.Tasks) != 1 {
		t.Errorf("got %d reqs, %d tasks; want 1, 1", len(set.Reqs), len(set.Tasks))
	}
	if len(set.Errors) != 1 {
		t.Fatalf("want 1 file error for broken file, got %v", set.Errors)
	}
	if filepath.Base(set.Errors[0].Path) != "TASK-002-broken.md" {
		t.Errorf("error attributed to %q", set.Errors[0].Path)
	}
	if set.Req("REQ-001") == nil || set.Task("TASK-001") == nil {
		t.Error("lookup by ID failed")
	}
	if set.Req("REQ-999") != nil {
		t.Error("lookup of missing ID should be nil")
	}
}

func TestLoadMissingDirs(t *testing.T) {
	root := t.TempDir()
	set, err := Load(filepath.Join(root, "nope1"), filepath.Join(root, "nope2"))
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Reqs) != 0 || len(set.Tasks) != 0 || len(set.Errors) != 0 {
		t.Errorf("want empty set, got %+v", set)
	}
}
