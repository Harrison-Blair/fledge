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
	"fledge/internal/catalog"
	"fledge/internal/picker"
	"fledge/internal/profile"
)

type fdBuffer struct {
	bytes.Buffer
	fd uintptr
}

func (b *fdBuffer) Fd() uintptr { return b.fd }

func testResolver(selectOne picker.SelectFunc) resolverFactory {
	return func(input io.Reader, output io.Writer) picker.Resolver {
		return picker.Resolver{
			Input:  input,
			Output: output,
			Models: func(context.Context, catalog.Harness) []string { return []string{"listed-model"} },
			Select: selectOne,
		}
	}
}

func TestSpawnFlagsBecomeOptions(t *testing.T) {
	ratio := 0.4
	configured, ok := profile.Get(profile.OrchestratorName)
	if !ok {
		t.Fatal("managed profile is missing")
	}
	for _, test := range []struct {
		name string
		args []string
		want internalagent.SpawnOptions
	}{
		{
			name: "harness and model",
			args: []string{"rev", "--harness", "claude", "--model", "opus"},
			want: internalagent.SpawnOptions{Name: "rev", Harness: "claude", Model: "opus"},
		},
		{
			name: "every placement flag",
			args: []string{"rev", "--harness", "claude", "--model", "opus", "--pane", "ws1:tab2:pane3", "--split", "down", "--ratio", "0.4", "--label", "review pass"},
			want: internalagent.SpawnOptions{Name: "rev", Harness: "claude", Model: "opus", Pane: "ws1:tab2:pane3", Split: "down", Ratio: &ratio, Label: "review pass"},
		},
		{
			name: "harness arguments after the dash",
			args: []string{"rev", "--harness", "codex", "--model", "gpt", "--", "--model", "x", "--extra"},
			want: internalagent.SpawnOptions{Name: "rev", Harness: "codex", Model: "gpt", Args: []string{"--model", "x", "--extra"}},
		},
		{
			name: "workspace placement",
			args: []string{"rev", "--harness", "pi", "--model", "provider/model", "--workspace", "new"},
			want: internalagent.SpawnOptions{Name: "rev", Harness: "pi", Model: "provider/model", Workspace: "new"},
		},
		{
			name: "tab placement with profile",
			args: []string{"rev", "--profile", profile.OrchestratorName, "--harness", "pi", "--model", "provider/model", "--tab", "ws1:tab2"},
			want: internalagent.SpawnOptions{Name: "rev", Harness: "pi", Model: "provider/model", Profile: &configured, Tab: "ws1:tab2"},
		},
		{
			name: "explicit no profile",
			args: []string{"rev", "--no-profile", "--harness", "cursor", "--model", "auto"},
			want: internalagent.SpawnOptions{Name: "rev", Harness: "cursor", Model: "auto"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
				called = true
				if !reflect.DeepEqual(options, test.want) {
					t.Fatalf("options = %#v, want %#v", options, test.want)
				}
				return internalagent.SpawnResult{}, nil
			}, func(int) bool { return false }, testResolver(nil))
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

func TestSpawnInteractiveResolutionUsesInjectedResolver(t *testing.T) {
	input := &fdBuffer{fd: 10}
	output := &fdBuffer{fd: 11}
	var prompts []string
	command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		if options.Harness != "codex" || options.Model != "listed-model" || options.Profile != nil {
			t.Fatalf("options = %#v, want interactively resolved no-profile Codex launch", options)
		}
		return internalagent.SpawnResult{}, nil
	}, func(fd int) bool { return fd == 10 || fd == 11 }, testResolver(func(_ io.Reader, _ io.Writer, title string, options []picker.Option) (picker.Option, error) {
		prompts = append(prompts, title)
		switch title {
		case "Select agent profile":
			if len(options) != 2 || options[0].Title != "None" || options[1].ID != profile.OrchestratorName {
				t.Fatalf("profile options = %#v", options)
			}
			return options[0], nil
		case "Select harness":
			return picker.Option{ID: "codex", Title: "codex"}, nil
		case "Model for codex":
			return picker.Option{ID: "listed-model", Title: "listed-model"}, nil
		default:
			t.Fatalf("unexpected prompt %q", title)
			return picker.Option{}, nil
		}
	}))
	command.SetIn(input)
	command.SetOut(output)
	command.SetArgs([]string{"rev"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := []string{"Select agent profile", "Select harness", "Model for codex"}
	if !reflect.DeepEqual(prompts, want) {
		t.Fatalf("prompts = %#v, want %#v", prompts, want)
	}
}

func TestSpawnNonInteractiveMissingChoicesFailsBeforeOperation(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "harness", args: []string{"rev"}, want: "harness is required in non-interactive mode"},
		{name: "model", args: []string{"rev", "--harness", "codex"}, want: "model is required in non-interactive mode"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := newCommand(func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
				t.Fatal("spawn operation called")
				return internalagent.SpawnResult{}, nil
			}, func(int) bool { return false }, testResolver(nil))
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			command.SetArgs(test.args)

			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSpawnRejectsRemovedKindFlag(t *testing.T) {
	command := newCommand(func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		t.Fatal("spawn operation called")
		return internalagent.SpawnResult{}, nil
	}, func(int) bool { return false }, testResolver(nil))
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"rev", "--kind", "claude", "--model", "opus"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --kind") {
		t.Fatalf("Execute() error = %v, want removed --kind flag", err)
	}
}

