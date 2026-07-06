package spec

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantFM   string
		wantBody string
		wantErr  bool
	}{
		{
			name:     "basic",
			in:       "---\nid: REQ-001\n---\n\n# REQ-001: X\n",
			wantFM:   "id: REQ-001\n",
			wantBody: "\n# REQ-001: X\n",
		},
		{
			name:     "crlf",
			in:       "---\r\nid: REQ-001\r\n---\r\nbody\r\n",
			wantFM:   "id: REQ-001\r\n",
			wantBody: "body\r\n",
		},
		{
			name:    "no leading delimiter",
			in:      "id: REQ-001\n---\n",
			wantErr: true,
		},
		{
			name:    "unterminated",
			in:      "---\nid: REQ-001\n",
			wantErr: true,
		},
		{
			name:     "closing delimiter at EOF without trailing newline",
			in:       "---\nid: REQ-001\n---",
			wantFM:   "id: REQ-001\n",
			wantBody: "",
		},
		{
			name:     "body containing --- line survives untouched",
			in:       "---\nid: REQ-001\n---\nabove\n---\nbelow\n",
			wantFM:   "id: REQ-001\n",
			wantBody: "above\n---\nbelow\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body, err := SplitFrontmatter([]byte(tt.in))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got fm=%q body=%q", fm, body)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(fm) != tt.wantFM {
				t.Errorf("fm = %q, want %q", fm, tt.wantFM)
			}
			if string(body) != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestParseTaskFile(t *testing.T) {
	in := `---
id: TASK-003
title: "Wire graph: waves"
requirement: REQ-001
status: blocked
priority: P1
depends_on: [TASK-001, TASK-002]
oversight: merge
authored: 2026-07-06T12:00:00Z
agent: fledge-orchestrate/planning
fledge_version: 0.1.0
extra_key: surprise
---

## Description
body text
`
	task, unknown, err := ParseTaskFile("spec/tasks/TASK-003-wire-graph.md", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "TASK-003" || task.Title != "Wire graph: waves" ||
		task.Requirement != "REQ-001" || task.Status != "blocked" ||
		task.Priority != "P1" || task.Oversight != "merge" ||
		task.Authored != "2026-07-06T12:00:00Z" ||
		task.Agent != "fledge-orchestrate/planning" || task.FledgeVersion != "0.1.0" {
		t.Errorf("parsed task fields wrong: %+v", task)
	}
	if len(task.DependsOn) != 2 || task.DependsOn[0] != "TASK-001" || task.DependsOn[1] != "TASK-002" {
		t.Errorf("depends_on = %v", task.DependsOn)
	}
	if len(unknown) != 1 || unknown[0] != "extra_key" {
		t.Errorf("unknown = %v, want [extra_key]", unknown)
	}
	if !bytes.Contains(task.Body, []byte("## Description")) {
		t.Errorf("body not preserved: %q", task.Body)
	}
}

func TestParseRequirementFile(t *testing.T) {
	in := `---
id: REQ-001
title: Deterministic CLI
status: draft
priority: P0
authored: 2026-07-06T12:00:00Z
agent: fledge-orchestrate/planning
fledge_version: 0.1.0
---
## Context
`
	req, unknown, err := ParseRequirementFile("spec/requirements/REQ-001-deterministic-cli.md", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if req.ID != "REQ-001" || req.Title != "Deterministic CLI" || req.Status != "draft" || req.Priority != "P0" {
		t.Errorf("parsed req fields wrong: %+v", req)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %v, want none", unknown)
	}
}

// A task round-trips: parse → emit frontmatter → reparse gives identical
// fields, and the body bytes are byte-identical.
func TestTaskRoundTrip(t *testing.T) {
	in := `---
id: TASK-001
title: plain title
requirement: REQ-001
status: ready
priority: P2
depends_on: []
authored: 2026-07-06T12:00:00Z
agent: fledge-orchestrate/planning
fledge_version: 0.1.0
---

## Description

Weird body bytes: trailing spaces
	tabs, unicode ✓, and a fake
---
frontmatter fence.
`
	task, _, err := ParseTaskFile("TASK-001-plain-title.md", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	out := task.Render()
	task2, _, err := ParseTaskFile("TASK-001-plain-title.md", out)
	if err != nil {
		t.Fatalf("reparse: %v\nrendered:\n%s", err, out)
	}
	if !bytes.Equal(task.Body, task2.Body) {
		t.Errorf("body not byte-preserved:\n%q\nvs\n%q", task.Body, task2.Body)
	}
	if task2.ID != task.ID || task2.Title != task.Title || task2.Status != task.Status ||
		task2.Oversight != task.Oversight || len(task2.DependsOn) != 0 {
		t.Errorf("fields drifted: %+v vs %+v", task, task2)
	}
}

// Oversight is omitted from emitted frontmatter when empty, present when set.
func TestTaskFrontmatterOversightOptional(t *testing.T) {
	task := &Task{
		ID: "TASK-001", Title: "x", Requirement: "REQ-001", Status: "ready",
		Priority: "P1", Authored: "2026-07-06T12:00:00Z",
		Agent: "a", FledgeVersion: "0.1.0",
	}
	fm := task.Frontmatter()
	if bytes.Contains(fm, []byte("oversight")) {
		t.Errorf("empty oversight should be omitted:\n%s", fm)
	}
	task.Oversight = "during"
	fm = task.Frontmatter()
	if !bytes.Contains(fm, []byte("oversight: during")) {
		t.Errorf("oversight missing:\n%s", fm)
	}
	if !bytes.Contains(fm, []byte("depends_on: []")) {
		t.Errorf("empty depends_on should emit []:\n%s", fm)
	}
}

// Titles needing quoting are quoted; reparse yields same title.
func TestFrontmatterTitleQuoting(t *testing.T) {
	for _, title := range []string{
		"plain",
		"has: colon",
		"has # hash",
		"\"quoted\"",
		"trailing space ",
	} {
		task := &Task{
			ID: "TASK-001", Title: title, Requirement: "REQ-001", Status: "ready",
			Priority: "P1", Authored: "2026-07-06T12:00:00Z", Agent: "a", FledgeVersion: "0.1.0",
			Body: []byte("b\n"),
		}
		task2, _, err := ParseTaskFile("TASK-001-x.md", task.Render())
		if err != nil {
			t.Errorf("title %q: reparse failed: %v\n%s", title, err, task.Render())
			continue
		}
		if task2.Title != title {
			t.Errorf("title %q round-tripped to %q", title, task2.Title)
		}
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.md")
	if err := WriteFileAtomic(path, []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("v2")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "v2" {
		t.Errorf("got %q", b)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("leftover temp files: %v", entries)
	}
}
