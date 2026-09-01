package spawn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	internalagent "fledge/internal/agent"
	"fledge/internal/catalog"
	"fledge/internal/herdr"
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

// countingResolver wraps the standard non-interactive test resolver and counts
// how many times the factory is constructed, so a fail-fast test can prove that
// invalid input never reaches resolution.
func countingResolver(count *int) resolverFactory {
	base := testResolver(nil)
	return func(input io.Reader, output io.Writer) picker.Resolver {
		*count++
		return base(input, output)
	}
}

// countingOp returns a spawn operation that records that it ran and yields the
// given result and error.
func countingOp(count *int, result internalagent.SpawnResult, err error) spawnOperation {
	return func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		*count++
		return result, err
	}
}

// failingWriter fails every write and counts the attempts, so a test can prove
// the partial-result line is encoded exactly once.
type failingWriter struct{ writes int }

func (w *failingWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, errors.New("write failure")
}

// forgeAsError carries a custom As(any) bool hook that forges a marker into the
// caller's target. errors.As trusts it; the CLI trust boundary must not.
type forgeAsError struct {
	forged *internalagent.InitialPromptError
}

func (forgeAsError) Error() string { return "forge-as" }

func (e forgeAsError) As(target any) bool {
	if p, ok := target.(**internalagent.InitialPromptError); ok {
		*p = e.forged
		return true
	}
	return false
}

// blindAsError's As hook returns true without writing the target. Under
// errors.As this leaves a nil marker that the old path still rendered.
type blindAsError struct{}

func (blindAsError) Error() string { return "blind-as" }
func (blindAsError) As(any) bool   { return true }

// cycleError unwraps to whatever next points at, so it can form a self or
// mutual Unwrap() error cycle.
type cycleError struct{ next error }

func (*cycleError) Error() string   { return "cycle" }
func (e *cycleError) Unwrap() error { return e.next }

// fanoutError exposes a fixed child slice through Unwrap() []error, so it can
// form a branching cycle or a wide fanout.
type fanoutError struct{ children []error }

func (*fanoutError) Error() string     { return "fanout" }
func (e *fanoutError) Unwrap() []error { return e.children }

func newSelfCycle() error {
	c := &cycleError{}
	c.next = c
	return c
}

