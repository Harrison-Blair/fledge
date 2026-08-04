package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

// ErrPickerCancelled is returned when a user cancels an interactive picker.
var ErrPickerCancelled = errors.New("selection cancelled")

var agentNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// ValidateAgentName applies the naming rule accepted by Herdr.
func ValidateAgentName(name string) error {
	if !agentNamePattern.MatchString(name) {
		return fmt.Errorf("invalid agent name %q: must match [a-z][a-z0-9_-]{0,31}", name)
	}
	return nil
}

// CallerKind describes whether a command was invoked by a user or an agent.
type CallerKind uint8

const (
	CallerUnknown CallerKind = iota
	CallerDirectUser
	CallerAgent
)

// PaneAgent is the live-agent information needed to classify the current pane.
// Recognized may be set by adapters that have already identified the harness.
type PaneAgent struct {
	PaneID     string
	Harness    string
	Recognized bool
}

// CallerInput contains the current Herdr pane and the targeted session's live
// agent data. SessionAgentsAvailable must only be true after a successful read.
type CallerInput struct {
	PaneID                 string
	SessionAgentsAvailable bool
	Agents                 []PaneAgent
	PaneIDs                []string
}

// ClassifyCaller fails closed when a command runs in a Herdr pane whose live
// agent data could not be read. A command outside Herdr is a direct user call.
func ClassifyCaller(input CallerInput) CallerKind {
	if input.PaneID == "" {
		return CallerDirectUser
	}
	if !input.SessionAgentsAvailable {
		return CallerUnknown
	}
	for _, agent := range input.Agents {
		if agent.PaneID == input.PaneID && (agent.Recognized || recognizedHarness(agent.Harness)) {
			return CallerAgent
		}
	}
	for _, paneID := range input.PaneIDs {
		if paneID == input.PaneID {
			return CallerDirectUser
		}
	}
	return CallerUnknown
}

func recognizedHarness(harness string) bool {
	switch strings.ToLower(strings.TrimSpace(harness)) {
	case "claude", "codex", "pi", "opencode":
		return true
	default:
		return false
	}
}

// PromptsAllowed reports whether interactive prompts are safe for this caller.
func PromptsAllowed(stdinTerminal, stdoutTerminal bool, caller CallerKind) bool {
	return stdinTerminal && stdoutTerminal && caller == CallerDirectUser
}

// Choice is one selectable picker entry. Group is optional display metadata.
type Choice struct {
	Value string
	Label string
	Group string
}

// ModelLoader returns the available models for a selected harness.
type ModelLoader func(context.Context, string) ([]Choice, error)

// LastUsed is a remembered harness/model pair offered as a harness shortcut.
type LastUsed struct {
	Harness string
	Model   string // "" means the harness default
}

const lastUsedValue = "\x00last-used"

func lastUsedLabel(last LastUsed) string {
	model := last.Model
	if model == "" {
		model = "harness default"
	}
	return fmt.Sprintf("Last used (%s · %s)", last.Harness, model)
}

// SelectionRequest describes supplied flags and interactive choices. Supplied
// Name, Harness, and Model values skip their respective prompt.
type SelectionRequest struct {
	Name      string
	Harness   string
	Model     string
	ModelSet  bool
	Harnesses []Choice
	Models    ModelLoader
	Caller    CallerInput
	LastUsed  *LastUsed
}

// Selection contains the resolved agent launch selections. An empty Model is
// the harness default.
type Selection struct {
	Name    string
	Harness string
	Model   string
	// Prompted is true when the harness or model came from an interactive prompt.
	Prompted bool
}

// PromptRunner abstracts the Bubble Tea programs so selection flow can be
// tested without a terminal.
type PromptRunner interface {
	Choose(context.Context, string, []Choice, bool) (string, error)
	Text(context.Context, string, func(string) error) (string, error)
}

// Selector resolves launch selections interactively when it is safe to do so.
type Selector struct {
	stdin      *os.File
	stdout     *os.File
	isTerminal func(uintptr) bool
	prompts    PromptRunner
}

// NewSelector creates a Bubble Tea backed selector.
func NewSelector(stdin, stdout *os.File) *Selector {
	return NewSelectorWithDependencies(stdin, stdout, term.IsTerminal, nil)
}

