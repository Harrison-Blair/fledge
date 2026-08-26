// Package picker presents a filterable terminal selection list. Terminal
// detection is supplied by the CLI boundary so this package does not depend on
// a particular file-descriptor implementation.
package picker

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// ErrCancelled reports that the user dismissed the selection without choosing.
var ErrCancelled = errors.New("selection cancelled")

// Option is one selectable row. A FreeText option prompts for a typed value
// instead of selecting itself.
type Option struct {
	ID          string
	Title       string
	Description string
	FreeText    bool
}

const (
	defaultWidth       = 72
	maxVisibleOptions  = 8
	listChromeHeight   = 6
	freeTextCharLimit  = 200
	freeTextInputWidth = 60
)

// Select runs a filterable single-select on the given terminal streams and
// returns the chosen option. It reports ErrCancelled when the user dismisses
// the list.
func Select(in io.Reader, out io.Writer, title string, options []Option) (Option, error) {
	if len(options) == 0 {
		return Option{}, fmt.Errorf("select %s: no options", title)
	}

	final, err := tea.NewProgram(newModel(title, options), tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return Option{}, fmt.Errorf("select %s: %w", title, err)
	}
	chosen, ok := final.(model)
	if !ok || !chosen.done {
		return Option{}, ErrCancelled
	}
	return chosen.chosen, nil
}

// item adapts an Option to the list's default delegate.
type item struct {
	option Option
}

func (i item) Title() string       { return i.option.Title }
func (i item) Description() string { return i.option.Description }
func (i item) FilterValue() string { return i.option.Title }

type model struct {
	list     list.Model
	input    textinput.Model
	prompt   string
	freeText bool
	done     bool
	chosen   Option
}

func newModel(title string, options []Option) model {
	rows := make([]list.Item, len(options))
	described := false
	for i, option := range options {
		rows[i] = item{option: option}
		described = described || option.Description != ""
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = described
	if !described {
		delegate.SetHeight(1)
		delegate.SetSpacing(0)
	}

	visible := min(len(options), maxVisibleOptions)
	height := listChromeHeight + visible*(delegate.Height()+delegate.Spacing())
	rendered := list.New(rows, delegate, defaultWidth, height)
	rendered.Title = title
	rendered.SetShowStatusBar(false)

	input := textinput.New()
	input.CharLimit = freeTextCharLimit
	input.Width = freeTextInputWidth

	return model{list: rendered, input: input}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		return m, nil
	case tea.KeyMsg:
		if m.freeText {
			return m.updateFreeText(msg)
		}
		switch {
		case msg.Type == tea.KeyCtrlC:
			return m, tea.Quit
		case m.list.FilterState() == list.Filtering:
			// The list owns every other key while the filter is being typed.
		case msg.Type == tea.KeyEsc, msg.String() == "q":
			return m, tea.Quit
		case msg.Type == tea.KeyEnter:
			return m.choose()
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// choose acts on the highlighted row, either selecting it or opening the
// free-text prompt.
func (m model) choose() (tea.Model, tea.Cmd) {
	selected, ok := m.list.SelectedItem().(item)
	if !ok {
		return m, nil
	}
	if selected.option.FreeText {
		m.freeText = true
		m.prompt = selected.option.Title
		m.input.SetValue("")
		return m, m.input.Focus()
	}
	m.done = true
	m.chosen = selected.option
	return m, tea.Quit
}

func (m model) updateFreeText(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		return m.closeFreeText(), nil
	case tea.KeyEnter:
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m.closeFreeText(), nil
		}
		m.done = true
		m.chosen = Option{ID: text, Title: text}
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) closeFreeText() model {
	m.freeText = false
	m.input.Blur()
	return m
}

func (m model) View() string {
	switch {
	case m.done:
		return fmt.Sprintf("%s %s\n", m.list.Title, m.chosen.Title)
	case m.freeText:
		return fmt.Sprintf("%s\n%s\n", m.prompt, m.input.View())
	default:
		return m.list.View()
	}
}