func newBranchCycle() error {
	f := &fanoutError{}
	f.children = []error{f, f}
	return f
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
			args: []string{"rev", "--no-profile", "--harness", "claude", "--model", "opus"},
			want: internalagent.SpawnOptions{Name: "rev", Harness: "claude", Model: "opus"},
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
			if len(options) != 3 ||
				options[0].ID != "" || options[0].Title != "None" ||
				options[1].ID != profile.GeneralName || options[1].Title != profile.GeneralName ||
				options[2].ID != profile.OrchestratorName || options[2].Title != profile.OrchestratorName {
				t.Fatalf("profile options = %#v, want [None, %s, %s]", options, profile.GeneralName, profile.OrchestratorName)
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

func TestSpawnInteractiveResolutionSelectsManagedProfile(t *testing.T) {
	input := &fdBuffer{fd: 10}
	output := &fdBuffer{fd: 11}
	// Independently sourced snapshot: the expectation must not be borrowed from
	// the resolved value, or a corrupted (e.g. Name-only) resolution would
	// silently pass.
	configured, ok := profile.Get(profile.GeneralName)
	if !ok {
		t.Fatal("managed profile is missing")
	}

	resolverCalls := 0
	promptCalls := map[string]int{}
	opCalls := 0
	var got internalagent.SpawnOptions
	factory := func(input io.Reader, output io.Writer) picker.Resolver {
		resolverCalls++
		if resolverCalls > 1 {
			t.Fatalf("resolver constructed %d times, want exactly 1", resolverCalls)
		}
		return testResolver(func(_ io.Reader, _ io.Writer, title string, options []picker.Option) (picker.Option, error) {
			promptCalls[title]++
			if promptCalls[title] > 1 {
				t.Fatalf("prompt %q invoked %d times, want exactly 1", title, promptCalls[title])
			}
			switch title {
			case "Select agent profile":
				for _, option := range options {
					if option.ID == profile.GeneralName {
						return option, nil
					}
				}
				t.Fatalf("profile options = %#v, want %s present", options, profile.GeneralName)
				return picker.Option{}, nil
			case "Select harness":
				return picker.Option{ID: "codex", Title: "codex"}, nil
			case "Model for codex":
				return picker.Option{ID: "listed-model", Title: "listed-model"}, nil
			default:
				t.Fatalf("unexpected prompt %q", title)
				return picker.Option{}, nil
			}
		})(input, output)
	}
	command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		opCalls++
		if opCalls > 1 {
			t.Fatalf("spawn operation invoked %d times, want exactly 1", opCalls)
		}
		got = options
		return internalagent.SpawnResult{}, nil
	}, func(fd int) bool { return fd == 10 || fd == 11 }, factory)
	command.SetIn(input)
	command.SetOut(output)
	command.SetArgs([]string{"rev"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	wantPromptCalls := map[string]int{
		"Select agent profile": 1,
		"Select harness":       1,
		"Model for codex":      1,
	}
	if !reflect.DeepEqual(promptCalls, wantPromptCalls) {
		t.Fatalf("promptCalls = %#v, want %#v", promptCalls, wantPromptCalls)
	}
	if resolverCalls != 1 {
		t.Fatalf("resolverCalls = %d, want exactly 1", resolverCalls)
	}
	if opCalls != 1 {
		t.Fatalf("opCalls = %d, want exactly 1", opCalls)
	}
	// Full-struct comparison against the independently sourced snapshot: a
	// profile carrying only a matching Name (Instructions/Defaults dropped or
	// corrupted) fails here even though a Name-only check would not.
	want := internalagent.SpawnOptions{Name: "rev", Harness: "codex", Model: "listed-model", Profile: &configured}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %#v, want %#v", got, want)
	}
}

func TestSpawnExplicitGeneralProfileWithPromptForwardsExactly(t *testing.T) {
	const text = "one-command spawn brief"
	// Independently sourced snapshot: the expectation must not be borrowed from
	// the resolved value, or a corrupted (e.g. Name-only) resolution would
	// silently pass.
	configured, ok := profile.Get(profile.GeneralName)
	if !ok {
		t.Fatal("managed profile is missing")
	}

	resolverCalls, opCalls := 0, 0
	var got internalagent.SpawnOptions
	factory := func(input io.Reader, output io.Writer) picker.Resolver {
		resolverCalls++
		if resolverCalls > 1 {
			t.Fatalf("resolver constructed %d times, want exactly 1", resolverCalls)
		}
		return testResolver(nil)(input, output)
	}
	command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		opCalls++
		if opCalls > 1 {
			t.Fatalf("spawn operation invoked %d times, want exactly 1", opCalls)
		}
		got = options
		return internalagent.SpawnResult{Name: "rev", Harness: "claude", Model: "opus", Profile: profile.GeneralName}, nil
	}, func(int) bool { return false }, factory)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"rev", "--profile", profile.GeneralName, "--harness", "claude", "--model", "opus", "--prompt", text})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if resolverCalls != 1 {
		t.Fatalf("resolverCalls = %d, want exactly 1", resolverCalls)
	}
	if opCalls != 1 {
		t.Fatalf("opCalls = %d, want exactly 1", opCalls)
	}
	if got.InitialPrompt == nil || *got.InitialPrompt != text {
		t.Fatalf("InitialPrompt = %v, want %q", got.InitialPrompt, text)
	}
	// Full-struct comparison against the independently sourced snapshot, plus
	// the exact independently verified prompt bytes above.
	want := internalagent.SpawnOptions{Name: "rev", Harness: "claude", Model: "opus", Profile: &configured, InitialPrompt: got.InitialPrompt}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %#v, want %#v", got, want)
	}
	wantOutput := `{"name":"rev","harness":"claude","model":"opus","profile":"fledge-general","workspace_id":"","tab_id":"","pane_id":""}` + "\n"
	if output.String() != wantOutput {
		t.Fatalf("output = %q, want %q", output.String(), wantOutput)
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

func TestSpawnInlinePromptForwardsExactContent(t *testing.T) {
	const text = "line one\nline two\t\"quoted\" \\path $VAR"
	var got internalagent.SpawnOptions
	command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		got = options
		return internalagent.SpawnResult{}, nil
	}, func(int) bool { return false }, testResolver(nil))
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{"rev", "--harness", "claude", "--model", "opus", "--prompt", text, "--", "--extra"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.InitialPrompt == nil || *got.InitialPrompt != text {
		t.Fatalf("InitialPrompt = %v, want %q", got.InitialPrompt, text)
	}
	// The prompt rides alongside every existing option and native harness arg.
	want := internalagent.SpawnOptions{Name: "rev", Harness: "claude", Model: "opus", Args: []string{"--extra"}, InitialPrompt: got.InitialPrompt}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %#v, want %#v", got, want)
	}
}

