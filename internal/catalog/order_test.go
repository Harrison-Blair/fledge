package catalog

import (
	"reflect"
	"slices"
	"testing"
)

func TestCompareIDs(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		want []string
	}{
		{
			name: "numeric",
			ids:  []string{"gpt-5.8", "gpt-5.10"},
			want: []string{"gpt-5.10", "gpt-5.8"},
		},
		{
			name: "letter-prefix",
			ids:  []string{"claude-opus-thinking", "claude-opus"},
			want: []string{"claude-opus", "claude-opus-thinking"},
		},
		{
			name: "provider-qualified numeric",
			ids:  []string{"openai-codex/gpt-5.9", "openai-codex/gpt-5.10"},
			want: []string{"openai-codex/gpt-5.10", "openai-codex/gpt-5.9"},
		},
		{
			name: "digit-continuation",
			ids:  []string{"claude-opus-4", "claude-opus-4-8"},
			want: []string{"claude-opus-4-8", "claude-opus-4"},
		},
		{
			name: "digit/exhausted/letter",
			ids:  []string{"gpt-5.3-codex-spark", "gpt-5.3.1", "gpt-5.3"},
			want: []string{"gpt-5.3.1", "gpt-5.3", "gpt-5.3-codex-spark"},
		},
		{
			name: "number-free",
			ids:  []string{"ollama/llama3", "opencode/big-pickle"},
			want: []string{"opencode/big-pickle", "ollama/llama3"},
		},
		{
			name: "leading zeros",
			ids:  []string{"gpt-5.0", "gpt-5.00"},
			want: []string{"gpt-5.00", "gpt-5.0"},
		},
		{
			name: "duplicates",
			ids:  []string{"claude-opus-4-8", "claude-opus-4-8"},
			want: []string{"claude-opus-4-8", "claude-opus-4-8"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := append([]string(nil), tc.ids...)
			slices.SortFunc(got, compareIDs)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SortFunc(%#v, compareIDs) = %#v, want %#v", tc.ids, got, tc.want)
			}
		})
	}

	t.Run("duplicates compare equal", func(t *testing.T) {
		if c := compareIDs("claude-opus-4-8", "claude-opus-4-8"); c != 0 {
			t.Fatalf("compareIDs(claude-opus-4-8, claude-opus-4-8) = %d, want 0", c)
		}
	})

	t.Run("reflexivity", func(t *testing.T) {
		ids := []string{
			"openai-codex/gpt-5.3-codex-spark",
			"openai-codex/gpt-5.4",
			"openai-codex/gpt-5.5",
			"opencode/big-pickle",
			"opencode/claude-fable-5",
			"opencode/claude-opus-4-8",
			"opencode/deepseek-v4-flash",
			"ollama/llama3",
			"auto",
			"gpt-5.3-codex-low",
			"composer-2.5",
			"claude-opus-5-thinking-high",
			"gemini-3.7-flash-high",
			"claude-opus-4-8",
			"claude-fable-5",
			"claude-sonnet-4-5",
			"claude-pi-only",
			"claude-opencode-only",
		}
		for _, id := range ids {
			if c := compareIDs(id, id); c != 0 {
				t.Fatalf("compareIDs(%q, %q) = %d, want 0", id, id, c)
			}
		}
	})
}

func TestCompareModels(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		want []string
	}{
		{
			name: "full ladder",
			ids:  []string{"claude-haiku-4-5", "claude-sonnet-4-5", "claude-opus-4-8", "claude-fable-5", "x-luna-1", "x-terra-1", "x-sol-1", "zzz-unmatched"},
			want: []string{"x-sol-1", "x-terra-1", "x-luna-1", "claude-fable-5", "claude-opus-4-8", "claude-sonnet-4-5", "claude-haiku-4-5", "zzz-unmatched"},
		},
		{
			name: "family beats version",
			ids:  []string{"claude-opus-4-8", "claude-fable-5"},
			want: []string{"claude-fable-5", "claude-opus-4-8"},
		},
		{
			name: "token not substring",
			ids:  []string{"consolidated-1", "claude-opus-4-8", "gpt-sol-2"},
			want: []string{"gpt-sol-2", "claude-opus-4-8", "consolidated-1"},
		},
		{
			name: "unmatched vs family",
			ids:  []string{"claude-opus-4-8", "aa-sol-1"},
			want: []string{"aa-sol-1", "claude-opus-4-8"},
		},
		{
			name: "dot separator",
			ids:  []string{"claude-opus-4-8", "aa.sol.1"},
			want: []string{"aa.sol.1", "claude-opus-4-8"},
		},
		{
			name: "unmatched natural sort",
			ids:  []string{"gpt-5.3-codex-spark", "gpt-5.5", "gpt-5.4"},
			want: []string{"gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark"},
		},
		{
			name: "within one family",
			ids:  []string{"claude-opus-4", "claude-opus-5-thinking-high", "claude-opus-4-8"},
			want: []string{"claude-opus-5-thinking-high", "claude-opus-4-8", "claude-opus-4"},
		},
		{
			name: "underscore/colon separators",
			ids:  []string{"vendor:claude_fable_5", "aa-unmatched", "claude-opus-4-8"},
			want: []string{"vendor:claude_fable_5", "claude-opus-4-8", "aa-unmatched"},
		},
		{
			name: "realistic mixed",
			ids:  []string{"gpt-5.3-codex-low", "gemini-3.7-flash-high", "composer-2.5", "claude-opus-5-thinking-high", "auto"},
			want: []string{"claude-opus-5-thinking-high", "gpt-5.3-codex-low", "gemini-3.7-flash-high", "composer-2.5", "auto"},
		},
		{
			name: "acceptance ladder",
			ids:  []string{"claude-haiku-4-5", "claude-opus-4-8", "gpt-5.5", "gpt-sol-2", "claude-fable-5"},
			want: []string{"gpt-sol-2", "claude-fable-5", "claude-opus-4-8", "claude-haiku-4-5", "gpt-5.5"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := append([]string(nil), tc.ids...)
			slices.SortFunc(got, compareModels)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SortFunc(%#v, compareModels) = %#v, want %#v", tc.ids, got, tc.want)
			}
		})
	}

	t.Run("reflexivity", func(t *testing.T) {
		ids := []string{
			"openai-codex/gpt-5.3-codex-spark",
			"openai-codex/gpt-5.4",
			"openai-codex/gpt-5.5",
			"opencode/big-pickle",
			"opencode/claude-fable-5",
			"opencode/claude-opus-4-8",
			"opencode/deepseek-v4-flash",
			"ollama/llama3",
			"auto",
			"gpt-5.3-codex-low",
			"composer-2.5",
			"claude-opus-5-thinking-high",
			"gemini-3.7-flash-high",
			"claude-opus-4-8",
			"claude-fable-5",
			"claude-sonnet-4-5",
			"claude-pi-only",
			"claude-opencode-only",
		}
		for _, id := range ids {
			if c := compareModels(id, id); c != 0 {
				t.Fatalf("compareModels(%q, %q) = %d, want 0", id, id, c)
			}
		}
	})
}
