package show

import (
	"bytes"
	"strings"
	"testing"

	internalprofile "fledge/internal/profile"
)

func TestShowPrintsMetadataAndExactInstructions(t *testing.T) {
	configured := internalprofile.Profile{
		Name:         internalprofile.OrchestratorName,
		Description:  "Delegates and verifies work.",
		Instructions: "first line\nsecond line\n",
		Defaults: internalprofile.Defaults{
			Harness: "codex",
			Model:   "gpt-5.6-sol",
			Args:    []string{"--effort", "high"},
		},
	}
	command := newCommand(func(name string) (internalprofile.Profile, bool) {
		if name != internalprofile.OrchestratorName {
			t.Fatalf("get name = %q, want %q", name, internalprofile.OrchestratorName)
		}
		return configured, true
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{internalprofile.OrchestratorName})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	wantPrefix := "Name: fledge-orchestrator\n" +
		"Description: Delegates and verifies work.\n" +
		"Default harness: codex\n" +
		"Default model: gpt-5.6-sol\n" +
		"Default arguments: --effort high\n\n" +
		"Instructions:\n"
	if output.String() != wantPrefix+configured.Instructions {
		t.Fatalf("output =\n%q\nwant\n%q", output.String(), wantPrefix+configured.Instructions)
	}
	if got := strings.TrimPrefix(output.String(), wantPrefix); got != configured.Instructions {
		t.Fatalf("instructions = %q, want exact %q", got, configured.Instructions)
	}
}

func TestShowPrintsCompleteManagedOrchestratorInstructions(t *testing.T) {
	configured, ok := internalprofile.Get(internalprofile.OrchestratorName)
	if !ok {
		t.Fatal("managed orchestrator profile is missing")
	}
	command := New()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{internalprofile.OrchestratorName})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	marker := "Instructions:\n"
	_, instructions, found := strings.Cut(output.String(), marker)
	if !found {
		t.Fatalf("output missing %q:\n%s", marker, output.String())
	}
	if instructions != configured.Instructions {
		t.Fatalf("instructions differ from managed profile:\ngot  %q\nwant %q", instructions, configured.Instructions)
	}
}

func TestShowPrintsMissingDefaults(t *testing.T) {
	command := newCommand(func(string) (internalprofile.Profile, bool) {
		return internalprofile.Profile{Name: internalprofile.OrchestratorName}, true
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{internalprofile.OrchestratorName})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Default harness: -\n", "Default model: -\n", "Default arguments: -\n"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, output.String())
		}
	}
}

func TestShowRejectsUnknownProfileAndWrongArity(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		command := newCommand(func(string) (internalprofile.Profile, bool) {
			return internalprofile.Profile{}, false
		})
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"missing"})

		err := command.Execute()
		if err == nil || !strings.Contains(err.Error(), `unknown profile "missing"`) {
			t.Fatalf("Execute() error = %v, want unknown profile error", err)
		}
	})

	for _, args := range [][]string{nil, {"one", "two"}} {
		t.Run("arity", func(t *testing.T) {
			command := newCommand(func(string) (internalprofile.Profile, bool) {
				t.Fatal("get operation called")
				return internalprofile.Profile{}, false
			})
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			command.SetArgs(args)

			if err := command.Execute(); err == nil {
				t.Fatal("Execute() error = nil, want argument error")
			}
		})
	}
}
