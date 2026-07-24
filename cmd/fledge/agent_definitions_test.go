package main

import (
	"encoding/json"
	"io"
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
		Version: agentcfg.IndexVersion,
		Agents:  map[string]agentcfg.AgentRecord{},
		Profiles: map[string]agentcfg.Config{
			"review-plan": {Integration: "claude", Model: "claude-opus-4", PermissionMode: "plan"},
			"haikucl":     {Integration: "claude", Model: "haiku"},
		},
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
	if len(entries) != 4 {
		t.Fatalf("entries = %+v", entries)
	}
	var found, foundForager, foundAnalyzer bool
	for _, e := range entries {
		if e.Name == "code-reviewer" {
			found = e.Profile == "review-plan" && e.Source == "user/code-reviewer/code-reviewer.agent.md" && len(e.Tools) == 2
		}
		if e.Name == "fledge-forager" {
			foundForager = e.Profile == "fledge-forager" && e.Workspace != nil && e.Workspace.Label == "fledge-context" && e.Workspace.Tab == "context"
		}
		if e.Name == "fledge-analyzer" {
			foundAnalyzer = e.Profile == "fledge-analyzer" && e.Workspace == nil
		}
	}
	if !found {
		t.Fatalf("user definition missing metadata: %+v", entries)
	}
	if !foundForager {
		t.Fatalf("managed forager missing workspace metadata: %+v", entries)
	}
	if !foundAnalyzer {
		t.Fatalf("managed analyzer missing default profile: %+v", entries)
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
	t.Setenv(protocol.CodexThreadIDEnv, "019f9131-984b-7a33-b67d-85a17555033d")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Chdir(root)

	out, err := captureRun(t, "agent", "ready", "--no-wait")
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
	if !strings.Contains(string(data), `"session_id":"019f9131-984b-7a33-b67d-85a17555033d"`) {
		t.Fatalf("fallback signal lost runtime session id: %s", data)
	}
}

func TestAgentReadyAuthenticatesBeforeWaitingForFirstMessage(t *testing.T) {
	root, _ := definitionWorkspace(t)
	t.Setenv("FLEDGE_FLOCK", "test")
	t.Setenv(protocol.AgentNameEnv, "code-reviewer-emperor")
	t.Setenv(protocol.ReadyTokenEnv, "one-use-token")
	t.Chdir(root)

	old := agentMsgRequest
	t.Cleanup(func() { agentMsgRequest = old })
	var requests []protocol.Request
	agentMsgRequest = func(_, _ string, req protocol.Request) (protocol.Response, error) {
		requests = append(requests, req)
		switch req.Op {
		case protocol.OpReady:
			return protocol.Response{Name: req.Name}, nil
		case protocol.OpReceive:
			if len(requests) != 2 || requests[0].Op != protocol.OpReady {
				t.Fatalf("wait happened before readiness: %+v", requests)
			}
			return protocol.Response{Message: &protocol.Message{
				ID: "message-1", From: "lead-emperor", To: req.As, Body: "begin",
			}}, nil
		case protocol.OpAck:
			if len(requests) != 3 || requests[1].Op != protocol.OpReceive {
				t.Fatalf("ack happened before receive: %+v", requests)
			}
			return protocol.Response{ID: req.ID}, nil
		default:
			t.Fatalf("unexpected request: %+v", req)
			return protocol.Response{}, nil
		}
	}

	out, err := captureRun(t, "agent", "ready")
	if err != nil {
		t.Fatal(err)
	}
	var got protocol.Message
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode ready output %q: %v", out, err)
	}
	if got.ID != "message-1" || got.From != "lead-emperor" || got.To != "code-reviewer-emperor" || got.Body != "begin" {
		t.Fatalf("ready output = %+v", got)
	}
	if len(requests) != 3 {
		t.Fatalf("requests = %+v, want ready, receive, then ack", requests)
	}
	wait := requests[1]
	if wait.Op != protocol.OpReceive || wait.As != "code-reviewer-emperor" || wait.Token != "one-use-token" ||
		wait.From != "" || wait.ReplyTo != "" || wait.TimeoutMS != 0 {
		t.Fatalf("wait request = %+v", wait)
	}
	ack := requests[2]
	if ack.Op != protocol.OpAck || ack.As != "code-reviewer-emperor" ||
		ack.ID != "message-1" || ack.Token != "one-use-token" {
		t.Fatalf("ack request = %+v", ack)
	}
}

func TestAgentReadyNoWaitPreservesNameOutput(t *testing.T) {
	root, _ := definitionWorkspace(t)
	t.Setenv("FLEDGE_FLOCK", "test")
	t.Setenv(protocol.AgentNameEnv, "code-reviewer-emperor")
	t.Setenv(protocol.ReadyTokenEnv, "one-use-token")
	t.Setenv(protocol.CodexThreadIDEnv, "019f9131-984b-7a33-b67d-85a17555033d")
	t.Chdir(root)

	old := agentMsgRequest
	t.Cleanup(func() { agentMsgRequest = old })
	var requests []protocol.Request
	agentMsgRequest = func(_, _ string, req protocol.Request) (protocol.Response, error) {
		requests = append(requests, req)
		return protocol.Response{Name: req.Name, InboxDelivery: "manual"}, nil
	}

	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	originalStderr := os.Stderr
	os.Stderr = stderrW
	out, err := captureRun(t, "agent", "ready", "-O")
	os.Stderr = originalStderr
	if closeErr := stderrW.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	stderr, readErr := io.ReadAll(stderrR)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if closeErr := stderrR.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "code-reviewer-emperor" {
		t.Fatalf("output = %q", out)
	}
	if !strings.Contains(string(stderr), "warning: automatic inbox delivery is unavailable") ||
		!strings.Contains(string(stderr), "fledge agent msg inbox") {
		t.Fatalf("stderr = %q", stderr)
	}
	if len(requests) != 1 || requests[0].Op != protocol.OpReady ||
		requests[0].SessionID != "019f9131-984b-7a33-b67d-85a17555033d" {
		t.Fatalf("requests = %+v, want one ready request", requests)
	}
}