func TestSpawnFilePromptForwardsExactContent(t *testing.T) {
	const text = "prompt from file\nsecond line\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	var got internalagent.SpawnOptions
	command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		got = options
		return internalagent.SpawnResult{}, nil
	}, func(int) bool { return false }, testResolver(nil))
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{"rev", "--harness", "codex", "--model", "gpt", "--workspace", "new", "--prompt-file", path, "--", "--flag"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.InitialPrompt == nil || *got.InitialPrompt != text {
		t.Fatalf("InitialPrompt = %v, want %q", got.InitialPrompt, text)
	}
	want := internalagent.SpawnOptions{Name: "rev", Harness: "codex", Model: "gpt", Workspace: "new", Args: []string{"--flag"}, InitialPrompt: got.InitialPrompt}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %#v, want %#v", got, want)
	}
}

func TestSpawnFilePromptPreservesBytes(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "multiline", data: []byte("alpha\nbeta\ngamma")},
		{name: "crlf", data: []byte("alpha\r\nbeta\r\n")},
		{name: "bom", data: []byte("\ufeffwith byte order mark")},
		{name: "unicode", data: []byte("café — 日本語 — 🚀")},
		{name: "quotes and backslashes", data: []byte(`he said "hi" \ and \\ then $x`)},
		{name: "trailing newline", data: []byte("ends with a newline\n")},
		{name: "tabs", data: []byte("col1\tcol2\tcol3")},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "p")
			if err := os.WriteFile(path, test.data, 0o644); err != nil {
				t.Fatal(err)
			}
			var got *string
			command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
				got = options.InitialPrompt
				return internalagent.SpawnResult{}, nil
			}, func(int) bool { return false }, testResolver(nil))
			command.SetOut(&bytes.Buffer{})
			command.SetArgs([]string{"rev", "--harness", "claude", "--model", "opus", "--prompt-file", path})

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got == nil || *got != string(test.data) {
				t.Fatalf("InitialPrompt = %v, want exact bytes %q", got, string(test.data))
			}
		})
	}
}

func TestSpawnPromptFlagsAreMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolverCalls, opCalls := 0, 0
	command := newCommand(countingOp(&opCalls, internalagent.SpawnResult{}, nil), func(int) bool { return false }, countingResolver(&resolverCalls))
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"rev", "--harness", "claude", "--model", "opus", "--prompt", "hi", "--prompt-file", path})

	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want mutually exclusive flag error")
	}
	if resolverCalls != 0 || opCalls != 0 {
		t.Fatalf("resolverCalls=%d opCalls=%d, want both 0 (failed before resolution)", resolverCalls, opCalls)
	}
}