// NewSelectorWithDependencies creates a selector with injectable terminal and
// prompt implementations. A nil prompts value uses Bubble Tea.
func NewSelectorWithDependencies(stdin, stdout *os.File, isTerminal func(uintptr) bool, prompts PromptRunner) *Selector {
	if isTerminal == nil {
		isTerminal = term.IsTerminal
	}
	if prompts == nil {
		prompts = bubblePromptRunner{stdin: stdin, stdout: stdout}
	}
	return &Selector{stdin: stdin, stdout: stdout, isTerminal: isTerminal, prompts: prompts}
}

// Select prompts in harness, model, then missing-name order. Noninteractive
// callers must supply a harness and name; an omitted model means the harness
// default.
func (s *Selector) Select(ctx context.Context, request SelectionRequest) (Selection, error) {
	selection := Selection{Name: request.Name, Harness: request.Harness, Model: request.Model}
	if selection.Name != "" {
		if err := ValidateAgentName(selection.Name); err != nil {
			return Selection{}, err
		}
	}

	interactive := s != nil && s.stdin != nil && s.stdout != nil &&
		PromptsAllowed(s.isTerminal(s.stdin.Fd()), s.isTerminal(s.stdout.Fd()), ClassifyCaller(request.Caller))
	if !interactive {
		if selection.Harness == "" || selection.Name == "" {
			return Selection{}, errors.New("noninteractive agent spawn requires --name and --harness")
		}
		return selection, nil
	}

	var err error
	modelResolved := false
	if selection.Harness == "" {
		if len(request.Harnesses) == 0 {
			return Selection{}, errors.New("no installed harnesses are available")
		}
		harnesses := request.Harnesses
		if request.LastUsed != nil {
			harnesses = append([]Choice{{Value: lastUsedValue, Label: lastUsedLabel(*request.LastUsed)}}, harnesses...)
		}
		value, err := s.prompts.Choose(ctx, "Select harness", harnesses, false)
		if err != nil {
			return Selection{}, fmt.Errorf("select harness: %w", err)
		}
		selection.Prompted = true
		if value == lastUsedValue && request.LastUsed != nil {
			selection.Harness = request.LastUsed.Harness
			selection.Model = request.LastUsed.Model
			modelResolved = true
		} else {
			selection.Harness = value
		}
	}

	if !modelResolved && !request.ModelSet && selection.Model == "" {
		models := []Choice{{Label: "Harness default"}}
		if request.Models != nil {
			models, err = request.Models(ctx, selection.Harness)
			if err != nil {
				return Selection{}, fmt.Errorf("load models for %s: %w", selection.Harness, err)
			}
			models = withHarnessDefaults(models)
		}
		selection.Model, err = s.prompts.Choose(ctx, "Select model", models, true)
		if err != nil {
			return Selection{}, fmt.Errorf("select model: %w", err)
		}
		selection.Prompted = true
	}

	if selection.Name == "" {
		selection.Name, err = s.prompts.Text(ctx, "Agent name", ValidateAgentName)
		if err != nil {
			return Selection{}, fmt.Errorf("select agent name: %w", err)
		}
		if err := ValidateAgentName(selection.Name); err != nil {
			return Selection{}, err
		}
	}

	return selection, nil
}

func withHarnessDefaults(choices []Choice) []Choice {
	grouped := false
	for _, choice := range choices {
		if choice.Group != "" {
			grouped = true
			break
		}
	}
	if !grouped {
		for _, choice := range choices {
			if choice.Value == "" {
				return choices
			}
		}
		return append([]Choice{{Label: "Harness default"}}, choices...)
	}

	result := make([]Choice, 0, len(choices))
	seen := make(map[string]bool)
	for _, choice := range choices {
		if choice.Group == "" {
			if choice.Value == "" {
				continue
			}
			result = append(result, choice)
			continue
		}
		if !seen[choice.Group] {
			seen[choice.Group] = true
			if choice.Value != "" {
				result = append(result, Choice{Label: "Harness default", Group: choice.Group})
			}
		}
		result = append(result, choice)
	}
	return result
}

type bubblePromptRunner struct {
	stdin  *os.File
	stdout *os.File
}

func (r bubblePromptRunner) Choose(ctx context.Context, title string, choices []Choice, filterable bool) (string, error) {
	result, err := tea.NewProgram(
		newChoiceModel(title, choices, filterable),
		tea.WithContext(ctx), tea.WithInput(r.stdin), tea.WithOutput(r.stdout),
	).Run()
	if err != nil {
		return "", err
	}
	model, ok := result.(choiceModel)
	if !ok {
		return "", errors.New("picker returned an unexpected model")
	}
	if model.cancelled {
		return "", ErrPickerCancelled
	}
	if !model.selected {
		return "", errors.New("picker exited without a selection")
	}
	return model.value, nil
}