func TestSpawnRejectsUnsupportedHarnessBeforeOperation(t *testing.T) {
	for _, harness := range []string{"opencode", "gemini"} {
		t.Run(harness, func(t *testing.T) {
			command := newCommand(func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
				t.Fatal("spawn operation called")
				return internalagent.SpawnResult{}, nil
			}, func(int) bool { return false }, testResolver(nil))
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			command.SetArgs([]string{"rev", "--harness", harness, "--model", "model"})

			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), "unsupported harness") {
				t.Fatalf("Execute() error = %v, want unsupported harness", err)
			}
		})
	}
}

func TestSpawnRejectsProfileAndNoProfileTogether(t *testing.T) {
	command := newCommand(func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		t.Fatal("spawn operation called")
		return internalagent.SpawnResult{}, nil
	}, func(int) bool { return false }, testResolver(nil))
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"rev", "--harness", "codex", "--model", "gpt", "--profile", profile.OrchestratorName, "--no-profile"})

	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want mutually exclusive flag error")
	}
}

func TestSpawnLeavesRatioUnsetWhenTheFlagIsAbsent(t *testing.T) {
	command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		if options.Ratio != nil {
			t.Fatalf("ratio = %v, want nil", *options.Ratio)
		}
		return internalagent.SpawnResult{}, nil
	}, func(int) bool { return false }, testResolver(nil))
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{"rev", "--harness", "claude", "--model", "opus"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestSpawnPrintsResultAsOneJSONLine(t *testing.T) {
	command := newCommand(func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		return internalagent.SpawnResult{Name: "rev", Harness: "claude", Model: "opus", Profile: profile.OrchestratorName, WorkspaceID: "ws1", TabID: "ws1:tab2", PaneID: "ws1:tab2:pane3"}, nil
	}, func(int) bool { return false }, testResolver(nil))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"rev", "--harness", "claude", "--model", "opus"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := `{"name":"rev","harness":"claude","model":"opus","profile":"fledge-orchestrator","workspace_id":"ws1","tab_id":"ws1:tab2","pane_id":"ws1:tab2:pane3"}` + "\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
	var decoded internalagent.SpawnResult
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestSpawnRejectsInvalidArguments(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "no name", args: []string{"--harness", "claude", "--model", "opus"}},
		{name: "two names", args: []string{"rev", "extra", "--harness", "claude", "--model", "opus"}},
		{name: "name only after the dash", args: []string{"--harness", "claude", "--model", "opus", "--", "rev"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := newCommand(func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
				t.Fatal("spawn operation called")
				return internalagent.SpawnResult{}, nil
			}, func(int) bool { return false }, testResolver(nil))
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
	}, func(int) bool { return false }, testResolver(nil))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"rev", "--harness", "claude", "--model", "opus"})

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
	}, func(int) bool { return false }, testResolver(nil))
	command.SetOut(io.Discard)
	command.SetArgs([]string{"--help"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}
