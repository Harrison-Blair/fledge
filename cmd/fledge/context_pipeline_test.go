package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/contextdoc"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

func TestAgentSpawnMapsWorkspaceAndTabSelectors(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "flock1")

	old := agentSpawnRequest
	t.Cleanup(func() { agentSpawnRequest = old })
	var got protocol.Request
	agentSpawnRequest = func(_ string, _ string, request protocol.Request) (protocol.Response, error) {
		got = request
		return protocol.Response{Name: "analyzer-emperor"}, nil
	}

	if _, err := captureRun(t, "agent", "spawn", "--profile", "haikucl",
		"--workspace", "fledge-context", "--tab", "analysis-1"); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if got.Workspace != "fledge-context" || got.Tab != "analysis-1" {
		t.Fatalf("spawn request selectors = workspace %q tab %q", got.Workspace, got.Tab)
	}

	if _, err := captureRun(t, "agent", "spawn", "--profile", "haikucl",
		"-W", "w9", "-B", "w9:t2"); err != nil {
		t.Fatalf("spawn with short selectors: %v", err)
	}
	if got.Workspace != "w9" || got.Tab != "w9:t2" {
		t.Fatalf("short spawn request selectors = workspace %q tab %q", got.Workspace, got.Tab)
	}
}

func TestAgentSpawnRequiresBothPlacementSelectors(t *testing.T) {
	for _, args := range [][]string{
		{"agent", "spawn", "--profile", "haikucl", "--workspace", "w1"},
		{"agent", "spawn", "--profile", "haikucl", "--tab", "analysis-1"},
	} {
		_, err := captureRun(t, args...)
		if err == nil || !strings.Contains(err.Error(), "--workspace and --tab must be used together") {
			t.Errorf("run(%q) err = %v", args, err)
		}
	}
}

func TestAgentMsgSendBodyFileAndStdin(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "flock1")
	t.Setenv(protocol.AgentNameEnv, "sender-emperor")
	t.Setenv(protocol.ReadyTokenEnv, "launch-token")

	old := agentMsgRequest
	t.Cleanup(func() { agentMsgRequest = old })
	var sent protocol.Request
	agentMsgRequest = func(_ string, _ string, request protocol.Request) (protocol.Response, error) {
		switch request.Op {
		case protocol.OpList:
			return protocol.Response{Agents: []protocol.Agent{{Name: "sender-emperor"}}}, nil
		case protocol.OpSend:
			sent = request
			return protocol.Response{ID: "message-1"}, nil
		default:
			t.Fatalf("unexpected op %q", request.Op)
			return protocol.Response{}, nil
		}
	}

	bodyPath := filepath.Join(t.TempDir(), "body.json")
	fileBody := "{\n  \"exact\": true\n}\n"
	if err := os.WriteFile(bodyPath, []byte(fileBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "agent", "msg", "send", "recipient",
		"--body-file", bodyPath, "--reply-to", "original"); err != nil {
		t.Fatalf("body file send: %v", err)
	}
	if sent.Body != fileBody || sent.ReplyTo != "original" || sent.To != "recipient" || sent.Token != "launch-token" {
		t.Fatalf("file send request = %+v", sent)
	}

	withStdin(t, "stdin body\n")
	if _, err := captureRun(t, "agent", "msg", "send", "recipient", "-F", "-"); err != nil {
		t.Fatalf("stdin body send: %v", err)
	}
	if sent.Body != "stdin body\n" {
		t.Fatalf("stdin body = %q", sent.Body)
	}
}

func TestAgentMsgReplySendsOnlyIdentityReferenceAndBody(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "flock1")
	t.Setenv(protocol.AgentNameEnv, "receiver-emperor")
	t.Setenv(protocol.AgentCredentialEnv, "durable-credential")

	old := agentMsgRequest
	t.Cleanup(func() { agentMsgRequest = old })
	var replied protocol.Request
	agentMsgRequest = func(_ string, _ string, request protocol.Request) (protocol.Response, error) {
		replied = request
		return protocol.Response{ID: "reply-1"}, nil
	}
	out, err := captureRun(t, "agent", "msg", "reply", "inbound-1", "answer")
	if err != nil {
		t.Fatal(err)
	}
	if out != "reply-1\n" || replied.Op != protocol.OpReply ||
		replied.From != "receiver-emperor" || replied.ID != "inbound-1" ||
		replied.Body != "answer" || replied.To != "" || replied.ReplyTo != "" ||
		replied.Credential != "durable-credential" {
		t.Fatalf("reply output/request = %q %+v", out, replied)
	}
}

