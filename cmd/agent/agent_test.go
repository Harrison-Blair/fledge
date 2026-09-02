package agent

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBareAgentShowsHelpWithSubcommands(t *testing.T) {
	command := New()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs(nil)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Spawn and drive Herder agents", "spawn", "message", "list", "stop"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("help output missing %q:\n%s", want, output.String())
		}
	}
}

func TestAgentRejectsArgumentsBelowARoot(t *testing.T) {
	root := &cobra.Command{Use: "fledge"}
	root.AddCommand(New())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"agent", "unexpected"})

	if err := root.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want argument error")
	}
}
