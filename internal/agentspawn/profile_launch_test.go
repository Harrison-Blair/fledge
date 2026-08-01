package agentspawn

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestBuildProfileArgsMappings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts ProfileLaunchOptions
		want []string
	}{
		{
			name: "claude",
			opts: ProfileLaunchOptions{Harness: "claude", Effort: "high", Instructions: "Follow the profile."},
			want: []string{"--effort", "high", "--append-system-prompt", "Follow the profile."},
		},
		{
			name: "codex",
			opts: ProfileLaunchOptions{Harness: "codex", Effort: "high", Instructions: "Follow the profile."},
			want: []string{
				"--config", `model_reasoning_effort="high"`,
				"--config", `developer_instructions="Follow the profile."`,
			},
		},
		{
			name: "pi",
			opts: ProfileLaunchOptions{Harness: "pi", Effort: "high"},
			want: []string{"--thinking", "high"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := BuildProfileArgs(test.opts)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("args = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestBuildProfileArgsMapsEverySupportedEffort(t *testing.T) {
	t.Parallel()

	harnessFlags := map[string]string{
		"claude": "--effort", "codex": "--config", "pi": "--thinking",
	}
	for harness, flag := range harnessFlags {
		for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
			t.Run(harness+"/"+effort, func(t *testing.T) {
				t.Parallel()
				got, err := BuildProfileArgs(ProfileLaunchOptions{Harness: harness, Effort: effort})
				if err != nil {
					t.Fatal(err)
				}
				wantValue := effort
				if harness == "codex" {
					wantValue = `model_reasoning_effort="` + effort + `"`
				}
				want := []string{flag, wantValue}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("args = %#v, want %#v", got, want)
				}
			})
		}
	}
}

func TestBuildProfileArgsRejectsUnsupportedOpenCodeProfileControls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, effort, instructions, setting string
	}{
		{name: "effort", effort: "high", setting: "managed effort"},
		{name: "instructions", instructions: "Review carefully.", setting: "managed instructions"},
		{name: "both", effort: "high", instructions: "Review carefully.", setting: "managed effort and instructions"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := BuildProfileArgs(ProfileLaunchOptions{
				Harness: "opencode", Effort: test.effort, Instructions: test.instructions,
			})
			if err == nil {
				t.Fatal("expected unsupported OpenCode transport error")
			}
			for _, fragment := range []string{"opencode", test.setting, "interactive OpenCode TUI", "unsupported"} {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("error %q does not contain %q", err, fragment)
				}
			}
		})
	}
}

func TestBuildProfileArgsRejectsPiInstructionsMatchingExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(path, []byte("file contents must not replace literal instructions"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := BuildProfileArgs(ProfileLaunchOptions{Harness: "pi", Instructions: path})
	if err == nil {
		t.Fatal("expected unsupported Pi instruction transport error")
	}
	for _, fragment := range []string{"pi", "managed instructions", "file paths", "unsupported"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q does not contain %q", err, fragment)
		}
	}
}

func TestBuildProfileArgsOmitsEmptyOptionalValues(t *testing.T) {
	t.Parallel()

	for _, harness := range []string{"claude", "codex", "opencode", "pi"} {
		t.Run(harness, func(t *testing.T) {
			t.Parallel()
			got, err := BuildProfileArgs(ProfileLaunchOptions{Harness: harness})
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || len(got) != 0 {
				t.Fatalf("args = %#v, want non-nil empty slice", got)
			}
		})
	}
}

func TestBuildProfileArgsEscapesCodexInstructionsAsConfigString(t *testing.T) {
	t.Parallel()

	got, err := BuildProfileArgs(ProfileLaunchOptions{
		Harness: "codex", Instructions: "quote: \"yes\"\npath: C:\\tmp\nmarker: <profile>",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"--config",
		`developer_instructions="quote: \"yes\"\npath: C:\\tmp\nmarker: \u003cprofile\u003e"`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestBuildProfileArgsPreservesNativeArgsInStableOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		harness string
		native  []string
	}{
		{harness: "claude", native: []string{"--verbose", "-dworker", "--prompt-suggestions", "--debug-file", "trace.log", "--plugin-dir=plugins/review"}},
		{harness: "codex", native: []string{"--search", "--image", "diagram.png", "--no-alt-screen"}},
		{harness: "opencode", native: []string{"--print-logs", "--logLevel", "DEBUG", "--mini"}},
		{harness: "pi", native: []string{"--verbose", "-ne", "--extension", "extensions/review.ts", "--offline"}},
	}

	for _, test := range tests {
		t.Run(test.harness, func(t *testing.T) {
			t.Parallel()
			got, err := BuildProfileArgs(ProfileLaunchOptions{Harness: test.harness, NativeArgs: test.native})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got[len(got)-len(test.native):], test.native) {
				t.Fatalf("passthrough = %#v, want %#v", got, test.native)
			}
		})
	}
}

