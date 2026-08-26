package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	internalversion "fledge/internal/version"
)

func TestBareRootShowsHelp(t *testing.T) {
	command := New()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs(nil)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Manage project-local Herder sessions", "agent", "init", "start", "stop"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("help output missing %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "toggle") || strings.Contains(output.String(), "Cobra is a CLI library") {
		t.Fatalf("help retains scaffold content:\n%s", output.String())
	}
}

func TestRootRejectsArguments(t *testing.T) {
	command := New()
	command.SetArgs([]string{"unexpected"})

	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want argument error")
	}
}

func TestRootHelpAndVersionShortCircuit(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "help", args: []string{"--help"}, want: "Usage:"},
		{name: "long version", args: []string{"--version"}, want: fmt.Sprintf("fledge version %s\n", internalversion.Version())},
		{name: "short version", args: []string{"-V"}, want: fmt.Sprintf("fledge version %s\n", internalversion.Version())},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := New()
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !strings.Contains(output.String(), test.want) {
				t.Fatalf("output = %q, want it to contain %q", output.String(), test.want)
			}
		})
	}
}

func TestFreshCommandTreesDoNotShareFlagState(t *testing.T) {
	first := New()
	first.SetOut(&bytes.Buffer{})
	first.SetArgs([]string{"--help"})
	if err := first.Execute(); err != nil {
		t.Fatal(err)
	}

	second := New()
	var output bytes.Buffer
	second.SetOut(&output)
	second.SetArgs([]string{"-V"})
	if err := second.Execute(); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("fledge version %s\n", internalversion.Version()); output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}
