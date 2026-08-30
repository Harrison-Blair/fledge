package profile

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBareProfileShowsHelpWithSubcommands(t *testing.T) {
	command := New()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs(nil)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Inspect Fledge-managed agent profiles", "list", "show"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("help output missing %q:\n%s", want, output.String())
		}
	}
}

func TestProfileRejectsArgumentsBelowARoot(t *testing.T) {
	root := &cobra.Command{Use: "fledge"}
	root.AddCommand(New())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"profile", "unexpected"})

	if err := root.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want argument error")
	}
}
