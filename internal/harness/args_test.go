package harness

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildArgsMapsModelsForEveryHarness(t *testing.T) {
	for harnessID, want := range map[string][]string{
		"claude":   {"--model", "provider/custom-model", "--permission-mode", "bypassPermissions", "--native", "value"},
		"codex":    {"--model", "provider/custom-model", "--native", "value"},
		"pi":       {"--model", "provider/custom-model", "--native", "value"},
		"opencode": {"--model", "provider/custom-model", "--native", "value"},
	} {
		t.Run(harnessID, func(t *testing.T) {
			got, err := BuildArgs(Harness{ID: harnessID}, "provider/custom-model", []string{"--native", "value"})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("BuildArgs() = %v, want %v", got, want)
			}
		})
	}
}

func TestBuildArgsHarnessDefaultOmitsModel(t *testing.T) {
	got, err := BuildArgs(Harness{ID: "codex"}, "", []string{"--search"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"--search"}; !reflect.DeepEqual(got, want) {
		t.Errorf("BuildArgs() = %v, want %v", got, want)
	}
}

func TestBuildArgsAcceptsUndiscoveredCustomModel(t *testing.T) {
	const customID = "private-provider/model-not-in-catalog"
	got, err := BuildArgs(Harness{ID: "pi"}, customID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"--model", customID}; !reflect.DeepEqual(got, want) {
		t.Errorf("BuildArgs() = %v, want %v", got, want)
	}
}

func TestBuildArgsPreservesNativeArgumentsWithoutAliasing(t *testing.T) {
	backing := []string{"untouched", "--one", "value", "--two=2", "tail"}
	native := backing[1:4]
	wantBacking := append([]string(nil), backing...)

	got, err := BuildArgs(Harness{ID: "claude"}, "sonnet", native)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--model", "sonnet", "--permission-mode", "bypassPermissions", "--one", "value", "--two=2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuildArgs() = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(backing, wantBacking) {
		t.Fatalf("BuildArgs() mutated input backing array: got %v, want %v", backing, wantBacking)
	}
	got[len(got)-1] = "changed"
	if !reflect.DeepEqual(backing, wantBacking) {
		t.Errorf("BuildArgs() result aliases input: got backing %v, want %v", backing, wantBacking)
	}
}

func TestBuildArgsClaudeDefaultsToBypassPermissions(t *testing.T) {
	got, err := BuildArgs(Harness{ID: "claude"}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"--permission-mode", "bypassPermissions"}; !reflect.DeepEqual(got, want) {
		t.Errorf("BuildArgs() = %v, want %v", got, want)
	}
}

func TestBuildArgsClaudeBypassFollowsModel(t *testing.T) {
	got, err := BuildArgs(Harness{ID: "claude"}, "sonnet", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--model", "sonnet", "--permission-mode", "bypassPermissions"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuildArgs() = %v, want %v", got, want)
	}
}

func TestBuildArgsClaudeUserPermissionFlagSuppressesDefault(t *testing.T) {
	for name, native := range map[string][]string{
		"separate_value": {"--permission-mode", "plan"},
		"equals_value":   {"--permission-mode=plan"},
		"skip_flag":      {"--dangerously-skip-permissions"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := BuildArgs(Harness{ID: "claude"}, "", native)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, native) {
				t.Errorf("BuildArgs() = %v, want %v", got, native)
			}
		})
	}
}

func TestBuildArgsOtherHarnessesGetNoPermissionMode(t *testing.T) {
	for _, harnessID := range []string{"codex", "pi", "opencode"} {
		t.Run(harnessID, func(t *testing.T) {
			got, err := BuildArgs(Harness{ID: harnessID}, "", []string{"--native"})
			if err != nil {
				t.Fatal(err)
			}
			if want := []string{"--native"}; !reflect.DeepEqual(got, want) {
				t.Errorf("BuildArgs() = %v, want %v", got, want)
			}
		})
	}
}

