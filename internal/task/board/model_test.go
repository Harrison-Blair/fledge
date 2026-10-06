package board

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func boardModel(t *testing.T, rs ...task.Record) *model {
	t.Helper()
	s, err := project(rs)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m := newModel(ctx, cancel, libagent.Client{}, nil, s)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	return m
}
func key(m *model, k string) tea.Cmd { return m.key(k) }
func TestTreeSelectionExpansionHistoryAndRefresh(t *testing.T) {
	rs := []task.Record{{ID: "11111111", Title: "parent", Status: task.Verified}, {ID: "22222222", Title: "child", Status: task.Created, Parent: tasktest.Ptr("11111111")}, {ID: "33333333", Status: task.Cancelled}, {ID: "44444444", Status: task.Created}}
	m := boardModel(t, rs...)
	if len(m.rows) != 3 {
		t.Fatalf("rows %v", m.rows)
	}
	key(m, "down")
	if m.selected() != "22222222" {
		t.Fatal(m.selected())
	}
	key(m, "left")
	if m.selected() != "11111111" {
		t.Fatal("parent")
	}
	key(m, "left")
	if len(m.rows) != 2 {
		t.Fatal("collapse")
	}
	s, _ := project(rs)
	m.Update(Observation{Source: Tasks, Snapshot: s})
	if len(m.rows) != 2 {
		t.Fatal("refresh overwrote collapse")
	}
	key(m, "h")
	if len(m.rows) != 3 {
		t.Fatal("history")
	}
	key(m, "down")
	if m.selected() != "33333333" {
		t.Fatal(m.selected())
	}
	key(m, "h")
	if m.selected() != "44444444" {
		t.Fatal("lost prior row position")
	}
	s, _ = project(nil)
	m.Update(Observation{Source: Tasks, Snapshot: s})
	if m.selected() != "" {
		t.Fatal("selection not cleared")
	}
}
func TestNewActiveGroupsExpandWithoutOverwritingChoices(t *testing.T) {
	m := boardModel(t, task.Record{ID: "11111111", Status: task.Created})
	s, _ := project([]task.Record{{ID: "11111111", Status: task.Created}, {ID: "22222222", Status: task.Created, Parent: tasktest.Ptr("11111111")}})
	m.Update(Observation{Source: Tasks, Snapshot: s})
	if len(m.rows) != 2 {
		t.Fatal("new group collapsed")
	}
	key(m, "left")
	m.Update(Observation{Source: Tasks, Snapshot: s})
	if len(m.rows) != 1 {
		t.Fatal("choice overwritten")
	}
}
func TestResizeFocusAndScrollStable(t *testing.T) {
	m := boardModel(t, task.Record{ID: "11111111", Status: task.Created})
	key(m, "enter")
	m.detailLines = strings.Split(strings.Repeat("line\n", 100), "\n")
	m.detailOffset = 10
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if !m.details || m.detailOffset != 10 {
		t.Fatal("narrow state lost")
	}
	if strings.Contains(m.View().Content, "Outline") {
		t.Fatal("narrow details must fill screen")
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	if !m.details || m.detailOffset != 10 {
		t.Fatal("wide state lost")
	}
	key(m, "esc")
	if m.details {
		t.Fatal("escape")
	}
	key(m, "tab")
	if !m.details {
		t.Fatal("tab")
	}
	m.Update(tea.WindowSizeMsg{Width: 5, Height: 2})
	for _, line := range strings.Split(m.View().Content, "\n") {
		if len(line) > 5 {
			t.Fatal("tiny overflow")
		}
	}
	if cmd := key(m, "q"); cmd == nil {
		t.Fatal("quit unavailable")
	}
}
func TestIndependentFailuresCoalescingAndStaleDetail(t *testing.T) {
	m := boardModel(t, task.Record{ID: "11111111", Status: task.Created})
	m.workers = map[string]Worker{"owner": {Name: "kept", Activity: "idle"}}
	m.workersGood = true
	m.inFlight[Tasks] = true
	m.inFlight[Workers] = true
	key(m, "r")
	key(m, "r")
	if !m.pending[Tasks] || !m.pending[Workers] {
		t.Fatal("manual refresh not coalesced")
	}
	_, cmd := m.Update(Observation{Source: Tasks, Err: errors.New("disk")})
	if cmd == nil || !m.inFlight[Tasks] || m.pending[Tasks] || m.selected() != "11111111" || m.taskError == "" {
		t.Fatal("task failure handling")
	}
	m.Update(Observation{Source: Workers, Err: errors.New("offline")})
	if m.workers["owner"].Name != "kept" || m.agentError == "" {
		t.Fatal("worker snapshot discarded")
	}
	old := m.detailGeneration
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 20})
	m.Update(detailMsg{generation: old, key: "old", lines: []string{"obsolete"}})
	if len(m.detailLines) > 0 && m.detailLines[0] == "obsolete" {
		t.Fatal("outdated formatting accepted")
	}
}
func TestLargeDetailsDoNotRunOnEventLoopAndCache(t *testing.T) {
	m := boardModel(t, task.Record{ID: "11111111", Status: task.Created, Result: tasktest.Ptr(strings.Repeat("large report 世界\n", 300000))})
	start := time.Now()
	cmd := m.requestDetail()
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("formatting blocked event loop")
	}
	if cmd == nil { // WindowSize already scheduled the same key. Force a width change.
		m.width = 122
		cmd = m.requestDetail()
	}
	if cmd == nil {
		t.Fatal("no async formatting")
	}
	msg := cmd()
	m.Update(msg)
	if len(m.detailLines) < 300000 {
		t.Fatal("missing report")
	}
	if m.requestDetail() != nil {
		t.Fatal("identical detail reformatted")
	}
}
func TestEmptyHistoryAndUnavailableWorkerView(t *testing.T) {
	m := boardModel(t, task.Record{ID: "11111111", Status: task.Verified})
	if !strings.Contains(m.View().Content, "h: history") {
		t.Fatal("history hint")
	}
	if !strings.Contains(m.View().Content, "unavailable") {
		t.Fatal("initial worker availability")
	}
	key(m, "h")
	if len(m.rows) != 1 {
		t.Fatal("history hidden")
	}
}

