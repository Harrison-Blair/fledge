package message

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"

	internalagent "fledge/internal/agent"
)

func TestMessageFlagsBecomeOptions(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want internalagent.MessageOptions
	}{
		{
			name: "target and text only",
			args: []string{"reviewer", "status?"},
			want: internalagent.MessageOptions{Target: "reviewer", Text: "status?"},
		},
		{
			name: "wait with repeated states and a timeout",
			args: []string{"reviewer", "status?", "--wait", "--until", "idle", "--until", "waiting", "--timeout", "2500"},
			want: internalagent.MessageOptions{Target: "reviewer", Text: "status?", Wait: true, Until: []string{"idle", "waiting"}, TimeoutMS: 2500},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			command := newCommand(func(_ context.Context, options internalagent.MessageOptions) (json.RawMessage, error) {
				called = true
				if !reflect.DeepEqual(options, test.want) {
					t.Fatalf("options = %#v, want %#v", options, test.want)
				}
				return json.RawMessage(`{}`), nil
			})
			command.SetOut(&bytes.Buffer{})
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !called {
				t.Fatal("message operation was not called")
			}
		})
	}
}

func TestMessagePrintsRawResult(t *testing.T) {
	command := newCommand(func(context.Context, internalagent.MessageOptions) (json.RawMessage, error) {
		return json.RawMessage(`{"type":"agent_prompted","agent_status":"idle"}`), nil
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"reviewer", "status?"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := `{"type":"agent_prompted","agent_status":"idle"}` + "\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestMessageRequiresTargetAndText(t *testing.T) {
	for _, args := range [][]string{{}, {"reviewer"}, {"reviewer", "status?", "extra"}} {
		command := newCommand(func(context.Context, internalagent.MessageOptions) (json.RawMessage, error) {
			t.Fatal("message operation called")
			return nil, nil
		})
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs(args)

		if err := command.Execute(); err == nil {
			t.Fatalf("Execute(%q) error = nil, want argument error", args)
		}
	}
}

func TestMessagePropagatesError(t *testing.T) {
	want := errors.New("prompt failed")
	command := newCommand(func(context.Context, internalagent.MessageOptions) (json.RawMessage, error) {
		return nil, want
	})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"reviewer", "status?"})

	if err := command.Execute(); !errors.Is(err, want) {
		t.Fatalf("Execute() error = %v, want %v", err, want)
	}
}

func TestMessageHelpDoesNotRunOperation(t *testing.T) {
	command := newCommand(func(context.Context, internalagent.MessageOptions) (json.RawMessage, error) {
		t.Fatal("message operation called")
		return nil, nil
	})
	command.SetOut(io.Discard)
	command.SetArgs([]string{"--help"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}
