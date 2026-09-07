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

func TestNumericSegments(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want []string
	}{
		{name: "dot version", id: "gpt-5.6-sol", want: []string{"5", "6"}},
		{name: "hyphen version", id: "claude-fable-5-1", want: []string{"5", "1"}},
		{name: "provider qualification", id: "openai-codex/gpt-5.6-sol", want: []string{"5", "6"}},
		{name: "provider with numeric token", id: "provider-2/gpt-5.6-sol", want: []string{"5", "6"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := numericSegments(tc.id); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("numericSegments(%q) = %#v, want %#v", tc.id, got, tc.want)
			}
		})
	}
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
			want: []string{"claude-fable-5", "claude-opus-4-8", "claude-sonnet-4-5", "claude-haiku-4-5", "x-sol-1", "x-terra-1", "x-luna-1", "zzz-unmatched"},
		},
		{
			name: "version beats family",
			ids:  []string{"claude-fable-4-8", "claude-opus-5"},
			want: []string{"claude-opus-5", "claude-fable-4-8"},
		},
		{
			name: "version beats family across gpt versions",
			ids:  []string{"gpt-5.6-sol", "gpt-6-astra", "gpt-5.10-terra"},
			want: []string{"gpt-6-astra", "gpt-5.10-terra", "gpt-5.6-sol"},
		},
		{
			name: "equal version uses family rank",
			ids:  []string{"claude-haiku-4-8", "claude-opus-4-8", "claude-sol-4-8", "claude-astra-4-8"},
			want: []string{"claude-astra-4-8", "claude-sol-4-8", "claude-opus-4-8", "claude-haiku-4-8"},
		},
		{
			name: "provider qualification preserves version",
			ids:  []string{"gpt-5.6-sol", "openai-codex/gpt-5.10-astra", "openai-codex/gpt-5.6-sol"},
			want: []string{"openai-codex/gpt-5.10-astra", "openai-codex/gpt-5.6-sol", "gpt-5.6-sol"},
		},
		{
			name: "token not substring",
			ids:  []string{"consolidated-1", "claude-opus-4-8", "gpt-sol-2"},
			want: []string{"claude-opus-4-8", "gpt-sol-2", "consolidated-1"},
		},
		{
			name: "unmatched vs family",
			ids:  []string{"claude-opus-4-8", "aa-sol-1"},
			want: []string{"claude-opus-4-8", "aa-sol-1"},
		},
		{
			name: "dot separator",
			ids:  []string{"claude-opus-4-8", "aa.sol.1"},
			want: []string{"claude-opus-4-8", "aa.sol.1"},
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
			want: []string{"gpt-5.3-codex-low", "claude-opus-5-thinking-high", "gemini-3.7-flash-high", "composer-2.5", "auto"},
		},
		{
			name: "acceptance ladder",
			ids:  []string{"claude-haiku-4-5", "claude-opus-4-8", "gpt-5.5", "gpt-sol-2", "claude-fable-5"},
			want: []string{"gpt-5.5", "claude-fable-5", "claude-opus-4-8", "claude-haiku-4-5", "gpt-sol-2"},
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

	t.Run("deterministic natural tie-breaker", func(t *testing.T) {
		ids := []string{
			"openai-codex/gpt-5.6-sol",
			"gpt-5.6-sol",
			"vendor/gpt-5.6-sol-thinking",
		}
		want := append([]string(nil), ids...)
		slices.SortFunc(want, compareModels)
		for i := 0; i < 20; i++ {
			got := append([]string(nil), ids...)
			slices.SortFunc(got, compareModels)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("SortFunc(%#v, compareModels) = %#v on run %d, want %#v", ids, got, i, want)
			}
		}
	})
}
