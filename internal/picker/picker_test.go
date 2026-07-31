package picker

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFilterFuzzyMatchesMetadataAndPreservesOrdering(t *testing.T) {
	items := []Item{
		{ID: "default", Title: "Harness default"},
		{ID: "openrouter/claude", Title: "Sonnet", Group: "Claude"},
		{ID: "openai/gpt", Title: "GPT", Group: "OpenAI"},
	}
	filtered := Filter(items, "snnt")
	if len(filtered) != 1 || filtered[0].Title != "Sonnet" {
		t.Fatalf("filtered = %#v", filtered)
	}
	filtered = Filter(items, "open")
	if len(filtered) != 2 || filtered[0].Title != "Sonnet" || filtered[1].Title != "GPT" {
		t.Fatalf("provider filter ordering = %#v", filtered)
	}
}

func TestSelectNavigationAndCancellationWithInjectedIO(t *testing.T) {
	items := []Item{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}}
	var output bytes.Buffer
	selected, err := Select(Options{
		Title: "Choose", Items: items, Input: bytes.NewBufferString("\x1b[B\r"), Output: &output,
	})
	if err != nil || selected.ID != "b" {
		t.Fatalf("selected=%#v err=%v output=%q", selected, err, output.String())
	}
	_, err = Select(Options{
		Title: "Choose", Items: items, Input: bytes.NewBufferString("\x1b"), Output: &bytes.Buffer{},
	})
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("cancel error = %v", err)
	}
}

func TestCollapsibleGroupsStartCollapsedAndCanExpand(t *testing.T) {
	model := newSelectModel(Options{
		Title: "Pi model",
		Items: []Item{
			{Title: "Harness default"},
			{ID: "openai/gpt", Title: "GPT", Group: "OpenAI"},
			{ID: "openai/o3", Title: "o3", Group: "OpenAI"},
			{ID: "anthropic/sonnet", Title: "Sonnet", Group: "Claude"},
		},
		CollapsibleGroups: true,
	})
	if len(model.rows) != 3 || model.rows[1].group != "OpenAI" || !model.rows[1].header {
		t.Fatalf("initial rows = %#v", model.rows)
	}
	if view := model.View(); !strings.Contains(view, "▶ OpenAI") ||
		strings.Contains(view, "GPT") {
		t.Fatalf("collapsed view = %q", view)
	}

	model.cursor = 1
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(selectModel)
	if len(model.rows) != 5 || model.collapsed["OpenAI"] {
		t.Fatalf("expanded rows = %#v collapsed=%v", model.rows, model.collapsed)
	}
	if view := model.View(); !strings.Contains(view, "▼ OpenAI") ||
		!strings.Contains(view, "GPT") {
		t.Fatalf("expanded view = %q", view)
	}
}

func TestCollapsibleGroupFilteringShowsSelectableModels(t *testing.T) {
	model := newSelectModel(Options{
		Title: "Pi model",
		Items: []Item{
			{ID: "openai/gpt", Title: "GPT", Group: "OpenAI"},
			{ID: "anthropic/sonnet", Title: "Sonnet", Group: "Claude"},
		},
		CollapsibleGroups: true,
	})
	model.query = "gpt"
	model.applyFilter()
	if len(model.rows) != 1 || model.rows[0].header || model.rows[0].item.ID != "openai/gpt" {
		t.Fatalf("filtered rows = %#v", model.rows)
	}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(selectModel)
	if cmd == nil || model.selected == nil || model.selected.ID != "openai/gpt" {
		t.Fatalf("selected=%#v cmd=%v", model.selected, cmd)
	}
}

func TestNestedGroupsExpandOneLevelAtATime(t *testing.T) {
	model := newSelectModel(Options{
		Title: "Pi model",
		Items: []Item{
			{Title: "Harness default"},
			{ID: "openai-codex/gpt-5", Title: "gpt-5", Group: "OpenAI Codex"},
			{
				ID: "opencode-go/anthropic/claude-4", Title: "anthropic/claude-4",
				Group: "OpenCode Go", Subgroup: "Claude",
			},
		},
		CollapsibleGroups: true,
	})
	if len(model.rows) != 3 || !model.rows[1].header || !model.rows[2].header {
		t.Fatalf("initial rows = %#v", model.rows)
	}

	model.cursor = 2
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(selectModel)
	if len(model.rows) != 4 || !model.rows[3].header || model.rows[3].subgroup != "Claude" {
		t.Fatalf("provider expansion rows = %#v", model.rows)
	}
	if !model.collapsed[subgroupPath("OpenCode Go", "Claude")] {
		t.Fatalf("creator should remain collapsed: %v", model.collapsed)
	}

	model.cursor = 3
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(selectModel)
	if len(model.rows) != 5 || model.rows[4].header ||
		model.rows[4].item.ID != "opencode-go/anthropic/claude-4" {
		t.Fatalf("creator expansion rows = %#v", model.rows)
	}
}

func TestNestedCollapseStateUsesCompletePath(t *testing.T) {
	model := newSelectModel(Options{
		Items: []Item{
			{ID: "go/claude", Title: "Claude Go", Group: "OpenCode Go", Subgroup: "Claude"},
			{ID: "zen/claude", Title: "Claude Zen", Group: "OpenCode Zen", Subgroup: "Claude"},
		},
		CollapsibleGroups: true,
	})
	model.setGroupCollapsed(groupPath("OpenCode Go"), false)
	model.setGroupCollapsed(groupPath("OpenCode Zen"), false)
	model.setGroupCollapsed(subgroupPath("OpenCode Go", "Claude"), false)

	if model.collapsed[subgroupPath("OpenCode Go", "Claude")] {
		t.Fatal("Go creator remained collapsed")
	}
	if !model.collapsed[subgroupPath("OpenCode Zen", "Claude")] {
		t.Fatal("Zen creator collapse state changed with Go creator")
	}
}

func TestNestedFilteringMatchesEveryHierarchyLevelAndRoute(t *testing.T) {
	items := []Item{
		{
			ID: "opencode-go/anthropic/claude-sonnet-4", Title: "anthropic/claude-sonnet-4",
			Group: "OpenCode Go", Subgroup: "Claude",
		},
		{
			ID: "opencode/google/gemini-2.5-pro", Title: "google/gemini-2.5-pro",
			Group: "OpenCode Zen", Subgroup: "Google",
		},
	}
	for _, query := range []string{"OpenCode Go", "Claude", "sonnet", "opencode-go/anthropic"} {
		model := newSelectModel(Options{Items: items, CollapsibleGroups: true})
		model.query = query
		model.applyFilter()
		if len(model.rows) != 1 || model.rows[0].header ||
			model.rows[0].item.ID != items[0].ID {
			t.Fatalf("query %q rows = %#v", query, model.rows)
		}
		if view := model.View(); !strings.Contains(view, "OpenCode Go") ||
			!strings.Contains(view, "Claude") {
			t.Fatalf("query %q hierarchy missing from view %q", query, view)
		}
	}
}

func TestInputAcceptsValueAndCancels(t *testing.T) {
	value, err := Input(Options{
		Title: "Name", Input: bytes.NewBufferString("worker\r"), Output: &bytes.Buffer{},
	})
	if err != nil || value != "worker" {
		t.Fatalf("value=%q err=%v", value, err)
	}
	_, err = Input(Options{
		Title: "Name", Input: bytes.NewBufferString("\x03"), Output: &bytes.Buffer{},
	})
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("cancel error = %v", err)
	}
}
