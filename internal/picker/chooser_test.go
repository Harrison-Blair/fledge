package picker

import (
	"context"
	"errors"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"fledge/internal/catalog"
	"fledge/internal/session"
)

type selectCall struct {
	title   string
	options []Option
}

// scriptedSelect answers each successive selection with one scripted result.
type scriptedSelect struct {
	answers []Option
	err     error
	calls   []selectCall
}

func (s *scriptedSelect) fn(_ io.Reader, _ io.Writer, title string, options []Option) (Option, error) {
	s.calls = append(s.calls, selectCall{title: title, options: options})
	if s.err != nil {
		return Option{}, s.err
	}
	answer := s.answers[len(s.calls)-1]
	return answer, nil
}

type modelLookup struct {
	models    map[catalog.Harness][]string
	requested chan catalog.Harness
}

func newModelLookup(models map[catalog.Harness][]string) *modelLookup {
	return &modelLookup{models: models, requested: make(chan catalog.Harness, len(catalog.Harnesses()))}
}

func (l *modelLookup) fn(_ context.Context, harness catalog.Harness) []string {
	l.requested <- harness
	return l.models[harness]
}

// awaitRequests collects the harnesses looked up, which happens concurrently
// with the harness question.
func (l *modelLookup) awaitRequests(t *testing.T, count int) []catalog.Harness {
	t.Helper()
	requested := make([]catalog.Harness, 0, count)
	for range count {
		select {
		case harness := <-l.requested:
			requested = append(requested, harness)
		case <-time.After(2 * time.Second):
			t.Fatalf("model lookups = %#v, want %d", requested, count)
		}
	}
	sort.Slice(requested, func(i, j int) bool { return requested[i] < requested[j] })
	return requested
}

func testChooser(script *scriptedSelect, lookup *modelLookup) AgentChooser {
	return AgentChooser{
		Input:            strings.NewReader(""),
		Output:           io.Discard,
		InputIsTerminal:  true,
		OutputIsTerminal: true,
		Models:           lookup.fn,
		selectFn:         script.fn,
	}
}

func optionTitles(options []Option) []string {
	titles := make([]string, len(options))
	for i, option := range options {
		titles[i] = option.Title
	}
	return titles
}

func TestChooseRequiresTerminalStreams(t *testing.T) {
	for _, test := range []struct {
		name    string
		chooser AgentChooser
	}{
		{name: "input", chooser: AgentChooser{Output: io.Discard, OutputIsTerminal: true}},
		{name: "output", chooser: AgentChooser{Input: strings.NewReader(""), InputIsTerminal: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			chooser := test.chooser
			_, err := chooser.Choose(context.Background())
			if err == nil || !strings.Contains(err.Error(), "requires terminal input and output") {
				t.Fatalf("Choose() error = %v, want terminal requirement", err)
			}
		})
	}
}

func TestChoosePresentsHarnessesThenShellOnly(t *testing.T) {
	script := &scriptedSelect{answers: []Option{{Title: shellOnlyTitle}}}
	lookup := newModelLookup(map[catalog.Harness][]string{})

	choice, err := testChooser(script, lookup).Choose(context.Background())
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if choice != (session.AgentChoice{}) {
		t.Fatalf("choice = %#v, want shell only", choice)
	}
	if len(script.calls) != 1 {
		t.Fatalf("selections = %d, want only the harness question", len(script.calls))
	}

	want := []string{"pi", "claude", "codex", "opencode", "cursor", shellOnlyTitle}
	if got := optionTitles(script.calls[0].options); !reflect.DeepEqual(got, want) {
		t.Fatalf("harness options = %#v, want %#v", got, want)
	}
	if last := script.calls[0].options[len(want)-1]; last.ID != "" {
		t.Fatalf("shell-only option ID = %q, want empty", last.ID)
	}
}

func TestChoosePresentsModelsForChosenHarness(t *testing.T) {
	script := &scriptedSelect{answers: []Option{
		{ID: "claude", Title: "claude"},
		{ID: "claude-sonnet-4-5", Title: "claude-sonnet-4-5"},
	}}
	lookup := newModelLookup(map[catalog.Harness][]string{
		catalog.Claude: {"claude-opus-4-8", "claude-sonnet-4-5"},
		catalog.Pi:     {"pi/one"},
	})

	choice, err := testChooser(script, lookup).Choose(context.Background())
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	want := session.AgentChoice{Harness: "claude", Model: "claude-sonnet-4-5"}
	if choice != want {
		t.Fatalf("choice = %#v, want %#v", choice, want)
	}
	if len(script.calls) != 2 {
		t.Fatalf("selections = %d, want harness and model questions", len(script.calls))
	}

	models := script.calls[1].options
	wantTitles := []string{defaultTitle, freeTextTitle, "claude-opus-4-8", "claude-sonnet-4-5"}
	if got := optionTitles(models); !reflect.DeepEqual(got, wantTitles) {
		t.Fatalf("model options = %#v, want %#v", got, wantTitles)
	}
	if models[0].ID != "" {
		t.Fatalf("default option ID = %q, want empty", models[0].ID)
	}
	if !models[1].FreeText {
		t.Fatal("second model option is not the free-text row")
	}
	if !strings.Contains(script.calls[1].title, "claude") {
		t.Fatalf("model question = %q, want the harness named", script.calls[1].title)
	}

	wantHarnesses := append([]catalog.Harness(nil), catalog.Harnesses()...)
	sort.Slice(wantHarnesses, func(i, j int) bool { return wantHarnesses[i] < wantHarnesses[j] })
	if got := lookup.awaitRequests(t, len(wantHarnesses)); !reflect.DeepEqual(got, wantHarnesses) {
		t.Fatalf("model lookups = %#v, want every harness prefetched", got)
	}
}

func TestChooseAcceptsHarnessDefaultAndFreeTextModels(t *testing.T) {
	for _, test := range []struct {
		name   string
		answer Option
		want   string
	}{
		{name: "default", answer: Option{Title: defaultTitle}, want: ""},
		{name: "free text", answer: Option{ID: "typed-model", Title: "typed-model"}, want: "typed-model"},
	} {
		t.Run(test.name, func(t *testing.T) {
			script := &scriptedSelect{answers: []Option{{ID: "pi", Title: "pi"}, test.answer}}
			lookup := newModelLookup(map[catalog.Harness][]string{})

			choice, err := testChooser(script, lookup).Choose(context.Background())
			if err != nil {
				t.Fatalf("Choose() error = %v", err)
			}
			if want := (session.AgentChoice{Harness: "pi", Model: test.want}); choice != want {
				t.Fatalf("choice = %#v, want %#v", choice, want)
			}
		})
	}
}

func TestChoosePropagatesCancellation(t *testing.T) {
	script := &scriptedSelect{err: ErrCancelled}
	lookup := newModelLookup(map[catalog.Harness][]string{})

	_, err := testChooser(script, lookup).Choose(context.Background())
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("Choose() error = %v, want ErrCancelled", err)
	}
}

func TestChooseRequiresModelLookup(t *testing.T) {
	chooser := AgentChooser{
		Input:            strings.NewReader(""),
		Output:           io.Discard,
		InputIsTerminal:  true,
		OutputIsTerminal: true,
	}
	if _, err := chooser.Choose(context.Background()); err == nil {
		t.Fatal("Choose() error = nil, want nil-lookup error")
	}
}
