package daemon

import (
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/protocol"
)

func TestDedicatedWorkspaceBootstrapFailureClosesWorkspace(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"workspace.create": dedicatedWorkspaceReply,
		"agent.start":      `{"id":"1","result":{"agent":{"pane_id":"w9:p2","terminal_id":"term_x"}}}`,
		"pane.send_input":  `{"id":"1","error":{"code":"failed","message":"no input"}}`,
	})
	d := boundDaemon(t, f)
	d.skipReadiness = false
	writeDedicatedDefinition(t, d.root, "claude-opus-4")

	_, err := d.spawn(&protocol.Request{Agent: "context-planner", TimeoutMS: 1000})
	if err == nil || !strings.Contains(err.Error(), "bootstrap") {
		t.Fatalf("error = %v", err)
	}
	if close := f.params("workspace.close"); close["workspace_id"] != "w9" {
		t.Fatalf("workspace rollback = %+v", close)
	}
	if got := agentState(d, "context-planner-emperor"); got != stateStopped {
		t.Fatalf("state = %q, want stopped", got)
	}
}

func TestDedicatedWorkspaceReadinessTimeoutClosesWorkspace(t *testing.T) {
	f := serveHerdr(t, map[string]string{
		"workspace.create": dedicatedWorkspaceReply,
		"agent.start":      `{"id":"1","result":{"agent":{"pane_id":"w9:p2","terminal_id":"term_x"}}}`,
	})
	d := boundDaemon(t, f)
	d.skipReadiness = false
	writeDedicatedDefinition(t, d.root, "claude-opus-4")

	_, err := d.spawn(&protocol.Request{Agent: "context-planner", TimeoutMS: 10})
	if err == nil || !strings.Contains(err.Error(), "did not become ready") {
		t.Fatalf("error = %v", err)
	}
	if close := f.params("workspace.close"); close["workspace_id"] != "w9" {
		t.Fatalf("workspace rollback = %+v", close)
	}
	if got := agentState(d, "context-planner-emperor"); got != stateStopped {
		t.Fatalf("state = %q, want stopped", got)
	}
}
