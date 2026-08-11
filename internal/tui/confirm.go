// Package tui contains Fledge's interactive terminal prompts.
package tui

import (
	"errors"
	"fmt"
	"os"
	"unicode"

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
	input     []rune
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

	switch key.Type {
	case tea.KeyEnter:
		answer := string(m.input)
		m.confirmed = answer == "y" || answer == "Y"
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyCtrlC:
		m.confirmed = false
		return m, tea.Quit
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
		return m, nil
	case tea.KeySpace:
		m.input = appendPrintableRunes(m.input, key.Runes)
		if len(key.Runes) == 0 {
			m.input = append(m.input, ' ')
		}
		return m, nil
	case tea.KeyRunes:
		m.input = appendPrintableRunes(m.input, key.Runes)
		return m, nil
	}

	return m, nil
}

func (m confirmModel) View() string {
	return m.question + " [y/N] " + string(m.input)
}

func appendPrintableRunes(destination, runes []rune) []rune {
	for _, value := range runes {
		if unicode.IsPrint(value) {
			destination = append(destination, value)
		}
	}
	return destination
}