func TestAgentMsgWaitSenderConstraintAndCredential(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "flock1")
	t.Setenv(protocol.AgentNameEnv, "sender-emperor")
	t.Setenv(protocol.ReadyTokenEnv, "launch-token")

	old := agentMsgRequest
	t.Cleanup(func() { agentMsgRequest = old })
	var waited, acked protocol.Request
	agentMsgRequest = func(_ string, _ string, request protocol.Request) (protocol.Response, error) {
		switch request.Op {
		case protocol.OpReceive:
			waited = request
			return protocol.Response{Message: &protocol.Message{
				ID: "reply-1", From: "analyzer-king", To: "sender-emperor", ReplyTo: "dispatch-1",
			}}, nil
		case protocol.OpAck:
			acked = request
			return protocol.Response{ID: request.ID}, nil
		default:
			t.Fatalf("unexpected op %q", request.Op)
			return protocol.Response{}, nil
		}
	}

	if _, err := captureRun(t, "agent", "msg", "wait",
		"--from", "analyzer-king", "--reply-to", "dispatch-1", "--timeout", "1s"); err != nil {
		t.Fatal(err)
	}
	if waited.Op != protocol.OpReceive || waited.As != "sender-emperor" ||
		waited.From != "analyzer-king" || waited.ReplyTo != "dispatch-1" ||
		waited.Token != "launch-token" {
		t.Fatalf("wait request = %+v", waited)
	}
	if acked.Op != protocol.OpAck || acked.As != "sender-emperor" ||
		acked.ID != "reply-1" || acked.Token != "launch-token" {
		t.Fatalf("ack request = %+v", acked)
	}
}

func TestAgentMsgInboxClaimsWithoutWaiting(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "flock1")
	t.Setenv(protocol.AgentNameEnv, "receiver-emperor")
	t.Setenv(protocol.ReadyTokenEnv, "launch-token")

	old := agentMsgRequest
	t.Cleanup(func() { agentMsgRequest = old })
	var checked, acked protocol.Request
	agentMsgRequest = func(_ string, _ string, request protocol.Request) (protocol.Response, error) {
		switch request.Op {
		case protocol.OpPeek:
			checked = request
			return protocol.Response{Message: &protocol.Message{
				ID: "message-1", From: "sender-king", To: "receiver-emperor",
			}}, nil
		case protocol.OpAck:
			acked = request
			return protocol.Response{ID: request.ID}, nil
		default:
			t.Fatalf("unexpected op %q", request.Op)
			return protocol.Response{}, nil
		}
	}

	out, err := captureRun(t, "agent", "msg", "inbox", "--from", "sender-king")
	if err != nil {
		t.Fatal(err)
	}
	if checked.Op != protocol.OpPeek || checked.As != "receiver-emperor" ||
		checked.From != "sender-king" || checked.Token != "launch-token" {
		t.Fatalf("inbox request = %+v", checked)
	}
	if acked.Op != protocol.OpAck || acked.As != "receiver-emperor" ||
		acked.ID != "message-1" || acked.Token != "launch-token" {
		t.Fatalf("ack request = %+v", acked)
	}
	if !strings.Contains(out, `"id":"message-1"`) {
		t.Fatalf("inbox output = %q", out)
	}

	agentMsgRequest = func(_ string, _ string, request protocol.Request) (protocol.Response, error) {
		return protocol.Response{}, nil
	}
	out, err = captureRun(t, "agent", "msg", "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if out != "null\n" {
		t.Fatalf("empty inbox output = %q, want null", out)
	}
}

func TestAgentMsgSendBodyExclusivityAndMalformedFlags(t *testing.T) {
	for _, args := range [][]string{
		{"agent", "msg", "send", "recipient"},
		{"agent", "msg", "send", "recipient", "body", "--body-file", "body.txt"},
		{"agent", "msg", "send", "recipient", "--body-file"},
		{"agent", "msg", "send", "recipient", "--unknown"},
	} {
		_, err := captureRun(t, args...)
		if err == nil || !strings.Contains(err.Error(), helpPages["agent msg send"]) {
			t.Errorf("run(%q) err = %v, want contextual usage error", args, err)
		}
	}
}

