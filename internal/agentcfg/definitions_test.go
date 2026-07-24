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

func writeAnalyzerCatalog(t *testing.T, root string) {
	t.Helper()
	idx := Index{
		Version:  IndexVersion,
		Agents:   map[string]AgentRecord{},
		Profiles: map[string]Config{"haikucl": {Integration: "claude", Model: "haiku"}},
	}
	data, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, scaffold.DirName, CatalogName), data, 0o644); err != nil {
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
  workspace:
    label: fledge-context
    tab: context
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
	if d.Workspace == nil || d.Workspace.Label != "fledge-context" || d.Workspace.Tab != "context" {
		t.Fatalf("workspace = %+v", d.Workspace)
	}
}

func TestParseDefinitionValidatesWorkspace(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{"missing label", "tab: context", "workspace.label"},
		{"missing tab", "label: fledge-context", "workspace.tab"},
		{"untrimmed label", "label: ' fledge-context'\n    tab: context", "trimmed label"},
		{"multiline tab", "label: fledge-context\n    tab: \"context\\nother\"", "trimmed label"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDefinition([]byte("---\nname: worker\ndescription: Work.\nfledge:\n  workspace:\n    " + tt.body + "\n---\nPrompt.\n"))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
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
	writeAnalyzerCatalog(t, root)
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
	managedName := filepath.Join(root, scaffold.DirName, ManagedIndexName)
	firstManaged, err := os.ReadFile(managedName)
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
	secondManaged, err := os.ReadFile(managedName)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstManaged) != string(secondManaged) {
		t.Fatal("managed index bytes changed without a source change")
	}
	for _, generated := range []string{FileName, ManagedIndexName} {
		if !strings.HasPrefix(generated, "agents/fledge/") {
			t.Errorf("generated index %q is outside the managed directory", generated)
		}
	}
	for _, legacy := range []string{legacyUserIndexName, legacyManagedIndexName} {
		if _, err := os.Stat(filepath.Join(root, scaffold.DirName, AgentsDir, legacy)); !os.IsNotExist(err) {
			t.Errorf("legacy index %s exists: %v", legacy, err)
		}
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

func TestSynchronizeIndexesWorkspaceDeterministically(t *testing.T) {
	root := t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	writeAnalyzerCatalog(t, root)
	writeDefinition(t, root, "user/context-planner/context-planner.agent.md", `---
name: context-planner
description: Plan context.
fledge:
  workspace:
    label: fledge-context
    tab: context
---
Plan.
`)
	if err := Synchronize(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, scaffold.DirName, FileName))
	if err != nil {
		t.Fatal(err)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatal(err)
	}
	workspace := idx.Agents["context-planner"].Workspace
	if workspace == nil || workspace.Label != "fledge-context" || workspace.Tab != "context" {
		t.Fatalf("indexed workspace = %+v", workspace)
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

func TestSynchronizePermitsReservedProfilesOnlyFromManagedDefinitions(t *testing.T) {
	t.Run("managed", func(t *testing.T) {
		root := t.TempDir()
		if _, err := scaffold.Ensure(root); err != nil {
			t.Fatal(err)
		}
		if err := Synchronize(root); err != nil {
			t.Fatalf("managed fledge-* profiles rejected: %v", err)
		}
		configs, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		if got := configs["fledge-analyzer"]; got.Model != "claude-haiku-4-5" || got.Integration != "claude" {
			t.Fatalf("managed analyzer profile = %+v", got)
		}
		if got := configs["fledge-forager"]; got.Model != "claude-sonnet-5" || got.Integration != "claude" {
			t.Fatalf("managed forager profile = %+v", got)
		}
	})

	t.Run("user reference", func(t *testing.T) {
		root := t.TempDir()
		if _, err := scaffold.Ensure(root); err != nil {
			t.Fatal(err)
		}
		writeDefinition(t, root, "user/worker/worker.agent.md", `---
name: worker
description: Work.
fledge:
  profile: fledge-context-haiku-auto
---
Work.
`)
		err := Synchronize(root)
		if err == nil || !strings.Contains(err.Error(), "reserved fledge-* namespace") {
			t.Fatalf("Synchronize error = %v", err)
		}
	})
}

func TestMigrateLegacyGeneratedIndexesAndCatalog(t *testing.T) {
	root := t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, scaffold.DirName, AgentsDir)
	for _, canonical := range []string{FileName, ManagedIndexName} {
		if err := os.Remove(filepath.Join(root, scaffold.DirName, canonical)); err != nil {
			t.Fatal(err)
		}
	}
	indexes := map[string]Index{
		legacyUserIndexName: {
			Version: IndexVersion, Agents: map[string]AgentRecord{"worker": {Source: "user/worker/worker.agent.md"}},
			Profiles: map[string]Config{"worker": {Integration: "codex", Model: "gpt-5"}},
		},
		legacyManagedIndexName: {
			Version: IndexVersion, Agents: map[string]AgentRecord{"fledge-old": {Source: "fledge/fledge-old/fledge-old.agent.md"}},
			Profiles: map[string]Config{},
		},
		legacyCatalogName: {
			Version: IndexVersion, Agents: map[string]AgentRecord{},
			Profiles: map[string]Config{"kept": {Integration: "claude", Model: "sonnet"}},
		},
	}
	for name, idx := range indexes {
		if err := writeIndexAtomic(filepath.Join(base, name), idx); err != nil {
			t.Fatal(err)
		}
	}
	if err := MigrateLegacyGenerated(root); err != nil {
		t.Fatal(err)
	}
	for legacy := range indexes {
		if _, err := os.Stat(filepath.Join(base, legacy)); !os.IsNotExist(err) {
			t.Errorf("legacy generated file %s remains: %v", legacy, err)
		}
	}
	for _, canonical := range []string{FileName, ManagedIndexName, CatalogName} {
		if _, err := os.Stat(filepath.Join(root, scaffold.DirName, canonical)); err != nil {
			t.Errorf("canonical generated file %s: %v", canonical, err)
		}
	}
	configs, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := configs["kept"]; got.Integration != "claude" || got.Model != "sonnet" {
		t.Fatalf("migrated catalog profile = %+v", got)
	}
}

func TestMigrateLegacyCatalogReplacesInvalidCanonicalCopy(t *testing.T) {
	root := t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, scaffold.DirName, AgentsDir)
	legacy := Index{
		Version: IndexVersion, Agents: map[string]AgentRecord{},
		Profiles: map[string]Config{"last-valid": {Integration: "codex", Model: "gpt-5"}},
	}
	if err := writeIndexAtomic(filepath.Join(base, legacyCatalogName), legacy); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(root, scaffold.DirName, CatalogName)
	if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(canonical, []byte(`{"version":1,"profiles":`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyGenerated(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, legacyCatalogName)); !os.IsNotExist(err) {
		t.Fatalf("legacy catalog remains: %v", err)
	}
	idx, err := readIndex(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if got := idx.Profiles["last-valid"]; got.Integration != "codex" || got.Model != "gpt-5" {
		t.Fatalf("preserved catalog profile = %+v", got)
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
