package picker

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"fledge/internal/catalog"
	"fledge/internal/profile"
)

type selectCall struct {
	title   string
	options []Option
}

type scriptedPrompts struct {
	answers    []Option
	selectErr  error
	calls      []selectCall
	confirmed  bool
	confirmErr error
	questions  []string
}

func (s *scriptedPrompts) selectFn(_ io.Reader, _ io.Writer, title string, options []Option) (Option, error) {
	s.calls = append(s.calls, selectCall{title: title, options: append([]Option(nil), options...)})
	if s.selectErr != nil {
		return Option{}, s.selectErr
	}
	if len(s.answers) < len(s.calls) {
		return Option{}, errors.New("unscripted selection")
	}
	return s.answers[len(s.calls)-1], nil
}

func (s *scriptedPrompts) confirmFn(_ io.Reader, _ io.Writer, question string) (bool, error) {
	s.questions = append(s.questions, question)
	return s.confirmed, s.confirmErr
}

type modelLookup struct {
	models map[catalog.Harness][]string
	calls  []catalog.Harness
}

func (l *modelLookup) fn(_ context.Context, harness catalog.Harness) []string {
	l.calls = append(l.calls, harness)
	return append([]string(nil), l.models[harness]...)
}

func testProfile() profile.Profile {
	return profile.Profile{
		Name:         "test-profile",
		Description:  "Test profile",
		Instructions: "test instructions",
		Defaults: profile.Defaults{
			Harness: "claude",
			Model:   "claude-default",
			Args:    []string{"--profile-default"},
		},
	}
}

func testResolver(prompts *scriptedPrompts, lookup *modelLookup, configured ...profile.Profile) Resolver {
	resolver := Resolver{
		Input:   strings.NewReader(""),
		Output:  io.Discard,
		Models:  lookup.fn,
		Select:  prompts.selectFn,
		Confirm: prompts.confirmFn,
	}
	if len(configured) == 0 {
		return resolver
	}

	byName := make(map[string]profile.Profile, len(configured))
	for _, item := range configured {
		byName[item.Name] = item
	}
	resolver.profilesFn = func() []profile.Profile {
		return append([]profile.Profile(nil), configured...)
	}
	resolver.profileFn = func(name string) (profile.Profile, bool) {
		item, ok := byName[name]
		return item, ok
	}
	return resolver
}

func optionTitles(options []Option) []string {
	titles := make([]string, len(options))
	for i, option := range options {
		titles[i] = option.Title
	}
	return titles
}