func TestBuildProfileArgsPreservesVariadicNativeValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, harness string
		native        []string
	}{
		{
			name: "claude files", harness: "claude",
			native: []string{"--file", "file_a:a", "file_b:b", "--verbose"},
		},
		{
			name: "codex images", harness: "codex",
			native: []string{"--image", "a.png", "b.png", "--search"},
		},
		{
			name: "codex images short", harness: "codex",
			native: []string{"-i", "a.png", "b.png", "--search"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := BuildProfileArgs(ProfileLaunchOptions{Harness: test.harness, NativeArgs: test.native})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.native) {
				t.Fatalf("args = %#v, want %#v", got, test.native)
			}
		})
	}
}

func TestBuildProfileArgsRejectsPositionalsAfterVariadicOptionsEnd(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		harness string
		args    []string
	}{
		{harness: "claude", args: []string{"--file", "file_a:a", "file_b:b", "--verbose", "smuggled prompt"}},
		{harness: "codex", args: []string{"--image", "a.png", "b.png", "--search", "smuggled prompt"}},
	} {
		t.Run(test.harness, func(t *testing.T) {
			t.Parallel()
			_, err := BuildProfileArgs(ProfileLaunchOptions{Harness: test.harness, NativeArgs: test.args})
			if err == nil || !strings.Contains(err.Error(), "positional prompts") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestBuildProfileArgsRejectsControlledNativeArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, harness, setting string
		args                   []string
	}{
		// Claude: separated/inline forms plus its relevant short aliases.
		{name: "claude model separated", harness: "claude", args: []string{"--model", "opus"}, setting: "model selection"},
		{name: "claude model inline", harness: "claude", args: []string{"--model=opus"}, setting: "model selection"},
		{name: "claude effort", harness: "claude", args: []string{"--effort=max"}, setting: "reasoning effort"},
		{name: "claude instructions", harness: "claude", args: []string{"--system-prompt", "replace"}, setting: "profile instructions"},
		{name: "claude appended instructions", harness: "claude", args: []string{"--append-system-prompt=replace"}, setting: "profile instructions"},
		{name: "claude agent identity", harness: "claude", args: []string{"--agent=reviewer"}, setting: "harness/profile identity"},
		{name: "claude project root", harness: "claude", args: []string{"--add-dir", "/tmp"}, setting: "working directory/project root"},
		{name: "claude permission", harness: "claude", args: []string{"--permission-mode=plan"}, setting: "permission/sandbox policy"},
		{name: "claude resume short", harness: "claude", args: []string{"-r=session"}, setting: "session/name placement"},
		{name: "claude name joined", harness: "claude", args: []string{"-nworker"}, setting: "session/name placement"},

		// Codex: config is entirely reserved because arbitrary keys can override
		// every managed setting.
		{name: "codex model separated", harness: "codex", args: []string{"-m", "native"}, setting: "model selection"},
		{name: "codex model joined", harness: "codex", args: []string{"-mnative"}, setting: "model selection"},
		{name: "codex model equals", harness: "codex", args: []string{"-m=native"}, setting: "model selection"},
		{name: "codex model long inline", harness: "codex", args: []string{"--model=native"}, setting: "model selection"},
		{name: "codex config separated", harness: "codex", args: []string{"--config", `model="native"`}, setting: "harness/profile configuration"},
		{name: "codex config inline", harness: "codex", args: []string{`--config=model_reasoning_effort="low"`}, setting: "harness/profile configuration"},
		{name: "codex config short", harness: "codex", args: []string{`-cdeveloper_instructions="replace"`}, setting: "harness/profile configuration"},
		{name: "codex profile", harness: "codex", args: []string{"-pmanaged"}, setting: "harness/profile identity"},
		{name: "codex cwd", harness: "codex", args: []string{"-C/tmp"}, setting: "working directory/project root"},
		{name: "codex sandbox", harness: "codex", args: []string{"-sdanger-full-access"}, setting: "permission/sandbox policy"},
		{name: "codex approval", harness: "codex", args: []string{"--ask-for-approval=never"}, setting: "permission/sandbox policy"},
		{name: "codex remote separated", harness: "codex", args: []string{"--remote", "ws://attacker.invalid"}, setting: "remote session placement"},
		{name: "codex remote inline", harness: "codex", args: []string{"--remote=ws://attacker.invalid"}, setting: "remote session placement"},
		{name: "codex remote auth separated", harness: "codex", args: []string{"--remote-auth-token-env", "OPENAI_API_KEY"}, setting: "remote session authentication"},
		{name: "codex remote auth inline", harness: "codex", args: []string{"--remote-auth-token-env=OPENAI_API_KEY"}, setting: "remote session authentication"},
		{name: "codex hook trust", harness: "codex", args: []string{"--dangerously-bypass-hook-trust"}, setting: "permission/sandbox policy"},
		{name: "codex hook trust inline", harness: "codex", args: []string{"--dangerously-bypass-hook-trust=true"}, setting: "permission/sandbox policy"},
		{name: "codex name", harness: "codex", args: []string{"--name=other"}, setting: "session/name placement"},
		{name: "codex bundled short", harness: "codex", args: []string{"-Vmnative"}, setting: "model selection"},

		// OpenCode controls its project positional, agent, variant, prompt,
		// permissions, and session selection.
		{name: "opencode model separated", harness: "opencode", args: []string{"-m", "provider/model"}, setting: "model selection"},
		{name: "opencode model inline", harness: "opencode", args: []string{"--model=provider/model"}, setting: "model selection"},
		{name: "opencode effort", harness: "opencode", args: []string{"--variant", "max"}, setting: "reasoning effort"},
		{name: "opencode instructions", harness: "opencode", args: []string{"--prompt=replace"}, setting: "profile instructions"},
		{name: "opencode instructions short", harness: "opencode", args: []string{"-preplace"}, setting: "profile instructions"},
		{name: "opencode identity", harness: "opencode", args: []string{"--agent", "plan"}, setting: "harness/profile identity"},
		{name: "opencode cwd", harness: "opencode", args: []string{"--dir=/tmp"}, setting: "working directory/project root"},
		{name: "opencode permission", harness: "opencode", args: []string{"--auto"}, setting: "permission/sandbox policy"},
		{name: "opencode dangerous kebab", harness: "opencode", args: []string{"--dangerously-skip-permissions"}, setting: "permission/sandbox policy"},
		{name: "opencode dangerous kebab inline", harness: "opencode", args: []string{"--dangerously-skip-permissions=true"}, setting: "permission/sandbox policy"},
		{name: "opencode dangerous camel", harness: "opencode", args: []string{"--dangerouslySkipPermissions"}, setting: "permission/sandbox policy"},
		{name: "opencode dangerous camel inline", harness: "opencode", args: []string{"--dangerouslySkipPermissions=true"}, setting: "permission/sandbox policy"},
		{name: "opencode session", harness: "opencode", args: []string{"-s=session"}, setting: "session/name placement"},
		{name: "opencode title", harness: "opencode", args: []string{"--title=other"}, setting: "session/name placement"},

		// Pi exposes long and multi-character short forms for the same managed
		// surfaces.
		{name: "pi model separated", harness: "pi", args: []string{"--model", "native"}, setting: "model selection"},
		{name: "pi provider inline", harness: "pi", args: []string{"--provider=openai"}, setting: "model selection"},
		{name: "pi effort", harness: "pi", args: []string{"--thinking=xhigh"}, setting: "reasoning effort"},
		{name: "pi instructions", harness: "pi", args: []string{"--append-system-prompt", "replace"}, setting: "profile instructions"},
		{name: "pi context short", harness: "pi", args: []string{"-nc"}, setting: "profile instructions"},
		{name: "pi tools short", harness: "pi", args: []string{"-nt"}, setting: "permission/sandbox policy"},
		{name: "pi exclude tools joined", harness: "pi", args: []string{"-xtbash"}, setting: "permission/sandbox policy"},
		{name: "pi approval short", harness: "pi", args: []string{"-na"}, setting: "permission/sandbox policy"},
		{name: "pi session", harness: "pi", args: []string{"--session-id=other"}, setting: "session/name placement"},
		{name: "pi name short", harness: "pi", args: []string{"-nother"}, setting: "session/name placement"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := BuildProfileArgs(ProfileLaunchOptions{Harness: test.harness, NativeArgs: test.args})
			if err == nil {
				t.Fatal("expected collision error")
			}
			for _, fragment := range []string{test.harness, strconv.Quote(test.args[0]), test.setting, "native argument 1"} {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("error %q does not contain %q", err, fragment)
				}
			}
		})
	}
}

