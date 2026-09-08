package catalog

import "testing"

func TestFamilyRank(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want int
	}{
		{name: "family list", id: "gpt-astra-2", want: 0},
		{name: "token match", id: "gpt-sol-2", want: 1},
		{name: "substring is not a token", id: "consolidated-1", want: len(families)},
		{name: "unmatched", id: "composer-2.5", want: len(families)},
		{name: "highest priority family wins", id: "claude-opus-astra-sol-1", want: 0},
		{name: "case-insensitive", id: "CLAUDE-OPUS-4-8", want: 5},
		{name: "dot separator", id: "aa.sol.1", want: 1},
		{name: "slash separator", id: "aa/sol/1", want: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := familyRank(tc.id); got != tc.want {
				t.Fatalf("familyRank(%q) = %d, want %d", tc.id, got, tc.want)
			}
		})
	}
}
