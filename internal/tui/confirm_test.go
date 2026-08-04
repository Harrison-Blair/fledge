package tui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConfirmModelKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key       string
		confirmed bool
	}{
		{key: "y", confirmed: true},
		{key: "Y", confirmed: true},
		{key: "n", confirmed: false},
		{key: "N", confirmed: false},
		{key: "q", confirmed: false},
		{key: "Q", confirmed: false},
		{key: "enter", confirmed: false},
		{key: "esc", confirmed: false},
		{key: "ctrl+c", confirmed: false},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			t.Parallel()

			updated, command := (confirmModel{}).Update(tea.KeyMsg{Type: keyType(test.key), Runes: keyRunes(test.key)})
			model := updated.(confirmModel)
			if model.confirmed != test.confirmed {
				t.Errorf("confirmed = %v, want %v", model.confirmed, test.confirmed)
			}
			if command == nil {
				t.Error("command = nil, want tea.Quit")
				return
			}
			if _, ok := command().(tea.QuitMsg); !ok {
				t.Error("command message is not tea.QuitMsg")
			}
		})
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

	if view := (confirmModel{question: "Continue?"}).View(); view != "Continue? [y/N] " {
		t.Errorf("View() = %q, want %q", view, "Continue? [y/N] ")
	}
}

func TestConfirmModelIgnoresOtherMessages(t *testing.T) {
	t.Parallel()

	initial := confirmModel{question: "Continue?", confirmed: true}
	tests := []struct {
		name    string
		message tea.Msg
	}{
		{name: "non-key", message: struct{}{}},
		{name: "other key", message: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			updated, command := initial.Update(test.message)
			model := updated.(confirmModel)
			if model != initial {
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

func keyType(key string) tea.KeyType {
	switch key {
	case "enter":
		return tea.KeyEnter
	case "esc":
		return tea.KeyEsc
	case "ctrl+c":
		return tea.KeyCtrlC
	default:
		return tea.KeyRunes
	}
}

func keyRunes(key string) []rune {
	if len(key) == 1 {
		return []rune(key)
	}
	return nil
}