func TestContextValidateAnalyzerRequestFileAndStdin(t *testing.T) {
	request := `{"schema_version":1,"group_id":"cli","purpose":"CLI","total_size":1,"files":[{"path":"a.go","size":1}]}`
	name := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(name, []byte(request), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "context", "validate", "analyzer-request", name); err != nil {
		t.Fatalf("request file: %v", err)
	}
	withStdin(t, request)
	if _, err := captureRun(t, "context", "validate", "analyzer-request", "-"); err != nil {
		t.Fatalf("request stdin: %v", err)
	}
}

func TestContextComposeAnalyzerRequestInjectsTemplateInstructions(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	bare := `{"schema_version":1,"group_id":"cli","purpose":"CLI files","total_size":1,"files":[{"path":"a.go","size":1}]}`
	name := filepath.Join(root, "request.json")
	if err := os.WriteFile(name, []byte(bare), 0o644); err != nil {
		t.Fatal(err)
	}

	worksheet := ".fledge/context/runs/r1/worksheets/cli.md"
	out, err := captureRun(t, "context", "compose", "analyzer-request", "--worksheet", worksheet, name)
	if err != nil {
		t.Fatalf("compose stdout mode: %v", err)
	}
	if err := contextdoc.ValidateComposedAnalyzerRequest([]byte(out)); err != nil {
		t.Fatalf("stdout output not a composed request: %v\n%s", err, out)
	}
	if !strings.Contains(out, `file group \"cli\"`) || !strings.Contains(out, "CLI files") ||
		!strings.Contains(out, worksheet) {
		t.Fatalf("stdout output missing substituted placeholders: %s", out)
	}
	if data, err := os.ReadFile(name); err != nil || string(data) != bare {
		t.Fatalf("stdout mode modified the input file: %q, %v", data, err)
	}

	if _, err := captureRun(t, "context", "compose", "analyzer-request", name); err == nil ||
		!strings.Contains(err.Error(), "{worksheet_path}") {
		t.Fatalf("missing --worksheet err = %v", err)
	}

	if _, err := captureRun(t, "context", "compose", "analyzer-request", "--in-place", "-E", worksheet, name); err != nil {
		t.Fatalf("compose in place: %v", err)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := contextdoc.ValidateComposedAnalyzerRequest(data); err != nil {
		t.Fatalf("in-place file not a composed request: %v\n%s", err, data)
	}
	if _, err := captureRun(t, "context", "validate", "analyzer-request", name); err != nil {
		t.Fatalf("composed request failed validate: %v", err)
	}
	if _, err := captureRun(t, "context", "compose", "analyzer-request", "-A", "-E", worksheet, name); err != nil {
		t.Fatalf("recompose in place: %v", err)
	}
}

func TestContextComposeWorksheetStampsAndWrites(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	bare := `{"schema_version":1,"group_id":"cli","purpose":"CLI files","total_size":1,"files":[{"path":"a.go","size":1}]}`
	name := filepath.Join(root, "request.json")
	if err := os.WriteFile(name, []byte(bare), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "context", "compose", "worksheet", name)
	if err != nil {
		t.Fatalf("worksheet stdout mode: %v", err)
	}
	for _, want := range []string{"group cli", "CLI files", "- [ ] `a.go` (1 bytes)"} {
		if !strings.Contains(out, want) {
			t.Errorf("worksheet output missing %q:\n%s", want, out)
		}
	}

	outName := filepath.Join(root, "runs", "r1", "worksheets", "cli.md")
	if _, err := captureRun(t, "context", "compose", "worksheet", "--output", outName, name); err != nil {
		t.Fatalf("worksheet output mode: %v", err)
	}
	data, err := os.ReadFile(outName)
	if err != nil {
		t.Fatalf("worksheet file missing: %v", err)
	}
	if string(data) != out {
		t.Errorf("written worksheet differs from stdout output")
	}

	templateName := filepath.Join(root, scaffold.DirName, "context", "templates", "analyzer-worksheet.md")
	if err := os.Remove(templateName); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "context", "compose", "worksheet", name); err == nil ||
		!strings.Contains(err.Error(), "fledge init") {
		t.Fatalf("missing template err = %v", err)
	}

	for _, args := range [][]string{
		{"context", "compose", "worksheet"},
		{"context", "compose", "worksheet", name, "extra"},
		{"context", "compose", "worksheet", "--unknown", name},
	} {
		_, err := captureRun(t, args...)
		if err == nil || !strings.Contains(err.Error(), helpPages["context compose worksheet"]) {
			t.Errorf("run(%q) err = %v, want contextual usage error", args, err)
		}
	}
}

