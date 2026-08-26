package agent

import (
	"bytes"
	"strings"
	"testing"
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

func TestAgentRejectsArguments(t *testing.T) {
	command := New()
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"unexpected"})

	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want argument error")
	}
}
