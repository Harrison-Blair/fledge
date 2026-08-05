package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcontext"
)

func reportFixture() agentcontext.Report {
	used := 21002
	window := 200000
	percent := 10.5
	observed := time.Date(2026, 8, 4, 23, 0, 0, 0, time.UTC)
	return agentcontext.Report{
		SchemaVersion: agentcontext.SchemaVersion,
		GeneratedAt:   time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
		Agents: []agentcontext.AgentContext{
			{Name: "orchestrator", Harness: "claude", Revision: 3, Status: agentcontext.StatusAvailable, Used: &used, Window: &window, Percent: &percent, ObservedAt: &observed},
		},
	}
}

func TestAgentContextRendersTextByDefault(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{contextResult: reportFixture()}
	command := newRootCommand(manager, func() (string, error) { return "/project/nested", nil })
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"agent", "context"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(manager.contextCalls) != 1 || manager.contextCalls[0] != (contextCall{"/project/nested", ""}) {
		t.Fatalf("Context() calls = %#v", manager.contextCalls)
	}
	if !strings.Contains(output.String(), "orchestrator (claude): 21002/200000 tokens (10.50%)") {
		t.Errorf("output = %q", output.String())
	}
}

func TestAgentContextNamePassedThrough(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{contextResult: reportFixture()}
	command := newRootCommand(manager, func() (string, error) { return "/project", nil })
	command.SetArgs([]string{"agent", "context", "orchestrator"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(manager.contextCalls) != 1 || manager.contextCalls[0].name != "orchestrator" {
		t.Fatalf("Context() calls = %#v", manager.contextCalls)
	}
}

func TestAgentContextJSONIsVersionedAndValid(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{contextResult: reportFixture()}
	command := newRootCommand(manager, func() (string, error) { return "/project", nil })
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"agent", "context", "--json"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var decoded agentcontext.Report
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output.String())
	}
	if decoded.SchemaVersion != agentcontext.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", decoded.SchemaVersion, agentcontext.SchemaVersion)
	}
	if len(decoded.Agents) != 1 || decoded.Agents[0].Name != "orchestrator" {
		t.Errorf("decoded agents = %#v", decoded.Agents)
	}
}

func TestAgentContextRejectsExtraArgs(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{}
	command := newRootCommand(manager, func() (string, error) { return "/project", nil })
	command.SetArgs([]string{"agent", "context", "one", "two"})
	if err := command.Execute(); err == nil {
		t.Error("Execute() error = nil, want too-many-args rejection")
	}
	if len(manager.contextCalls) != 0 {
		t.Errorf("Context() should not be called on bad args: %#v", manager.contextCalls)
	}
}
