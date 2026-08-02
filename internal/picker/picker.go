package picker

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/ui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

var ErrCancelled = errors.New("picker cancelled")

type Item struct {
	ID              string
	Title           string
	Description     string
	Group           string
	Subgroup        string
	SeparatorBefore bool
}

type Options struct {
	Title             string
	Placeholder       string
	Items             []Item
	Input             io.Reader
	Output            io.Writer
	CollapsibleGroups bool
	Theme             *ui.Theme
}

func Input(opts Options) (string, error) {
	model := newInputModel(opts)
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

func newInputModel(opts Options) inputModel {
	field := textinput.New()
	field.Placeholder = opts.Placeholder
	field.PromptStyle = opts.Theme.Style(ui.RoleAccent)
	field.TextStyle = opts.Theme.Plain()
	field.PlaceholderStyle = opts.Theme.Plain()
	field.CompletionStyle = opts.Theme.Plain()
	field.Cursor.Style = opts.Theme.Style(ui.RoleAccent)
	field.Cursor.TextStyle = opts.Theme.Plain()
	field.CursorStyle = opts.Theme.Style(ui.RoleAccent)
	field.Focus()
	return inputModel{title: opts.Title, input: field, theme: opts.Theme}
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
	theme     *ui.Theme
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
	return fmt.Sprintf("%s\n\n%s\n\nEnter to continue • Esc to cancel\n",
		m.theme.Accent(m.title), m.input.View())
}

type nodePath struct {
	group    string
	subgroup string
}

type selectRow struct {
	item     Item
	group    string
	subgroup string
	path     nodePath
	depth    int
	header   bool
}

type selectModel struct {
	title             string
	query             string
	all               []Item
	rows              []selectRow
	collapsibleGroups bool
	collapsed         map[nodePath]bool
	cursor            int
	selected          *Item
	cancelled         bool
	theme             *ui.Theme
}

func newSelectModel(opts Options) selectModel {
	model := selectModel{
		title: opts.Title, all: append([]Item(nil), opts.Items...),
		collapsibleGroups: opts.CollapsibleGroups,
		collapsed:         make(map[nodePath]bool),
		theme:             opts.Theme,
	}
	if model.collapsibleGroups {
		for _, item := range model.all {
			if item.Group != "" {
				model.collapsed[nodePath{group: item.Group}] = true
			}
			if item.Group != "" && item.Subgroup != "" {
				model.collapsed[nodePath{group: item.Group, subgroup: item.Subgroup}] = true
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
	visible := Filter(m.all, m.query)
	rows := m.rows[:0]
	if m.groupsAreCollapsible() {
		m.rows = m.appendGroupedRows(rows, visible)
	} else {
		m.rows = appendItemRows(rows, visible)
	}
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
}

func (m selectModel) groupsAreCollapsible() bool {
	return m.collapsibleGroups && m.query == ""
}

func appendItemRows(rows []selectRow, items []Item) []selectRow {
	for _, item := range items {
		rows = append(rows, selectRow{
			item: item, group: item.Group, subgroup: item.Subgroup,
		})
	}
	return rows
}

func (m selectModel) appendGroupedRows(rows []selectRow, items []Item) []selectRow {
	var walk groupWalk
	for _, item := range items {
		newGroup, newSubgroup := walk.advance(item)
		groupKey := nodePath{group: item.Group}
		subgroupKey := nodePath{group: item.Group, subgroup: item.Subgroup}
		if newGroup && item.Group != "" {
			rows = append(rows, selectRow{group: item.Group, path: groupKey, header: true})
		}
		if item.Group != "" && m.collapsed[groupKey] {
			continue
		}
		if newSubgroup && item.Subgroup != "" {
			rows = append(rows, selectRow{
				group: item.Group, subgroup: item.Subgroup, path: subgroupKey,
				depth: 1, header: true,
			})
		}
		if item.Subgroup != "" && m.collapsed[subgroupKey] {
			continue
		}
		rows = append(rows, selectRow{
			item: item, group: item.Group, subgroup: item.Subgroup, depth: itemDepth(item),
		})
	}
	return rows
}

func itemDepth(item Item) int {
	depth := 0
	if item.Group != "" {
		depth++
	}
	if item.Subgroup != "" {
		depth++
	}
	return depth
}

func (m *selectModel) currentHeader() (nodePath, bool) {
	if m.query != "" || m.cursor < 0 || m.cursor >= len(m.rows) || !m.rows[m.cursor].header {
		return nodePath{}, false
	}
	return m.rows[m.cursor].path, true
}

func (m *selectModel) setGroupCollapsed(path nodePath, collapsed bool) {
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
	fmt.Fprintf(&out, "%s\n", m.theme.Accent(m.title))
	if m.query != "" {
		fmt.Fprintf(&out, "%s %s\n", m.theme.Accent("Filter:"), m.query)
	}
	if m.groupsAreCollapsible() {
		m.renderCollapsibleRows(&out)
		m.writeFooter(&out, "Enter/←/→ to expand/collapse")
	} else {
		m.renderFlatRows(&out)
		m.writeFooter(&out, "Enter to select")
	}
	return out.String()
}

func (m selectModel) renderCollapsibleRows(out *strings.Builder) {
	for index, row := range m.rows {
		prefix := m.cursorPrefix(index)
		if row.header {
			indicator := "▶"
			if !m.collapsed[row.path] {
				indicator = "▼"
			}
			label := row.group
			if row.subgroup != "" {
				label = row.subgroup
			}
			fmt.Fprintf(out, "%s%s%s %s\n", prefix, strings.Repeat("  ", row.depth), indicator, m.theme.Accent(label))
			continue
		}
		writeItem(out, prefix+strings.Repeat("  ", row.depth), row.item, index == m.cursor, m.theme)
	}
}

func (m selectModel) renderFlatRows(out *strings.Builder) {
	var walk groupWalk
	for index, row := range m.rows {
		item := row.item
		newGroup, newSubgroup := walk.advance(item)
		if item.SeparatorBefore && index > 0 && !newGroup {
			out.WriteByte('\n')
		}
		if newGroup {
			if item.Group != "" {
				fmt.Fprintf(out, "\n%s\n", m.theme.Accent(item.Group))
			} else {
				out.WriteByte('\n')
			}
		}
		if newSubgroup && item.Subgroup != "" {
			fmt.Fprintf(out, "  %s\n", m.theme.Accent(item.Subgroup))
		}
		prefix := m.cursorPrefix(index)
		if m.collapsibleGroups && item.Group != "" {
			prefix += "  "
		}
		if item.Subgroup != "" {
			prefix += "  "
		}
		writeItem(out, prefix, item, index == m.cursor, m.theme)
	}
}

func (m selectModel) cursorPrefix(index int) string {
	if index == m.cursor {
		return m.theme.Accent("> ")
	}
	return "  "
}

func (m selectModel) writeFooter(out *strings.Builder, action string) {
	if len(m.rows) == 0 {
		out.WriteString("\n  No matches\n")
	}
	fmt.Fprintf(out, "\nType to filter • ↑/↓ to navigate • %s • Esc to cancel\n", action)
}

func writeItem(out *strings.Builder, prefix string, item Item, selected bool, theme *ui.Theme) {
	title := item.Title
	if selected {
		title = theme.Accent(title)
	}
	fmt.Fprintf(out, "%s%s", prefix, title)
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

// groupWalk tracks group and subgroup boundaries while items are visited in
// order, so row building and flat rendering share one notion of where a
// heading starts.
type groupWalk struct {
	path    nodePath
	started bool
}

func (w *groupWalk) advance(item Item) (newGroup, newSubgroup bool) {
	newGroup = !w.started || item.Group != w.path.group
	newSubgroup = newGroup || item.Subgroup != w.path.subgroup
	w.path = nodePath{group: item.Group, subgroup: item.Subgroup}
	w.started = true
	return newGroup, newSubgroup
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