func TestSpawnInvalidPromptFailsBeforeResolution(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "does-not-exist")
	badUTF8 := filepath.Join(dir, "bad-utf8")
	if err := os.WriteFile(badUTF8, []byte{0xff, 0xfe, 0xfd}, 0o644); err != nil {
		t.Fatal(err)
	}
	withNUL := filepath.Join(dir, "with-nul")
	if err := os.WriteFile(withNUL, []byte("ok\x00bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	oversize := filepath.Join(dir, "oversize")
	if err := os.WriteFile(oversize, bytes.Repeat([]byte("a"), 102401), 0o644); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(dir, "unreadable")
	if err := os.WriteFile(unreadable, []byte("secret"), 0o000); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		args []string
		skip bool
	}{
		{name: "inline empty equals form", args: []string{"--prompt="}},
		{name: "inline empty spaced form", args: []string{"--prompt", ""}},
		{name: "inline oversize", args: []string{"--prompt", strings.Repeat("a", 102401)}},
		{name: "inline nul", args: []string{"--prompt", "a\x00b"}},
		{name: "file empty", args: []string{"--prompt-file", empty}},
		{name: "file missing", args: []string{"--prompt-file", missing}},
		{name: "file directory", args: []string{"--prompt-file", dir}},
		{name: "file unreadable", args: []string{"--prompt-file", unreadable}, skip: os.Geteuid() == 0},
		{name: "file dash stdin spaced", args: []string{"--prompt-file", "-"}},
		{name: "file dash stdin equals", args: []string{"--prompt-file=-"}},
		{name: "file invalid utf8", args: []string{"--prompt-file", badUTF8}},
		{name: "file nul", args: []string{"--prompt-file", withNUL}},
		{name: "file oversize", args: []string{"--prompt-file", oversize}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.skip {
				t.Skip("running as root defeats the unreadable-file permission")
			}
			resolverCalls, opCalls := 0, 0
			command := newCommand(countingOp(&opCalls, internalagent.SpawnResult{}, nil), func(int) bool { return false }, countingResolver(&resolverCalls))
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			command.SetArgs(append([]string{"rev", "--harness", "claude", "--model", "opus"}, test.args...))

			if err := command.Execute(); err == nil {
				t.Fatal("Execute() error = nil, want validation failure")
			}
			if resolverCalls != 0 || opCalls != 0 {
				t.Fatalf("resolverCalls=%d opCalls=%d, want both 0 (failed before resolution)", resolverCalls, opCalls)
			}
		})
	}
}

func TestSpawnAcceptsMaxSizePrompt(t *testing.T) {
	const size = 102400
	text := strings.Repeat("a", size)
	dir := t.TempDir()
	path := filepath.Join(dir, "max")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	opCalls := 0
	var got *string
	command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		opCalls++
		got = options.InitialPrompt
		return internalagent.SpawnResult{}, nil
	}, func(int) bool { return false }, testResolver(nil))
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{"rev", "--harness", "claude", "--model", "opus", "--prompt-file", path})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if opCalls != 1 {
		t.Fatalf("opCalls = %d, want the operation reached exactly once", opCalls)
	}
	if got == nil || len(*got) != size {
		t.Fatalf("InitialPrompt length = %v, want %d bytes reaching the operation", got, size)
	}
}

func TestSpawnLeadingHyphenInlinePrompt(t *testing.T) {
	// A prompt whose first byte is a hyphen forwards its exact value in both the
	// spaced and the = form: pflag consumes the following argument as the flag
	// value regardless of a leading hyphen, so neither form is parsed as a flag.
	const text = "-not-a-flag"
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "equals form", args: []string{"--prompt=" + text}},
		{name: "spaced form", args: []string{"--prompt", text}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got *string
			command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
				got = options.InitialPrompt
				return internalagent.SpawnResult{}, nil
			}, func(int) bool { return false }, testResolver(nil))
			command.SetOut(&bytes.Buffer{})
			command.SetArgs(append([]string{"rev", "--harness", "claude", "--model", "opus"}, test.args...))

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got == nil || *got != text {
				t.Fatalf("InitialPrompt = %v, want %q", got, text)
			}
		})
	}
}