func TestContextComposeAnalyzerRequestFailsLoud(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	bare := `{"schema_version":1,"group_id":"cli","purpose":"CLI files","total_size":1,"files":[{"path":"a.go","size":1}]}`
	name := filepath.Join(root, "request.json")
	if err := os.WriteFile(name, []byte(bare), 0o644); err != nil {
		t.Fatal(err)
	}
	templateName := filepath.Join(root, scaffold.DirName,
		"context", "templates", "analyzer-request.md")

	if err := os.WriteFile(templateName, []byte("<instructions_after>After.</instructions_after>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "context", "compose", "analyzer-request", name); err == nil ||
		!strings.Contains(err.Error(), "<instructions_before>") {
		t.Fatalf("missing tag err = %v", err)
	}

	if err := os.Remove(templateName); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "context", "compose", "analyzer-request", name); err == nil ||
		!strings.Contains(err.Error(), "fledge init") {
		t.Fatalf("missing template err = %v", err)
	}

	for _, args := range [][]string{
		{"context", "compose", "analyzer-request"},
		{"context", "compose", "analyzer-request", name, "extra"},
		{"context", "compose", "analyzer-request", "--unknown", name},
	} {
		_, err := captureRun(t, args...)
		if err == nil || !strings.Contains(err.Error(), helpPages["context compose analyzer-request"]) {
			t.Errorf("run(%q) err = %v, want contextual usage error", args, err)
		}
	}
}

func TestContextValidateAnalyzerReplyParsingAndCorrelation(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "request.json")
	replyPath := filepath.Join(dir, "reply.json")
	request := `{"schema_version":1,"group_id":"cli","purpose":"CLI","total_size":1,"files":[{"path":"a.go","size":1}]}`
	reply := `{"schema_version":1,"status":"error","group_id":"cli","errors":[{"code":"read","message":"failed"}]}`
	if err := os.WriteFile(requestPath, []byte(request), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replyPath, []byte(reply), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "context", "validate", "analyzer-reply",
		"--request", requestPath, replyPath); err != nil {
		t.Fatalf("reply file: %v", err)
	}
	withStdin(t, reply)
	if _, err := captureRun(t, "context", "validate", "analyzer-reply",
		"-Q", requestPath); err != nil {
		t.Fatalf("reply stdin: %v", err)
	}

	for _, args := range [][]string{
		{"context", "validate", "analyzer-reply", replyPath},
		{"context", "validate", "analyzer-reply", "--request"},
		{"context", "validate", "analyzer-reply", "--request", requestPath, replyPath, "extra"},
	} {
		_, err := captureRun(t, args...)
		if err == nil || !strings.Contains(err.Error(), helpPages["context validate analyzer-reply"]) {
			t.Errorf("run(%q) err = %v, want contextual usage error", args, err)
		}
	}
}