func TestResolveExplicitChoicesOverrideProfileDefaults(t *testing.T) {
	prompts := &scriptedPrompts{}
	lookup := &modelLookup{}
	configured := testProfile()

	choice, err := testResolver(prompts, lookup, configured).Resolve(context.Background(), LaunchRequest{
		Harness: "codex",
		Model:   "gpt-explicit",
		Profile: configured.Name,
		Args:    []string{"--explicit"},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if choice.Harness != catalog.Codex || choice.Model != "gpt-explicit" {
		t.Fatalf("choice harness/model = %q/%q, want codex/gpt-explicit", choice.Harness, choice.Model)
	}
	if choice.Profile == nil || choice.Profile.Name != configured.Name {
		t.Fatalf("choice profile = %#v, want %q", choice.Profile, configured.Name)
	}
	if want := []string{"--profile-default", "--explicit"}; !reflect.DeepEqual(choice.Args, want) {
		t.Fatalf("choice args = %#v, want %#v", choice.Args, want)
	}
	if len(prompts.calls) != 0 || len(lookup.calls) != 0 {
		t.Fatalf("prompts = %d, model lookups = %#v, want none", len(prompts.calls), lookup.calls)
	}
}

func TestResolveUsesProfileDefaults(t *testing.T) {
	configured := testProfile()
	choice, err := testResolver(&scriptedPrompts{}, &modelLookup{}, configured).Resolve(context.Background(), LaunchRequest{
		Profile: configured.Name,
		Args:    []string{"--explicit"},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if choice.Harness != catalog.Claude || choice.Model != "claude-default" {
		t.Fatalf("choice harness/model = %q/%q, want claude/claude-default", choice.Harness, choice.Model)
	}
	if want := []string{"--profile-default", "--explicit"}; !reflect.DeepEqual(choice.Args, want) {
		t.Fatalf("choice args = %#v, want %#v", choice.Args, want)
	}

	configured.Defaults.Args[0] = "changed"
	if choice.Args[0] != "--profile-default" {
		t.Fatalf("choice args changed through source alias: %#v", choice.Args)
	}
}

func TestResolveNoProfileDisablesDefaultProfile(t *testing.T) {
	choice, err := testResolver(&scriptedPrompts{}, &modelLookup{}, testProfile()).Resolve(context.Background(), LaunchRequest{
		Harness:        "pi",
		Model:          "provider/model",
		NoProfile:      true,
		DefaultProfile: "test-profile",
		Args:           []string{"--raw"},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if choice.Profile != nil {
		t.Fatalf("choice profile = %#v, want nil", choice.Profile)
	}
	if want := []string{"--raw"}; !reflect.DeepEqual(choice.Args, want) {
		t.Fatalf("choice args = %#v, want %#v", choice.Args, want)
	}
}

func TestResolveRejectsInvalidProfileSelectionsBeforeModelDiscovery(t *testing.T) {
	tests := []struct {
		name    string
		request LaunchRequest
		wantErr string
	}{
		{
			name:    "profile and no-profile",
			request: LaunchRequest{Profile: "test-profile", NoProfile: true},
			wantErr: "--profile and --no-profile cannot be used together",
		},
		{
			name:    "literal none",
			request: LaunchRequest{Profile: "none"},
			wantErr: `profile name "none" is reserved; use --no-profile`,
		},
		{
			name:    "literal none default",
			request: LaunchRequest{DefaultProfile: "none"},
			wantErr: `profile name "none" is reserved; use --no-profile`,
		},
		{
			name:    "unknown",
			request: LaunchRequest{Profile: "missing"},
			wantErr: `unknown profile "missing"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lookup := &modelLookup{}
			_, err := testResolver(&scriptedPrompts{}, lookup, testProfile()).Resolve(context.Background(), test.request)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Resolve() error = %v, want containing %q", err, test.wantErr)
			}
			if len(lookup.calls) != 0 {
				t.Fatalf("model lookups = %#v, want none", lookup.calls)
			}
		})
	}
}

func TestResolveRejectsUnsupportedHarnessesBeforeModelDiscovery(t *testing.T) {
	for _, harness := range []string{"opencode", "gemini"} {
		t.Run(harness, func(t *testing.T) {
			lookup := &modelLookup{}
			_, err := testResolver(&scriptedPrompts{}, lookup).Resolve(context.Background(), LaunchRequest{
				Harness: harness,
				Model:   "model",
			})
			if err == nil || !strings.Contains(err.Error(), `unsupported harness "`+harness+`"`) {
				t.Fatalf("Resolve() error = %v, want unsupported harness", err)
			}
			if len(lookup.calls) != 0 {
				t.Fatalf("model lookups = %#v, want none", lookup.calls)
			}
		})
	}
}

func TestResolveNonInteractiveRequiresHarnessAndModel(t *testing.T) {
	tests := []struct {
		name    string
		request LaunchRequest
		wantErr string
	}{
		{name: "harness", request: LaunchRequest{Model: "model"}, wantErr: "harness is required in non-interactive mode; pass --harness"},
		{name: "model", request: LaunchRequest{Harness: "codex"}, wantErr: "model is required in non-interactive mode; pass --model"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := testResolver(&scriptedPrompts{}, &modelLookup{}).Resolve(context.Background(), test.request)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Resolve() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestResolveNonInteractiveProfilePromptMeansNoProfile(t *testing.T) {
	prompts := &scriptedPrompts{}
	choice, err := testResolver(prompts, &modelLookup{}, testProfile()).Resolve(context.Background(), LaunchRequest{
		Harness:       "codex",
		Model:         "gpt",
		PromptProfile: true,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if choice.Profile != nil || len(prompts.calls) != 0 {
		t.Fatalf("profile = %#v, prompts = %d, want nil and none", choice.Profile, len(prompts.calls))
	}
}

func TestResolveInteractiveSpawnPromptsProfileHarnessAndModel(t *testing.T) {
	configured := testProfile()
	configured.Defaults = profile.Defaults{}
	prompts := &scriptedPrompts{answers: []Option{
		{ID: configured.Name, Title: configured.Name},
		{ID: "claude", Title: "claude"},
		{ID: "claude-sonnet", Title: "claude-sonnet"},
	}}
	lookup := &modelLookup{models: map[catalog.Harness][]string{catalog.Claude: {"claude-sonnet"}}}

	choice, err := testResolver(prompts, lookup, configured).Resolve(context.Background(), LaunchRequest{
		PromptProfile: true,
		Interactive:   true,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if choice.Harness != catalog.Claude || choice.Model != "claude-sonnet" || choice.Profile == nil {
		t.Fatalf("choice = %#v, want profiled claude/claude-sonnet", choice)
	}
	if len(prompts.calls) != 3 {
		t.Fatalf("selection count = %d, want 3", len(prompts.calls))
	}
	if got, want := optionTitles(prompts.calls[0].options), []string{noProfileTitle, configured.Name}; !reflect.DeepEqual(got, want) {
		t.Fatalf("profile options = %#v, want %#v", got, want)
	}
	if prompts.calls[0].options[0].ID != "" {
		t.Fatalf("None profile ID = %q, want empty", prompts.calls[0].options[0].ID)
	}
	if prompts.calls[1].title != harnessPrompt {
		t.Fatalf("harness prompt = %q, want %q", prompts.calls[1].title, harnessPrompt)
	}
	if got, want := optionTitles(prompts.calls[1].options), []string{"pi", "claude", "codex", "cursor"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("harness options = %#v, want %#v", got, want)
	}
	if got, want := lookup.calls, []catalog.Harness{catalog.Claude}; !reflect.DeepEqual(got, want) {
		t.Fatalf("model lookups = %#v, want %#v", got, want)
	}
}

func TestResolveInteractiveSpawnCanPickNoProfile(t *testing.T) {
	prompts := &scriptedPrompts{answers: []Option{{Title: noProfileTitle}}}
	choice, err := testResolver(prompts, &modelLookup{}, testProfile()).Resolve(context.Background(), LaunchRequest{
		Harness:       "codex",
		Model:         "gpt",
		PromptProfile: true,
		Interactive:   true,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if choice.Profile != nil {
		t.Fatalf("choice profile = %#v, want nil", choice.Profile)
	}
}

func TestResolveShellOnlyClearsAgentLaunchFields(t *testing.T) {
	configured := testProfile()
	configured.Defaults.Harness = ""
	prompts := &scriptedPrompts{answers: []Option{{Title: shellOnlyTitle}}}
	lookup := &modelLookup{}
	choice, err := testResolver(prompts, lookup, configured).Resolve(context.Background(), LaunchRequest{
		Model:          "unused-model",
		Profile:        configured.Name,
		Args:           []string{"--unused"},
		AllowShellOnly: true,
		Interactive:    true,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !reflect.DeepEqual(choice, LaunchChoice{}) {
		t.Fatalf("choice = %#v, want shell-only zero value", choice)
	}
	if len(lookup.calls) != 0 {
		t.Fatalf("model lookups = %#v, want none", lookup.calls)
	}
}

func TestResolveHarnessPickerIncludesShellOnlyOnlyWhenAllowed(t *testing.T) {
	for _, test := range []struct {
		name    string
		allowed bool
		want    []string
	}{
		{name: "allowed", allowed: true, want: []string{"pi", "claude", "codex", "cursor", shellOnlyTitle}},
		{name: "not allowed", want: []string{"pi", "claude", "codex", "cursor"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			prompts := &scriptedPrompts{answers: []Option{{ID: "pi", Title: "pi"}}}
			choice, err := testResolver(prompts, &modelLookup{}).Resolve(context.Background(), LaunchRequest{
				Model:          "provider/model",
				AllowShellOnly: test.allowed,
				Interactive:    true,
			})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if choice.Harness != catalog.Pi {
				t.Fatalf("choice harness = %q, want pi", choice.Harness)
			}
			if got := optionTitles(prompts.calls[0].options); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("harness options = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestResolveChoosesHarnessBeforeDiscoveringModels(t *testing.T) {
	prompts := &scriptedPrompts{answers: []Option{
		{ID: "codex", Title: "codex"},
		{ID: "gpt-5", Title: "gpt-5"},
	}}
	lookup := &modelLookup{models: map[catalog.Harness][]string{catalog.Codex: {"gpt-5"}}}

	choice, err := testResolver(prompts, lookup).Resolve(context.Background(), LaunchRequest{Interactive: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if choice.Harness != catalog.Codex || choice.Model != "gpt-5" {
		t.Fatalf("choice = %#v, want codex/gpt-5", choice)
	}
	if got, want := lookup.calls, []catalog.Harness{catalog.Codex}; !reflect.DeepEqual(got, want) {
		t.Fatalf("model lookups = %#v, want only chosen harness %#v", got, want)
	}
}

func TestResolveDoesNotDiscoverModelsWhenExplicitModelGiven(t *testing.T) {
	prompts := &scriptedPrompts{answers: []Option{{ID: "codex", Title: "codex"}}}
	lookup := &modelLookup{}
	choice, err := testResolver(prompts, lookup).Resolve(context.Background(), LaunchRequest{
		Model:       "gpt",
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if choice.Harness != catalog.Codex || choice.Model != "gpt" {
		t.Fatalf("choice = %#v, want codex/gpt", choice)
	}
	if len(lookup.calls) != 0 {
		t.Fatalf("model lookups = %#v, want none", lookup.calls)
	}
}

func TestResolveModelPickerKeepsHarnessDefaultChoice(t *testing.T) {
	prompts := &scriptedPrompts{answers: []Option{{Title: defaultTitle}}}
	lookup := &modelLookup{models: map[catalog.Harness][]string{catalog.Pi: {"provider/model"}}}
	choice, err := testResolver(prompts, lookup).Resolve(context.Background(), LaunchRequest{
		Harness:     "pi",
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if choice.Model != "" {
		t.Fatalf("choice model = %q, want harness default", choice.Model)
	}
	if got, want := optionTitles(prompts.calls[0].options), []string{defaultTitle, freeTextTitle, "provider/model"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("model options = %#v, want %#v", got, want)
	}
	if prompts.calls[0].options[0].ID != "" || !prompts.calls[0].options[1].FreeText {
		t.Fatalf("default/free-text options = %#v, want explicit default then free text", prompts.calls[0].options[:2])
	}
}

func TestResolveCursorProfileCompatibility(t *testing.T) {
	tests := []struct {
		name        string
		interactive bool
		confirmed   bool
		confirmErr  error
		wantErr     string
		wantCancel  bool
		wantOK      bool
	}{
		{name: "non-interactive", wantErr: "cannot load profile", interactive: false},
		{name: "accepted", interactive: true, confirmed: true, wantOK: true},
		{name: "declined", interactive: true, wantErr: "declined to continue"},
		{name: "cancelled", interactive: true, confirmErr: ErrCancelled, wantErr: "selection cancelled", wantCancel: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prompts := &scriptedPrompts{confirmed: test.confirmed, confirmErr: test.confirmErr}
			choice, err := testResolver(prompts, &modelLookup{}).Resolve(context.Background(), LaunchRequest{
				Harness:        "cursor",
				Model:          "cursor-model",
				DefaultProfile: profile.OrchestratorName,
				Args:           []string{"--raw"},
				Interactive:    test.interactive,
			})
			if test.wantOK {
				if err != nil {
					t.Fatalf("Resolve() error = %v", err)
				}
				if choice.Profile != nil || !reflect.DeepEqual(choice.Args, []string{"--raw"}) {
					t.Fatalf("choice = %#v, want Cursor without profile and raw args", choice)
				}
				if len(prompts.questions) != 1 || !strings.Contains(prompts.questions[0], profile.OrchestratorName) {
					t.Fatalf("confirmation questions = %#v, want profile named", prompts.questions)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Resolve() error = %v, want containing %q", err, test.wantErr)
			}
			if test.wantCancel && !errors.Is(err, ErrCancelled) {
				t.Fatalf("Resolve() error = %v, want ErrCancelled", err)
			}
			if !test.interactive && !strings.Contains(err.Error(), "--no-profile") {
				t.Fatalf("Resolve() error = %v, want --no-profile guidance", err)
			}
		})
	}
}

func TestResolveCursorWithNoProfileNeedsNoConfirmation(t *testing.T) {
	prompts := &scriptedPrompts{}
	choice, err := testResolver(prompts, &modelLookup{}).Resolve(context.Background(), LaunchRequest{
		Harness:   "cursor",
		Model:     "cursor-model",
		NoProfile: true,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if choice.Harness != catalog.Cursor || len(prompts.questions) != 0 {
		t.Fatalf("choice = %#v, questions = %#v, want Cursor without confirmation", choice, prompts.questions)
	}
}

func TestResolveRejectsProfileInstructionArgumentConflicts(t *testing.T) {
	tests := []struct {
		harness string
		args    []string
	}{
		{harness: "pi", args: []string{"--append-system-prompt=mine.md"}},
		{harness: "claude", args: []string{"--system-prompt-file", "mine.md"}},
		{harness: "codex", args: []string{"-c", "developer_instructions=mine"}},
	}
	for _, test := range tests {
		t.Run(test.harness, func(t *testing.T) {
			_, err := testResolver(&scriptedPrompts{}, &modelLookup{}).Resolve(context.Background(), LaunchRequest{
				Harness:        test.harness,
				Model:          "model",
				DefaultProfile: profile.OrchestratorName,
				Args:           test.args,
			})
			if err == nil || !strings.Contains(err.Error(), "conflicts with profile instruction delivery") {
				t.Fatalf("Resolve() error = %v, want instruction conflict", err)
			}
		})
	}
}

func TestResolvePropagatesSelectionCancellation(t *testing.T) {
	for _, request := range []LaunchRequest{
		{PromptProfile: true, Interactive: true},
		{Model: "model", Interactive: true},
		{Harness: "codex", Interactive: true},
	} {
		_, err := testResolver(&scriptedPrompts{selectErr: ErrCancelled}, &modelLookup{}, testProfile()).Resolve(context.Background(), request)
		if !errors.Is(err, ErrCancelled) {
			t.Fatalf("Resolve(%#v) error = %v, want ErrCancelled", request, err)
		}
	}
}

func TestResolveInteractivePromptRequiresStreamsAndModelLookup(t *testing.T) {
	tests := []struct {
		name     string
		resolver Resolver
		request  LaunchRequest
		wantErr  string
	}{
		{
			name:     "input",
			resolver: Resolver{Output: io.Discard},
			request:  LaunchRequest{Model: "model", Interactive: true},
			wantErr:  "terminal input is nil",
		},
		{
			name:     "output",
			resolver: Resolver{Input: strings.NewReader("")},
			request:  LaunchRequest{Model: "model", Interactive: true},
			wantErr:  "terminal output is nil",
		},
		{
			name: "model lookup",
			resolver: Resolver{
				Input:  strings.NewReader(""),
				Output: io.Discard,
			},
			request: LaunchRequest{Harness: "codex", Interactive: true},
			wantErr: "model lookup is nil",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.resolver.Resolve(context.Background(), test.request)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Resolve() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestSessionChooserPreservesResolvedLaunch(t *testing.T) {
	configured := testProfile()
	chooser := SessionChooser{
		Resolver: testResolver(&scriptedPrompts{}, &modelLookup{}, configured),
		Request: LaunchRequest{
			Harness: "codex",
			Model:   "gpt",
			Profile: configured.Name,
			Args:    []string{"--raw"},
		},
	}
	choice, err := chooser.Choose(context.Background())
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if choice.Harness != "codex" || choice.Model != "gpt" {
		t.Fatalf("choice harness/model = %q/%q, want codex/gpt", choice.Harness, choice.Model)
	}
	if choice.Profile == nil || choice.Profile.Name != configured.Name {
		t.Fatalf("choice profile = %#v, want %q", choice.Profile, configured.Name)
	}
	if want := []string{"--profile-default", "--raw"}; !reflect.DeepEqual(choice.Args, want) {
		t.Fatalf("choice args = %#v, want %#v", choice.Args, want)
	}
}