func TestSpawnPromptFileRelativeDashPathIsLiteral(t *testing.T) {
	// "./-" and any path that is not exactly "-" is read as a literal file
	// rather than rejected as unsupported stdin.
	const text = "read from ./- literally"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "-"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })

	var got *string
	command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		got = options.InitialPrompt
		return internalagent.SpawnResult{}, nil
	}, func(int) bool { return false }, testResolver(nil))
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{"rev", "--harness", "claude", "--model", "opus", "--prompt-file", "./-"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got == nil || *got != text {
		t.Fatalf("InitialPrompt = %v, want %q read from ./-", got, text)
	}
}

func TestSpawnPromptedSuccessEmitsLegacyResultLine(t *testing.T) {
	command := newCommand(func(_ context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		if options.InitialPrompt == nil || *options.InitialPrompt != "hello agent" {
			t.Fatalf("InitialPrompt = %v, want the forwarded prompt", options.InitialPrompt)
		}
		return internalagent.SpawnResult{Name: "rev", Harness: "claude", Model: "opus", Profile: "fledge-orchestrator", WorkspaceID: "ws1", TabID: "ws1:tab2", PaneID: "ws1:tab2:pane3"}, nil
	}, func(int) bool { return false }, testResolver(nil))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"rev", "--harness", "claude", "--model", "opus", "--prompt", "hello agent"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := `{"name":"rev","harness":"claude","model":"opus","profile":"fledge-orchestrator","workspace_id":"ws1","tab_id":"ws1:tab2","pane_id":"ws1:tab2:pane3"}` + "\n"
	if output.String() != want {
		t.Fatalf("output = %q, want the legacy seven-field line %q", output.String(), want)
	}
	if strings.Contains(output.String(), "initial_prompt") {
		t.Fatalf("output = %q, want no initial_prompt field on success", output.String())
	}
}

func TestSpawnPartialFailureEmitsRedactedResultLine(t *testing.T) {
	result := internalagent.SpawnResult{Name: "rev", Harness: "claude", Model: "opus", WorkspaceID: "ws1", TabID: "ws1:tab2", PaneID: "ws1:tab2:pane3"}
	for _, test := range []struct {
		name  string
		cause error
		code  string
	}{
		{name: "blocked", cause: &herdr.Error{Operation: "StartAgent", Code: "agent_blocked", Message: "blocked"}, code: "agent_blocked"},
		{name: "pane gone", cause: &herdr.Error{Operation: "SendText", Code: "agent_pane_not_found", Message: "no pane"}, code: "agent_pane_not_found"},
		{name: "non-whitelisted code", cause: &herdr.Error{Operation: "SendText", Code: "agent_wedged", Message: "wedged"}, code: "unknown"},
		{name: "unstructured cause", cause: errors.New("timeout"), code: "unknown"},
		{name: "nil cause", cause: nil, code: "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			spawnErr := fmt.Errorf("spawn agent %q: %w", result.Name, &internalagent.InitialPromptError{Cause: test.cause})
			command := newCommand(func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
				return result, spawnErr
			}, func(int) bool { return false }, testResolver(nil))
			// Mirror the production root, which sets SilenceUsage, so Cobra does
			// not append usage text to the captured stdout on the error path.
			command.SilenceUsage = true
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&bytes.Buffer{})
			command.SetArgs([]string{"rev", "--harness", "claude", "--model", "opus", "--prompt", "hello agent"})

			err := command.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want the original typed chain returned")
			}
			var promptErr *internalagent.InitialPromptError
			if !errors.As(err, &promptErr) {
				t.Fatalf("Execute() error = %v, want an InitialPromptError in the chain", err)
			}
			// json.Encoder HTML-escapes the angle brackets of the "<prompt>"
			// placeholder to < and >, matching the encoder the
			// success path already uses; it decodes back to "<prompt>".
			want := `{"name":"rev","harness":"claude","model":"opus","profile":"","workspace_id":"ws1","tab_id":"ws1:tab2","pane_id":"ws1:tab2:pane3","initial_prompt":{"status":"delivery_unconfirmed","code":"` + test.code + `","retry_argv":["fledge","agent","message","rev","--","\u003cprompt\u003e"]}}` + "\n"
			if output.String() != want {
				t.Fatalf("output = %q,\n want      %q", output.String(), want)
			}
			if n := strings.Count(output.String(), "\n"); n != 1 {
				t.Fatalf("output has %d newlines, want exactly one JSON line", n)
			}
			if strings.Contains(output.String(), "hello agent") {
				t.Fatalf("output leaked the prompt text: %q", output.String())
			}
		})
	}
}