func TestBuildProfileArgsRejectsMalformedAndSmuggledNativeArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, harness string
		args          []string
		want          string
	}{
		{name: "empty", harness: "claude", args: []string{""}, want: "must not be empty"},
		{name: "nul", harness: "codex", args: []string{"--search\x00hidden"}, want: "NUL byte"},
		{name: "invalid UTF-8", harness: "opencode", args: []string{"--flag=" + string([]byte{0xff})}, want: "valid UTF-8"},
		{name: "terminator", harness: "pi", args: []string{"--", "--model", "native"}, want: "option terminators"},
		{name: "lone dash", harness: "claude", args: []string{"-"}, want: "lone dash"},
		{name: "positional prompt", harness: "codex", args: []string{"do work"}, want: "positional prompts"},
		{name: "positional after boolean", harness: "opencode", args: []string{"--print-logs", "project"}, want: "positional prompts"},
		{name: "malformed long", harness: "pi", args: []string{"--=value"}, want: "expected --flag"},
		{name: "invalid long name", harness: "claude", args: []string{"--bad_flag=value"}, want: "expected --flag"},
		{name: "empty inline value", harness: "codex", args: []string{"--image="}, want: "must not be empty"},
		{name: "empty short variadic value", harness: "codex", args: []string{"-i="}, want: "must not be empty"},
		{name: "empty short value", harness: "pi", args: []string{"-e="}, want: "must not be empty"},
		{name: "missing known value", harness: "opencode", args: []string{"--log-level"}, want: "requires a value"},
		{name: "flag smuggled as known value", harness: "pi", args: []string{"--extension", "--model=native"}, want: "requires a value before"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := BuildProfileArgs(ProfileLaunchOptions{Harness: test.harness, NativeArgs: test.args})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want fragment %q", err, test.want)
			}
			if !strings.Contains(err.Error(), test.harness) {
				t.Fatalf("error %q lacks harness context", err)
			}
		})
	}
}

