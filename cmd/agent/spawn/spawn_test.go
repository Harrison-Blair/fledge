package spawn

import (
	"bytes"
	"strings"
	"testing"
)

func TestNoFlagsWithoutTerminalKeepsValidationError(t *testing.T) {
	// Never reach a live Herdr socket, even if the terminal guard regresses.
	t.Setenv("HERDR_ENV", "")
	t.Setenv("HERDR_SOCKET_PATH", "")
	var out bytes.Buffer
	cmd := New()
	cmd.SetArgs([]string{})
	cmd.SetIn(strings.NewReader("claude\n\nworker\n\n"))
	cmd.SetOut(&out)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected validation failure")
	}
	if got, want := out.String(), "rejected: --name must match [a-z][a-z0-9_-]{0,31} (validation)\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLongHelpMentionsInteractiveMode(t *testing.T) {
	if !strings.Contains(New().Long, "interactive") {
		t.Fatal(New().Long)
	}
}