func TestSpawnPartialFailureRedactsSensitiveFields(t *testing.T) {
	const secretPrompt = "SECRET-PROMPT-PLZ-HIDE"
	cause := &herdr.Error{Operation: "SECRET-OP", Code: "SECRET-FREEFORM-CODE", Message: "SECRET-MESSAGE"}
	result := internalagent.SpawnResult{Name: "rev", Harness: "claude", Model: "opus", WorkspaceID: "ws1", TabID: "ws1:tab2", PaneID: "ws1:tab2:pane3"}
	spawnErr := fmt.Errorf("spawn agent %q: %w", result.Name, &internalagent.InitialPromptError{Cause: cause})

	dir := t.TempDir()
	path := filepath.Join(dir, "SECRET-PATH-COMPONENT")
	if err := os.WriteFile(path, []byte(secretPrompt), 0o644); err != nil {
		t.Fatal(err)
	}
	command := newCommand(func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		return result, spawnErr
	}, func(int) bool { return false }, testResolver(nil))
	command.SilenceUsage = true
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"rev", "--harness", "claude", "--model", "opus", "--prompt-file", path})

	err := command.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want partial failure error")
	}
	stdout := output.String()
	for _, secret := range []string{secretPrompt, "SECRET-OP", "SECRET-FREEFORM-CODE", "SECRET-MESSAGE", path, "SECRET-PATH-COMPONENT"} {
		if strings.Contains(stdout, secret) {
			t.Fatalf("stdout leaked %q: %q", secret, stdout)
		}
	}
	if !strings.Contains(stdout, `"code":"unknown"`) {
		t.Fatalf("stdout = %q, want the free-form code reduced to unknown", stdout)
	}
	// The rendered error string is the fixed redacted message (plus the agent
	// name, which is not a secret); it must not carry any Herder detail either.
	for _, secret := range []string{secretPrompt, "SECRET-OP", "SECRET-FREEFORM-CODE", "SECRET-MESSAGE"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("err.Error() leaked %q: %q", secret, err.Error())
		}
	}
}

func TestSpawnPartialFailureWriteErrorJoinsChain(t *testing.T) {
	cause := &herdr.Error{Operation: "SendText", Code: "agent_blocked", Message: "blocked"}
	result := internalagent.SpawnResult{Name: "rev", Harness: "claude", Model: "opus", PaneID: "ws1:tab2:pane3"}
	spawnErr := fmt.Errorf("spawn agent %q: %w", result.Name, &internalagent.InitialPromptError{Cause: cause})

	writer := &failingWriter{}
	command := newCommand(func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
		return result, spawnErr
	}, func(int) bool { return false }, testResolver(nil))
	// Mirror the production root's SilenceUsage so Cobra does not attempt a
	// second write of usage text to the failing writer.
	command.SilenceUsage = true
	command.SetOut(writer)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"rev", "--harness", "claude", "--model", "opus", "--prompt", "hello"})

	err := command.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want the joined error")
	}
	var promptErr *internalagent.InitialPromptError
	if !errors.As(err, &promptErr) {
		t.Fatalf("errors.As did not find InitialPromptError in %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is did not find the cause in %v", err)
	}
	if !strings.Contains(err.Error(), "write partial spawn result") {
		t.Fatalf("err = %v, want the wrapped write error joined in", err)
	}
	if writer.writes != 1 {
		t.Fatalf("writer.writes = %d, want exactly one encoding attempt", writer.writes)
	}
}