func TestContextRenderProjectWiringAndErrors(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	runDir := filepath.Join(root, scaffold.DirName, "context", "runs", "empty")
	for _, dir := range []string{
		runDir,
		filepath.Join(runDir, "requests"),
		filepath.Join(runDir, "replies"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"scan.json":       `{"schema_version":1,"root":` + strconv.Quote(root) + `,"file_count":0,"total_size":0,"files":[]}`,
		"synthesis.json":  `{"schema_version":1,"project_overview":"Empty project.","routing":[],"cross_group_flows":[],"global_invariants":[]}`,
		"provenance.json": `{"schema_version":1,"forager":{"name":"forager-emperor","profile":"opus","model":"claude-opus"},"analyzers":[]}`,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := captureRun(t, "context", "render-project", runDir)
	if err != nil {
		t.Fatalf("render-project: %v", err)
	}
	var result contextdoc.RenderResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("render result %q: %v", out, err)
	}
	if result.Path != ".fledge/context/project.md" || len(result.SHA256) != 64 || len(result.Warnings) != 0 {
		t.Fatalf("render result = %+v", result)
	}
	if !strings.Contains(out, `"warnings": []`) {
		t.Fatalf("render result warnings must be an array: %s", out)
	}
	project := filepath.Join(root, scaffold.DirName, "context", "project.md")
	if data, err := os.ReadFile(project); err != nil || !strings.Contains(string(data), "Empty project.") {
		t.Fatalf("project.md = %q, %v", data, err)
	}
	if _, err := captureRun(t, "context", "render-project", filepath.Join(root, "missing")); err == nil {
		t.Fatal("render-project missing directory succeeded")
	}
	if _, err := captureRun(t, "context", "render-project"); err == nil ||
		!strings.Contains(err.Error(), helpPages["context render-project"]) {
		t.Fatalf("render-project missing arg err = %v", err)
	}
}

func TestContextScanJSONIsUnchangedRenderInput(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	scanJSON, err := captureRun(t, "context", "scan", "--json")
	if err != nil {
		t.Fatalf("context scan: %v", err)
	}
	var scanDoc contextdoc.Scan
	if err := json.Unmarshal([]byte(scanJSON), &scanDoc); err != nil {
		t.Fatal(err)
	}

	runDir := filepath.Join(root, scaffold.DirName, "context", "runs", "unchanged-scan")
	for _, dir := range []string{
		filepath.Join(runDir, "requests"),
		filepath.Join(runDir, "replies"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(runDir, "scan.json"), []byte(scanJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	request := contextdoc.AnalyzerRequest{
		SchemaVersion: 1,
		GroupID:       "all",
		Purpose:       "Own all visible files.",
		TotalSize:     scanDoc.TotalSize,
		Files:         scanDoc.Files,
	}
	writeContextJSON(t, filepath.Join(runDir, "requests", "all.json"), request)
	summaries := make([]contextdoc.FileSummary, len(scanDoc.Files))
	for i, file := range scanDoc.Files {
		summaries[i] = contextdoc.FileSummary{Path: file.Path, ContentKind: "text", Summary: "Visible file."}
	}
	writeContextJSON(t, filepath.Join(runDir, "replies", "all.json"), contextdoc.AnalyzerSuccessReply{
		SchemaVersion:    1,
		Status:           "ok",
		GroupID:          "all",
		SubsystemSummary: "All visible files.",
		EntryPoints:      []contextdoc.EntryPoint{},
		KeySymbols:       []contextdoc.KeySymbol{},
		Dependencies: contextdoc.Dependencies{
			Internal: []contextdoc.InternalDependency{},
			External: []contextdoc.ExternalDependency{},
		},
		DataFlows:  []contextdoc.DataFlow{},
		Invariants: []string{},
		Tests:      []contextdoc.TestReference{},
		Files:      summaries,
	})
	writeContextJSON(t, filepath.Join(runDir, "synthesis.json"), contextdoc.Synthesis{
		SchemaVersion:    1,
		ProjectOverview:  "Scanned project.",
		Routing:          []contextdoc.Routing{{PathPrefix: ".", GroupID: "all", Guidance: "Route all files."}},
		CrossGroupFlows:  []contextdoc.CrossGroupFlow{},
		GlobalInvariants: []string{},
	})
	writeContextJSON(t, filepath.Join(runDir, "provenance.json"), contextdoc.Provenance{
		SchemaVersion: 1,
		Forager:       contextdoc.Identity{Name: "forager", Profile: "test", Model: "test"},
		Analyzers: []contextdoc.AnalyzerIdentity{
			{GroupID: "all", Name: "analyzer", Profile: "test", Model: "test"},
		},
	})
	if _, err := captureRun(t, "context", "render-project", runDir); err != nil {
		t.Fatalf("unchanged scan render: %v", err)
	}
}

func writeContextJSON(t *testing.T, name string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
