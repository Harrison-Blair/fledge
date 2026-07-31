package picker

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

var ErrCancelled = errors.New("picker cancelled")

type Item struct {
	ID          string
	Title       string
	Description string
	Group       string
	Subgroup    string
}

type Options struct {
	Title             string
	Placeholder       string
	Items             []Item
	Input             io.Reader
	Output            io.Writer
	CollapsibleGroups bool
}

func Input(opts Options) (string, error) {
	field := textinput.New()
	field.Placeholder = opts.Placeholder
	field.Focus()
	model := inputModel{title: opts.Title, input: field}
	result, err := tea.NewProgram(model, tea.WithInput(opts.Input), tea.WithOutput(opts.Output)).Run()
	if err != nil {
		return "", err
	}
	final := result.(inputModel)
	if final.cancelled {
		return "", ErrCancelled
	}
	return strings.TrimSpace(final.value), nil
}

func Select(opts Options) (Item, error) {
	model := newSelectModel(opts)
	result, err := tea.NewProgram(model, tea.WithInput(opts.Input), tea.WithOutput(opts.Output)).Run()
	if err != nil {
		return Item{}, err
	}
	final := result.(selectModel)
	if final.cancelled {
		return Item{}, ErrCancelled
	}
	if final.selected == nil {
		return Item{}, ErrCancelled
	}
	return *final.selected, nil
}

type inputModel struct {
	title     string
	input     textinput.Model
	value     string
	cancelled bool
}

func (m inputModel) Init() tea.Cmd { return textinput.Blink }

func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			m.value = m.input.Value()
			if strings.TrimSpace(m.value) != "" {
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m inputModel) View() string {
	return fmt.Sprintf("%s\n\n%s\n\nEnter to continue • Esc to cancel\n", m.title, m.input.View())
}

type selectRow struct {
	item     Item
	group    string
	subgroup string
	path     string
	depth    int
	header   bool
}

type selectModel struct {
	title             string
	query             string
	all               []Item
	visible           []Item
	rows              []selectRow
	collapsibleGroups bool
	collapsed         map[string]bool
	cursor            int
	selected          *Item
	cancelled         bool
}

func newSelectModel(opts Options) selectModel {
	model := selectModel{
		title: opts.Title, all: append([]Item(nil), opts.Items...),
		collapsibleGroups: opts.CollapsibleGroups,
		collapsed:         make(map[string]bool),
	}
	if model.collapsibleGroups {
		for _, item := range model.all {
			if item.Group != "" {
				model.collapsed[groupPath(item.Group)] = true
			}
			if item.Group != "" && item.Subgroup != "" {
				model.collapsed[subgroupPath(item.Group, item.Subgroup)] = true
			}
		}
	}
	model.applyFilter()
	return model
}

func (m selectModel) Init() tea.Cmd { return nil }

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c", "esc":
		m.cancelled = true
		return m, tea.Quit
	case "up", "ctrl+p":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "ctrl+n":
		if m.cursor+1 < len(m.rows) {
			m.cursor++
		}
	case "enter":
		if len(m.rows) != 0 {
			row := m.rows[m.cursor]
			if row.header {
				m.setGroupCollapsed(row.path, !m.collapsed[row.path])
				return m, nil
			}
			m.selected = &row.item
			return m, tea.Quit
		}
	case "left":
		if group, ok := m.currentHeader(); ok {
			m.setGroupCollapsed(group, true)
		}
	case "right":
		if group, ok := m.currentHeader(); ok {
			m.setGroupCollapsed(group, false)
		}
	case "backspace", "ctrl+h":
		if len(m.query) > 0 {
			runes := []rune(m.query)
			m.query = string(runes[:len(runes)-1])
			m.applyFilter()
		}
	default:
		if len(key.Runes) > 0 {
			m.query += string(key.Runes)
			m.applyFilter()
		}
	}
	return m, nil
}

