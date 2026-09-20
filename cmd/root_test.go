package cmd

import (
	"bytes"
	"strings"
	"testing"

	internalversion "github.com/Harrison-Blair/fledge/internal/version"
)

func TestHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}} {
		var out bytes.Buffer
		if err := ExecuteWithArgs(args, &out); err != nil {
			t.Fatalf("ExecuteWithArgs(%v): %v", args, err)
		}
		if !strings.Contains(out.String(), "fledge") || !strings.Contains(out.String(), "Usage:") {
			t.Fatalf("expected help, got %q", out.String())
		}
	}
}

func TestInvalidInput(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"--unknown"}, {"version"}, {"-v"}} {
		var out bytes.Buffer
		if err := ExecuteWithArgs(args, &out); err == nil {
			t.Fatalf("ExecuteWithArgs(%v) succeeded", args)
		}
	}
}

func TestVersion(t *testing.T) {
	for _, flag := range []string{"--version", "-V"} {
		var out bytes.Buffer
		if err := ExecuteWithArgs([]string{flag}, &out); err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), "fledge "+internalversion.Version()+"\n"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestFreshCommands(t *testing.T) {
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := ExecuteWithArgs([]string{"--unknown"}, &out); err == nil {
		t.Fatal("help flag state leaked into a subsequent execution")
	}
}

func TestUpdateCommand(t *testing.T) {
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"update", "--help"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--check", "--yes"} {
		if !strings.Contains(out.String(), flag) {
			t.Errorf("missing %s in %q", flag, out.String())
		}
	}
	for _, args := range [][]string{{"update", "extra"}, {"update", "--unknown"}} {
		if err := ExecuteWithArgs(args, &out); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