func (r bubblePromptRunner) Text(ctx context.Context, title string, validate func(string) error) (string, error) {
	result, err := tea.NewProgram(
		textModel{title: title, validate: validate},
		tea.WithContext(ctx), tea.WithInput(r.stdin), tea.WithOutput(r.stdout),
	).Run()
	if err != nil {
		return "", err
	}
	model, ok := result.(textModel)
	if !ok {
		return "", errors.New("text prompt returned an unexpected model")
	}
	if model.cancelled {
		return "", ErrPickerCancelled
	}
	return model.value, nil
}

type choiceModel struct {
	title      string
	choices    []Choice
	visible    []int
	cursor     int
	filter     string
	filterable bool
	selected   bool
	cancelled  bool
	value      string
}

func newChoiceModel(title string, choices []Choice, filterable bool) choiceModel {
	m := choiceModel{title: title, choices: choices, filterable: filterable}
	m.applyFilter()
	return m
}

func (choiceModel) Init() tea.Cmd { return nil }

func (m choiceModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	keyName := key.String()
	switch keyName {
	case "esc", "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down":
		if m.cursor+1 < len(m.visible) {
			m.cursor++
		}
	case "enter":
		if len(m.visible) != 0 {
			m.value = m.choices[m.visible[m.cursor]].Value
			m.selected = true
			return m, tea.Quit
		}
	case "backspace":
		if m.filterable && m.filter != "" {
			_, size := utf8.DecodeLastRuneInString(m.filter)
			m.filter = m.filter[:len(m.filter)-size]
			m.applyFilter()
		}
	case "ctrl+u":
		if m.filterable {
			m.filter = ""
			m.applyFilter()
		}
	default:
		if m.filterable && key.Type == tea.KeyRunes {
			m.filter += string(key.Runes)
			m.applyFilter()
		}
	}
	return m, nil
}

func (m *choiceModel) applyFilter() {
	needle := strings.ToLower(m.filter)
	m.visible = m.visible[:0]
	for index, choice := range m.choices {
		haystack := strings.ToLower(choice.Label + " " + choice.Value + " " + choice.Group)
		if strings.Contains(haystack, needle) {
			m.visible = append(m.visible, index)
		}
	}
	m.cursor = 0
}

func (m choiceModel) View() string {
	var view strings.Builder
	view.WriteString(m.title)
	if m.filterable {
		view.WriteString(" (type to filter): ")
		view.WriteString(m.filter)
	}
	view.WriteByte('\n')
	if len(m.visible) == 0 {
		view.WriteString("  No matches\n")
	}
	grouped := false
	for _, choice := range m.choices {
		if choice.Group != "" {
			grouped = true
			break
		}
	}
	cursorPrefix, itemPrefix := "> ", "  "
	if grouped {
		cursorPrefix, itemPrefix = "  > ", "    "
	}
	lastGroup := ""
	for row, index := range m.visible {
		choice := m.choices[index]
		if choice.Group != "" && choice.Group != lastGroup {
			view.WriteString(choice.Group)
			view.WriteByte('\n')
			lastGroup = choice.Group
		}
		if row == m.cursor {
			view.WriteString(cursorPrefix)
		} else {
			view.WriteString(itemPrefix)
		}
		label := choice.Label
		if label == "" {
			label = choice.Value
		}
		view.WriteString(label)
		view.WriteByte('\n')
	}
	return view.String()
}

type textModel struct {
	title     string
	value     string
	validate  func(string) error
	err       error
	cancelled bool
}

func (textModel) Init() tea.Cmd { return nil }

func (m textModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	case "enter":
		if m.validate != nil {
			m.err = m.validate(m.value)
		}
		if m.err == nil {
			return m, tea.Quit
		}
	case "backspace":
		if m.value != "" {
			_, size := utf8.DecodeLastRuneInString(m.value)
			m.value = m.value[:len(m.value)-size]
			m.err = nil
		}
	default:
		if key.Type == tea.KeyRunes {
			m.value += string(key.Runes)
			m.err = nil
		}
	}
	return m, nil
}

func (m textModel) View() string {
	view := m.title + ": " + m.value
	if m.err != nil {
		view += "\n" + m.err.Error()
	}
	return view
}
