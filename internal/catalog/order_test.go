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
