package start

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"fledge/internal/picker"
	"fledge/internal/session"
)

type fdBuffer struct {
	bytes.Buffer
	fd uintptr
}

func (b *fdBuffer) Fd() uintptr { return b.fd }

func TestStartPathAndDependencies(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "working directory", want: "."},
		{name: "explicit path", args: []string{"somewhere"}, want: "somewhere"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := &fdBuffer{fd: 10}
			output := &fdBuffer{fd: 11}
			stderr := &bytes.Buffer{}

			called := false
			command := newCommand(func(_ context.Context, path string, deps session.StartDependencies) error {
				called = true
				if path != test.want {
					t.Fatalf("path = %q, want %q", path, test.want)
				}
				if deps.Herder == nil || deps.Entropy == nil || deps.Now == nil || deps.Getenv == nil {
					t.Fatalf("dependencies contain nil: %#v", deps)
				}
				if deps.Scoped == nil {
					t.Fatal("scoped Herder client was not supplied")
				}
				if deps.Scoped("fledge-project-00000001") == nil {
					t.Fatal("scoped Herder client is nil")
				}
				if deps.Diagnostics != stderr {
					t.Fatal("diagnostics were not inherited")
				}
				chooser, ok := deps.Chooser.(picker.AgentChooser)
				if !ok {
					t.Fatalf("chooser type = %T", deps.Chooser)
				}
				if chooser.Input != input || chooser.Output != output {
					t.Fatal("selection streams were not inherited")
				}
				if !chooser.InputIsTerminal || chooser.OutputIsTerminal {
					t.Fatalf("terminal state = (%v, %v), want (true, false)", chooser.InputIsTerminal, chooser.OutputIsTerminal)
				}
				if chooser.Models == nil {
					t.Fatal("model lookup was not supplied")
				}
				return nil
			}, func(fd int) bool { return fd == 10 })
			command.SetIn(input)
			command.SetOut(output)
			command.SetErr(stderr)
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !called {
				t.Fatal("start operation was not called")
			}
		})
	}
}

func TestStartTreatsNonFilesAsNonTerminals(t *testing.T) {
	command := newCommand(func(_ context.Context, _ string, deps session.StartDependencies) error {
		chooser := deps.Chooser.(picker.AgentChooser)
		if chooser.InputIsTerminal || chooser.OutputIsTerminal {
			t.Fatal("non-file streams detected as terminals")
		}
		return nil
	}, func(int) bool {
		t.Fatal("terminal detector called without file descriptor")
		return false
	})
	command.SetIn(&bytes.Buffer{})
	command.SetOut(&bytes.Buffer{})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestStartRejectsExtraArguments(t *testing.T) {
	command := newCommand(func(context.Context, string, session.StartDependencies) error {
		t.Fatal("start operation called")
		return nil
	}, func(int) bool { return false })
	command.SetArgs([]string{"one", "two"})

	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want argument error")
	}
}

func TestStartPropagatesError(t *testing.T) {
	want := errors.New("start failed")
	command := newCommand(func(context.Context, string, session.StartDependencies) error {
		return want
	}, func(int) bool { return false })

	if err := command.Execute(); !errors.Is(err, want) {
		t.Fatalf("Execute() error = %v, want %v", err, want)
	}
}

func TestStartNewFlagSetsDependency(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want bool
	}{
		{name: "default", args: nil, want: false},
		{name: "new", args: []string{"--new"}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := &fdBuffer{fd: 10}
			output := &fdBuffer{fd: 11}

			called := false
			command := newCommand(func(_ context.Context, _ string, deps session.StartDependencies) error {
				called = true
				if deps.New != test.want {
					t.Fatalf("New = %v, want %v", deps.New, test.want)
				}
				return nil
			}, func(int) bool { return false })
			command.SetIn(input)
			command.SetOut(output)
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !called {
				t.Fatal("start operation was not called")
			}
		})
	}
}

func TestStartHelpDoesNotRunOperation(t *testing.T) {
	command := newCommand(func(context.Context, string, session.StartDependencies) error {
		t.Fatal("start operation called")
		return nil
	}, func(int) bool { return false })
	var help bytes.Buffer
	command.SetOut(&help)
	command.SetArgs([]string{"--help"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(help.String(), "--new") {
		t.Fatalf("help = %q, want the --new flag documented", help.String())
	}
}
