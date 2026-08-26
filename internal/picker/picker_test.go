package picker

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

var selectOptions = []Option{
	{ID: "pi", Title: "pi"},
	{ID: "claude", Title: "claude"},
	{FreeText: true, Title: "enter model ID"},
	{ID: "", Title: "none — shell only"},
}

func press(t *testing.T, start tea.Model, msgs ...tea.Msg) (model, tea.Cmd) {
	t.Helper()
	current := start
	var cmd tea.Cmd
	for _, msg := range msgs {
		current, cmd = current.Update(msg)
	}
	final, ok := current.(model)
	if !ok {
		t.Fatalf("model type = %T", current)
	}
	return final, cmd
}

// altScreenEnter is the escape sequence Bubble Tea writes when a program takes
// over the alternate screen.
const altScreenEnter = "\x1b[?1049h"

func runes(text string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}
}

func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestSelectModelChoosesHighlightedOption(t *testing.T) {
	final, cmd := press(t, newModel("harness", selectOptions),
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter},
	)
	if !final.done {
		t.Fatal("model did not finish")
	}
	if final.chosen.ID != "claude" {
		t.Fatalf("chosen = %#v, want claude", final.chosen)
	}
	if !quits(cmd) {
		t.Fatal("enter did not quit the program")
	}
}

func TestSelectModelCancelKeys(t *testing.T) {
	for _, test := range []struct {
		name string
		key  tea.KeyMsg
	}{
		{name: "esc", key: tea.KeyMsg{Type: tea.KeyEsc}},
		{name: "ctrl+c", key: tea.KeyMsg{Type: tea.KeyCtrlC}},
		{name: "q", key: runes("q")},
	} {
		t.Run(test.name, func(t *testing.T) {
			final, cmd := press(t, newModel("harness", selectOptions), test.key)
			if final.done {
				t.Fatalf("%s selected %#v", test.name, final.chosen)
			}
			if !quits(cmd) {
				t.Fatalf("%s did not quit the program", test.name)
			}
		})
	}
}

func TestSelectModelFilteringOwnsItsKeys(t *testing.T) {
	final, cmd := press(t, newModel("harness", selectOptions), runes("/"), runes("q"))
	if final.list.FilterState() != list.Filtering {
		t.Fatalf("filter state = %v, want filtering", final.list.FilterState())
	}
	if quits(cmd) {
		t.Fatal("q quit the program while filtering")
	}
	if got := final.list.FilterInput.Value(); got != "q" {
		t.Fatalf("filter text = %q, want q", got)
	}
}

func TestSelectModelEscapeCancelsWithFilterApplied(t *testing.T) {
	filtered := newModel("harness", selectOptions)
	filtered.list.SetFilterText("claude")

	final, cmd := press(t, filtered, tea.KeyMsg{Type: tea.KeyEsc})
	if final.done {
		t.Fatalf("escape selected %#v", final.chosen)
	}
	if !quits(cmd) {
		t.Fatal("escape did not quit the program with a filter applied")
	}
}

func TestSelectModelChoosesFilteredOption(t *testing.T) {
	filtered := newModel("harness", selectOptions)
	filtered.list.SetFilterText("claude")

	final, _ := press(t, filtered, tea.KeyMsg{Type: tea.KeyEnter})
	if !final.done || final.chosen.ID != "claude" {
		t.Fatalf("chosen = %#v (done %v), want claude", final.chosen, final.done)
	}
}

func TestSelectModelFreeTextEntry(t *testing.T) {
	opened, _ := press(t, newModel("model", selectOptions),
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter},
	)
	if !opened.freeText {
		t.Fatal("free-text row did not open the prompt")
	}
	if opened.done {
		t.Fatal("free-text row was selected as a normal option")
	}

	final, cmd := press(t, opened, runes(" gpt-5 "), tea.KeyMsg{Type: tea.KeyEnter})
	if !final.done {
		t.Fatal("free-text entry did not finish")
	}
	if want := (Option{ID: "gpt-5", Title: "gpt-5"}); final.chosen != want {
		t.Fatalf("chosen = %#v, want %#v", final.chosen, want)
	}
	if !quits(cmd) {
		t.Fatal("free-text entry did not quit the program")
	}
}

func TestSelectModelFreeTextReturnsToList(t *testing.T) {
	open := []tea.Msg{
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter},
	}
	for _, test := range []struct {
		name string
		keys []tea.Msg
	}{
		{name: "escape", keys: []tea.Msg{tea.KeyMsg{Type: tea.KeyEsc}}},
		{name: "empty entry", keys: []tea.Msg{runes("   "), tea.KeyMsg{Type: tea.KeyEnter}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			final, cmd := press(t, newModel("model", selectOptions), append(open, test.keys...)...)
			if final.freeText {
				t.Fatal("prompt stayed open")
			}
			if final.done {
				t.Fatalf("prompt selected %#v", final.chosen)
			}
			if quits(cmd) {
				t.Fatal("prompt quit the program")
			}
		})
	}
}

func TestSelectModelFreeTextCancels(t *testing.T) {
	final, cmd := press(t, newModel("model", selectOptions),
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter},
		runes("gpt-5"),
		tea.KeyMsg{Type: tea.KeyCtrlC},
	)
	if final.done {
		t.Fatalf("ctrl+c selected %#v", final.chosen)
	}
	if !quits(cmd) {
		t.Fatal("ctrl+c did not quit the program")
	}
}

func TestSelectModelViewKeepsChoiceInScrollback(t *testing.T) {
	final, _ := press(t, newModel("Model for claude", selectOptions), tea.KeyMsg{Type: tea.KeyEnter})
	view := final.View()
	if !strings.Contains(view, "Model for claude") || !strings.Contains(view, "pi") {
		t.Fatalf("final view = %q, want the question and the choice", view)
	}
}

func TestSelectStaysOutOfTheAlternateScreen(t *testing.T) {
	var output bytes.Buffer
	if _, err := Select(strings.NewReader("\r"), &output, "harness", selectOptions); err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if strings.Contains(output.String(), altScreenEnter) {
		t.Fatal("Select() entered the alternate screen, which hides the choice from scrollback")
	}
}

func TestSelectRequiresOptions(t *testing.T) {
	if _, err := Select(strings.NewReader(""), io.Discard, "harness", nil); err == nil {
		t.Fatal("Select() error = nil, want no-options error")
	}
}

func TestSelectReportsCancellation(t *testing.T) {
	chosen, err := Select(strings.NewReader("q"), io.Discard, "harness", selectOptions)
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("Select() error = %v, want ErrCancelled", err)
	}
	if chosen != (Option{}) {
		t.Fatalf("Select() option = %#v, want zero", chosen)
	}
}

func TestSelectReturnsChoice(t *testing.T) {
	chosen, err := Select(strings.NewReader("\r"), io.Discard, "harness", selectOptions)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if chosen.ID != "pi" {
		t.Fatalf("Select() option = %#v, want pi", chosen)
	}
}
