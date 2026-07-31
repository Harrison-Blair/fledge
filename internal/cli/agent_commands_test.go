package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentspawn"
	"github.com/Harrison-Blair/fledge/internal/fledge"
	"github.com/Harrison-Blair/fledge/internal/picker"
)

func TestPromptRequiresExactlyOneSource(t *testing.T) {
	for _, args := range [][]string{
		{"agent", "prompt", "worker"},
		{"agent", "prompt", "worker", "hello", "--file", "prompt.txt"},
	} {
		var stderr bytes.Buffer
		if code := Execute(context.Background(), args, bytes.NewBuffer(nil), &bytes.Buffer{}, &stderr); code != 2 {
			t.Fatalf("%v: exit=%d stderr=%s", args, code, stderr.String())
		}
	}
}

func TestAgentStartWasRemoved(t *testing.T) {
	var stderr bytes.Buffer
	code := Execute(context.Background(), []string{"agent", "start", "worker", "--kind", "codex"},
		bytes.NewBuffer(nil), &bytes.Buffer{}, &stderr)
	if code != 2 || (!strings.Contains(stderr.String(), "unknown command") &&
		!strings.Contains(stderr.String(), "unknown flag")) {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
}

func TestSpawnNonTTYRequiresNameAndHarnessBeforeRuntimeAccess(t *testing.T) {
	for _, args := range [][]string{
		{"agent", "spawn"},
		{"agent", "spawn", "--name", "worker"},
		{"agent", "spawn", "--harness", "codex"},
		{"agent", "spawn", "--json"},
	} {
		var stderr bytes.Buffer
		code := Execute(context.Background(), args, bytes.NewBuffer(nil), &bytes.Buffer{}, &stderr)
		if code != 2 || !strings.Contains(stderr.String(), "--name and --harness") {
			t.Fatalf("%v: exit=%d stderr=%s", args, code, stderr.String())
		}
	}
}

func TestPiModelPickerItemsUseProviderAndCreatorHierarchy(t *testing.T) {
	items := modelPickerItems("pi", []agentspawn.Model{
		{Name: "Harness default", Default: true},
		{
			ID: "openai-codex/gpt-5", Name: "gpt-5",
			Provider: "openai-codex", Maker: "OpenAI",
		},
		{
			ID: "opencode-go/anthropic/claude-4", Name: "anthropic/claude-4",
			Provider: "opencode-go", Maker: "Claude",
		},
		{
			ID: "opencode/google/gemini-2.5-pro", Name: "google/gemini-2.5-pro",
			Provider: "opencode", Maker: "Google",
		},
		{
			ID: "custom/model", Name: "model",
			Provider: "custom", Maker: "Custom",
		},
	})
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.Group+"/"+item.Subgroup)
	}
	want := []string{"/", "OpenAI Codex/", "OpenCode Go/Claude", "OpenCode Zen/Google", "Custom/"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("picker hierarchy = %v, want %v", got, want)
	}
}

func TestSpawnNativeArgumentsRequireDelimiter(t *testing.T) {
	var stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"agent", "spawn", "--name", "worker", "--harness", "codex", "prompt",
	}, bytes.NewBuffer(nil), &bytes.Buffer{}, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "must follow --") {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
}

func TestSpawnRejectsNativeModelFlagAndAcceptsCustomTopLevelModel(t *testing.T) {
	root := initializedProject(t)
	env := &environment{
		in: bytes.NewBuffer(nil), out: &bytes.Buffer{}, errOut: &bytes.Buffer{},
		stdinTTY: func() bool { return false },
		lookPath: func(name string) (string, error) { return "/bin/" + name, nil },
		getenv:   func(string) string { return "" },
		cwd:      root, stateDir: t.TempDir(), herdrBin: "/does/not/exist",
	}
	cmd := newAgentSpawn(env)
	cmd.SetContext(t.Context())
	if err := runAgentSpawn(cmd, env, agentSpawnFlags{
		name: "worker", harness: "codex", timeout: 30 * time.Second,
		nativeArgs: []string{"--model", "native"},
	}); err == nil || !strings.Contains(err.Error(), "must not contain") {
		t.Fatalf("duplicate model error = %v", err)
	}
	for _, args := range [][]string{{"--model=native"}, {"-mnative"}, {"-m=native"}} {
		if !duplicateModelFlag(args) {
			t.Fatalf("duplicate model syntax not detected: %v", args)
		}
	}

	// Custom launch IDs are accepted during argument validation and do not
	// trigger catalog discovery; runtime access is the next expected failure.
	err := runAgentSpawn(cmd, env, agentSpawnFlags{
		name: "worker", harness: "codex", model: "broker/custom-model",
		timeout: 30 * time.Second,
	})
	if err == nil || strings.Contains(err.Error(), "model") {
		t.Fatalf("custom model was rejected during validation: %v", err)
	}
}

func TestUnknownIsOnlySentWhenExplicitlySelected(t *testing.T) {
	if err := validateStates(nil); err != nil {
		t.Fatal(err)
	}
	if err := validateStates([]string{"unknown"}); err != nil {
		t.Fatal(err)
	}
	if err := validateStates([]string{"stopped"}); err == nil {
		t.Fatal("expected stopped to be rejected as a Herdr wait state")
	}
}

func TestHandlePickerResultTranslatesCancellationAndFailure(t *testing.T) {
	failure := errors.New("terminal unavailable")
	for _, test := range []struct {
		name          string
		err           error
		wantCancelled bool
		wantOutput    string
		wantCode      string
	}{
		{name: "selection made", err: nil},
		{name: "cancelled", err: picker.ErrCancelled, wantCancelled: true, wantOutput: "Cancelled.\n"},
		{
			name: "wrapped cancellation", err: fmt.Errorf("harness picker: %w", picker.ErrCancelled),
			wantCancelled: true, wantOutput: "Cancelled.\n",
		},
		{name: "picker failure", err: failure, wantCode: "picker_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			env := &environment{out: &stdout, errOut: &stderr}

			cancelled, err := handlePickerResult(env, test.err)

			if cancelled != test.wantCancelled {
				t.Fatalf("cancelled=%t want %t", cancelled, test.wantCancelled)
			}
			if stdout.String() != test.wantOutput {
				t.Fatalf("output=%q want %q", stdout.String(), test.wantOutput)
			}
			if stderr.String() != "" {
				t.Fatalf("stderr=%q want empty", stderr.String())
			}
			if test.wantCode == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var serviceErr *fledge.Error
			if !errors.As(err, &serviceErr) || serviceErr.Code != test.wantCode {
				t.Fatalf("error=%v want code %s", err, test.wantCode)
			}
			if serviceErr.Message != test.err.Error() {
				t.Fatalf("message=%q want %q", serviceErr.Message, test.err.Error())
			}
			if !errors.Is(err, test.err) {
				t.Fatal("wrapped error lost its cause")
			}
		})
	}
}
