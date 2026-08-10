package tui

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestValidateAgentName(t *testing.T) {
	t.Parallel()

	valid := []string{"a", "orchestrator", "worker-1", "agent_name", "a1234567890123456789012345678901"}
	for _, name := range valid {
		if err := ValidateAgentName(name); err != nil {
			t.Errorf("ValidateAgentName(%q) error = %v", name, err)
		}
	}

	invalid := []string{"", "A", "1agent", "agent.name", "agent name", "a12345678901234567890123456789012"}
	for _, name := range invalid {
		if err := ValidateAgentName(name); err == nil {
			t.Errorf("ValidateAgentName(%q) error = nil, want error", name)
		}
	}
}

func TestClassifyCaller(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input CallerInput
		want  CallerKind
	}{
		{name: "outside Herdr", input: CallerInput{}, want: CallerDirectUser},
		{name: "snapshot failed", input: CallerInput{PaneID: "pane-1"}, want: CallerUnknown},
		{name: "recognized harness", input: CallerInput{PaneID: "pane-1", SessionAgentsAvailable: true, Agents: []PaneAgent{{PaneID: "pane-1", Harness: "codex"}}}, want: CallerAgent},
		{name: "adapter recognized", input: CallerInput{PaneID: "pane-1", SessionAgentsAvailable: true, Agents: []PaneAgent{{PaneID: "pane-1", Harness: "custom", Recognized: true}}}, want: CallerAgent},
		{name: "known shell pane", input: CallerInput{PaneID: "pane-1", SessionAgentsAvailable: true, PaneIDs: []string{"pane-1"}}, want: CallerDirectUser},
		{name: "different session pane", input: CallerInput{PaneID: "pane-1", SessionAgentsAvailable: true, PaneIDs: []string{"pane-2"}, Agents: []PaneAgent{{PaneID: "pane-2", Harness: "claude"}}}, want: CallerUnknown},
		{name: "unrecognized occupant", input: CallerInput{PaneID: "pane-1", SessionAgentsAvailable: true, PaneIDs: []string{"pane-1"}, Agents: []PaneAgent{{PaneID: "pane-1", Harness: "shell"}}}, want: CallerDirectUser},
		{name: "unrecognized harness and pane absent from layout", input: CallerInput{PaneID: "pane-1", SessionAgentsAvailable: true, PaneIDs: []string{"pane-2"}, Agents: []PaneAgent{{PaneID: "pane-1", Harness: "shell"}}}, want: CallerUnknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyCaller(test.input); got != test.want {
				t.Errorf("ClassifyCaller() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestPromptsAllowed(t *testing.T) {
	t.Parallel()

	if !PromptsAllowed(true, true, CallerDirectUser) {
		t.Error("PromptsAllowed() = false for terminal direct user")
	}
	for _, input := range []struct {
		stdin, stdout bool
		caller        CallerKind
	}{{false, true, CallerDirectUser}, {true, false, CallerDirectUser}, {true, true, CallerAgent}, {true, true, CallerUnknown}} {
		if PromptsAllowed(input.stdin, input.stdout, input.caller) {
			t.Errorf("PromptsAllowed(%v, %v, %v) = true", input.stdin, input.stdout, input.caller)
		}
	}
}

// TestSelectorPromptOrdering table-drives the interactive harness/model prompt
// flow. Each case pins the selection, the exact prompt-call order, and — via its
// own verify hook — the bespoke choice-list assertions (which index of
// seenChoices matters differs per case). The models factory closes over the
// subtest t so a case can assert the loader's harness argument or assert it is
// never called.
func TestSelectorPromptOrdering(t *testing.T) {
	t.Parallel()

	harnessCodexPi := []Choice{{Value: "codex", Label: "Codex"}, {Value: "pi", Label: "Pi"}}

	tests := []struct {
		name          string
		harnesses     []Choice
		lastUsed      *LastUsed
		chosen        []string
		models        func(t *testing.T) func(context.Context, string) ([]Choice, error)
		wantSelection Selection
		wantCalls     []string
		verify        func(t *testing.T, prompts *fakePromptRunner)
	}{
		{
			name:      "prompts harness then model in order",
			harnesses: harnessCodexPi,
			chosen:    []string{"pi", "openai-codex/gpt-5"},
			models: func(t *testing.T) func(context.Context, string) ([]Choice, error) {
				return func(_ context.Context, harness string) ([]Choice, error) {
					if harness != "pi" {
						t.Errorf("model harness = %q, want pi", harness)
					}
					return []Choice{{Value: "openai-codex/gpt-5", Label: "GPT-5"}}, nil
				}
			},
			wantSelection: Selection{Name: "worker-1", Harness: "pi", Model: "openai-codex/gpt-5", Prompted: true},
			wantCalls:     []string{"choose:Select harness:false", "choose:Select model:true", "text:Agent name"},
			verify: func(t *testing.T, prompts *fakePromptRunner) {
				if len(prompts.seenChoices[1]) != 2 || prompts.seenChoices[1][0].Label != "Harness default" {
					t.Errorf("model choices = %#v, want prepended harness default", prompts.seenChoices[1])
				}
			},
		},
		{
			name:      "last used selects harness and model, skipping the model prompt",
			harnesses: harnessCodexPi,
			lastUsed:  &LastUsed{Harness: "pi", Model: "openai-codex/gpt-5"},
			chosen:    []string{lastUsedValue},
			models: func(t *testing.T) func(context.Context, string) ([]Choice, error) {
				return func(context.Context, string) ([]Choice, error) {
					t.Error("model loader called, want skipped")
					return nil, nil
				}
			},
			wantSelection: Selection{Name: "worker-1", Harness: "pi", Model: "openai-codex/gpt-5", Prompted: true},
			wantCalls:     []string{"choose:Select harness:false", "text:Agent name"},
			verify: func(t *testing.T, prompts *fakePromptRunner) {
				want := append([]Choice{{Value: lastUsedValue, Label: "Last used (pi · openai-codex/gpt-5)"}}, harnessCodexPi...)
				if !reflect.DeepEqual(prompts.seenChoices[0], want) {
					t.Errorf("harness choices = %#v, want %#v", prompts.seenChoices[0], want)
				}
			},
		},
		{
			name:      "last used with harness-default model skips the model prompt",
			harnesses: []Choice{{Value: "claude", Label: "Claude"}},
			lastUsed:  &LastUsed{Harness: "claude"},
			chosen:    []string{lastUsedValue},
			models: func(t *testing.T) func(context.Context, string) ([]Choice, error) {
				return func(context.Context, string) ([]Choice, error) {
					t.Error("model loader called, want skipped")
					return nil, nil
				}
			},
			wantSelection: Selection{Name: "worker-1", Harness: "claude", Prompted: true},
			wantCalls:     []string{"choose:Select harness:false", "text:Agent name"},
			verify: func(t *testing.T, prompts *fakePromptRunner) {
				if got := prompts.seenChoices[0][0].Label; got != "Last used (claude · harness default)" {
					t.Errorf("last used label = %q, want %q", got, "Last used (claude · harness default)")
				}
			},
		},
		{
			name:      "declining last used still prompts for the model",
			harnesses: []Choice{{Value: "codex", Label: "Codex"}},
			lastUsed:  &LastUsed{Harness: "pi", Model: "openai-codex/gpt-5"},
			chosen:    []string{"codex", "openai-codex/gpt-5"},
			models: func(t *testing.T) func(context.Context, string) ([]Choice, error) {
				return func(context.Context, string) ([]Choice, error) {
					return []Choice{{Value: "openai-codex/gpt-5", Label: "GPT-5"}}, nil
				}
			},
			wantSelection: Selection{Name: "worker-1", Harness: "codex", Model: "openai-codex/gpt-5", Prompted: true},
			wantCalls:     []string{"choose:Select harness:false", "choose:Select model:true", "text:Agent name"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			prompts := &fakePromptRunner{choices: test.chosen, text: "worker-1"}
			selector := testSelector(t, true, prompts)
			selection, err := selector.Select(context.Background(), SelectionRequest{
				Harnesses: test.harnesses,
				LastUsed:  test.lastUsed,
				Models:    test.models(t),
			})
			if err != nil {
				t.Fatal(err)
			}
			if selection != test.wantSelection {
				t.Errorf("Select() = %#v, want %#v", selection, test.wantSelection)
			}
			if !reflect.DeepEqual(prompts.calls, test.wantCalls) {
				t.Errorf("prompt calls = %#v, want %#v", prompts.calls, test.wantCalls)
			}
			if test.verify != nil {
				test.verify(t, prompts)
			}
		})
	}
}

func TestSelectorExplicitValuesSkipPrompts(t *testing.T) {
	t.Parallel()

	prompts := &fakePromptRunner{}
	selector := testSelector(t, true, prompts)
	got, err := selector.Select(context.Background(), SelectionRequest{
		Name: "worker", Harness: "codex", Model: "custom/model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := (Selection{Name: "worker", Harness: "codex", Model: "custom/model"}); got != want {
		t.Errorf("Select() = %#v, want %#v", got, want)
	}
	if len(prompts.calls) != 0 {
		t.Errorf("prompt calls = %#v, want none", prompts.calls)
	}
	if got.Prompted {
		t.Error("Prompted = true, want false")
	}
}

func TestSelectorNoninteractiveRequirements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request SelectionRequest
		wantErr bool
	}{
		{name: "complete", request: SelectionRequest{Name: "worker", Harness: "pi"}},
		{name: "missing name", request: SelectionRequest{Harness: "pi"}, wantErr: true},
		{name: "missing harness", request: SelectionRequest{Name: "worker"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			selector := testSelector(t, false, &fakePromptRunner{})
			got, err := selector.Select(context.Background(), test.request)
			if (err != nil) != test.wantErr {
				t.Fatalf("Select() error = %v, wantErr %v", err, test.wantErr)
			}
			if !test.wantErr && got.Model != "" {
				t.Errorf("default model = %q, want empty", got.Model)
			}
		})
	}
}

func TestSelectorAgentAndUnknownCallersCannotPrompt(t *testing.T) {
	t.Parallel()

	for name, caller := range map[string]CallerInput{
		"agent":   {PaneID: "1", SessionAgentsAvailable: true, Agents: []PaneAgent{{PaneID: "1", Harness: "opencode"}}},
		"unknown": {PaneID: "1"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			selector := testSelector(t, true, &fakePromptRunner{})
			_, err := selector.Select(context.Background(), SelectionRequest{Caller: caller})
			if err == nil || !strings.Contains(err.Error(), "noninteractive") {
				t.Errorf("Select() error = %v, want noninteractive error", err)
			}
		})
	}
}

func TestChoiceModelNavigationFilteringAndSelection(t *testing.T) {
	t.Parallel()

	model := newChoiceModel("Model", []Choice{
		{Value: "anthropic/sonnet", Label: "Sonnet", Group: "Anthropic"},
		{Value: "openai-codex/gpt-5", Label: "GPT-5", Group: "OpenAI Codex"},
		{Value: "opencode-go/minimax", Label: "MiniMax", Group: "OpenCode Go"},
	}, true)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("codex")})
	model = updated.(choiceModel)
	if len(model.visible) != 1 || model.choices[model.visible[0]].Value != "openai-codex/gpt-5" {
		t.Fatalf("filtered visible choices = %#v", model.visible)
	}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(choiceModel)
	if !model.selected || model.value != "openai-codex/gpt-5" || command == nil {
		t.Errorf("selected model = %#v, command nil = %v", model, command == nil)
	}

	model = newChoiceModel("Harness", []Choice{{Value: "a"}, {Value: "b"}}, false)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(choiceModel)
	if model.cursor != 1 {
		t.Errorf("cursor after down = %d, want 1", model.cursor)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := updated.(choiceModel).cursor; got != 0 {
		t.Errorf("cursor after up = %d, want 0", got)
	}
}

