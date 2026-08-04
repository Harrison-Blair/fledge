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
