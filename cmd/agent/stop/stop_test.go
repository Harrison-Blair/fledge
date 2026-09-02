package stop

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func TestStopPrintsClosedPane(t *testing.T) {
	command := newCommand(func(_ context.Context, target string) (string, error) {
		if target != "reviewer" {
			t.Fatalf("target = %q, want reviewer", target)
		}
		return "ws1:tab2:pane3", nil
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"reviewer"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.String() != "closed ws1:tab2:pane3\n" {
		t.Fatalf("output = %q, want %q", output.String(), "closed ws1:tab2:pane3\n")
	}
}

func TestStopRequiresOneTarget(t *testing.T) {
	for _, args := range [][]string{{}, {"one", "two"}} {
		command := newCommand(func(context.Context, string) (string, error) {
			t.Fatal("stop operation called")
			return "", nil
		})
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs(args)

		if err := command.Execute(); err == nil {
			t.Fatalf("Execute(%q) error = nil, want argument error", args)
		}
	}
}

func TestStopPropagatesError(t *testing.T) {
	want := errors.New("unknown agent")
	command := newCommand(func(context.Context, string) (string, error) {
		return "", want
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"ghost"})

	if err := command.Execute(); !errors.Is(err, want) {
		t.Fatalf("Execute() error = %v, want %v", err, want)
	}
	if bytes.Contains(output.Bytes(), []byte("closed")) {
		t.Fatalf("output = %q, want no closure line", output.String())
	}
}

func TestStopHelpDoesNotRunOperation(t *testing.T) {
	command := newCommand(func(context.Context, string) (string, error) {
		t.Fatal("stop operation called")
		return "", nil
	})
	command.SetOut(io.Discard)
	command.SetArgs([]string{"--help"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}
