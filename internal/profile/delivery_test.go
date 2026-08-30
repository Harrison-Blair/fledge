package profile

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestLaunchArgsUsesNativeDelivery(t *testing.T) {
	tests := []struct {
		name         string
		harness      string
		instructions string
		path         string
		args         []string
		want         []string
	}{
		{
			name:    "pi file",
			harness: "pi",
			path:    "/runtime/profile.md",
			args:    []string{"--thinking", "high"},
			want:    []string{"--append-system-prompt", "/runtime/profile.md", "--thinking", "high"},
		},
		{
			name:    "claude file",
			harness: "claude",
			path:    "/runtime/profile.md",
			args:    []string{"--effort", "xhigh"},
			want:    []string{"--append-system-prompt-file", "/runtime/profile.md", "--effort", "xhigh"},
		},
		{
			name:         "codex TOML",
			harness:      "codex",
			instructions: "say \"quoted\"\nC:\\tmp\\file\t\b\f\r\v snowman ☃",
			path:         "/unused/profile.md",
			args:         []string{"-c", `model_reasoning_effort="xhigh"`},
			want: []string{
				"-c",
				`developer_instructions="say \"quoted\"\nC:\\tmp\\file\t\b\f\r\u000B snowman ☃"`,
				"-c",
				`model_reasoning_effort="xhigh"`,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configured := Profile{Name: "test", Instructions: test.instructions}
			got, err := LaunchArgs(configured, test.harness, test.path, test.args)
			if err != nil {
				t.Fatalf("LaunchArgs() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("LaunchArgs() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestLaunchArgsDoesNotMutateCallerArgs(t *testing.T) {
	args := make([]string, 2, 6)
	copy(args, []string{"--effort", "high"})
	got, err := LaunchArgs(Profile{Name: "test"}, "claude", "/profile.md", args)
	if err != nil {
		t.Fatalf("LaunchArgs() error = %v", err)
	}
	got[len(got)-1] = "changed"
	if !reflect.DeepEqual(args, []string{"--effort", "high"}) {
		t.Fatalf("caller args mutated to %#v", args)
	}
}

func TestLaunchArgsRejectsInstructionConflicts(t *testing.T) {
	tests := []struct {
		name    string
		harness string
		args    []string
	}{
		{name: "pi system split", harness: "pi", args: []string{"--system-prompt", "mine"}},
		{name: "pi system equals", harness: "pi", args: []string{"--system-prompt=mine"}},
		{name: "pi append split", harness: "pi", args: []string{"--append-system-prompt", "mine"}},
		{name: "pi append equals", harness: "pi", args: []string{"--append-system-prompt=mine"}},
		{name: "claude system split", harness: "claude", args: []string{"--system-prompt", "mine"}},
		{name: "claude system equals", harness: "claude", args: []string{"--system-prompt=mine"}},
		{name: "claude system file split", harness: "claude", args: []string{"--system-prompt-file", "mine"}},
		{name: "claude system file equals", harness: "claude", args: []string{"--system-prompt-file=mine"}},
		{name: "claude append split", harness: "claude", args: []string{"--append-system-prompt", "mine"}},
		{name: "claude append equals", harness: "claude", args: []string{"--append-system-prompt=mine"}},
		{name: "claude append file split", harness: "claude", args: []string{"--append-system-prompt-file", "mine"}},
		{name: "claude append file equals", harness: "claude", args: []string{"--append-system-prompt-file=mine"}},
		{name: "codex short split", harness: "codex", args: []string{"-c", `developer_instructions="mine"`}},
		{name: "codex short equals", harness: "codex", args: []string{`-c=developer_instructions="mine"`}},
		{name: "codex long split", harness: "codex", args: []string{"--config", `developer_instructions="mine"`}},
		{name: "codex long equals", harness: "codex", args: []string{`--config=developer_instructions="mine"`}},
		{name: "codex spaces around key", harness: "codex", args: []string{"-c", ` developer_instructions = "mine"`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LaunchArgs(Profile{Name: "test"}, test.harness, "/profile.md", test.args)
			var conflict *InstructionArgumentConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("LaunchArgs() error = %v, want InstructionArgumentConflictError", err)
			}
			if conflict.Harness != test.harness {
				t.Fatalf("conflict harness = %q, want %q", conflict.Harness, test.harness)
			}
		})
	}
}

func TestLaunchArgsAllowsUnrelatedArguments(t *testing.T) {
	tests := []struct {
		harness string
		args    []string
	}{
		{harness: "pi", args: []string{"--append-system-prompter=mine", "--thinking", "low"}},
		{harness: "claude", args: []string{"--plugin-dir", "/plugin"}},
		{harness: "codex", args: []string{"-c", `model_reasoning_effort="high"`, "--config=features.search=true"}},
	}
	for _, test := range tests {
		t.Run(test.harness, func(t *testing.T) {
			got, err := LaunchArgs(Profile{Name: "test", Instructions: "profile"}, test.harness, "/profile.md", test.args)
			if err != nil {
				t.Fatalf("LaunchArgs() error = %v", err)
			}
			for _, arg := range test.args {
				if !contains(got, arg) {
					t.Fatalf("LaunchArgs() = %#v, missing unrelated argument %q", got, arg)
				}
			}
		})
	}
}

func TestLaunchArgsRequiresFileForFileBackedHarness(t *testing.T) {
	for _, harness := range []string{"pi", "claude"} {
		t.Run(harness, func(t *testing.T) {
			_, err := LaunchArgs(Profile{Name: "test"}, harness, "", nil)
			if err == nil || !strings.Contains(err.Error(), "instruction file path is empty") {
				t.Fatalf("LaunchArgs() error = %v, want empty path error", err)
			}
		})
	}
}

func TestLaunchArgsReportsCursorUnsupported(t *testing.T) {
	_, err := LaunchArgs(Profile{Name: OrchestratorName}, "cursor", "/profile.md", nil)
	var unsupported *UnsupportedHarnessError
	if !errors.As(err, &unsupported) {
		t.Fatalf("LaunchArgs() error = %v, want UnsupportedHarnessError", err)
	}
	if unsupported.Harness != "cursor" {
		t.Fatalf("unsupported harness = %q, want cursor", unsupported.Harness)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