func (m *selectModel) applyFilter() {
	m.visible = Filter(m.all, m.query)
	m.rows = m.rows[:0]
	if !m.collapsibleGroups || m.query != "" {
		for _, item := range m.visible {
			m.rows = append(m.rows, selectRow{
				item: item, group: item.Group, subgroup: item.Subgroup,
			})
		}
	} else {
		lastGroup := "\x00"
		lastSubgroup := "\x00"
		for _, item := range m.visible {
			if item.Group != lastGroup {
				if item.Group != "" {
					path := groupPath(item.Group)
					m.rows = append(m.rows, selectRow{
						group: item.Group, path: path, header: true,
					})
				}
				lastGroup = item.Group
				lastSubgroup = "\x00"
			}
			if item.Group != "" && m.collapsed[groupPath(item.Group)] {
				continue
			}
			if item.Subgroup != lastSubgroup {
				if item.Subgroup != "" {
					path := subgroupPath(item.Group, item.Subgroup)
					m.rows = append(m.rows, selectRow{
						group: item.Group, subgroup: item.Subgroup, path: path,
						depth: 1, header: true,
					})
				}
				lastSubgroup = item.Subgroup
			}
			if item.Subgroup != "" && m.collapsed[subgroupPath(item.Group, item.Subgroup)] {
				continue
			}
			depth := 0
			if item.Group != "" {
				depth++
			}
			if item.Subgroup != "" {
				depth++
			}
			m.rows = append(m.rows, selectRow{
				item: item, group: item.Group, subgroup: item.Subgroup, depth: depth,
			})
		}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
}

func (m *selectModel) currentHeader() (string, bool) {
	if m.query != "" || m.cursor < 0 || m.cursor >= len(m.rows) || !m.rows[m.cursor].header {
		return "", false
	}
	return m.rows[m.cursor].path, true
}

func (m *selectModel) setGroupCollapsed(path string, collapsed bool) {
	m.collapsed[path] = collapsed
	m.applyFilter()
	for index, row := range m.rows {
		if row.header && row.path == path {
			m.cursor = index
			return
		}
	}
}

func (m selectModel) View() string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s\n", m.title)
	if m.query != "" {
		fmt.Fprintf(&out, "Filter: %s\n", m.query)
	}
	if m.collapsibleGroups && m.query == "" {
		for index, row := range m.rows {
			prefix := "  "
			if index == m.cursor {
				prefix = "> "
			}
			if row.header {
				indicator := "▶"
				if !m.collapsed[row.path] {
					indicator = "▼"
				}
				label := row.group
				if row.subgroup != "" {
					label = row.subgroup
				}
				fmt.Fprintf(&out, "%s%s%s %s\n", prefix, strings.Repeat("  ", row.depth), indicator, label)
				continue
			}
			writeItem(&out, prefix+strings.Repeat("  ", row.depth), row.item)
		}
		if len(m.rows) == 0 {
			out.WriteString("\n  No matches\n")
		}
		out.WriteString("\nType to filter • ↑/↓ to navigate • Enter/←/→ to expand/collapse • Esc to cancel\n")
		return out.String()
	}

	lastGroup := "\x00"
	lastSubgroup := "\x00"
	for index, row := range m.rows {
		item := row.item
		if item.Group != lastGroup {
			if item.Group != "" {
				fmt.Fprintf(&out, "\n%s\n", item.Group)
			} else {
				out.WriteByte('\n')
			}
			lastGroup = item.Group
			lastSubgroup = "\x00"
		}
		if item.Subgroup != lastSubgroup {
			if item.Subgroup != "" {
				fmt.Fprintf(&out, "  %s\n", item.Subgroup)
			}
			lastSubgroup = item.Subgroup
		}
		prefix := "  "
		if index == m.cursor {
			prefix = "> "
		}
		if m.collapsibleGroups && item.Group != "" {
			prefix += "  "
		}
		if item.Subgroup != "" {
			prefix += "  "
		}
		writeItem(&out, prefix, item)
	}
	if len(m.rows) == 0 {
		out.WriteString("\n  No matches\n")
	}
	out.WriteString("\nType to filter • ↑/↓ to navigate • Enter to select • Esc to cancel\n")
	return out.String()
}

func writeItem(out *strings.Builder, prefix string, item Item) {
	fmt.Fprintf(out, "%s%s", prefix, item.Title)
	if item.Description != "" {
		fmt.Fprintf(out, " — %s", item.Description)
	}
	out.WriteByte('\n')
}

func Filter(items []Item, query string) []Item {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]Item(nil), items...)
	}
	haystacks := make([]string, len(items))
	exact := make([]Item, 0, len(items))
	for index, item := range items {
		haystacks[index] = itemHaystack(item)
		if strings.Contains(haystacks[index], query) {
			exact = append(exact, item)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	out := make([]Item, 0, len(items))
	for index, item := range items {
		if fuzzyMatch(haystacks[index], query) {
			out = append(out, item)
		}
	}
	return out
}

func itemHaystack(item Item) string {
	return strings.ToLower(item.Title + " " + item.Description + " " +
		item.Group + " " + item.Subgroup + " " + item.ID)
}

func groupPath(group string) string {
	return group
}

func subgroupPath(group, subgroup string) string {
	return groupPath(group) + "\x00" + subgroup
}

func fuzzyMatch(value, query string) bool {
	if strings.Contains(value, query) {
		return true
	}
	position := 0
	runes := []rune(value)
	for _, wanted := range []rune(query) {
		found := false
		for position < len(runes) {
			if runes[position] == wanted {
				found = true
				position++
				break
			}
			position++
		}
		if !found {
			return false
		}
	}
	return true
}
