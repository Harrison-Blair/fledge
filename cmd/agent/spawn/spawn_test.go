package spawn

import (
	"bytes"
	"strings"
	"testing"
)

func TestNoFlagsWithoutTerminalKeepsValidationError(t *testing.T) {
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
