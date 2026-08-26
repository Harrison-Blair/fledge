package spawn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	internalagent "fledge/internal/agent"
)

func TestSpawnFlagsBecomeOptions(t *testing.T) {
	ratio := 0.4
	for _, test := range []struct {
		name string
		args []string
		want internalagent.SpawnOptions
	}{
		{
			name: "name and kind only",
			args: []string{"rev", "--kind", "claude"},
			want: internalagent.SpawnOptions{Name: "rev", Kind: "claude"},
		},
		{
			name: "every placement flag",
			args: []string{"rev", "--kind", "claude", "--model", "opus", "--pane", "ws1:tab2:pane3", "--split", "down", "--ratio", "0.4", "--label", "review pass"},
			want: internalagent.SpawnOptions{Name: "rev", Kind: "claude", Model: "opus", Pane: "ws1:tab2:pane3", Split: "down", Ratio: &ratio, Label: "review pass"},
		},
		{
			name: "harness arguments after the dash",
			args: []string{"rev", "--kind", "codex", "--", "--model", "x", "--extra"},
			want: internalagent.SpawnOptions{Name: "rev", Kind: "codex", Args: []string{"--model", "x", "--extra"}},
		},
		{
			name: "workspace placement",
			args: []string{"rev", "--kind", "pi", "--workspace", "new"},
			want: internalagent.SpawnOptions{Name: "rev", Kind: "pi", Workspace: "new"},
		},
		{
			name: "tab placement",
			args: []string{"rev", "--kind", "pi", "--tab", "ws1:tab2"},
			want: internalagent.SpawnOptions{Name: "rev", Kind: "pi", Tab: "ws1:tab2"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
				called = true
				if !reflect.DeepEqual(options, test.want) {
					t.Fatalf("options = %#v, want %#v", options, test.want)
				}
				if options.Ratio != nil && test.want.Ratio != nil && *options.Ratio != *test.want.Ratio {
					t.Fatalf("ratio = %v, want %v", *options.Ratio, *test.want.Ratio)
				}
				return internalagent.SpawnResult{}, nil
			})
			command.SetOut(&bytes.Buffer{})
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !called {
				t.Fatal("spawn operation was not called")
			}
		})
	}
}

func TestSpawnLeavesRatioUnsetWhenTheFlagIsAbsent(t *testing.T) {
	command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		if options.Ratio != nil {
			t.Fatalf("ratio = %v, want nil", *options.Ratio)
		}
		return internalagent.SpawnResult{}, nil
	})
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{"rev", "--kind", "claude"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestSpawnPrintsResultAsOneJSONLine(t *testing.T) {
	command := newCommand(func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		return internalagent.SpawnResult{Name: "rev", Kind: "claude", Model: "opus", WorkspaceID: "ws1", TabID: "ws1:tab2", PaneID: "ws1:tab2:pane3"}, nil
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"rev", "--kind", "claude"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := `{"name":"rev","kind":"claude","model":"opus","workspace_id":"ws1","tab_id":"ws1:tab2","pane_id":"ws1:tab2:pane3"}` + "\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
	var decoded internalagent.SpawnResult
	if err := json.Unmarshal([]byte(output.String()), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestSpawnRejectsInvalidArguments(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "missing kind", args: []string{"rev"}},
		{name: "no name", args: []string{"--kind", "claude"}},
		{name: "two names", args: []string{"rev", "extra", "--kind", "claude"}},
		{name: "name only after the dash", args: []string{"--kind", "claude", "--", "rev"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := newCommand(func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
				t.Fatal("spawn operation called")
				return internalagent.SpawnResult{}, nil
			})
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			command.SetArgs(test.args)

			if err := command.Execute(); err == nil {
				t.Fatal("Execute() error = nil, want argument error")
			}
		})
	}
}

func TestSpawnPropagatesError(t *testing.T) {
	want := errors.New("spawn failed")
	command := newCommand(func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		return internalagent.SpawnResult{}, want
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"rev", "--kind", "claude"})

	if err := command.Execute(); !errors.Is(err, want) {
		t.Fatalf("Execute() error = %v, want %v", err, want)
	}
	if strings.Contains(output.String(), "{") {
		t.Fatalf("output = %q, want no result line", output.String())
	}
}

func TestSpawnHelpDoesNotRunOperation(t *testing.T) {
	command := newCommand(func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		t.Fatal("spawn operation called")
		return internalagent.SpawnResult{}, nil
	})
	command.SetOut(io.Discard)
	command.SetArgs([]string{"--help"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}