func TestPickerModelsCancel(t *testing.T) {
	t.Parallel()

	choice := newChoiceModel("Model", []Choice{{Value: "default"}}, true)
	updated, command := choice.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !updated.(choiceModel).cancelled || command == nil {
		t.Error("choice escape did not cancel and quit")
	}

	text := textModel{title: "Name", validate: ValidateAgentName}
	updated, command = text.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !updated.(textModel).cancelled || command == nil {
		t.Error("text ctrl+c did not cancel and quit")
	}
}

func TestTextModelRequiresValidName(t *testing.T) {
	t.Parallel()

	model := textModel{title: "Name", value: "Bad", validate: ValidateAgentName}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(textModel)
	if model.err == nil || command != nil {
		t.Errorf("invalid enter error = %v, command = %v", model.err, command)
	}
	model.value = "good-name"
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(textModel).err != nil || command == nil {
		t.Errorf("valid enter error = %v, command nil = %v", updated.(textModel).err, command == nil)
	}
}

func TestChoiceModelViewGroupedLayout(t *testing.T) {
	t.Parallel()

	choices := []Choice{
		{Value: "", Label: "Harness default", Group: "OpenAI Codex"},
		{Value: "gpt-5.1-codex", Group: "OpenAI Codex"},
		{Value: "", Label: "Harness default", Group: "OpenCode Go / Claude"},
		{Value: "claude-sonnet-5", Group: "OpenCode Go / Claude"},
	}

	model := newChoiceModel("Select model", choices, true)
	want := "Select model (type to filter): \n" +
		"OpenAI Codex\n" +
		"  > Harness default\n" +
		"    gpt-5.1-codex\n" +
		"OpenCode Go / Claude\n" +
		"    Harness default\n" +
		"    claude-sonnet-5\n"
	if got := model.View(); got != want {
		t.Errorf("View() = %q, want %q", got, want)
	}

	t.Run("filtered keeps group indentation", func(t *testing.T) {
		t.Parallel()
		filtered, _ := newChoiceModel("Select model", choices, true).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("gpt")})
		want := "Select model (type to filter): gpt\n" +
			"OpenAI Codex\n" +
			"  > gpt-5.1-codex\n"
		if got := filtered.(choiceModel).View(); got != want {
			t.Errorf("View() = %q, want %q", got, want)
		}
	})

	t.Run("ungrouped stays flush left", func(t *testing.T) {
		t.Parallel()
		got := newChoiceModel("Select harness", []Choice{{Value: "a"}, {Value: "b"}}, false).View()
		if want := "Select harness\n> a\n  b\n"; got != want {
			t.Errorf("View() = %q, want %q", got, want)
		}
	})
}

func TestSelectPrependsGroupDefaultsPerGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		models []Choice
		want   []Choice
	}{
		{
			name: "catalog shape",
			models: []Choice{
				{Value: "", Label: "Harness default"},
				{Value: "openai-codex/gpt-5", Label: "GPT-5", Group: "OpenAI Codex"},
				{Value: "opencode-go/claude-sonnet-5", Label: "Sonnet 5", Group: "OpenCode Go"},
			},
			want: []Choice{
				{Value: "", Label: "Harness default", Group: "OpenAI Codex"},
				{Value: "openai-codex/gpt-5", Label: "GPT-5", Group: "OpenAI Codex"},
				{Value: "", Label: "Harness default", Group: "OpenCode Go"},
				{Value: "opencode-go/claude-sonnet-5", Label: "Sonnet 5", Group: "OpenCode Go"},
			},
		},
		{
			name: "already has per-group defaults",
			models: []Choice{
				{Value: "", Label: "Harness default", Group: "OpenAI Codex"},
				{Value: "openai-codex/gpt-5", Label: "GPT-5", Group: "OpenAI Codex"},
				{Value: "", Label: "Harness default", Group: "OpenCode Go"},
				{Value: "opencode-go/claude-sonnet-5", Label: "Sonnet 5", Group: "OpenCode Go"},
			},
			want: []Choice{
				{Value: "", Label: "Harness default", Group: "OpenAI Codex"},
				{Value: "openai-codex/gpt-5", Label: "GPT-5", Group: "OpenAI Codex"},
				{Value: "", Label: "Harness default", Group: "OpenCode Go"},
				{Value: "opencode-go/claude-sonnet-5", Label: "Sonnet 5", Group: "OpenCode Go"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			prompts := &fakePromptRunner{choices: []string{"openai-codex/gpt-5"}, text: "worker-1"}
			selector := testSelector(t, true, prompts)
			if _, err := selector.Select(context.Background(), SelectionRequest{
				Harness: "pi",
				Models:  func(context.Context, string) ([]Choice, error) { return test.models, nil },
			}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(prompts.seenChoices[0], test.want) {
				t.Errorf("model choices = %#v, want %#v", prompts.seenChoices[0], test.want)
			}
		})
	}
}

func TestChoiceModelFilterEditing(t *testing.T) {
	t.Parallel()

	choices := []Choice{{Value: "codex", Label: "Codex"}, {Value: "claude", Label: "Claude"}}
	model := newChoiceModel("Harness", choices, true)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("cla")})
	model = updated.(choiceModel)
	if model.filter != "cla" || len(model.visible) != 1 || model.choices[model.visible[0]].Value != "claude" {
		t.Fatalf("after typing: filter=%q visible=%#v", model.filter, model.visible)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(choiceModel)
	if model.filter != "cl" || len(model.visible) != 1 || model.choices[model.visible[0]].Value != "claude" {
		t.Fatalf("after backspace: filter=%q visible=%#v", model.filter, model.visible)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model = updated.(choiceModel)
	if model.filter != "" || len(model.visible) != len(choices) {
		t.Fatalf("after ctrl+u: filter=%q visible=%#v", model.filter, model.visible)
	}

	// A non-filterable model ignores filter-editing keys entirely.
	plain := newChoiceModel("Harness", choices, false)
	updated, _ = plain.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if got := updated.(choiceModel).filter; got != "" {
		t.Errorf("non-filterable filter = %q, want empty", got)
	}
}

func TestTextModelBackspaceClearsError(t *testing.T) {
	t.Parallel()

	model := textModel{title: "Name", value: "Bad", validate: ValidateAgentName}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(textModel)
	if model.err == nil {
		t.Fatal("expected validation error after invalid enter")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(textModel)
	if model.err != nil {
		t.Errorf("err after backspace = %v, want cleared", model.err)
	}
	if model.value != "Ba" {
		t.Errorf("value after backspace = %q, want %q", model.value, "Ba")
	}
}

func TestSelectorWrapsPromptErrorsPerStage(t *testing.T) {
	t.Parallel()

	failure := errors.New("prompt boom")
	tests := []struct {
		name    string
		request SelectionRequest
		want    string
	}{
		{
			name:    "harness stage",
			request: SelectionRequest{Harnesses: []Choice{{Value: "codex"}}},
			want:    "select harness:",
		},
		{
			name: "model stage",
			request: SelectionRequest{
				Harness: "pi",
				Models:  func(context.Context, string) ([]Choice, error) { return []Choice{{Value: "m"}}, nil },
			},
			want: "select model:",
		},
		{
			name:    "name stage",
			request: SelectionRequest{Harness: "pi", Model: "custom/model"},
			want:    "select agent name:",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			prompts := &fakePromptRunner{err: failure}
			selector := testSelector(t, true, prompts)
			_, err := selector.Select(context.Background(), test.request)
			if err == nil || !strings.Contains(err.Error(), test.want) || !errors.Is(err, failure) {
				t.Fatalf("Select() error = %v, want %q wrapping %v", err, test.want, failure)
			}
		})
	}
}

func TestSelectorWrapsModelLoaderFailure(t *testing.T) {
	t.Parallel()

	failure := errors.New("catalog down")
	prompts := &fakePromptRunner{choices: []string{"ignored"}, text: "worker-1"}
	selector := testSelector(t, true, prompts)
	_, err := selector.Select(context.Background(), SelectionRequest{
		Harness: "pi",
		Models:  func(context.Context, string) ([]Choice, error) { return nil, failure },
	})
	if err == nil || !strings.Contains(err.Error(), "load models for pi:") || !errors.Is(err, failure) {
		t.Fatalf("Select() error = %v, want 'load models for pi:' wrapping %v", err, failure)
	}
}

func TestSelectorRequiresInstalledHarnesses(t *testing.T) {
	t.Parallel()

	prompts := &fakePromptRunner{}
	selector := testSelector(t, true, prompts)
	_, err := selector.Select(context.Background(), SelectionRequest{})
	if err == nil || !strings.Contains(err.Error(), "no installed harnesses are available") {
		t.Fatalf("Select() error = %v, want no-harnesses error", err)
	}
	if len(prompts.calls) != 0 {
		t.Errorf("prompt calls = %#v, want none", prompts.calls)
	}
}

func TestWithHarnessDefaultsDoesNotDoublePrependDefault(t *testing.T) {
	t.Parallel()

	// Ungrouped choices that already carry a harness-default entry are returned
	// unchanged rather than gaining a second one.
	choices := []Choice{{Value: "", Label: "Harness default"}, {Value: "gpt-5", Label: "GPT-5"}}
	got := withHarnessDefaults(choices)
	if !reflect.DeepEqual(got, choices) {
		t.Errorf("withHarnessDefaults() = %#v, want unchanged %#v", got, choices)
	}
}

func testSelector(t *testing.T, terminal bool, prompts PromptRunner) *Selector {
	t.Helper()
	input, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	output, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		input.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { input.Close(); output.Close() })
	return NewSelectorWithDependencies(input, output, func(uintptr) bool { return terminal }, prompts)
}

type fakePromptRunner struct {
	choices     []string
	text        string
	err         error
	calls       []string
	seenChoices [][]Choice
}

func (f *fakePromptRunner) Choose(_ context.Context, title string, choices []Choice, filterable bool) (string, error) {
	f.calls = append(f.calls, "choose:"+title+":"+map[bool]string{true: "true", false: "false"}[filterable])
	f.seenChoices = append(f.seenChoices, append([]Choice(nil), choices...))
	if f.err != nil {
		return "", f.err
	}
	if len(f.choices) == 0 {
		return "", errors.New("unexpected choice prompt")
	}
	choice := f.choices[0]
	f.choices = f.choices[1:]
	return choice, nil
}

func (f *fakePromptRunner) Text(_ context.Context, title string, _ func(string) error) (string, error) {
	f.calls = append(f.calls, "text:"+title)
	return f.text, f.err
}
