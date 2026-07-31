package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentspawn"
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
