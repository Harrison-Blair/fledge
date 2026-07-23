package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/client"
	"github.com/Harrison-Blair/fledge/internal/daemon"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

func definitionWorkspace(t *testing.T) (root, source string) {
	t.Helper()
	root = t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	catalog := agentcfg.Index{
		Version:  agentcfg.IndexVersion,
		Agents:   map[string]agentcfg.AgentRecord{},
		Profiles: map[string]agentcfg.Config{"review-plan": {Integration: "claude", Model: "claude-opus-4", PermissionMode: "plan"}},
	}
	data, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	catalogName := filepath.Join(root, scaffold.DirName, agentcfg.CatalogName)
	if err := os.WriteFile(catalogName, data, 0o644); err != nil {
		t.Fatal(err)
	}
	source = filepath.Join(root, scaffold.DirName, agentcfg.AgentsDir, agentcfg.UserDir, "code-reviewer", "code-reviewer.agent.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`---
name: code-reviewer
description: Review code for concrete defects.
tools: [read, search]
fledge:
  profile: review-plan
---
Report findings first.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, source
}

func TestAgentTypesJSONListsPortableDefinitions(t *testing.T) {
	root, _ := definitionWorkspace(t)
	t.Chdir(root)
	out, err := captureRun(t, "agent", "types", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []agentTypeEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %+v", entries)
	}
	var found, foundForager bool
	for _, e := range entries {
		if e.Name == "code-reviewer" {
			found = e.Profile == "review-plan" && e.Source == "user/code-reviewer/code-reviewer.agent.md" && len(e.Tools) == 2
		}
		if e.Name == "fledge-forager" {
			foundForager = e.Profile == "" && e.Workspace != nil && e.Workspace.Label == "fledge-context" && e.Workspace.Tab == "context"
		}
	}
	if !found {
		t.Fatalf("user definition missing metadata: %+v", entries)
	}
	if !foundForager {
		t.Fatalf("managed forager missing workspace metadata: %+v", entries)
	}
}

func TestAgentRegisterDefinitionCarriesMetadata(t *testing.T) {
	root, source := definitionWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("FLEDGE_FLOCK", "test")
	startDaemon(t, root, "test")
	t.Chdir(root)
	out, err := captureRun(t, "agent", "register", source, "--pid", strconv.Itoa(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	name := strings.TrimSpace(out)
	resp, err := client.Do(root, "test", protocol.Request{Op: protocol.OpList})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Agents) != 1 {
		t.Fatalf("agents = %+v", resp.Agents)
	}
	a := resp.Agents[0]
	if name != "code-reviewer-emperor" || a.Agent != "code-reviewer" || a.Profile != "review-plan" || a.Source != "user/code-reviewer/code-reviewer.agent.md" {
		t.Fatalf("registered = %+v, output %q", a, name)
	}
}

func TestAgentReadyRequiresInjectedEnvironment(t *testing.T) {
	t.Setenv(protocol.AgentNameEnv, "")
	t.Setenv(protocol.ReadyTokenEnv, "")
	if _, err := captureRun(t, "agent", "ready"); err == nil || !strings.Contains(err.Error(), "Fledge-started") {
		t.Fatalf("error = %v", err)
	}
}

func TestAgentReadyFallsBackToWorkspaceSignalWhenSocketIsUnavailable(t *testing.T) {
	root, _ := definitionWorkspace(t)
	t.Setenv("FLEDGE_FLOCK", "test")
	t.Setenv(protocol.AgentNameEnv, "code-reviewer-emperor")
	t.Setenv(protocol.ReadyTokenEnv, "one-use-token")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Chdir(root)

	out, err := captureRun(t, "agent", "ready")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "code-reviewer-emperor" {
		t.Fatalf("output = %q", out)
	}
	data, err := os.ReadFile(daemon.ReadySignalPath(root, "test", "code-reviewer-emperor"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "one-use-token") {
		t.Fatal("fallback signal exposed the readiness token")
	}
}
