package stop

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"fledge/internal/session"
)

type fdBuffer struct {
	bytes.Buffer
	fd uintptr
}

func (b *fdBuffer) Fd() uintptr { return b.fd }

func TestStopPathStreamsAndTerminalDetection(t *testing.T) {
	input := &fdBuffer{fd: 10}
	output := &fdBuffer{fd: 11}
	stderr := &bytes.Buffer{}

	command := newCommand(func(_ context.Context, path string, deps session.StopDependencies) error {
		if path != "project" {
			t.Fatalf("path = %q, want project", path)
		}
		if deps.Herder == nil || deps.Getenv == nil || deps.Scoped == nil || deps.Entropy == nil {
			t.Fatalf("dependencies contain nil: %#v", deps)
		}
		if deps.Output != output {
			t.Fatal("output was not inherited")
		}
		confirmer, ok := deps.Confirmer.(session.TerminalConfirmer)
		if !ok {
			t.Fatalf("confirmer type = %T", deps.Confirmer)
		}
		if confirmer.Input != input || confirmer.Output != output {
			t.Fatal("confirmation streams were not inherited")
		}
		if !confirmer.InputIsTerminal || confirmer.OutputIsTerminal {
			t.Fatalf("terminal state = (%v, %v), want (true, false)", confirmer.InputIsTerminal, confirmer.OutputIsTerminal)
		}
		return nil
	}, func(fd int) bool { return fd == 10 })
	command.SetIn(input)
	command.SetOut(output)
	command.SetErr(stderr)
	command.SetArgs([]string{"project"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestStopDefaultsPathAndTreatsNonFilesAsNonTerminals(t *testing.T) {
	command := newCommand(func(_ context.Context, path string, deps session.StopDependencies) error {
		if path != "." {
			t.Fatalf("path = %q, want .", path)
		}
		confirmer := deps.Confirmer.(session.TerminalConfirmer)
		if confirmer.InputIsTerminal || confirmer.OutputIsTerminal {
			t.Fatal("non-file streams detected as terminals")
		}
		return nil
	}, func(int) bool {
		t.Fatal("terminal detector called without file descriptor")
		return false
	})
	command.SetIn(bytes.NewBufferString("yes\n"))
	command.SetOut(&bytes.Buffer{})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestStopRejectsExtraArguments(t *testing.T) {
	command := newCommand(func(context.Context, string, session.StopDependencies) error {
		t.Fatal("stop operation called")
		return nil
	}, func(int) bool { return false })
	command.SetArgs([]string{"one", "two"})

	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want argument error")
	}
}

func TestStopPropagatesError(t *testing.T) {
	want := errors.New("stop failed")
	command := newCommand(func(context.Context, string, session.StopDependencies) error {
		return want
	}, func(int) bool { return false })

	if err := command.Execute(); !errors.Is(err, want) {
		t.Fatalf("Execute() error = %v, want %v", err, want)
	}
}

func TestStopHelpDoesNotRunOperation(t *testing.T) {
	command := newCommand(func(context.Context, string, session.StopDependencies) error {
		t.Fatal("stop operation called")
		return nil
	}, func(int) bool { return false })
	command.SetOut(io.Discard)
	command.SetArgs([]string{"--help"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}
