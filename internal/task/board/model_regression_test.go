package board

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

func TestTabSwitchesPanelsOnlyWhenWide(t *testing.T) {
	for _, width := range []int{80, 99, 100, 120} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := boardModel(t, task.Record{ID: "11111111", Status: task.Created})
			m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
			key(m, "tab")
			if m.details != (width >= 100) {
				t.Fatalf("Tab from outline at width %d: details=%v", width, m.details)
			}
			key(m, "enter")
			key(m, "tab")
			if m.details != (width < 100) {
				t.Fatalf("Tab from details at width %d: details=%v", width, m.details)
			}
		})
	}
}

func TestDetailArrowsScrollFormattedContent(t *testing.T) {
	m := boardModel(t, task.Record{ID: "11111111", Status: task.Created, Brief: strings.Repeat("report line\n", 100)})
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m.Update(cmd())
	key(m, "enter")
	for _, keys := range [][2]string{{"down", "up"}, {"right", "left"}} {
		before := m.View().Content
		key(m, keys[0])
		if m.detailOffset != 1 || m.View().Content == before {
			t.Fatalf("%s did not scroll the displayed details: offset=%d", keys[0], m.detailOffset)
		}
		if m.selected() != "11111111" {
			t.Fatal("scrolling details changed task selection")
		}
		key(m, keys[1])
		if m.detailOffset != 0 || m.View().Content != before {
			t.Fatalf("%s did not restore the displayed details", keys[1])
		}
	}
}

func TestRightReopensCollapsedGroup(t *testing.T) {
	m := boardModel(t,
		task.Record{ID: "11111111", Title: "parent", Status: task.Created},
		task.Record{ID: "22222222", Title: "child", Status: task.Created, Parent: tasktest.Ptr("11111111")},
	)
	key(m, "left")
	if len(m.rows) != 1 {
		t.Fatal("fixture group did not collapse")
	}
	key(m, "right")
	if len(m.rows) != 2 || !strings.Contains(m.View().Content, "22222222 child") {
		t.Fatal("Right did not reveal the collapsed group's child")
	}
	if m.selected() != "11111111" {
		t.Fatal("expanding a group changed selection")
	}
	key(m, "right")
	if m.selected() != "22222222" {
		t.Fatal("Right on an expanded group did not select its child")
	}
}

func TestReturningToTaskReusesFormattedDetails(t *testing.T) {
	m := boardModel(t,
		task.Record{ID: "11111111", Status: task.Created, Brief: "first task report"},
		task.Record{ID: "22222222", Status: task.Created, Brief: "second task report"},
	)
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(cmd())
	first := slices.Clone(m.detailLines)
	cmd = key(m, "down")
	if cmd == nil {
		t.Fatal("new selection did not schedule detail formatting")
	}
	m.Update(cmd())
	if !strings.Contains(strings.Join(m.detailLines, "\n"), "second task report") {
		t.Fatal("second task details were not displayed")
	}
	if cmd = key(m, "up"); cmd != nil {
		t.Fatal("returning to a cached task scheduled formatting again")
	}
	if !slices.Equal(m.detailLines, first) {
		t.Fatal("cached first-task details were not immediately restored")
	}
}

func TestObservedMissingWorkerDisplaysNotLive(t *testing.T) {
	m := boardModel(t, task.Record{ID: "11111111", Status: task.Assigned, Owner: tasktest.Ptr("owner")})
	if !strings.Contains(m.View().Content, "owner · unavailable") {
		t.Fatal("unobserved worker should be unavailable")
	}
	m.Update(Observation{Source: Workers, Workers: map[string]Worker{}})
	view := m.View().Content
	if !strings.Contains(view, "owner · not live") || strings.Contains(view, "unavailable") {
		t.Fatalf("successful empty worker observation mislabeled: %s", view)
	}
}

func TestInitRefreshesBothSourcesImmediately(t *testing.T) {
	cwd := identitytest.Repository(t)
	id := tasktest.Seed(t, cwd, task.Record{Title: "initial task", Status: task.Created})
	a := tasktest.Agent("w1:p1", "term", "worker")
	rec := tasktest.Register(t, cwd, a)
	m := boardModel(t)
	m.client = tasktest.Client(t, cwd, "", herdrscript.Call{Method: "agent.list", Result: herdr.AgentListResult{Type: "agent_list", Agents: []herdr.AgentDetails{a.Agent}}})
	m.repository = repository(t, cwd)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init scheduled no observations")
	}
	// Execute the batch as Bubble Tea does, so a timer-only Init cannot pass.
	results := make(chan tea.Msg, 3)
	go func() {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, child := range batch {
				go func() { results <- child() }()
			}
		} else {
			results <- msg
		}
	}()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	seen := map[Source]bool{}
	for len(seen) < 2 {
		select {
		case msg := <-results:
			observation, ok := msg.(Observation)
			if !ok {
				t.Fatalf("expected immediate observation, got %T", msg)
			}
			if observation.Err != nil {
				t.Fatal(observation.Err)
			}
			seen[observation.Source] = true
			m.Update(observation)
		case <-deadline.C:
			t.Fatalf("Init did not refresh both sources before the periodic tick: observed=%v", seen)
		}
	}
	if m.selected() != id || m.workers[rec.ID].Name != "worker" {
		t.Fatal("initial observations did not populate tasks and workers")
	}
}

func TestPeriodicRefreshUsesTwoSecondInterval(t *testing.T) {
	if m := boardModel(t); m.interval != 2*time.Second {
		t.Fatalf("refresh interval = %s, want 2s", m.interval)
	}
}

func TestInitAndTickScheduleNextTickAfterInterval(t *testing.T) {
	m := boardModel(t)
	m.interval = 20 * time.Millisecond
	m.inFlight = [2]bool{true, true}
	schedulers := map[string]func() tea.Cmd{
		"Init":    m.Init,
		"tickMsg": func() tea.Cmd { _, cmd := m.Update(tickMsg{}); return cmd },
	}
	for name, schedule := range schedulers {
		start := time.Now()
		cmd := schedule()
		if cmd == nil {
			t.Fatalf("%s scheduled no periodic tick", name)
		}
		result := make(chan tea.Msg, 1)
		go func() { result <- cmd() }()
		select {
		case msg := <-result:
			if _, ok := msg.(tickMsg); !ok {
				t.Fatalf("%s periodic command returned %T, want tickMsg", name, msg)
			}
			if elapsed := time.Since(start); elapsed < m.interval {
				t.Fatalf("%s periodic tick arrived after %s, want at least %s", name, elapsed, m.interval)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s periodic tick did not arrive within one second of a %s interval", name, m.interval)
		}
	}
}

func TestResizeReformatsSelectedDetailsAtNewWidth(t *testing.T) {
	r := task.Record{ID: "11111111", Status: task.Created, Brief: strings.Repeat("世界", 100)}
	m := boardModel(t, r)
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd == nil {
		t.Fatal("resize did not schedule selected detail formatting")
	}
	m.Update(cmd())
	before := slices.Clone(m.detailLines)
	for _, width := range []int{60, 120} {
		_, cmd = m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		if cmd == nil {
			t.Fatalf("resize to %d did not schedule selected detail formatting", width)
		}
		m.Update(cmd())
		want := formatDetail(context.Background(), m.snapshot, r.ID, "unassigned", m.detailWidth())
		if !slices.Equal(m.detailLines, want) || slices.Equal(m.detailLines, before) {
			t.Fatalf("resize to %d did not rewrap the displayed report", width)
		}
		before = slices.Clone(m.detailLines)
	}
}