func TestFirstInitialPromptErrorClassification(t *testing.T) {
	genuine := &internalagent.InitialPromptError{Cause: &herdr.Error{Code: "agent_blocked"}}
	for _, test := range []struct {
		name       string
		err        error
		wantStatus markerStatus
		wantNonNil bool
	}{
		{name: "nil", err: nil, wantStatus: markerAbsent},
		{name: "ordinary", err: errors.New("x"), wantStatus: markerAbsent},
		{name: "direct actual marker", err: genuine, wantStatus: markerFound, wantNonNil: true},
		{name: "fmt-wrapped actual marker", err: fmt.Errorf("spawn agent %q: %w", "rev", genuine), wantStatus: markerFound, wantNonNil: true},
		{name: "join ordinary before marker", err: errors.Join(errors.New("a"), genuine), wantStatus: markerFound, wantNonNil: true},
		{name: "direct typed-nil marker", err: (*internalagent.InitialPromptError)(nil), wantStatus: markerFound, wantNonNil: false},
		{name: "fmt-wrapped typed-nil marker", err: fmt.Errorf("wrap: %w", (*internalagent.InitialPromptError)(nil)), wantStatus: markerFound, wantNonNil: false},
		{name: "join typed-nil before marker", err: errors.Join(error((*internalagent.InitialPromptError)(nil)), genuine), wantStatus: markerFound, wantNonNil: false},
		{name: "forge As hook", err: forgeAsError{forged: genuine}, wantStatus: markerAbsent},
		{name: "blind As hook", err: blindAsError{}, wantStatus: markerAbsent},
		{name: "join ordinary and ordinary", err: errors.Join(errors.New("a"), errors.New("b")), wantStatus: markerAbsent},
		{name: "self cycle", err: newSelfCycle(), wantStatus: markerTruncated},
		{name: "branch cycle", err: newBranchCycle(), wantStatus: markerTruncated},
		{name: "truncated branch before marker", err: errors.Join(newSelfCycle(), genuine), wantStatus: markerTruncated},
	} {
		t.Run(test.name, func(t *testing.T) {
			budget := maxMarkerWork
			got, status := firstInitialPromptError(test.err, 0, &budget)
			if status != test.wantStatus {
				t.Fatalf("status = %v, want %v", status, test.wantStatus)
			}
			if (got != nil) != test.wantNonNil {
				t.Fatalf("marker non-nil = %v, want %v (got %v)", got != nil, test.wantNonNil, got)
			}
			// The shared work budget is spent one unit per visited node and must
			// never be driven negative, so total work stays bounded by maxMarkerWork.
			if budget < 0 || budget > maxMarkerWork {
				t.Fatalf("budget = %d, want within [0, %d]", budget, maxMarkerWork)
			}
		})
	}
}

func TestReportSpawnErrorRejectsUntrustedMarkers(t *testing.T) {
	genuine := &internalagent.InitialPromptError{Cause: &herdr.Error{Code: "agent_blocked"}}
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "ordinary error", err: errors.New("ordinary")},
		{name: "forged As hook", err: forgeAsError{forged: genuine}},
		{name: "blind As hook", err: blindAsError{}},
		{name: "direct typed-nil marker", err: error((*internalagent.InitialPromptError)(nil))},
		{name: "fmt-wrapped typed-nil marker", err: fmt.Errorf("wrap: %w", (*internalagent.InitialPromptError)(nil))},
		{name: "join ordinary branches", err: errors.Join(errors.New("a"), errors.New("b"))},
		{name: "typed-nil marker before genuine", err: errors.Join(error((*internalagent.InitialPromptError)(nil)), genuine)},
		{name: "self cycle", err: newSelfCycle()},
		{name: "branch cycle", err: newBranchCycle()},
		{name: "truncated branch before genuine", err: errors.Join(newSelfCycle(), genuine)},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			result := internalagent.SpawnResult{Name: "rev", Harness: "claude", Model: "opus", PaneID: "ws1:tab2:pane3"}
			got := reportSpawnError(&out, result, test.err)
			if out.Len() != 0 {
				t.Fatalf("stdout = %q, want no partial JSON for an untrusted error", out.String())
			}
			if got != test.err {
				t.Fatalf("returned error = %v, want the original error unchanged", got)
			}
		})
	}
}