func TestOutlineShowsStateAndWorkerWithLongTitle(t *testing.T) {
	m := boardModel(t, task.Record{ID: "11111111", Title: strings.Repeat("title ", 100), Status: task.Assigned, Owner: tasktest.Ptr("owner")})
	m.workersGood = true
	m.workers = map[string]Worker{"owner": {Name: "worker", Activity: "blocked (codex)"}}
	view := m.View().Content
	for _, want := range []string{"11111111", "title", "assigned", "worker", "blocked"} {
		if !strings.Contains(view, want) {
			t.Errorf("outline hides %s", want)
		}
	}
}

func TestRefreshPreservesDetailScrollAndContentKey(t *testing.T) {
	r := task.Record{ID: "11111111", Status: task.Created, Brief: strings.Repeat("line\n", 100)}
	m := boardModel(t, r)
	m.width = 122
	cmd := m.requestDetail()
	m.Update(cmd())
	m.detailOffset = 20
	key(m, "enter")
	oldKey := m.detailKey
	s, _ := project([]task.Record{r})
	m.Update(Observation{Source: Tasks, Snapshot: s})
	if m.detailOffset != 20 || m.detailKey != oldKey || m.detailLines == nil {
		t.Fatal("ordinary refresh discarded scroll or cache")
	}
	r.Brief += "changed"
	s, _ = project([]task.Record{r})
	_, cmd = m.Update(Observation{Source: Tasks, Snapshot: s})
	if cmd == nil || m.detailKey == oldKey || m.detailOffset != 20 {
		t.Fatal("changed report not refreshed")
	}
}

func TestStaleWorkersDisableFocusAndRecoveryEnablesIt(t *testing.T) {
	m := boardModel(t, task.Record{ID: "11111111", Status: task.Assigned, Owner: tasktest.Ptr("owner")})
	m.workersGood = true
	m.workers = map[string]Worker{"owner": {Name: "live", Activity: "working"}}
	m.Update(Observation{Source: Workers, Err: errors.New("offline")})
	if key(m, "g") != nil || m.focusing {
		t.Fatal("stale worker navigation enabled")
	}
	m.Update(Observation{Source: Workers, Workers: m.workers})
	if key(m, "g") == nil || !m.focusing {
		t.Fatal("recovery didn't enable navigation")
	}
	if key(m, "g") != nil {
		t.Fatal("overlapping focus")
	}
}

func TestBothObservationErrorsRemainVisible(t *testing.T) {
	m := boardModel(t)
	m.taskError = strings.Repeat("task read error ", 50)
	m.agentError = "daemon offline"
	view := m.View().Content
	if !strings.Contains(view, "Tasks stale") || !strings.Contains(view, "workers unavailable") {
		t.Fatal("one error obscured the other source status")
	}
}
