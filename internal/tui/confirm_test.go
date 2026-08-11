package tui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConfirmModelWaitsForEnterBeforeConfirming(t *testing.T) {
	t.Parallel()

	for _, answer := range []string{"y", "Y"} {
		t.Run(answer, func(t *testing.T) {
			t.Parallel()

			updated, command := (confirmModel{question: "Continue?"}).Update(tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune(answer),
			})
			model := updated.(confirmModel)
			if command != nil {
				t.Fatal("typing an answer returned a command, want nil")
			}
			if model.confirmed {
				t.Error("typing an answer confirmed before Enter")
			}
			if got := model.View(); got != "Continue? [y/N] "+answer {
				t.Errorf("View() = %q, want visible answer", got)
			}

			updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model = updated.(confirmModel)
			if !model.confirmed {
				t.Error("Enter did not confirm an exact affirmative answer")
			}
			assertQuitCommand(t, command)
		})
	}
}

func TestConfirmModelEnterSubmissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		answer    string
		confirmed bool
	}{
		{name: "empty"},
		{name: "lowercase no", answer: "n"},
		{name: "uppercase no", answer: "N"},
		{name: "lowercase quit", answer: "q"},
		{name: "uppercase quit", answer: "Q"},
		{name: "leading whitespace", answer: " y"},
		{name: "trailing whitespace", answer: "y "},
		{name: "word", answer: "yes"},
		{name: "other", answer: "anything"},
		{name: "lowercase yes", answer: "y", confirmed: true},
		{name: "uppercase yes", answer: "Y", confirmed: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			model := confirmModel{input: []rune(test.answer)}
			updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model = updated.(confirmModel)
			if model.confirmed != test.confirmed {
				t.Errorf("confirmed = %v, want %v", model.confirmed, test.confirmed)
			}
			assertQuitCommand(t, command)
		})
	}
}

func TestConfirmModelQRequiresEnter(t *testing.T) {
	t.Parallel()

	updated, command := (confirmModel{}).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	model := updated.(confirmModel)
	if command != nil {
		t.Fatal("typing q returned a command, want nil")
	}
	if got := string(model.input); got != "q" {
		t.Fatalf("input = %q, want %q", got, "q")
	}

	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(confirmModel).confirmed {
		t.Error("q followed by Enter confirmed")
	}
	assertQuitCommand(t, command)
}

func TestConfirmModelEscapeAndControlCCancelImmediately(t *testing.T) {
	t.Parallel()

	for _, keyType := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		t.Run(keyType.String(), func(t *testing.T) {
			t.Parallel()

			updated, command := (confirmModel{input: []rune("y"), confirmed: true}).Update(tea.KeyMsg{Type: keyType})
			if updated.(confirmModel).confirmed {
				t.Error("cancel key left the model confirmed")
			}
			assertQuitCommand(t, command)
		})
	}
}

func TestConfirmModelBackspaceRemovesOneRune(t *testing.T) {
	t.Parallel()

	model := confirmModel{input: []rune("aé界")}
	for _, want := range []string{"aé", "a", "", ""} {
		updated, command := model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		model = updated.(confirmModel)
		if command != nil {
			t.Fatal("Backspace returned a command, want nil")
		}
		if got := string(model.input); got != want {
			t.Fatalf("input after Backspace = %q, want %q", got, want)
		}
	}
}

func TestConfirmModelBuffersPrintableInput(t *testing.T) {
	t.Parallel()

	model := confirmModel{question: "Continue?"}
	updated, command := model.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'a', ' ', '雪', '\n', 0},
	})
	model = updated.(confirmModel)
	if command != nil {
		t.Fatal("printable input returned a command, want nil")
	}
	if got := model.View(); got != "Continue? [y/N] a 雪" {
		t.Errorf("View() = %q, want buffered printable input", got)
	}

	updated, command = model.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	model = updated.(confirmModel)
	if command != nil {
		t.Fatal("Space returned a command, want nil")
	}
	if got := string(model.input); got != "a 雪 " {
		t.Errorf("input = %q, want a trailing space", got)
	}
}

func TestConfirmModelInit(t *testing.T) {
	t.Parallel()

	if command := (confirmModel{}).Init(); command != nil {
		t.Error("Init() command != nil, want nil")
	}
}

func TestConfirmModelView(t *testing.T) {
	t.Parallel()

	if view := (confirmModel{question: "Continue?", input: []rune("typed")}).View(); view != "Continue? [y/N] typed" {
		t.Errorf("View() = %q, want %q", view, "Continue? [y/N] typed")
	}
}

func TestConfirmModelIgnoresOtherMessages(t *testing.T) {
	t.Parallel()

	initial := confirmModel{question: "Continue?", input: []rune("keep")}
	tests := []struct {
		name    string
		message tea.Msg
	}{
		{name: "non-key", message: struct{}{}},
		{name: "unsupported key", message: tea.KeyMsg{Type: tea.KeyUp}},
		{name: "non-printable key", message: tea.KeyMsg{Type: tea.KeyCtrlA}},
		{name: "non-printable rune", message: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\n'}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			updated, command := initial.Update(test.message)
			model := updated.(confirmModel)
			if model.question != initial.question || string(model.input) != string(initial.input) || model.confirmed != initial.confirmed {
				t.Errorf("Update() model = %#v, want %#v", model, initial)
			}
			if command != nil {
				t.Error("Update() command != nil, want nil")
			}
		})
	}
}

func TestConfirmerRejectsNonTerminal(t *testing.T) {
	t.Parallel()

	input, inputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer inputWriter.Close()
	outputReader, output, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer outputReader.Close()
	defer output.Close()

	confirmed, err := NewConfirmer(input, output).Confirm("Continue?")
	if err == nil {
		t.Fatal("Confirm() error = nil, want error")
	}
	if confirmed {
		t.Error("Confirm() = true, want false")
	}
}

func assertQuitCommand(t *testing.T, command tea.Cmd) {
	t.Helper()
	if command == nil {
		t.Fatal("command = nil, want tea.Quit")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Error("command message is not tea.QuitMsg")
	}
}