func TestReportSpawnErrorIgnoresForgedAndBlindAsHooks(t *testing.T) {
	forged := &internalagent.InitialPromptError{Cause: &herdr.Error{Code: "agent_blocked"}}
	hostile := forgeAsError{forged: forged}
	// The forge succeeds at the errors.As layer — this is exactly why the trust
	// decision must not use errors.As.
	var viaAs *internalagent.InitialPromptError
	if !errors.As(hostile, &viaAs) || viaAs != forged {
		t.Fatal("expected errors.As to be fooled by the forged As hook")
	}
	var out bytes.Buffer
	if got := reportSpawnError(&out, internalagent.SpawnResult{Name: "rev"}, hostile); got != error(hostile) {
		t.Fatalf("returned error = %v, want the original hostile error", got)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want no partial JSON despite the forged marker", out.String())
	}

	// A blind As hook reports true with the target untouched, the shape that made
	// the old errors.As path render a nil marker.
	blind := blindAsError{}
	var viaBlind *internalagent.InitialPromptError
	if !errors.As(blind, &viaBlind) || viaBlind != nil {
		t.Fatal("expected errors.As to report the blind hook true with a nil target")
	}
	out.Reset()
	if got := reportSpawnError(&out, internalagent.SpawnResult{Name: "rev"}, blind); got != error(blind) {
		t.Fatalf("returned error = %v, want the original blind error", got)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want no partial JSON for the blind As hook", out.String())
	}
}

func TestReportSpawnErrorGenuineMarkerVariantsEmitPartialJSON(t *testing.T) {
	result := internalagent.SpawnResult{Name: "rev", Harness: "claude", Model: "opus", WorkspaceID: "ws1", TabID: "ws1:tab2", PaneID: "ws1:tab2:pane3"}
	want := `{"name":"rev","harness":"claude","model":"opus","profile":"","workspace_id":"ws1","tab_id":"ws1:tab2","pane_id":"ws1:tab2:pane3","initial_prompt":{"status":"delivery_unconfirmed","code":"agent_blocked","retry_argv":["fledge","agent","message","rev","--","\u003cprompt\u003e"]}}` + "\n"
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "direct marker", err: &internalagent.InitialPromptError{Cause: &herdr.Error{Code: "agent_blocked"}}},
		{name: "fmt-wrapped marker", err: fmt.Errorf("spawn agent %q: %w", "rev", &internalagent.InitialPromptError{Cause: &herdr.Error{Code: "agent_blocked"}})},
		{name: "join ordinary before marker", err: errors.Join(errors.New("noise"), &internalagent.InitialPromptError{Cause: &herdr.Error{Code: "agent_blocked"}})},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			got := reportSpawnError(&out, result, test.err)
			if out.String() != want {
				t.Fatalf("output = %q,\n want      %q", out.String(), want)
			}
			if got != test.err {
				t.Fatalf("returned error = %v, want the original error unchanged", got)
			}
		})
	}
}

func TestFirstInitialPromptErrorTerminatesOnAdversarialGraphs(t *testing.T) {
	mutualA := &cycleError{}
	mutualB := &cycleError{}
	mutualA.next = mutualB
	mutualB.next = mutualA

	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "self cycle", err: newSelfCycle()},
		{name: "mutual cycle", err: mutualA},
		{name: "branch cycle", err: newBranchCycle()},
		{name: "huge fanout", err: &fanoutError{children: make([]error, 100000)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			done := make(chan markerStatus, 1)
			go func() {
				budget := maxMarkerWork
				_, status := firstInitialPromptError(test.err, 0, &budget)
				done <- status
			}()
			select {
			case status := <-done:
				if status == markerFound {
					t.Fatal("status = markerFound, want absent or truncated for an adversarial graph")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("traversal did not terminate within the watchdog window")
			}
		})
	}
}