func TestBuildProfileArgsRejectsUnsupportedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts ProfileLaunchOptions
		want string
	}{
		{name: "empty harness", opts: ProfileLaunchOptions{}, want: `harness ""`},
		{name: "unknown harness", opts: ProfileLaunchOptions{Harness: "gemini"}, want: `harness "gemini"`},
		{name: "case changed harness", opts: ProfileLaunchOptions{Harness: "Codex"}, want: `harness "Codex"`},
		{name: "unknown effort", opts: ProfileLaunchOptions{Harness: "codex", Effort: "ultra"}, want: `effort "ultra"`},
		{name: "case changed effort", opts: ProfileLaunchOptions{Harness: "pi", Effort: "HIGH"}, want: `effort "HIGH"`},
		{name: "whitespace effort", opts: ProfileLaunchOptions{Harness: "claude", Effort: " "}, want: `effort " "`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := BuildProfileArgs(test.opts)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want fragment %q", err, test.want)
			}
		})
	}
}

func TestBuildProfileArgsRejectsMalformedInstructions(t *testing.T) {
	t.Parallel()

	for name, instructions := range map[string]string{
		"nul":           "before\x00after",
		"invalid UTF-8": string([]byte{0xff}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := BuildProfileArgs(ProfileLaunchOptions{Harness: "codex", Instructions: instructions})
			if err == nil || !strings.Contains(err.Error(), "profile instructions") ||
				!strings.Contains(err.Error(), "codex") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestBuildProfileArgsDoesNotMutateOrAliasNativeArgs(t *testing.T) {
	t.Parallel()

	backing := []string{"sentinel", "--verbose", "tail"}
	native := backing[1:2]
	wantBacking := append([]string(nil), backing...)
	wantNative := append([]string(nil), native...)

	got, err := BuildProfileArgs(ProfileLaunchOptions{
		Harness: "claude", Effort: "high", Instructions: "managed", NativeArgs: native,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(backing, wantBacking) || !reflect.DeepEqual(native, wantNative) {
		t.Fatalf("input mutated: backing=%#v native=%#v", backing, native)
	}

	got[len(got)-1] = "--changed"
	if !reflect.DeepEqual(backing, wantBacking) || !reflect.DeepEqual(native, wantNative) {
		t.Fatalf("result aliases input: backing=%#v native=%#v", backing, native)
	}
}
