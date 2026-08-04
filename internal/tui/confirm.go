// Package tui contains Fledge's interactive terminal prompts.
package tui

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

// Confirmer asks yes-or-no questions in an interactive terminal.
type Confirmer struct {
	stdin  *os.File
	stdout *os.File
}

// NewConfirmer creates an interactive confirmer.
func NewConfirmer(stdin, stdout *os.File) *Confirmer {
	return &Confirmer{stdin: stdin, stdout: stdout}
}

// Confirm asks question and defaults to no.
func (c *Confirmer) Confirm(question string) (bool, error) {
	if !term.IsTerminal(c.stdin.Fd()) || !term.IsTerminal(c.stdout.Fd()) {
		return false, errors.New("confirmation requires an interactive terminal")
	}

	result, err := tea.NewProgram(
		confirmModel{question: question},
		tea.WithInput(c.stdin),
		tea.WithOutput(c.stdout),
	).Run()
	if err != nil {
		return false, fmt.Errorf("run confirmation prompt: %w", err)
	}

	model, ok := result.(confirmModel)
	if !ok {
		return false, errors.New("confirmation prompt returned an unexpected model")
	}

	return model.confirmed, nil
}

type confirmModel struct {
	question  string
	confirmed bool
}

func (confirmModel) Init() tea.Cmd {
	return nil
}

func (m confirmModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "y", "Y":
		m.confirmed = true
		return m, tea.Quit
	case "enter", "n", "N", "q", "Q", "esc", "ctrl+c":
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m confirmModel) View() string {
	return m.question + " [y/N] "
}
