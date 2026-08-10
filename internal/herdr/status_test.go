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
