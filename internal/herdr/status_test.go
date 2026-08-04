package herdr

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// captureFile configures the helper binary to record its invocation and
// returns the path that recording is written to.
func captureFile(t *testing.T) string {
	t.Helper()

	capture := filepath.Join(t.TempDir(), "invocation.json")
	configureHelper(t, capture)
	return capture
}

func TestClientProtocol(t *testing.T) {
	capture := captureFile(t)
	t.Setenv(helperStdoutEnv, `{"client":{"version":"0.8.0","channel":"stable","protocol":19,"binary":"/usr/bin/herdr"},"server":{"running":false}}`)

	protocol, err := NewClient(helperBinary(t), nil, nil, nil).Protocol(context.Background())
	if err != nil {
		t.Fatalf("Protocol() error = %v", err)
	}
	if protocol != 19 {
		t.Fatalf("Protocol() = %d, want 19", protocol)
	}
	assertStrings(t, "args", readInvocation(t, capture).Args, []string{"status", "--json"})
}

func TestClientProtocolErrors(t *testing.T) {
	tests := []struct {
		name    string
		stdout  string
		wantErr string
	}{
		{name: "empty response", stdout: ``, wantErr: "empty response"},
		{name: "missing protocol", stdout: `{"client":{"version":"0.8.0"}}`, wantErr: "missing client protocol"},
		{name: "malformed", stdout: `{`, wantErr: "decode JSON response"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captureFile(t)
			t.Setenv(helperStdoutEnv, tt.stdout)

			_, err := NewClient(helperBinary(t), nil, nil, nil).Protocol(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Protocol() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestClientSnapshotDecodesAgentStatus(t *testing.T) {
	captureFile(t)
	t.Setenv(helperStdoutEnv, `{"id":"1","result":{"type":"session_snapshot","snapshot":{"panes":[{"pane_id":"w1:p1","tab_id":"t1","workspace_id":"w1","agent_status":"blocked"}],"agents":[{"name":"reviewer","agent":"claude","pane_id":"w1:p1","tab_id":"t1","workspace_id":"w1","agent_status":"working"}]}}}`)

	snapshot, err := NewClient(helperBinary(t), nil, nil, nil).Snapshot(context.Background(), "session-name")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(snapshot.Panes) != 1 || snapshot.Panes[0].AgentStatus != "blocked" {
		t.Fatalf("Snapshot() panes = %#v, want agent_status blocked", snapshot.Panes)
	}
	if len(snapshot.Agents) != 1 || snapshot.Agents[0].AgentStatus != "working" {
		t.Fatalf("Snapshot() agents = %#v, want agent_status working", snapshot.Agents)
	}
}

func TestClientListDecodesSocketPath(t *testing.T) {
	captureFile(t)
	t.Setenv(helperStdoutEnv, `{"sessions":[{"name":"fledge-demo-0a1b2c3d","running":true,"socket_path":"/home/user/.config/herdr/sessions/fledge-demo-0a1b2c3d/herdr.sock"}]}`)

	sessions, err := NewClient(helperBinary(t), nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 1 || sessions[0].SocketPath != "/home/user/.config/herdr/sessions/fledge-demo-0a1b2c3d/herdr.sock" {
		t.Fatalf("List() = %#v, want socket_path decoded", sessions)
	}
}
