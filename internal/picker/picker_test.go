package picker

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
)

var pickerANSIPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripPickerANSI(value string) string { return pickerANSIPattern.ReplaceAllString(value, "") }

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
	if len(model.rows) != 5 || model.collapsed[nodePath{group: "OpenAI"}] {
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
	if !model.collapsed[nodePath{group: "OpenCode Go", subgroup: "Claude"}] {
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
	model.setGroupCollapsed(nodePath{group: "OpenCode Go"}, false)
	model.setGroupCollapsed(nodePath{group: "OpenCode Zen"}, false)
	model.setGroupCollapsed(nodePath{group: "OpenCode Go", subgroup: "Claude"}, false)

	if model.collapsed[nodePath{group: "OpenCode Go", subgroup: "Claude"}] {
		t.Fatal("Go creator remained collapsed")
	}
	if !model.collapsed[nodePath{group: "OpenCode Zen", subgroup: "Claude"}] {
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

func TestFlatViewSeparatesItemsWithoutAddingASelectableRow(t *testing.T) {
	model := newSelectModel(Options{
		Title: "Agent harness",
		Items: []Item{
			{ID: "last", Title: "Last used"},
			{ID: "claude", Title: "Claude Code", SeparatorBefore: true},
			{ID: "codex", Title: "Codex"},
		},
	})
	want := "Agent harness\n\n> Last used\n\n  Claude Code\n  Codex\n" +
		"\nType to filter • ↑/↓ to navigate • Enter to select • Esc to cancel\n"
	if view := model.View(); view != want {
		t.Fatalf("view =\n%q\nwant\n%q", view, want)
	}
	if len(model.rows) != 3 {
		t.Fatalf("separator created a selectable row: %#v", model.rows)
	}

	model.query = "claude"
	model.applyFilter()
	if view := model.View(); strings.Contains(view, "claude\n\n\n") {
		t.Fatalf("filtered view retained an orphan separator: %q", view)
	}
}

func TestViewRendersEachModeByteForByte(t *testing.T) {
	items := []Item{
		{Title: "Harness default"},
		{ID: "openai/gpt", Title: "GPT", Description: "fast", Group: "OpenAI"},
		{ID: "openai/o3", Title: "o3", Group: "OpenAI"},
		{ID: "go/claude", Title: "Claude Go", Group: "OpenCode Go", Subgroup: "Claude"},
		{ID: "go/gemini", Title: "Gemini Go", Group: "OpenCode Go", Subgroup: "Google"},
		{ID: "zen/claude", Title: "Claude Zen", Group: "OpenCode Zen", Subgroup: "Claude"},
	}
	sharedSubgroup := []Item{
		{ID: "go/claude", Title: "Claude Go", Group: "OpenCode Go", Subgroup: "Claude"},
		{ID: "zen/claude", Title: "Claude Zen", Group: "OpenCode Zen", Subgroup: "Claude"},
	}
	collapsibleFooter := "\nType to filter • ↑/↓ to navigate • Enter/←/→ to expand/collapse • Esc to cancel\n"
	flatFooter := "\nType to filter • ↑/↓ to navigate • Enter to select • Esc to cancel\n"

	tests := []struct {
		name  string
		build func() selectModel
		want  string
	}{
		{
			name: "collapsed",
			build: func() selectModel {
				return newSelectModel(Options{Title: "Pi model", Items: items, CollapsibleGroups: true})
			},
			want: "Pi model\n> Harness default\n  ▶ OpenAI\n  ▶ OpenCode Go\n  ▶ OpenCode Zen\n" +
				collapsibleFooter,
		},
		{
			name: "nested expansion",
			build: func() selectModel {
				model := newSelectModel(Options{Title: "Pi model", Items: items, CollapsibleGroups: true})
				model.setGroupCollapsed(nodePath{group: "OpenCode Go"}, false)
				model.setGroupCollapsed(nodePath{group: "OpenCode Go", subgroup: "Claude"}, false)
				return model
			},
			want: "Pi model\n  Harness default\n  ▶ OpenAI\n  ▼ OpenCode Go\n>   ▼ Claude\n" +
				"      Claude Go\n    ▶ Google\n  ▶ OpenCode Zen\n" + collapsibleFooter,
		},
		{
			name: "collapsible filtered falls back to flat rendering",
			build: func() selectModel {
				model := newSelectModel(Options{Title: "Pi model", Items: items, CollapsibleGroups: true})
				model.query = "claude"
				model.applyFilter()
				return model
			},
			want: "Pi model\nFilter: claude\n\nOpenCode Go\n  Claude\n>     Claude Go\n" +
				"\nOpenCode Zen\n  Claude\n      Claude Zen\n" + flatFooter,
		},
		{
			name: "flat",
			build: func() selectModel {
				model := newSelectModel(Options{Title: "Pi model", Items: items})
				model.cursor = 3
				return model
			},
			want: "Pi model\n\n  Harness default\n\nOpenAI\n  GPT — fast\n  o3\n\nOpenCode Go\n" +
				"  Claude\n>   Claude Go\n  Google\n    Gemini Go\n\nOpenCode Zen\n  Claude\n" +
				"    Claude Zen\n" + flatFooter,
		},
		{
			name: "flat without matches",
			build: func() selectModel {
				model := newSelectModel(Options{Title: "Pi model", Items: items})
				model.query = "xyzzy"
				model.applyFilter()
				return model
			},
			want: "Pi model\nFilter: xyzzy\n\n  No matches\n" + flatFooter,
		},
		{
			name: "flat repeats a subgroup name shared with the previous group",
			build: func() selectModel {
				return newSelectModel(Options{Title: "Pi model", Items: sharedSubgroup})
			},
			want: "Pi model\n\nOpenCode Go\n  Claude\n>   Claude Go\n\nOpenCode Zen\n  Claude\n" +
				"    Claude Zen\n" + flatFooter,
		},
		{
			name: "collapsible repeats a subgroup name shared with the previous group",
			build: func() selectModel {
				model := newSelectModel(Options{
					Title: "Pi model", Items: sharedSubgroup, CollapsibleGroups: true,
				})
				for _, path := range []nodePath{
					{group: "OpenCode Go"}, {group: "OpenCode Go", subgroup: "Claude"},
					{group: "OpenCode Zen"}, {group: "OpenCode Zen", subgroup: "Claude"},
				} {
					model.setGroupCollapsed(path, false)
				}
				return model
			},
			want: "Pi model\n  ▼ OpenCode Go\n    ▼ Claude\n      Claude Go\n  ▼ OpenCode Zen\n" +
				">   ▼ Claude\n      Claude Zen\n" + collapsibleFooter,
		},
		{
			name: "collapsible without items",
			build: func() selectModel {
				return newSelectModel(Options{Title: "Empty", CollapsibleGroups: true})
			},
			want: "Empty\n\n  No matches\n" + collapsibleFooter,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if view := test.build().View(); view != test.want {
				t.Fatalf("view =\n%q\nwant\n%q", view, test.want)
			}
		})
	}
}

func TestStyledViewsPreservePickerStructure(t *testing.T) {
	items := []Item{
		{ID: "last", Title: "Last used — Codex · gpt", Description: "saved"},
		{ID: "gpt", Title: "GPT", Description: "fast", Group: "OpenAI", SeparatorBefore: true},
		{ID: "claude", Title: "Claude", Group: "OpenCode Go", Subgroup: "Anthropic"},
	}
	theme := ui.NewThemeWithProfile(&bytes.Buffer{}, termenv.TrueColor)

	tests := []struct {
		name  string
		build func(*ui.Theme) selectModel
	}{
		{
			name: "selection and last-used separator",
			build: func(theme *ui.Theme) selectModel {
				return newSelectModel(Options{Title: "Harness", Items: items, Theme: theme})
			},
		},
		{
			name: "filter and nested hierarchy",
			build: func(theme *ui.Theme) selectModel {
				model := newSelectModel(Options{Title: "Model", Items: items, CollapsibleGroups: true, Theme: theme})
				model.query = "claude"
				model.applyFilter()
				return model
			},
		},
		{
			name: "expanded groups",
			build: func(theme *ui.Theme) selectModel {
				model := newSelectModel(Options{Title: "Model", Items: items, CollapsibleGroups: true, Theme: theme})
				model.setGroupCollapsed(nodePath{group: "OpenAI"}, false)
				return model
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			styled := test.build(theme).View()
			plain := test.build(nil).View()
			if !strings.Contains(styled, "\x1b[") {
				t.Fatalf("styled view contains no ANSI: %q", styled)
			}
			if got := stripPickerANSI(styled); got != plain {
				t.Fatalf("stripped styled view =\n%q\nplain view =\n%q", got, plain)
			}
		})
	}
}

func TestInputUsesThemeForPromptAndSuppressesBuiltInColors(t *testing.T) {
	styledTheme := ui.NewThemeWithProfile(&bytes.Buffer{}, termenv.TrueColor)
	styled := newInputModel(Options{Title: "Agent name", Placeholder: "worker", Theme: styledTheme})
	if !strings.Contains(styled.View(), "\x1b[") ||
		!strings.Contains(styled.input.Cursor.Style.Render("x"), "\x1b[") {
		t.Fatalf("input title, prompt, or cursor was not styled: %q", styled.View())
	}

	plainTheme := ui.NewThemeWithProfile(&bytes.Buffer{}, termenv.Ascii)
	plain := newInputModel(Options{Title: "Agent name", Placeholder: "worker", Theme: plainTheme})
	for name, rendered := range map[string]string{
		"view":        plain.View(),
		"prompt":      plain.input.PromptStyle.Render("> "),
		"placeholder": plain.input.PlaceholderStyle.Render("worker"),
		"cursor":      plain.input.Cursor.Style.Render("x"),
	} {
		if strings.Contains(rendered, "\x1b[") {
			t.Fatalf("plain %s contains ANSI: %q", name, rendered)
		}
	}
}
