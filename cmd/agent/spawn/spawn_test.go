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

// Spawn has no interactive mode and no split placement flags.
func TestSpawnHasNoInteractiveModeOrSplitFlags(t *testing.T) {
	cmd := New()
	if strings.Contains(cmd.Long, "interactive terminal") || strings.Contains(strings.ToLower(cmd.Long), "split") {
		t.Fatal(cmd.Long)
	}
	for _, name := range []string{"tab-id", "direction", "ratio"} {
		if cmd.Flags().Lookup(name) != nil {
			t.Errorf("--%s still defined", name)
		}
	}
	if tab := cmd.Flags().Lookup("tab"); tab == nil || !strings.Contains(tab.Usage, "new tab") {
		t.Fatalf("%+v", tab)
	}
}
