package board

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *model) View() tea.View {
	var lines []string
	if m.width < 20 || m.height < 7 {
		lines = []string{clip("Resize terminal · q quit", m.width)}
	} else {
		heading := "Task board · active"
		if m.history {
			heading = "Task board · all history"
		}
		taskStatus := "Tasks current"
		if m.taskError != "" {
			taskStatus = "Tasks stale: " + m.taskError
		}
		agentStatus := "workers current"
		switch {
		case m.agentError != "" && m.workersGood:
			agentStatus = "workers stale: " + m.agentError
		case m.agentError != "":
			agentStatus = "workers unavailable: " + m.agentError
		case !m.workersGood:
			agentStatus = "workers unavailable (loading)"
		}
		statusWidth := (m.width - 3) / 2
		status := pad(taskStatus, statusWidth) + " · " + clip(agentStatus, m.width-statusWidth-3)
		lines = append(lines, clip(heading, m.width), status)
		wide := m.width >= 100
		label := "Outline"
		if !m.details {
			label = "> Outline"
		}
		detailLabel := "Details"
		if m.details {
			detailLabel = "> Details"
		}
		if wide {
			width := (m.width - 3) / 2
			lines = append(lines, pad(label, width)+" │ "+detailLabel)
			for i := 0; i < m.bodyHeight(); i++ {
				lines = append(lines, pad(m.outlineLine(i, width), width)+" │ "+clip(m.detailLine(i), m.detailWidth()))
			}
		} else if m.details {
			lines = append(lines, detailLabel)
			for i := 0; i < m.bodyHeight(); i++ {
				lines = append(lines, clip(m.detailLine(i), m.width))
			}
		} else {
			lines = append(lines, label)
			for i := 0; i < m.bodyHeight(); i++ {
				lines = append(lines, m.outlineLine(i, m.width))
			}
		}
		lines = append(lines, clip(m.notice, m.width), clip("↑↓ select/scroll · ←→ tree · Enter details · Tab/Esc panels · h: history · r refresh · g worker · q quit", m.width))
	}
	v := tea.NewView(strings.Join(lines[:min(len(lines), max(0, m.height))], "\n"))
	v.AltScreen = true
	return v
}
func pad(s string, width int) string {
	s = clip(s, width)
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}
func (m *model) outlineLine(line, width int) string {
	if len(m.rows) == 0 {
		if line == 0 {
			return clip("No active tasks. h: history", width)
		}
		return ""
	}
	i := m.treeOffset + line/3
	if i >= len(m.rows) {
		return ""
	}
	row := m.rows[i]
	n := m.snapshot.Nodes[row.id]
	marker := " "
	if i == m.index {
		marker = ">"
	}
	branch := "·"
	if len(m.snapshot.Children[row.id]) > 0 {
		branch = "▸"
		if m.expanded[row.id] {
			branch = "▾"
		}
	}
	indent := strings.Repeat("  ", min(row.depth, max(0, width/2-8)))
	text := marker + indent + branch + " " + n.ID + " " + clip(n.Title, width)
	if line%3 == 1 {
		text = " " + indent + "  [" + n.State + "]"
		if n.Progress != "" {
			text += " " + n.Progress
		}
		if n.MissingParent {
			text += " [missing parent]"
		}
	} else if line%3 == 2 {
		text = " " + indent + "  " + m.worker(row.id)
	}
	return clip(text, width)
}
func (m *model) detailLine(line int) string {
	if m.selected() == "" {
		if line == 0 {
			return "Select a task"
		}
		return ""
	}
	if m.detailLines == nil {
		if line == 0 {
			return "Loading details…"
		}
		return ""
	}
	i := m.detailOffset + line
	if i >= len(m.detailLines) {
		return ""
	}
	return m.detailLines[i]
}
