package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/fledge"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/ui"
	"github.com/muesli/termenv"
)

var cliANSIPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripCLIANSI(value string) string { return cliANSIPattern.ReplaceAllString(value, "") }

func TestColorFlagModesAndShortForm(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, test := range []struct {
		name string
		args []string
		ansi bool
	}{
		{name: "auto redirected", args: []string{"version"}},
		{name: "never", args: []string{"version", "--color", "never"}},
		{name: "always overrides environment", args: []string{"version", "--color", "always"}, ansi: true},
		{name: "short always", args: []string{"version", "-c", "always"}, ansi: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Execute(context.Background(), test.args, nil, &stdout, &stderr)
			if code != 0 || stderr.Len() != 0 {
				t.Fatalf("exit=%d stderr=%q", code, stderr.String())
			}
			if got := strings.Contains(stdout.String(), "\x1b["); got != test.ansi {
				t.Fatalf("ANSI=%t want=%t output=%q", got, test.ansi, stdout.String())
			}
			if !strings.HasPrefix(stripCLIANSI(stdout.String()), "fledge v0.0.1\n") {
				t.Fatalf("plain output changed: %q", stripCLIANSI(stdout.String()))
			}
		})
	}
}

func TestInvalidColorModeIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"version", "--color", "sometimes"}, nil, &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid color mode") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestJSONDisablesColorForSuccessAndError(t *testing.T) {
	tests := []struct {
		name string
		args []string
		out  func(*bytes.Buffer, *bytes.Buffer) *bytes.Buffer
		code int
	}{
		{
			name: "success", args: []string{"version", "--json", "--color", "always"}, code: 0,
			out: func(stdout, _ *bytes.Buffer) *bytes.Buffer { return stdout },
		},
		{
			name: "error", args: []string{"agent", "prompt", "worker", "--json", "--color", "always"}, code: 2,
			out: func(_, stderr *bytes.Buffer) *bytes.Buffer { return stderr },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Execute(context.Background(), test.args, nil, &stdout, &stderr)
			if code != test.code {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			encoded := test.out(&stdout, &stderr)
			if strings.Contains(encoded.String(), "\x1b[") {
				t.Fatalf("JSON contains ANSI: %q", encoded.String())
			}
			var envelope map[string]any
			if err := json.Unmarshal(encoded.Bytes(), &envelope); err != nil {
				t.Fatalf("invalid JSON: %v (%q)", err, encoded.String())
			}
		})
	}
}

func TestHelpRemainsUnstyledWithColorAlways(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--color", "always", "--help"}, nil, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "\x1b[") || !strings.Contains(stdout.String(), "--color color") {
		t.Fatalf("unexpected help output: %q", stdout.String())
	}
}

func TestStyledTablesRetainPlainAlignment(t *testing.T) {
	agents := []fledge.StopAgentInspection{
		{Name: "alpha", Harness: "codex", State: "idle", WorkspaceID: "workspace-a", PaneID: "pane-a"},
		{Name: "long-worker", Harness: "claude", State: "blocked", WorkspaceID: "w", PaneID: "pane-b"},
	}
	var plain, styled bytes.Buffer
	printStopAgents(&plain, agents)
	printStopAgents(&styled, agents, ui.NewThemeWithProfile(&styled, termenv.TrueColor))
	if !strings.Contains(styled.String(), "\x1b[") {
		t.Fatalf("styled table contains no ANSI: %q", styled.String())
	}
	if got := stripCLIANSI(styled.String()); got != plain.String() {
		t.Fatalf("stripped table =\n%q\nplain table =\n%q", got, plain.String())
	}
	for _, line := range strings.Split(plain.String(), "\n")[1:3] {
		if !strings.Contains(line, "  ") {
			t.Fatalf("table was not column-aligned: %q", line)
		}
	}
}

func TestStyledMessageBlockLeavesBodyUntouched(t *testing.T) {
	message := &messaging.Message{
		ID: "msg_1", RunID: "run_1", CreatedAt: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		Sender: "user", Recipient: "worker", Status: messaging.StatusUncertain,
		Body: "Warning: blocked\nID: this is message content",
		DeliveryAttempts: []messaging.DeliveryAttempt{{
			Timestamp: time.Date(2026, 7, 31, 12, 1, 0, 0, time.UTC), Outcome: messaging.OutcomeFailed,
		}},
	}
	var plain, styled bytes.Buffer
	printMessageBlock(&plain, message)
	printMessageBlock(&styled, message, ui.NewThemeWithProfile(&styled, termenv.TrueColor))
	if !strings.Contains(styled.String(), "\x1b[") {
		t.Fatalf("styled message contains no ANSI: %q", styled.String())
	}
	if got := stripCLIANSI(styled.String()); got != plain.String() {
		t.Fatalf("stripped message =\n%q\nplain message =\n%q", got, plain.String())
	}
	if !strings.Contains(styled.String(), "\n\n"+message.Body+"\n") {
		t.Fatalf("message body was changed: %q", styled.String())
	}
}

func TestHumanErrorUsesErrorRole(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"agent", "prompt", "worker", "--color", "always",
	}, nil, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "\x1b[") {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if got := stripCLIANSI(stderr.String()); !strings.HasPrefix(got, "Error [usage_error]:") {
		t.Fatalf("plain error changed: %q", got)
	}
}

func TestUnknownCommandHonorsLateColorFlag(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, test := range []struct {
		name string
		args []string
		ansi bool
	}{
		{name: "always", args: []string{"nonsense", "--color", "always"}, ansi: true},
		{name: "never", args: []string{"nonsense", "--color", "never"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := Execute(context.Background(), test.args, nil, &bytes.Buffer{}, &stderr)
			if code != 2 {
				t.Fatalf("exit=%d stderr=%q", code, stderr.String())
			}
			if got := strings.Contains(stderr.String(), "\x1b["); got != test.ansi {
				t.Fatalf("ANSI=%t want=%t stderr=%q", got, test.ansi, stderr.String())
			}
		})
	}
}
