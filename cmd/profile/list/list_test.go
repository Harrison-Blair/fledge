package list

import (
	"bytes"
	"testing"

	internalprofile "fledge/internal/profile"
)

func TestListPrintsManagedProfilesDeterministically(t *testing.T) {
	command := newCommand(func() []internalprofile.Profile {
		return []internalprofile.Profile{
			{
				Name:        "fledge-reviewer",
				Description: "Reviews a completed unit.",
				Defaults: internalprofile.Defaults{
					Harness: "claude",
					Model:   "claude-opus-4-8",
				},
			},
			{
				Name:        internalprofile.OrchestratorName,
				Description: "Delegates and verifies work.",
			},
		}
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs(nil)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "NAME                 HARNESS  MODEL            DESCRIPTION\n" +
		"fledge-orchestrator  -        -                Delegates and verifies work.\n" +
		"fledge-reviewer      claude   claude-opus-4-8  Reviews a completed unit.\n"
	if output.String() != want {
		t.Fatalf("output =\n%q\nwant\n%q", output.String(), want)
	}
}

func TestListRejectsArguments(t *testing.T) {
	command := newCommand(func() []internalprofile.Profile {
		t.Fatal("list operation called")
		return nil
	})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"unexpected"})

	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want argument error")
	}
}