func TestValidateNativeArgsRejectsModelFlags(t *testing.T) {
	for _, arg := range []string{"--model", "--model=opus", "--model=", "-m", "-mopus", "-m=opus"} {
		t.Run(strings.ReplaceAll(arg, "=", "_equals_"), func(t *testing.T) {
			err := ValidateNativeArgs([]string{"--safe", arg, "value"})
			if err == nil {
				t.Fatalf("ValidateNativeArgs(%q) error = nil", arg)
			}
			if !strings.Contains(err.Error(), `native argument 2`) || !strings.Contains(err.Error(), arg) {
				t.Errorf("error = %q, want argument index and value", err)
			}
		})
	}
}

func TestValidateNativeArgsPreservesOtherArguments(t *testing.T) {
	for _, args := range [][]string{
		{"--model-provider", "custom"},
		{"--", "positional"},
		{"--safe", "-x", "value"},
	} {
		if err := ValidateNativeArgs(args); err != nil {
			t.Errorf("ValidateNativeArgs(%v) error = %v", args, err)
		}
	}
}

func TestBuildArgsRejectsUnsupportedHarness(t *testing.T) {
	got, err := BuildArgs(Harness{ID: "other"}, "model", nil)
	if err == nil || !strings.Contains(err.Error(), `unsupported harness "other"`) {
		t.Fatalf("BuildArgs() = %v, %v", got, err)
	}
}

func TestBuildArgsValidatesNativeArgumentsBeforeHarness(t *testing.T) {
	_, err := BuildArgs(Harness{ID: "other"}, "model", []string{"--model=duplicate"})
	if err == nil || !strings.Contains(err.Error(), "native argument") {
		t.Fatalf("BuildArgs() error = %v", err)
	}
}

func TestAppendOrchestratorInstructions(t *testing.T) {
	const instructions = "First line\nquoted: \"value\"; Unicode: 雪"
	const promptPath = ".fledge/profiles/generated/orchestrator.md"
	tests := []struct {
		harnessID string
		want      []string
	}{
		{harnessID: "claude", want: []string{"--user", "value", "--append-system-prompt-file", promptPath}},
		{harnessID: "codex", want: []string{"--user", "value", "-c", `developer_instructions="First line\nquoted: \"value\"; Unicode: 雪"`}},
		{harnessID: "pi", want: []string{"--user", "value", "--append-system-prompt", promptPath}},
		{harnessID: "opencode", want: []string{"--user", "value"}},
	}
	for _, test := range tests {
		t.Run(test.harnessID, func(t *testing.T) {
			input := []string{"--user", "value"}
			got, err := AppendOrchestratorInstructions(Harness{ID: test.harnessID}, input, instructions, promptPath)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("AppendOrchestratorInstructions() = %#v, want %#v", got, test.want)
			}
			got[0] = "changed"
			if input[0] != "--user" {
				t.Fatal("AppendOrchestratorInstructions() result aliases input")
			}
		})
	}
}

func TestAppendOrchestratorInstructionsFledgeOverrideIsLast(t *testing.T) {
	for _, test := range []struct {
		harnessID string
		ownedFlag string
	}{
		{harnessID: "claude", ownedFlag: "--append-system-prompt-file"},
		{harnessID: "codex", ownedFlag: "-c"},
		{harnessID: "pi", ownedFlag: "--append-system-prompt"},
	} {
		t.Run(test.harnessID, func(t *testing.T) {
			got, err := AppendOrchestratorInstructions(
				Harness{ID: test.harnessID},
				[]string{test.ownedFlag, "user value"},
				"Fledge value",
				".fledge/profiles/generated/orchestrator.md",
			)
			if err != nil {
				t.Fatal(err)
			}
			if got[len(got)-2] != test.ownedFlag {
				t.Fatalf("args = %#v, want final owned flag %q", got, test.ownedFlag)
			}
		})
	}
}

func TestCodexOrchestratorInstructionsAreTOMLSafe(t *testing.T) {
	const instructions = "quote \" slash \\ newline\n tab\t delete\x7f snow 雪"
	got, err := AppendOrchestratorInstructions(Harness{ID: "codex"}, nil, instructions, ".fledge/profiles/generated/orchestrator.md")
	if err != nil {
		t.Fatal(err)
	}
	want := `developer_instructions="quote \" slash \\ newline\n tab\t delete\u007F snow 雪"`
	if len(got) != 2 || got[0] != "-c" || got[1] != want {
		t.Fatalf("Codex args = %#v, want [-c %q]", got, want)
	}
}
