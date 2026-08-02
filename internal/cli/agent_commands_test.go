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
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

func TestMessageSendRequiresExactlyOneSource(t *testing.T) {
	for _, args := range [][]string{
		{"agent", "message", "send", "worker"},
		{"agent", "message", "send", "worker", "hello", "--file", "message.txt"},
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

func TestAgentStopHelpDescribesDefaultDedicatedTabCleanup(t *testing.T) {
	cmd := newAgentStop(&environment{})
	if !strings.Contains(cmd.Short, "clean up its dedicated tab") {
		t.Fatalf("stop help = %q", cmd.Short)
	}
	if got := cmd.Flags().Lookup("force").Usage; !strings.Contains(got, "skip graceful shutdown") {
		t.Fatalf("force help = %q", got)
	}
	if cmd.Flags().Lookup("close-tab") != nil {
		t.Fatal("stop unexpectedly exposes an opt-in close-tab flag")
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

func TestHarnessPickerItemsPutSearchableLastSelectionFirst(t *testing.T) {
	installed := []agentspawn.Harness{
		{ID: "claude", Name: "Claude Code"},
		{ID: "codex", Name: "Codex"},
	}
	for _, test := range []struct {
		name      string
		last      *state.SpawnSelection
		wantFirst string
		wantCount int
	}{
		{
			name: "saved model", last: &state.SpawnSelection{Harness: "codex", Model: "gpt-5.6"},
			wantFirst: "Last used — Codex · gpt-5.6", wantCount: 3,
		},
		{
			name: "default model", last: &state.SpawnSelection{Harness: "claude"},
			wantFirst: "Last used — Claude Code · default model", wantCount: 3,
		},
		{
			name: "custom model", last: &state.SpawnSelection{Harness: "codex", Model: "broker/custom-model"},
			wantFirst: "Last used — Codex · broker/custom-model", wantCount: 3,
		},
		{
			name: "missing harness", last: &state.SpawnSelection{Harness: "pi", Model: "provider/model"},
			wantFirst: "Claude Code", wantCount: 2,
		},
		{name: "no history", wantFirst: "Claude Code", wantCount: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			items := harnessPickerItems(installed, test.last)
			if len(items) != test.wantCount || items[0].Title != test.wantFirst {
				t.Fatalf("items = %#v", items)
			}
			if strings.HasPrefix(test.wantFirst, "Last used") {
				if !items[1].SeparatorBefore {
					t.Fatalf("first regular option has no separator: %#v", items)
				}
				matches := picker.Filter(items, strings.TrimPrefix(test.wantFirst, "Last used — "))
				if len(matches) != 1 || matches[0].ID != lastSpawnSelectionItemID {
					t.Fatalf("last selection was not searchable: %#v", matches)
				}
			}
		})
	}
}

func TestResolveSpawnSelectionShortcutRestoresMissingValuesAndHonorsExplicitModel(t *testing.T) {
	installed := []agentspawn.Harness{{ID: "codex", Name: "Codex", Path: "/bin/codex"}}
	for _, test := range []struct {
		name           string
		lastModel      string
		requestedModel string
		wantModel      string
	}{
		{name: "saved model", lastModel: "gpt-5.6", wantModel: "gpt-5.6"},
		{name: "saved default", wantModel: ""},
		{name: "saved custom", lastModel: "broker/custom", wantModel: "broker/custom"},
		{
			name: "explicit model wins", lastModel: "gpt-5.6",
			requestedModel: "override/model", wantModel: "override/model",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			env := &environment{
				in: bytes.NewBufferString("\r"), out: &output, errOut: &bytes.Buffer{},
			}
			cmd := newAgentSpawn(env)
			cmd.SetContext(t.Context())
			selection, cancelled, err := resolveSpawnSelection(
				cmd, env, installed, "", test.requestedModel,
				&state.SpawnSelection{Harness: "codex", Model: test.lastModel}, true,
			)
			if err != nil || cancelled || selection.Harness.ID != "codex" ||
				selection.Model != test.wantModel || !selection.Remember {
				t.Fatalf("selection=%#v cancelled=%t err=%v", selection, cancelled, err)
			}
			if strings.Contains(output.String(), "Codex model") {
				t.Fatalf("shortcut opened a redundant model picker: %q", output.String())
			}
		})
	}
}

func TestResolveSpawnSelectionOnlyPickerParticipationIsRemembered(t *testing.T) {
	installed := []agentspawn.Harness{
		{ID: "claude", Name: "Claude Code", Path: "/bin/claude"},
		{ID: "codex", Name: "Codex", Path: "/bin/codex"},
	}
	for _, test := range []struct {
		name             string
		input            string
		requestedHarness string
		requestedModel   string
		wantRemember     bool
		wantHarness      string
	}{
		{
			name: "name prompt only", requestedHarness: "codex", requestedModel: "gpt-5.6",
			wantHarness: "codex", wantRemember: false,
		},
		{
			name: "harness picker", input: "\r", requestedModel: "sonnet",
			wantHarness: "claude", wantRemember: true,
		},
		{
			name: "model picker", input: "\r", requestedHarness: "claude",
			wantHarness: "claude", wantRemember: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := &environment{
				in: bytes.NewBufferString(test.input), out: &bytes.Buffer{}, errOut: &bytes.Buffer{},
			}
			cmd := newAgentSpawn(env)
			cmd.SetContext(t.Context())
			selection, cancelled, err := resolveSpawnSelection(
				cmd, env, installed, test.requestedHarness, test.requestedModel, nil, true,
			)
			if err != nil || cancelled || selection.Harness.ID != test.wantHarness ||
				selection.Remember != test.wantRemember {
				t.Fatalf("selection=%#v cancelled=%t err=%v", selection, cancelled, err)
			}
		})
	}

	env := &environment{
		in: bytes.NewBufferString("\x1b"), out: &bytes.Buffer{}, errOut: &bytes.Buffer{},
	}
	cmd := newAgentSpawn(env)
	cmd.SetContext(t.Context())
	selection, cancelled, err := resolveSpawnSelection(cmd, env, installed, "", "sonnet", nil, true)
	if err != nil || !cancelled || selection != (resolvedSpawnSelection{}) {
		t.Fatalf("cancelled selection=%#v cancelled=%t err=%v", selection, cancelled, err)
	}
}

func TestReadLastSpawnSelectionIsIsolatedPerProject(t *testing.T) {
	stateDir := t.TempDir()
	rootWithHistory := initializedProject(t)
	rootWithoutHistory := initializedProject(t)
	store, err := state.New(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	want := state.SpawnSelection{Harness: "codex", Model: "gpt-5.6"}
	if err := store.WithLocked(project.SessionName(rootWithHistory), rootWithHistory, func(st *state.Session) error {
		selection := want
		st.LastSpawnSelection = &selection
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	selection, err := readLastSpawnSelection(&environment{cwd: rootWithHistory, stateDir: stateDir})
	if err != nil || selection == nil || *selection != want {
		t.Fatalf("project selection = %#v, err=%v", selection, err)
	}
	selection, err = readLastSpawnSelection(&environment{cwd: rootWithoutHistory, stateDir: stateDir})
	if err != nil || selection != nil {
		t.Fatalf("unrelated project selection = %#v, err=%v", selection, err)
	}
}

func TestSpawnNativeArgumentsRequireDelimiter(t *testing.T) {
	var stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"agent", "spawn", "profile", "prompt",
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
