package board

import (
	"context"
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

type row struct {
	id    string
	depth int
}
type tickMsg struct{}

const refreshInterval = 2 * time.Second

type focusMsg struct{ err error }
type detailMsg struct {
	generation uint64
	key        string
	lines      []string
}

type model struct {
	ctx                             context.Context
	cancel                          context.CancelFunc
	client                          libagent.Client
	repository                      *Repository
	snapshot                        *Snapshot
	workers                         map[string]Worker
	workersGood                     bool
	taskError, agentError, notice   string
	inFlight, pending               [2]bool
	focusing                        bool
	width, height                   int
	history, details                bool
	expanded                        map[string]bool
	rows                            []row
	index, treeOffset, detailOffset int
	detailLines                     []string
	detailKey                       string
	detailGeneration                uint64
	detailCancel                    context.CancelFunc
	cache                           map[string][]string
	cacheOrder                      []string
	interval                        time.Duration
}

func newModel(ctx context.Context, cancel context.CancelFunc, c libagent.Client, r *Repository, s *Snapshot) *model {
	m := &model{ctx: ctx, cancel: cancel, client: c, repository: r, snapshot: s, expanded: map[string]bool{}, workers: map[string]Worker{}, cache: map[string][]string{}, interval: refreshInterval}
	m.rebuild()
	return m
}
func (m *model) tick() tea.Cmd {
	return tea.Tick(m.interval, func(time.Time) tea.Msg { return tickMsg{} })
}
func (m *model) Init() tea.Cmd {
	return tea.Batch(m.refresh(Tasks, false), m.refresh(Workers, false), m.tick())
}
func (m *model) refresh(source Source, manual bool) tea.Cmd {
	if m.inFlight[source] {
		if manual {
			m.pending[source] = true
		}
		return nil
	}
	m.inFlight[source] = true
	ctx, c, r := m.ctx, m.client, m.repository
	return func() tea.Msg { return Load(ctx, c, r, source) }
}
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
		m.ensureVisible()
		return m, m.requestDetail()
	case tea.KeyPressMsg:
		return m, m.key(v.String())
	case tickMsg:
		return m, tea.Batch(m.refresh(Tasks, false), m.refresh(Workers, false), m.tick())
	case Observation:
		m.inFlight[v.Source] = false
		if v.Source == Tasks {
			if v.Err != nil {
				m.taskError = singleLine(v.Err.Error())
			} else {
				m.taskError = ""
				m.snapshot = v.Snapshot
				m.rebuild()
			}
		} else {
			if v.Err != nil {
				m.agentError = singleLine(v.Err.Error())
			} else {
				m.agentError = ""
				m.workers = v.Workers
				m.workersGood = true
			}
		}
		var again tea.Cmd
		if m.pending[v.Source] {
			m.pending[v.Source] = false
			again = m.refresh(v.Source, false)
		}
		return m, tea.Batch(again, m.requestDetail())
	case detailMsg:
		if v.generation != m.detailGeneration || v.key != m.detailKey {
			return m, nil
		}
		m.detailLines = v.lines
		m.detailOffset = min(m.detailOffset, max(0, len(v.lines)-m.bodyHeight()))
		m.cache[v.key] = v.lines
		m.cacheOrder = append(m.cacheOrder, v.key)
		if len(m.cacheOrder) > 4 {
			delete(m.cache, m.cacheOrder[0])
			m.cacheOrder = m.cacheOrder[1:]
		}
	case focusMsg:
		m.focusing = false
		if v.err != nil {
			m.notice = singleLine(v.err.Error())
		} else {
			m.notice = "Worker focused; board remains open in its tab"
		}
	}
	return m, nil
}
func (m *model) selected() string {
	if m.index < 0 || m.index >= len(m.rows) {
		return ""
	}
	return m.rows[m.index].id
}
func (m *model) rebuild() {
	prior := m.selected()
	position := m.index
	m.rows = nil
	if m.snapshot != nil {
		for id, children := range m.snapshot.Children {
			if len(children) > 0 && m.snapshot.Active[id] {
				if _, chosen := m.expanded[id]; !chosen {
					m.expanded[id] = true
				}
			}
		}
		var walk func([]string, int)
		walk = func(ids []string, depth int) {
			for _, id := range ids {
				if !m.history && !m.snapshot.Active[id] {
					continue
				}
				m.rows = append(m.rows, row{id, depth})
				if m.expanded[id] {
					walk(m.snapshot.Children[id], depth+1)
				}
			}
		}
		walk(m.snapshot.Roots, 0)
	}
	m.index = slices.IndexFunc(m.rows, func(r row) bool { return r.id == prior })
	if m.index < 0 {
		m.index = min(max(0, position), len(m.rows)-1)
	}
	if m.selected() != prior {
		m.detailOffset = 0
	}
	m.ensureVisible()
}
func (m *model) bodyHeight() int { return max(1, m.height-5) }
func (m *model) treeHeight() int { return max(1, m.bodyHeight()/3) }
func (m *model) ensureVisible() {
	if m.index < m.treeOffset {
		m.treeOffset = max(0, m.index)
	}
	if m.index >= m.treeOffset+m.treeHeight() {
		m.treeOffset = m.index - m.treeHeight() + 1
	}
	m.treeOffset = min(m.treeOffset, max(0, len(m.rows)-m.treeHeight()))
}
func (m *model) detailWidth() int {
	if m.width >= 100 {
		return max(1, m.width-(m.width-3)/2-3)
	}
	return max(1, m.width)
}
func (m *model) worker(id string) string {
	r, ok := m.snapshot.Records[id]
	if !ok || r.Owner == nil {
		return "unassigned"
	}
	label := singleLine(*r.Owner)
	activity := "unavailable"
	if w, ok := m.workers[*r.Owner]; ok {
		label = w.Name
		activity = w.Activity
	} else if m.workersGood {
		activity = "not live"
	}
	if m.agentError != "" || !m.workersGood {
		if m.workersGood {
			activity += " [stale]"
		} else {
			activity = "unavailable"
		}
	}
	return label + " · " + activity
}
func (m *model) requestDetail() tea.Cmd {
	id := m.selected()
	key := ""
	if id != "" {
		key = fmt.Sprintf("%s/%s/%d/%s", id, m.snapshot.Keys[id], m.detailWidth(), m.worker(id))
	}
	if key == m.detailKey {
		return nil
	}
	m.detailKey = key
	m.detailGeneration++
	if m.detailCancel != nil {
		m.detailCancel()
	}
	if id == "" {
		m.detailLines = nil
		return nil
	}
	if lines, ok := m.cache[key]; ok {
		m.detailLines = lines
		m.detailOffset = min(m.detailOffset, max(0, len(lines)-m.bodyHeight()))
		return nil
	}
	// Keep the scroll position during refresh/resize, while hiding obsolete content.
	m.detailLines = nil
	ctx, cancel := context.WithCancel(m.ctx)
	m.detailCancel = cancel
	s, width, worker, generation := m.snapshot, m.detailWidth(), m.worker(id), m.detailGeneration
	return func() tea.Msg {
		return detailMsg{generation: generation, key: key, lines: formatDetail(ctx, s, id, worker, width)}
	}
}
func (m *model) key(key string) tea.Cmd {
	prior := m.selected()
	switch key {
	case "q", "ctrl+c":
		m.cancel()
		return tea.Quit
	case "r":
		return tea.Batch(m.refresh(Tasks, true), m.refresh(Workers, true))
	case "h":
		m.history = !m.history
		m.rebuild()
	case "enter":
		if prior != "" {
			m.details = true
		}
	case "esc":
		m.details = false
	case "tab":
		if m.width >= 100 {
			m.details = !m.details
		}
	case "g":
		if m.focusing {
			return nil
		}
		if !m.workersGood || m.agentError != "" {
			m.notice = "Worker observation unavailable or stale; refresh before visiting"
			return nil
		}
		r, ok := m.snapshot.Records[prior]
		if !ok || r.Owner == nil {
			m.notice = "Selected task has no worker"
			return nil
		}
		if _, ok := m.workers[*r.Owner]; !ok {
			m.notice = "Selected worker is not live"
			return nil
		}
		m.focusing = true
		m.notice = "Visiting worker…"
		ctx, c, id, owner := m.ctx, m.client, prior, *r.Owner
		return func() tea.Msg { return focusMsg{err: FocusOwner(ctx, c, id, owner)} }
	case "up", "down", "left", "right":
		if m.details {
			delta := 1
			if key == "up" || key == "left" {
				delta = -1
			}
			m.detailOffset = max(0, min(m.detailOffset+delta, max(0, len(m.detailLines)-m.bodyHeight())))
		} else {
			switch key {
			case "up":
				m.index = max(0, m.index-1)
			case "down":
				m.index = min(len(m.rows)-1, m.index+1)
			case "right":
				if prior != "" && len(m.snapshot.Children[prior]) > 0 {
					if !m.expanded[prior] {
						m.expanded[prior] = true
						m.rebuild()
					} else if m.index+1 < len(m.rows) && m.rows[m.index+1].depth > m.rows[m.index].depth {
						m.index++
					}
				}
			case "left":
				if prior != "" {
					if m.expanded[prior] && len(m.snapshot.Children[prior]) > 0 {
						m.expanded[prior] = false
						m.rebuild()
					} else if p := m.snapshot.Records[prior].Parent; p != nil {
						if i := slices.IndexFunc(m.rows, func(r row) bool { return r.id == *p }); i >= 0 {
							m.index = i
						}
					}
				}
			}
		}
	}
	if m.selected() != prior {
		m.detailOffset = 0
	}
	m.ensureVisible()
	return m.requestDetail()
}
