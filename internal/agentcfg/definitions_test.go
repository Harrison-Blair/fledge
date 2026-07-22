package agentcfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

func writeDefinition(t *testing.T, root, source, body string) {
	t.Helper()
	name := filepath.Join(root, scaffold.DirName, AgentsDir, filepath.FromSlash(source))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseDefinitionYAMLAndPrompt(t *testing.T) {
	d, err := ParseDefinition([]byte(`---
name: code-reviewer
description: Review changes.
tools: [read, search]
model: claude-opus-4
fledge:
  profile: opus-plan
  launch:
    permission_mode: plan
    cwd: .
    argv: [--verbose]
    env: {REVIEW: "1"}
---
Find concrete bugs first.
`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "code-reviewer" || d.Profile != "opus-plan" || d.Prompt != "Find concrete bugs first.\n" {
		t.Fatalf("definition = %+v", d)
	}
	if d.Launch.PermissionMode != "plan" || d.Launch.Env["REVIEW"] != "1" || len(d.Tools) != 2 {
		t.Fatalf("frontmatter fields = %+v", d)
	}
}

func TestParseDefinitionRejectsUnsupportedWorktree(t *testing.T) {
	_, err := ParseDefinition([]byte("---\nname: worker\ndescription: Work.\nfledge:\n  worktree: {}\n---\nPrompt.\n"))
	if err == nil || !strings.Contains(err.Error(), "worktree") {
		t.Fatalf("error = %v", err)
	}
}

func TestSynchronizeDerivesProfileAndWritesDeterministicIndex(t *testing.T) {
	root := t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	writeDefinition(t, root, "user/code-reviewer/code-reviewer.agent.md", `---
name: code-reviewer
description: Review changes.
model: claude-opus-4
fledge:
  profile: opus-plan
  launch:
    permission_mode: plan
---
Review.
`)
	if err := Synchronize(root); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(root, scaffold.DirName, FileName)
	first, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := Synchronize(root); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("index bytes changed without a source change")
	}
	var idx Index
	if err := json.Unmarshal(first, &idx); err != nil {
		t.Fatal(err)
	}
	if idx.Version != IndexVersion || idx.Profiles["opus-plan"].Integration != "claude" || idx.Profiles["opus-plan"].PermissionMode != "plan" {
		t.Fatalf("index = %+v", idx)
	}
	if idx.Agents["code-reviewer"].Source != "user/code-reviewer/code-reviewer.agent.md" || idx.Agents["code-reviewer"].PromptHash == "" {
		t.Fatalf("agent record = %+v", idx.Agents["code-reviewer"])
	}
}

func TestSynchronizeValidatesPathAndNamespaces(t *testing.T) {
	for _, tt := range []struct{ name, source, front string }{
		{"mismatched path", "user/worker/other.agent.md", "worker"},
		{"reserved user", "user/fledge-worker/fledge-worker.agent.md", "fledge-worker"},
		{"managed namespace", "fledge/worker/worker.agent.md", "worker"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if _, err := scaffold.Ensure(root); err != nil {
				t.Fatal(err)
			}
			writeDefinition(t, root, tt.source, "---\nname: "+tt.front+"\ndescription: Work.\n---\nPrompt.\n")
			if err := Synchronize(root); err == nil {
				t.Fatal("Synchronize succeeded")
			}
		})
	}
}

func TestSynchronizeRejectsConflictingProfileDeclarations(t *testing.T) {
	root := t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		mode := "plan"
		if name == "two" {
			mode = "acceptEdits"
		}
		writeDefinition(t, root, "user/"+name+"/"+name+".agent.md", "---\nname: "+name+"\ndescription: Work.\nmodel: claude-opus-4\nfledge:\n  profile: shared\n  launch:\n    permission_mode: "+mode+"\n---\nPrompt.\n")
	}
	if err := Synchronize(root); err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("error = %v", err)
	}
}
