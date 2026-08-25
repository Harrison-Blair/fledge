package start

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"fledge/internal/session"
)

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
			called := false
			command := newCommand(func(_ context.Context, path string, deps session.StartDependencies) error {
				called = true
				if path != test.want {
					t.Fatalf("path = %q, want %q", path, test.want)
				}
				if deps.Herder == nil || deps.Entropy == nil || deps.Now == nil || deps.Getenv == nil {
					t.Fatalf("dependencies contain nil: %#v", deps)
				}
				return nil
			})
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

func TestStartRejectsExtraArguments(t *testing.T) {
	command := newCommand(func(context.Context, string, session.StartDependencies) error {
		t.Fatal("start operation called")
		return nil
	})
	command.SetArgs([]string{"one", "two"})

	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want argument error")
	}
}

func TestStartPropagatesError(t *testing.T) {
	want := errors.New("start failed")
	command := newCommand(func(context.Context, string, session.StartDependencies) error {
		return want
	})

	if err := command.Execute(); !errors.Is(err, want) {
		t.Fatalf("Execute() error = %v, want %v", err, want)
	}
}

func TestStartHelpDoesNotRunOperation(t *testing.T) {
	command := newCommand(func(context.Context, string, session.StartDependencies) error {
		t.Fatal("start operation called")
		return nil
	})
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{"--help"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}
